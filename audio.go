package main

import (
	"os/exec"
	"strings"
)

type AudioDevice struct {
	ID      string
	Name    string
	Default bool
}

func SelectAudioDevice(config *AppConfig, device AudioDevice) {
	if config == nil {
		return
	}
	config.Audio = AudioConfig{
		ID:     device.ID,
		Name:   device.Name,
		Active: device.ID != "" || device.Name != "",
	}
}

func ClearAudioDevice(config *AppConfig) {
	if config == nil {
		return
	}
	config.Audio = AudioConfig{}
}

func SelectedAudioDevice(config *AppConfig) AudioConfig {
	if config == nil {
		return AudioConfig{}
	}
	return config.Audio
}

func ListAudioDevices() []AudioDevice {
	out, err := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", audioDevicePowerShellCommand()).Output()
	if err != nil {
		return nil
	}
	return parsePowerShellAudioDevices(string(out))
}

func audioDevicePowerShellCommand() string {
	return "$utf8 = [System.Text.UTF8Encoding]::new($false); " +
		"[Console]::OutputEncoding = $utf8; " +
		"$OutputEncoding = $utf8; " +
		"Get-PnpDevice -Class AudioEndpoint -Status OK | " +
		"ForEach-Object { ($_.FriendlyName -replace '\\|', ' ') + '|' + $_.InstanceId }"
}

func parsePowerShellAudioDevices(raw string) []AudioDevice {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	devices := make([]AudioDevice, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		id := strings.TrimSpace(parts[1])
		if strings.EqualFold(name, "Name") || strings.EqualFold(name, "FriendlyName") {
			continue
		}
		if name == "" && id == "" {
			continue
		}
		devices = append(devices, AudioDevice{ID: id, Name: name})
	}
	return devices
}
