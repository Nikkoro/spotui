package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogoutKeepsDeviceID(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "state.json"),
		[]byte(`{"device_id":"abc","credentials":{"username":"u","data":"eA=="},"last_volume":50}`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "webapi_token.json"), []byte(`{}`), 0o600)

	if err := Logout(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	var state map[string]any
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state["credentials"]; ok {
		t.Error("credentials still stored")
	}
	if state["device_id"] != "abc" {
		t.Error("device id lost")
	}
	if _, err := os.Stat(filepath.Join(dir, "webapi_token.json")); !os.IsNotExist(err) {
		t.Error("web API token still there")
	}
	if err := Logout(t.TempDir()); err != nil {
		t.Errorf("logout with nothing stored: %v", err)
	}
}
