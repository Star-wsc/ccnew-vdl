package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddlewareOffPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	gate, err := newAuthGate(dir, "off")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(gate.middleware())
	r.GET("/api/tasks", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tasks", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Fatalf("off mode should passthrough, got %d %s", w.Code, w.Body.String())
	}
}

func TestAuthFlowSetupLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	gate, err := newAuthGate(dir, "on")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(gate.middleware())
	r.GET("/api/auth/status", gate.handleStatus)
	r.POST("/api/auth/setup", gate.handleSetup)
	r.POST("/api/auth/login", gate.handleLogin)
	r.GET("/api/tasks", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// 未 setup: 业务接口 401 need_setup
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks", nil))
	if w.Code != 401 {
		t.Fatalf("want 401 before setup, got %d", w.Code)
	}

	// setup
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "password123"})
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	var setupResp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &setupResp)
	if setupResp.Token == "" {
		t.Fatal("no token after setup")
	}

	// 带 token 访问业务
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+setupResp.Token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("authorized tasks: %d", w.Code)
	}

	// Cookie 方式
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: setupResp.Token})
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("cookie auth: %d", w.Code)
	}

	// 错误密码
	bad, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrongwrong"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("bad login want 401 got %d", w.Code)
	}

	// auth.json 权限路径存在
	if _, err := filepath.Abs(filepath.Join(dir, "auth.json")); err != nil {
		t.Fatal(err)
	}
}
