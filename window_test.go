package main

import "testing"

func TestMonitorBoundsUsesFullVirtualDesktopCoordinates(t *testing.T) {
	monitor := MonitorConfig{PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920}

	bounds := monitorBounds(monitor)

	if bounds != (WindowBounds{X: -2160, Y: -395, Width: 1080, Height: 1920}) {
		t.Fatalf("unexpected bounds: %+v", bounds)
	}
}

func TestWindowNeedsRepairDetectsMovedWindow(t *testing.T) {
	desired := WindowBounds{X: -2160, Y: -395, Width: 1080, Height: 1920}
	current := WindowBounds{X: -1080, Y: -395, Width: 1080, Height: 1920}

	if !windowNeedsRepair(current, desired) {
		t.Fatal("expected moved window to need repair")
	}
}

func TestWindowNeedsRepairAcceptsExactBounds(t *testing.T) {
	desired := WindowBounds{X: -2160, Y: -395, Width: 1080, Height: 1920}

	if windowNeedsRepair(desired, desired) {
		t.Fatal("expected exact bounds to be accepted")
	}
}
