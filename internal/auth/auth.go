package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotConfigured  = errors.New("auth not configured")
	ErrAlreadySetup   = errors.New("auth already configured")
	ErrBadCredentials = errors.New("invalid username or password")
	ErrInvalidToken   = errors.New("invalid or expired token")
	ErrWeakPassword   = errors.New("password too short")
)

const (
	minPasswordLen = 8
	sessionTTL     = 30 * 24 * time.Hour
	tokenBytes     = 32
)

// Store 账密与会话。authMode=off 时 Service 仍可存在，但中间件不调用。
type Store struct {
	mu       sync.Mutex
	path     string
	username string
	hash     []byte               // bcrypt
	sessions map[string]time.Time // token hash -> expiry
}

type fileRecord struct {
	Username string `json:"username"`
	Algo     string `json:"algo"`
	Hash     string `json:"hash"`
	Updated  string `json:"updated_at"`
}

// NewStore path 为 auth.json 完整路径。
func NewStore(path string) (*Store, error) {
	s := &Store{
		path:     path,
		sessions: map[string]time.Time{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var rec fileRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("auth.json invalid: %w", err)
	}
	if rec.Username == "" || rec.Hash == "" {
		return nil
	}
	raw, err := hex.DecodeString(rec.Hash)
	if err != nil {
		return fmt.Errorf("auth.json hash: %w", err)
	}
	s.username = rec.Username
	s.hash = raw
	return nil
}

// Configured 是否已设置账密。
func (s *Store) Configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.username != "" && len(s.hash) > 0
}

// Setup 首次设置账密。已设置则返回 ErrAlreadySetup。
func (s *Store) Setup(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username required")
	}
	if len(password) < minPasswordLen {
		return ErrWeakPassword
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.username != "" {
		return ErrAlreadySetup
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	rec := fileRecord{
		Username: username,
		Algo:     "bcrypt",
		Hash:     hex.EncodeToString(hash),
		Updated:  time.Now().Format(time.RFC3339),
	}
	if err := s.persist(rec); err != nil {
		return err
	}
	s.username = username
	s.hash = hash
	return nil
}

func (s *Store) persist(rec fileRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Login 校验账密，成功返回 session token（明文，仅此一次返回）。
func (s *Store) Login(username, password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.username == "" || len(s.hash) == 0 {
		return "", ErrNotConfigured
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(s.username)) != 1 {
		// 仍跑一次 bcrypt，避免用户名枚举耗时差
		_ = bcrypt.CompareHashAndPassword(s.hash, []byte(password))
		return "", ErrBadCredentials
	}
	if err := bcrypt.CompareHashAndPassword(s.hash, []byte(password)); err != nil {
		return "", ErrBadCredentials
	}
	var raw [tokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw[:])
	s.sessions[hashToken(token)] = time.Now().Add(sessionTTL)
	s.gcSessionsLocked()
	return token, nil
}

// Logout 吊销 token。
func (s *Store) Logout(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, hashToken(token))
}

// Validate token 是否有效。
func (s *Store) Validate(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[hashToken(token)]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, hashToken(token))
		return false
	}
	return true
}

// Username 当前配置的用户名（未配置返回空）。
func (s *Store) Username() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.username
}

func (s *Store) gcSessionsLocked() {
	now := time.Now()
	for k, exp := range s.sessions {
		if now.After(exp) {
			delete(s.sessions, k)
		}
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ResolveMode 解析 AUTH_MODE。
// 显式 on/off 优先；否则桌面模式 off，服务端 on。
func ResolveMode(explicit string, desktop bool) string {
	v := strings.ToLower(strings.TrimSpace(explicit))
	if v == "on" || v == "off" {
		return v
	}
	if desktop {
		return "off"
	}
	return "on"
}
