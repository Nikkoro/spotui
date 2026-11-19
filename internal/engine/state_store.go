package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	librespot "github.com/devgianlu/go-librespot"
)

// fileStateStore persists the device id, credentials and last volume as JSON.
type fileStateStore struct {
	path string
}

func newFileStateStore(path string) *fileStateStore {
	return &fileStateStore{path: path}
}

func (s *fileStateStore) Load() (*librespot.AppState, error) {
	state := &librespot.AppState{}

	content, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(content, state); err != nil {
			return nil, fmt.Errorf("parsing state file: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	return state, nil
}

func (s *fileStateStore) Save(state *librespot.AppState) error {
	state.Lock()
	defer state.Unlock()

	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp state file: %w", err)
	}
	tmpPath := tmp.Name()

	if err := json.NewEncoder(tmp).Encode(state); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing state: %w", err)
	}
	// Windows cannot rename a file that still has an open handle.
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		// Windows cannot rename over an existing file.
		if rmErr := os.Remove(s.path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("replacing state file: %w", rmErr)
		}
		if err := os.Rename(tmpPath, s.path); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("replacing state file: %w", err)
		}
	}
	return nil
}

// Logout forgets the stored Spotify login (and the Web API token) so the
// next start asks to pair again. The device id and volume are kept.
func Logout(configDir string) error {
	path := filepath.Join(configDir, "state.json")
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		var state map[string]json.RawMessage
		if err := json.Unmarshal(b, &state); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		delete(state, "credentials")
		out, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join(configDir, "webapi_token.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
