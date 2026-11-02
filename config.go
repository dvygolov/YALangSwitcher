//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config lives in %APPDATA%\YALangSwitcher\config.json and is rewritten by the
// tray menu, so it is also the single place to hand-edit the layout ids.
type Config struct {
	EnglishLayout    string `json:"englishLayout"`
	RussianLayout    string `json:"russianLayout"`
	Mode             string `json:"mode"`           // "tap" - on release of a lone Shift, "down" - on press
	FallbackHotkey   string `json:"fallbackHotkey"` // alt+shift | ctrl+shift | win+space | none
	Autostart        bool   `json:"autostart"`
	AdminMode        bool   `json:"adminMode"`
	AutoUpdate       bool   `json:"autoUpdate"`
	UpdateCheckHours int    `json:"updateCheckHours"`
	Notifications    bool   `json:"notifications"`

	// AcceptInjected lets the switcher react to synthesised keystrokes. Off by
	// default: it is only meant for automated tests and for people who drive
	// the app from AutoHotkey.
	AcceptInjected bool `json:"acceptInjected,omitempty"`
	// InstalledExe remembers which exe the scheduled task points at.
	InstalledExe string `json:"installedExe,omitempty"`
}

func defaultConfig() Config {
	return Config{
		EnglishLayout:    "00000409",
		RussianLayout:    "00000419",
		Mode:             "tap",
		FallbackHotkey:   "alt+shift",
		Autostart:        true,
		AdminMode:        true,
		AutoUpdate:       true,
		UpdateCheckHours: 6,
		Notifications:    true,
	}
}

func dataDir() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, appName)
	}
	if exe := os.Getenv("APPDATA"); exe != "" {
		return filepath.Join(exe, appName)
	}
	return appName
}

func configPath() string { return filepath.Join(dataDir(), "config.json") }

// loadConfig reads the config, filling in defaults for missing or invalid
// values and writing the file back on first start.
func loadConfig() (Config, bool) {
	cfg := defaultConfig()
	raw, err := os.ReadFile(configPath())
	if err != nil {
		_ = cfg.save()
		return cfg, true
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		logf("config: %s is unreadable (%v), using defaults", configPath(), err)
		return defaultConfig(), false
	}
	cfg.normalize()
	return cfg, false
}

func (c *Config) normalize() {
	d := defaultConfig()
	if strings.TrimSpace(c.EnglishLayout) == "" {
		c.EnglishLayout = d.EnglishLayout
	}
	if strings.TrimSpace(c.RussianLayout) == "" {
		c.RussianLayout = d.RussianLayout
	}
	switch c.Mode {
	case "tap", "down":
	default:
		c.Mode = d.Mode
	}
	switch c.FallbackHotkey {
	case "alt+shift", "ctrl+shift", "win+space", "none":
	default:
		c.FallbackHotkey = d.FallbackHotkey
	}
	if c.UpdateCheckHours <= 0 || c.UpdateCheckHours > 720 {
		c.UpdateCheckHours = d.UpdateCheckHours
	}
}

func (c Config) save() error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(configPath(), raw, 0o644)
}
