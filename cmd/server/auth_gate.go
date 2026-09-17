package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/auth"
	"github.com/gin-gonic/gin"
)

const sessionCookieName = "doubi_session"

type authGate struct {
	store *auth.Store
	mode  string // "on" | "off"
	mu    sync.Mutex
	// 简单登录限速: key -> 失败时间列表
	fails map[string][]time.Time
}

func newAuthGate(cfgDir string, mode string) (*authGate, error) {
	store, err := auth.NewStore(filepath.Join(cfgDir, "auth.json"))
	if err != nil {
		return nil, err
	}
	return &authGate{
		store: store,
		mode:  mode,
		fails: map[string][]time.Time{},
	}, nil
}

func (g *authGate) enabled() bool { return g.mode == "on" }

func (g *authGate) tokenFromRequest(c *gin.Context) string {
	if v := c.GetHeader("Authorization"); v != "" {
		if strings.HasPrefix(v, "Bearer ") {
			return strings.TrimSpace(v[7:])
		}
	}
	if v := c.GetHeader("X-Auth-Token"); v != "" {
		return strings.TrimSpace(v)
	}
	if ck, err := c.Cookie(sessionCookieName); err == nil {
		return strings.TrimSpace(ck)
	}
	return ""
}

func (g *authGate) allow(c *gin.Context) bool {
	if !g.enabled() {
		return true
	}
	return g.store.Validate(g.tokenFromRequest(c))
}

func (g *authGate) tooManyFails(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	window := 5 * time.Minute
	kept := g.fails[ip][:0]
	for _, t := range g.fails[ip] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	g.fails[ip] = kept
	return len(kept) >= 10
}

func (g *authGate) recordFail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fails[ip] = append(g.fails[ip], time.Now())
}

func (g *authGate) clearFails(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, ip)
}

// middleware 保护 /api/*（auth 路由在注册时已排除在保护逻辑外，由 handler 自身判断）。
func (g *authGate) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.enabled() {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if isPublicAuthPath(path) {
			c.Next()
			return
		}
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		if !g.store.Configured() {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "need_setup",
				"need_setup": true,
				"auth_mode":  "on",
			})
			c.Abort()
			return
		}
		if !g.store.Validate(g.tokenFromRequest(c)) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":     "unauthorized",
				"need_auth": true,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func isPublicAuthPath(path string) bool {
	switch path {
	case "/api/auth/status", "/api/auth/setup", "/api/auth/login", "/api/auth/logout":
		return true
	}
	return false
}

type authSetupReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (g *authGate) handleStatus(c *gin.Context) {
	needSetup := g.enabled() && !g.store.Configured()
	loggedIn := g.allow(c) && g.enabled() && g.store.Configured()
	if !g.enabled() {
		loggedIn = true
	}
	c.JSON(http.StatusOK, gin.H{
		"auth_mode":  g.mode,
		"need_setup": needSetup,
		"logged_in":  loggedIn,
		"username":   g.store.Username(),
	})
}

func (g *authGate) handleSetup(c *gin.Context) {
	if !g.enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth disabled"})
		return
	}
	var req authSetupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效请求"})
		return
	}
	if err := g.store.Setup(req.Username, req.Password); err != nil {
		status := http.StatusBadRequest
		msg := err.Error()
		if err == auth.ErrAlreadySetup {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}
	// setup 成功后自动登录
	tok, err := g.store.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "username": g.store.Username()})
		return
	}
	g.setSessionCookie(c, tok)
	c.JSON(http.StatusOK, gin.H{"ok": true, "username": g.store.Username(), "token": tok})
}

func (g *authGate) handleLogin(c *gin.Context) {
	if !g.enabled() {
		c.JSON(http.StatusOK, gin.H{"ok": true, "auth_mode": "off"})
		return
	}
	if !g.store.Configured() {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "need_setup", "need_setup": true})
		return
	}
	ip := c.ClientIP()
	if g.tooManyFails(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试过于频繁，请稍后再试"})
		return
	}
	var req authLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效请求"})
		return
	}
	tok, err := g.store.Login(req.Username, req.Password)
	if err != nil {
		g.recordFail(ip)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	g.clearFails(ip)
	g.setSessionCookie(c, tok)
	c.JSON(http.StatusOK, gin.H{"ok": true, "username": g.store.Username(), "token": tok})
}

func (g *authGate) handleLogout(c *gin.Context) {
	tok := g.tokenFromRequest(c)
	g.store.Logout(tok)
	g.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (g *authGate) setSessionCookie(c *gin.Context, token string) {
	// HttpOnly；SameSite=Lax 便于反代同站；Secure 由 HTTPS 反代场景可选，HTTP 本机也能用
	c.SetCookie(sessionCookieName, token, 30*24*3600, "/", "", false, true)
}

func (g *authGate) clearSessionCookie(c *gin.Context) {
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func authConfigDir() string {
	// 与 config.getConfigFilePath 同目录策略：~/.config/ccnew-vdl
	if env := os.Getenv("AUTH_DIR"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "ccnew-vdl")
}
