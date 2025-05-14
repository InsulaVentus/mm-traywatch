package main

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

type Theme string

type Config struct {
	Pat   string `yaml:"pat"`
	Host  string `yaml:"host"`
	Theme Theme  `yaml:"theme"`
}

const (
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
)

func LoadConfig() (*Config, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}

	c := Config{}

	file, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("could not read config file: %w", err)
	} else if err := yaml.Unmarshal(file, &c); err != nil {
		return nil, fmt.Errorf("could not parse config file: %w", err)
	}

	if v := os.Getenv("MM_TRAYWATCH_PAT"); v != "" {
		c.Pat = v
	}

	if v := os.Getenv("MM_TRAYWATCH_HOST"); v != "" {
		c.Host = v
	}

	if v := os.Getenv("MM_TRAYWATCH_THEME"); v != "" {
		c.Theme = Theme(v)
	}

	if c.Theme != ThemeDark && c.Theme != ThemeLight {
		return nil, fmt.Errorf("invalid theme: %q (must be %q or %q)", c.Theme, ThemeLight, ThemeDark)
	}

	if c.Pat == "" {
		return nil, errors.New("pat is required")
	}

	if c.Host == "" {
		return nil, errors.New("host is required")
	}

	return &c, nil
}

func defaultConfigPath() (string, error) {
	if p := os.Getenv("MM_TRAYWATCH_CONFIG"); p != "" {
		return p, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not get user config dir: %w", err)
	}
	return filepath.Join(dir, "mm-traywatch", "config.yaml"), nil
}
