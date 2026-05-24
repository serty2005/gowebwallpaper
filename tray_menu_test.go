package main

import "testing"

func TestBuildMonitorMenuOptionsKeepsMissingActiveMonitorCheckedDisabled(t *testing.T) {
	config := &AppConfig{Monitors: []MonitorConfig{
		{Name: `\\.\DISPLAY2`, Active: true, PositionX: -2560, PositionY: 0, Width: 2560, Height: 1440},
	}}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY1`, IsPrimary: true, PositionX: 0, PositionY: 0, Width: 1920, Height: 1080},
	}

	options, activeConnectedName := buildMonitorMenuOptions(config, connected)

	if activeConnectedName != "" {
		t.Fatalf("expected missing active monitor to have no connected name, got %q", activeConnectedName)
	}
	if len(options) != 2 {
		t.Fatalf("expected connected monitor plus missing active monitor, got %d", len(options))
	}
	if options[0].Monitor.Name != `\\.\DISPLAY1` || !options[0].Enabled || options[0].Checked {
		t.Fatalf("unexpected connected option: %#v", options[0])
	}
	if options[1].Monitor.Name != `\\.\DISPLAY2` {
		t.Fatalf("expected missing active monitor as second option, got %#v", options[1])
	}
	if !options[1].Checked {
		t.Fatal("expected missing active monitor to stay checked")
	}
	if options[1].Enabled {
		t.Fatal("expected missing active monitor to be disabled")
	}
}

func TestBuildMonitorMenuOptionsUsesResolvedConnectedMonitorWhenActiveStillMatches(t *testing.T) {
	config := &AppConfig{Monitors: []MonitorConfig{
		{Name: `\\.\DISPLAY9`, Active: true, PositionX: -1920, PositionY: 0, Width: 1920, Height: 1080},
	}}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY3`, PositionX: -1920, PositionY: 0, Width: 1920, Height: 1080},
		{Name: `\\.\DISPLAY1`, IsPrimary: true, PositionX: 0, PositionY: 0, Width: 1920, Height: 1080},
	}

	options, activeConnectedName := buildMonitorMenuOptions(config, connected)

	if activeConnectedName != `\\.\DISPLAY3` {
		t.Fatalf("expected active monitor to resolve to DISPLAY3, got %q", activeConnectedName)
	}
	if len(options) != len(connected) {
		t.Fatalf("expected only connected monitors, got %d options", len(options))
	}
	if !options[0].Checked || !options[0].Enabled {
		t.Fatalf("expected resolved connected monitor checked and enabled, got %#v", options[0])
	}
}

func TestSetConfigURLTrimsAndIgnoresEmptyURL(t *testing.T) {
	config := &AppConfig{URL: "https://old.test"}

	changed := setConfigURL(config, " https://new.test/wallpaper ")
	if !changed {
		t.Fatal("expected non-empty URL to be applied")
	}
	if config.URL != "https://new.test/wallpaper" {
		t.Fatalf("expected trimmed URL, got %q", config.URL)
	}

	changed = setConfigURL(config, "   ")
	if changed {
		t.Fatal("expected blank URL to be ignored")
	}
	if config.URL != "https://new.test/wallpaper" {
		t.Fatalf("expected URL to stay unchanged, got %q", config.URL)
	}
}
