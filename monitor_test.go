package main

import (
	"context"
	"testing"
	"time"
)

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

func TestResolveTargetMonitorSnapshotUsesStoredTarget(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY2`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920, Active: true}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY1`, PositionX: -1080, PositionY: -393, Width: 1080, Height: 1920},
		{Name: `\\.\DISPLAY2`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920},
	}

	match, ok, reason := resolveTargetMonitorSnapshot(target, connected)

	if !ok {
		t.Fatalf("expected stored target to be found, reason=%s", reason)
	}
	if match.Name != target.Name {
		t.Fatalf("expected %s, got %s", target.Name, match.Name)
	}
	if reason != "exact-name" {
		t.Fatalf("expected exact-name match, got %s", reason)
	}
}

func TestResolveTargetMonitorSnapshotReportsMissingTarget(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY2`, PositionX: -2160, PositionY: -395, Width: 1080, Height: 1920, Active: true}
	connected := []MonitorConfig{
		{Name: `\\.\DISPLAY1`, PositionX: -1080, PositionY: -393, Width: 1920, Height: 1080},
	}

	_, ok, reason := resolveTargetMonitorSnapshot(target, connected)

	if ok {
		t.Fatal("expected missing stored target to be reported")
	}
	if reason != "no-candidate" {
		t.Fatalf("expected no-candidate reason, got %s", reason)
	}
}

func TestShouldLogMonitorSearchAttemptThrottlesRepeatedChecks(t *testing.T) {
	if !shouldLogMonitorSearchAttempt(1) {
		t.Fatal("expected first monitor search attempt to be logged")
	}
	if shouldLogMonitorSearchAttempt(2) {
		t.Fatal("did not expect second monitor search attempt to be logged")
	}
	if !shouldLogMonitorSearchAttempt(60) {
		t.Fatal("expected periodic monitor search heartbeat to be logged")
	}
}

func TestWaitForTargetMonitorSurvivesShortDisconnect(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY2`, PositionX: 1920, PositionY: 0, Width: 1920, Height: 1080, Active: true}
	primary := MonitorConfig{Name: `\\.\DISPLAY1`, IsPrimary: true, PositionX: 0, PositionY: 0, Width: 1280, Height: 720}
	config := &AppConfig{Monitors: []MonitorConfig{target}}
	snapshots := [][]MonitorConfig{
		{primary},
		{primary},
		{primary, target},
	}
	calls := 0

	match, err := waitForTargetMonitorWithOptions(context.Background(), config, monitorWaitOptions{
		pollInterval: time.Second,
		getMonitors: func() []MonitorConfig {
			if calls >= len(snapshots) {
				return snapshots[len(snapshots)-1]
			}
			snapshot := snapshots[calls]
			calls++
			return snapshot
		},
		sleep: func(context.Context, time.Duration) error {
			return nil
		},
	})

	if err != nil {
		t.Fatalf("expected monitor to return without timeout, got %v", err)
	}
	if match.Name != target.Name {
		t.Fatalf("expected %s, got %s", target.Name, match.Name)
	}
	if calls != 3 {
		t.Fatalf("expected three monitor snapshots, got %d", calls)
	}
}

func TestWaitForTargetMonitorSurvivesLongDisconnect(t *testing.T) {
	target := MonitorConfig{Name: `\\.\DISPLAY2`, PositionX: 1920, PositionY: 0, Width: 1920, Height: 1080, Active: true}
	primary := MonitorConfig{Name: `\\.\DISPLAY1`, IsPrimary: true, PositionX: 0, PositionY: 0, Width: 1280, Height: 720}
	config := &AppConfig{Monitors: []MonitorConfig{target}}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	returnsAt := now.Add(10 * 24 * time.Hour)
	sleepCalls := 0

	match, err := waitForTargetMonitorWithOptions(context.Background(), config, monitorWaitOptions{
		pollInterval: 24 * time.Hour,
		getMonitors: func() []MonitorConfig {
			if now.Before(returnsAt) {
				return []MonitorConfig{primary}
			}
			return []MonitorConfig{primary, target}
		},
		sleep: func(_ context.Context, duration time.Duration) error {
			sleepCalls++
			now = now.Add(duration)
			return nil
		},
		now: func() time.Time {
			return now
		},
	})

	if err != nil {
		t.Fatalf("expected monitor to return after a long disconnect, got %v", err)
	}
	if match.Name != target.Name {
		t.Fatalf("expected %s, got %s", target.Name, match.Name)
	}
	if sleepCalls != 10 {
		t.Fatalf("expected ten simulated days of waiting, got %d", sleepCalls)
	}
}

func TestMonitorEnumCallbackValueIsReused(t *testing.T) {
	first := monitorEnumCallbackValue()
	second := monitorEnumCallbackValue()

	if first == 0 {
		t.Fatal("expected monitor enum callback to be initialized")
	}
	if first != second {
		t.Fatal("expected monitor enum callback to be reused between calls")
	}
}
