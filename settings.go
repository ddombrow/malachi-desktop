package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Settings are the desktop app's own preferences, kept apart from malachi's
// ~/.malachi/settings.json (which the user edits, and which malachi never
// writes). They are applied to each session the app opens.
type Settings struct {
	Model    string `json:"model,omitempty"`    // "provider/model"
	Thinking string `json:"thinking,omitempty"` // thinking level
	CodeMode string `json:"codeMode,omitempty"` // off, on or explore
}

var settingsMu sync.Mutex

// settingsPath is $XDG_CONFIG_HOME/malachi-desktop/settings.json.
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "malachi-desktop", "settings.json"), nil
}

func loadSettings() (Settings, error) {
	var st Settings
	p, err := settingsPath()
	if err != nil {
		return st, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

// GetSettings returns the saved preferences.
func (m *MalachiService) GetSettings() (Settings, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return loadSettings()
}

// UpdateSettings merges the non-empty fields of patch into the saved
// preferences and writes them atomically.
func (m *MalachiService) UpdateSettings(patch Settings) (Settings, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	st, err := loadSettings()
	if err != nil {
		st = Settings{} // a corrupt file is replaced, not fatal
	}
	if patch.Model != "" {
		st.Model = patch.Model
	}
	if patch.Thinking != "" {
		st.Thinking = patch.Thinking
	}
	if patch.CodeMode != "" {
		st.CodeMode = patch.CodeMode
	}
	p, err := settingsPath()
	if err != nil {
		return st, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return st, err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return st, err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return st, err
	}
	return st, os.Rename(tmp, p)
}
