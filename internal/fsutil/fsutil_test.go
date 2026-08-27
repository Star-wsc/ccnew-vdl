package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "data.json")

	if err := WriteFileAtomic(target, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("first write: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != `{"v":1}` {
		t.Fatalf("readback v1: %q err=%v", got, err)
	}

	if err := WriteFileAtomic(target, []byte(`{"v":2}`)); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _ = os.ReadFile(target)
	if string(got) != `{"v":2}` {
		t.Fatalf("readback v2: %q", got)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp0" || contains(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
