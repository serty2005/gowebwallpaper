package main

import (
	"strings"
	"testing"
)

func TestSelectAudioDeviceStoresActiveDevice(t *testing.T) {
	config := &AppConfig{}
	device := AudioDevice{ID: "browser-device-1", Name: "Speakers"}

	SelectAudioDevice(config, device)

	if !config.Audio.Active {
		t.Fatal("expected audio selection to be active")
	}
	if config.Audio.ID != device.ID {
		t.Fatalf("expected ID %q, got %q", device.ID, config.Audio.ID)
	}
	if config.Audio.Name != device.Name {
		t.Fatalf("expected Name %q, got %q", device.Name, config.Audio.Name)
	}
}

func TestClearAudioDeviceDisablesRouting(t *testing.T) {
	config := &AppConfig{Audio: AudioConfig{ID: "old", Name: "Old", Active: true}}

	ClearAudioDevice(config)

	if config.Audio.Active {
		t.Fatal("expected audio routing to be inactive")
	}
	if config.Audio.ID != "" || config.Audio.Name != "" {
		t.Fatalf("expected empty audio config, got %+v", config.Audio)
	}
}

func TestParsePowerShellAudioDevices(t *testing.T) {
	raw := "Name|InstanceId\r\nSpeakers (Realtek(R) Audio)|HDAUDIO\\FUNC_01\r\nUSB DAC|SWD\\MMDEVAPI\\{0.0.0.00000000}.{abc}\r\n"

	devices := parsePowerShellAudioDevices(raw)

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].Name != "Speakers (Realtek(R) Audio)" {
		t.Fatalf("unexpected first device: %+v", devices[0])
	}
	if devices[1].ID != `SWD\MMDEVAPI\{0.0.0.00000000}.{abc}` {
		t.Fatalf("unexpected second device ID: %+v", devices[1])
	}
}

func TestParsePowerShellAudioDevicesPreservesCyrillic(t *testing.T) {
	raw := "Динамики (Realtek(R) Audio)|HDAUDIO\\FUNC_01\r\nНаушники USB|SWD\\MMDEVAPI\\{abc}\r\n"

	devices := parsePowerShellAudioDevices(raw)

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].Name != "Динамики (Realtek(R) Audio)" {
		t.Fatalf("unexpected first device name: %q", devices[0].Name)
	}
	if devices[1].Name != "Наушники USB" {
		t.Fatalf("unexpected second device name: %q", devices[1].Name)
	}
}

func TestAudioDevicePowerShellCommandForcesUtf8Output(t *testing.T) {
	command := audioDevicePowerShellCommand()

	if !strings.Contains(command, "[Console]::OutputEncoding") {
		t.Fatalf("expected command to set Console.OutputEncoding, got %s", command)
	}
	if !strings.Contains(command, "[System.Text.UTF8Encoding]::new($false)") {
		t.Fatalf("expected command to use UTF-8 without BOM, got %s", command)
	}
}
