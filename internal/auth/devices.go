package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const deviceKeyPrefix = "dbl_"

var ErrDeviceNotFound = errors.New("device not found")

type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	KeyHash    string `json:"key_hash"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	Revoked    bool   `json:"revoked"`
}

type deviceFile struct {
	Devices []Device `json:"devices"`
}

// DeviceStore 第三方接入密钥（Hermes/龙虾等机器客户端）。
type DeviceStore struct {
	mu   sync.Mutex
	path string
	list []Device
}

func NewDeviceStore(dir string) (*DeviceStore, error) {
	s := &DeviceStore{path: filepath.Join(dir, "devices.json")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DeviceStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var f deviceFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("devices.json invalid: %w", err)
	}
	s.list = f.Devices
	return nil
}

func (s *DeviceStore) persist() error {
	data, err := json.MarshalIndent(deviceFile{Devices: s.list}, "", "  ")
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

func hashDeviceKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// CreateDevice 生成接入密钥，返回 id 与明文 key（仅此一次）。
func (s *DeviceStore) CreateDevice(name string) (id, plaintext string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Hermes"
	}
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	plaintext = deviceKeyPrefix + hex.EncodeToString(raw[:])
	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes)
	id = hex.EncodeToString(idBytes)
	prefix := plaintext
	if len(prefix) > 12 {
		prefix = prefix[:12] + "…"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append(s.list, Device{
		ID:        id,
		Name:      name,
		Prefix:    prefix,
		KeyHash:   hashDeviceKey(plaintext),
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	if err := s.persist(); err != nil {
		return "", "", err
	}
	return id, plaintext, nil
}

func (s *DeviceStore) ListDevices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.list))
	for _, d := range s.list {
		// 不返回 hash
		out = append(out, Device{
			ID:         d.ID,
			Name:       d.Name,
			Prefix:     d.Prefix,
			CreatedAt:  d.CreatedAt,
			LastUsedAt: d.LastUsedAt,
			Revoked:    d.Revoked,
		})
	}
	return out
}

// RevokeDevice 吊销并从列表删除该设备记录（密钥立即失效，界面不再显示）。
func (s *DeviceStore) RevokeDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.list[:0]
	found := false
	for _, d := range s.list {
		if d.ID == id {
			found = true
			continue
		}
		out = append(out, d)
	}
	if !found {
		return ErrDeviceNotFound
	}
	s.list = out
	if s.list == nil {
		s.list = []Device{}
	}
	return s.persist()
}

// ValidateKey 校验明文密钥，成功则更新 last_used。
func (s *DeviceStore) ValidateKey(key string) bool {
	if key == "" || !strings.HasPrefix(key, deviceKeyPrefix) {
		return false
	}
	h := hashDeviceKey(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	for i := range s.list {
		if s.list[i].Revoked {
			continue
		}
		if s.list[i].KeyHash == h {
			s.list[i].LastUsedAt = now
			_ = s.persist()
			return true
		}
	}
	return false
}
