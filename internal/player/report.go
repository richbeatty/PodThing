package player

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// saveSessionReport writes the last host-mode session atomically into
// the same persistent state directory as the user's settings.
func (a *App) saveSessionReport() error {
	b, err := a.Report()
	if err != nil { return err }
	path := filepath.Join(a.stateDir, "last-session.json")
	tmp := path+".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil { return err }
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// SavedSessionReport never fabricates a host report from a recovery snapshot.
func (a *App) SavedSessionReport() ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(a.stateDir, "last-session.json"))
	if err != nil { return nil, err }
	if !json.Valid(b) { return nil, errors.New("saved session report is malformed") }
	return b, nil
}
