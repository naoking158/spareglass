// Package config loads and saves the host configuration.
//
// The file lives at <UserConfigDir>/spareglass/config.json. Every field is
// optional; missing fields keep their defaults. Command-line flags override it.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const appDir = "spareglass"

// Config is the host configuration.
type Config struct {
	// Addr is the HTTP listen address. Empty means automatic: port 80 so the
	// URL needs no port, falling back to 7878 if 80 is unavailable.
	Addr string `json:"addr,omitempty"`
	// FPS is the target capture/encode frame rate.
	FPS int `json:"fps"`
	// BitrateKbps is the target video bitrate per display.
	BitrateKbps int `json:"bitrate_kbps"`
	// MaxClients caps how many displays can be connected at once.
	MaxClients int `json:"max_clients"`
	// RenderScale sizes the virtual display, in points, relative to the
	// client's CSS pixels. 1 keeps text legible. On macOS a client with a
	// devicePixelRatio of at least 2×RenderScale gets a HiDPI display, which
	// is already sharp; elsewhere the devicePixelRatio gives the sharpest
	// picture at a higher bitrate.
	RenderScale float64 `json:"render_scale"`
	// NoHiDPI gives every client a display with one pixel per point, even
	// a Retina one that would get a HiDPI display: lower latency, softer text.
	NoHiDPI bool `json:"no_hidpi,omitempty"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		FPS:         60,
		BitrateKbps: 12000,
		MaxClients:  2,
		RenderScale: 1,
	}
}

// Validate reports the first invalid field.
func (c Config) Validate() error {
	switch {
	case c.FPS < 1 || c.FPS > 240:
		return fmt.Errorf("config: fps %d out of range 1-240", c.FPS)
	case c.BitrateKbps < 500 || c.BitrateKbps > 200000:
		return fmt.Errorf("config: bitrate_kbps %d out of range 500-200000", c.BitrateKbps)
	case c.MaxClients < 1 || c.MaxClients > 8:
		return fmt.Errorf("config: max_clients %d out of range 1-8", c.MaxClients)
	case c.RenderScale < 0.5 || c.RenderScale > 4:
		return fmt.Errorf("config: render_scale %g out of range 0.5-4", c.RenderScale)
	}
	return nil
}

// Dir returns the directory holding config.json and devices.json.
func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appDir), nil
}

// Path returns the config file location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// DevicesPath returns where approved devices are stored.
func DevicesPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "devices.json"), nil
}

// Load reads the config file over the defaults. A missing file is not an error.
func Load() (Config, string, error) {
	cfg := Default()
	p, err := Path()
	if err != nil {
		return cfg, "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, p, nil
	}
	if err != nil {
		return cfg, p, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, p, fmt.Errorf("parse %s: %w", p, err)
	}
	return cfg, p, cfg.Validate()
}

// Init writes the defaults to the config path unless a file already exists.
func Init() (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return p, fmt.Errorf("%s already exists", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return p, err
	}
	b, _ := json.MarshalIndent(Default(), "", "  ")
	return p, os.WriteFile(p, append(b, '\n'), 0o600)
}
