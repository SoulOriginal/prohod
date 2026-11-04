package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (c *Controller) settingsPath() string { return filepath.Join(c.cfgDir, "settings.json") }

func (c *Controller) loadSettings() {
	b, err := os.ReadFile(c.settingsPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &c.settings)
}

func (c *Controller) Settings() Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settings
}

// SaveSettings persists settings; engine-affecting changes apply on the next connect.
func (c *Controller) SaveSettings(s Settings) error {
	c.mu.Lock()
	c.settings = s
	c.mu.Unlock()
	b, _ := json.MarshalIndent(s, "", " ")
	tmp := c.settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.settingsPath())
}
