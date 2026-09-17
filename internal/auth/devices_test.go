package auth

import (
	"testing"
)

func TestDeviceKeyLifecycle(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, key, err := ds.CreateDevice("Hermes")
	if err != nil {
		t.Fatal(err)
	}
	if key == "" || len(key) < 20 {
		t.Fatalf("weak key %q", key)
	}
	if !ds.ValidateKey(key) {
		t.Fatal("valid key rejected")
	}
	if ds.ValidateKey("dbl_notreal") {
		t.Fatal("fake key accepted")
	}
	if err := ds.RevokeDevice(id); err != nil {
		t.Fatal(err)
	}
	if ds.ValidateKey(key) {
		t.Fatal("revoked key still valid")
	}
	if len(ds.ListDevices()) != 0 {
		t.Fatalf("revoked device should be removed from list, got %+v", ds.ListDevices())
	}
	ds2, err := NewDeviceStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ds2.ValidateKey(key) {
		t.Fatal("revoked key valid after reload")
	}
	if len(ds2.ListDevices()) != 0 {
		t.Fatalf("list after reload should be empty: %+v", ds2.ListDevices())
	}
}
