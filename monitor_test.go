package main

import "testing"

func TestFindBestMonitorPrefersExactNameWhenResolutionsDuplicate(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY2`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920, Active: true}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY1`, PositionX: -1080, PositionY: -393, Width: 1080, Height: 1920},
		{Name: `\\.\DISPLAY2`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920},
	}

	match, ok := FindBestMonitor(target, connected)

	if !ok {
		t.Fatal("expected a match")
	}
	if match.Name != `\\.\DISPLAY2` {
		t.Fatalf("expected DISPLAY2, got %s", match.Name)
	}
}

func TestFindBestMonitorFallsBackToBoundsWhenNameChanges(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY9`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920, Active: true}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY1`, PositionX: -1080, PositionY: -393, Width: 1080, Height: 1920},
		{Name: `\\.\DISPLAY4`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920},
	}

	match, ok := FindBestMonitor(target, connected)

	if !ok {
		t.Fatal("expected a fallback match")
	}
	if match.Name != `\\.\DISPLAY4` {
		t.Fatalf("expected DISPLAY4, got %s", match.Name)
	}
}

func TestFindBestMonitorRejectsAmbiguousSizeOnlyFallback(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY9`, PositionX: 999, PositionY: 999, Width: 1080, Height: 1920, Active: true}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY1`, PositionX: -1080, PositionY: -393, Width: 1080, Height: 1920},
		{Name: `\\.\DISPLAY4`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920},
	}

	_, ok := FindBestMonitor(target, connected)

	if ok {
		t.Fatal("expected ambiguous size-only fallback to be rejected")
	}
}
