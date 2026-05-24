package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const defaultURL = "http://localhost:3100/#/columns-fullscreen"

var configPathOverride string

type MonitorConfig struct {
	Name      string `json:"Name"`
	IsPrimary bool   `json:"IsPrimary"`
	Active    bool   `json:"Active"`
	PositionX int    `json:"PositionX"`
	PositionY int    `json:"PositionY"`
	Width     int    `json:"Width"`
	Height    int    `json:"Height"`
}

type AudioConfig struct {
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	Active bool   `json:"Active"`
}

type AppConfig struct {
	URL      string          `json:"URL"`
	Monitors []MonitorConfig `json:"Monitors"`
	Audio    AudioConfig     `json:"Audio"`
	Log      string          `json:"log,omitempty"`
}

func getConfigPath() (string, error) {
	if configPathOverride != "" {
		return configPathOverride, nil
	}
	if envPath := os.Getenv("GOWEBWALLPAPER_CONFIG"); envPath != "" {
		return envPath, nil
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "config.json"), nil
}

func configExists() (bool, error) {
	path, err := getConfigPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func loadConfig() (*AppConfig, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}
	debugLogf("loadConfig input path=%q", path)
	file, err := os.ReadFile(path)
	if err != nil {
		debugLogf("loadConfig output error=%v", err)
		return nil, err
	}
	var config AppConfig
	if err := json.Unmarshal(file, &config); err != nil {
		debugLogf("loadConfig output error=%v bytes=%d", err, len(file))
		return nil, err
	}
	normalizeConfig(&config)
	debugLogf("loadConfig output url=%q monitors=%d audioActive=%t log=%q", config.URL, len(config.Monitors), config.Audio.Active, config.Log)
	return &config, nil
}

func saveConfig(config *AppConfig) error {
	if config == nil {
		return errors.New("config is nil")
	}
	debugLogf("saveConfig input url=%q monitors=%d audioActive=%t log=%q", config.URL, len(config.Monitors), config.Audio.Active, config.Log)
	normalizeConfig(config)
	path, err := getConfigPath()
	if err != nil {
		debugLogf("saveConfig output error=%v", err)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		debugLogf("saveConfig output error=%v path=%q", err, path)
		return err
	}
	file, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		debugLogf("saveConfig output error=%v", err)
		return err
	}
	if err := os.WriteFile(path, file, 0644); err != nil {
		debugLogf("saveConfig output error=%v path=%q bytes=%d", err, path, len(file))
		return err
	}
	debugLogf("saveConfig output path=%q bytes=%d", path, len(file))
	return nil
}

func normalizeConfig(config *AppConfig) {
	if config.URL == "" {
		config.URL = defaultURL
	}
	config.Log = strings.ToLower(strings.TrimSpace(config.Log))
	if config.Log != logLevelDebug {
		config.Log = ""
	}
	activeSeen := false
	for i := range config.Monitors {
		if config.Monitors[i].Active {
			if activeSeen {
				config.Monitors[i].Active = false
				continue
			}
			activeSeen = true
		}
	}
	if !activeSeen && len(config.Monitors) > 0 {
		config.Monitors[0].Active = true
	}
	if config.Audio.ID == "" && config.Audio.Name == "" {
		config.Audio.Active = false
	}
}

func activeMonitor(config *AppConfig) (MonitorConfig, bool) {
	if config == nil {
		return MonitorConfig{}, false
	}
	for _, monitor := range config.Monitors {
		if monitor.Active {
			return monitor, true
		}
	}
	if len(config.Monitors) == 0 {
		return MonitorConfig{}, false
	}
	return config.Monitors[0], true
}

func replaceConfigMonitors(config *AppConfig, connected []MonitorConfig, activeName string) {
	if config == nil {
		return
	}
	if activeName == "" {
		if current, ok := activeMonitor(config); ok {
			activeName = current.Name
		}
	}
	config.Monitors = append([]MonitorConfig(nil), connected...)
	activeSet := false
	for i := range config.Monitors {
		config.Monitors[i].Active = config.Monitors[i].Name == activeName
		if config.Monitors[i].Active {
			activeSet = true
		}
	}
	if !activeSet && len(config.Monitors) > 0 {
		config.Monitors[0].Active = true
	}
}

func setActiveMonitor(monitorName string) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	replaceConfigMonitors(config, getMonitors(), monitorName)
	return saveConfig(config)
}
