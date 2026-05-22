# Topmost Window Audio Tray Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Go Web Wallpaper reliably keep one borderless fullscreen WebView2 window on the selected Windows monitor, above other windows, with tray selection for monitor and audio output.

**Architecture:** Split the current single-file flow into testable selection/config helpers plus Windows-specific runtime services. A supervisor owns the WebView2 lifecycle, repeatedly enforces window bounds/topmost state, and restarts/repositions when monitor or audio settings change. Audio output selection is best-effort: list Windows render devices, save the selected device, inject JavaScript that applies `HTMLMediaElement.setSinkId()` when available, and report audio activity back to Go.

**Tech Stack:** Go, WebView2 via `github.com/jchv/go-webview2`, `github.com/getlantern/systray`, Win32 APIs through `golang.org/x/sys/windows` and `syscall`.

---

## File Structure

- Create `docs/superpowers/plans/2026-05-22-topmost-window-audio-tray.md`: this plan.
- Modify `config.go`: add stable config fields, config path handling, validation, and save helpers.
- Create `monitor.go`: monitor discovery and target matching by name/bounds instead of resolution only.
- Create `monitor_test.go`: tests for monitor matching with duplicate resolutions.
- Create `audio.go`: audio device model, fallback Windows device listing through PowerShell, and selected-device persistence helpers.
- Create `audio_test.go`: tests for audio selection/config helpers that do not require live devices.
- Create `window.go`: Win32 topmost/borderless/bounds enforcement helpers.
- Create `wallpaper.go`: WebView2 creation, audio JS injection, audio status callbacks, and supervisor loop.
- Replace tray logic in `main.go`: minimal menu for monitor, audio output, restart, exit.
- Update `.gitignore`: ignore local `config.json` and keep `config.example.json`.
- Create `config.example.json`: documented example config without machine-specific monitor positions.

---

### Task 1: Config Model and Monitor Matching

**Files:**
- Modify: `config.go`
- Create: `monitor.go`
- Create: `monitor_test.go`

- [ ] **Step 1: Write failing monitor matching tests**

```go
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
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./...`
Expected: FAIL because `FindBestMonitor` is not defined.

- [ ] **Step 3: Implement config and monitor matching**

Add these concepts:

```go
type AudioConfig struct {
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	Active bool   `json:"Active"`
}

type AppConfig struct {
	URL      string          `json:"URL"`
	Monitors []MonitorConfig `json:"Monitors"`
	Audio    AudioConfig     `json:"Audio"`
}
```

`FindBestMonitor` must score exact name highest, then exact bounds, then size as last fallback. Size-only matches must be accepted only if they are unique.

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./...`
Expected: PASS.

---

### Task 2: Window Enforcement Helpers

**Files:**
- Create: `window.go`
- Modify: `main.go`

- [ ] **Step 1: Write non-WinAPI unit tests for rectangle equality**

Create pure helpers such as `monitorBounds(m MonitorConfig) WindowBounds` and test equality/tolerance without touching WinAPI.

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./...`
Expected: FAIL because helper types/functions are missing.

- [ ] **Step 3: Implement Win32 helpers**

Implement:
- `makeWindowBorderless(hwnd windows.HWND) error`
- `forceWindowTopmost(hwnd windows.HWND, bounds WindowBounds) error`
- `readWindowBounds(hwnd windows.HWND) (WindowBounds, error)`
- `windowNeedsRepair(current, desired WindowBounds) bool`

Use `HWND_TOPMOST` and `SWP_SHOWWINDOW|SWP_FRAMECHANGED|SWP_NOACTIVATE`.

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

---

### Task 3: Audio Device Listing and Selection

**Files:**
- Create: `audio.go`
- Create: `audio_test.go`
- Modify: `config.go`

- [ ] **Step 1: Write failing tests for selected audio config**

Test that selecting an audio device persists `Audio.ID`, `Audio.Name`, and `Audio.Active`, and that selecting an empty ID clears active audio routing.

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./...`
Expected: FAIL because audio helpers are missing.

- [ ] **Step 3: Implement audio helpers**

Implement:
- `type AudioDevice struct { ID string; Name string; Default bool }`
- `ListAudioDevices() []AudioDevice`
- `SetActiveAudioDevice(id string, name string) error`
- `SelectedAudioDevice(config *AppConfig) AudioConfig`

For now, `ListAudioDevices` may use PowerShell `Get-PnpDevice -Class AudioEndpoint -Status OK` as a robust fallback that requires no extra Go dependency. Return an empty list on command failure and keep the app usable.

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

---

### Task 4: WebView2 Supervisor and Audio Probe

**Files:**
- Create: `wallpaper.go`
- Modify: `main.go`

- [ ] **Step 1: Write tests for supervisor state transitions where possible**

Test pure state helpers only: selected monitor refresh, restart decision after monitor/audio change, and no duplicate start while running.

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./...`
Expected: FAIL because supervisor helpers are missing.

- [ ] **Step 3: Implement supervisor**

Implement:
- `type WallpaperController`
- `Start() error`
- `Stop()`
- `Restart() error`
- `SetMonitor(name string) error`
- `SetAudio(device AudioDevice) error`

The supervisor must:
- resolve the current target using fresh `getMonitors()` data;
- create WebView2 with `Debug: false`;
- inject JS before navigation to observe media elements;
- call `setSinkId(selectedAudioID)` on every audio/video element when supported;
- bind a Go callback for audio status messages;
- enforce topmost/bounds every second until stopped.

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

---

### Task 5: Minimal Tray Menu

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Replace old tray flow**

Keep only:
- Start/Restart
- Monitor submenu
- Audio Output submenu
- Exit

Every monitor/audio selection saves config and restarts/repositions through `WallpaperController`.

- [ ] **Step 2: Manual build verification**

Run: `go test ./...`
Expected: PASS.

Run: `go build -o gowebwallpaper.exe .`
Expected: executable builds.

---

### Task 6: Config Hygiene and Documentation

**Files:**
- Modify: `.gitignore`
- Create: `config.example.json`
- Modify: `README.md`

- [ ] **Step 1: Ignore local config**

Add:

```gitignore
*.exe
config.json
```

- [ ] **Step 2: Add example config**

Use an example with one active monitor and optional inactive audio:

```json
{
  "URL": "http://localhost:3100/#/columns-fullscreen",
  "Monitors": [
    {
      "Name": "\\\\.\\DISPLAY2",
      "IsPrimary": false,
      "Active": true,
      "PositionX": -2160,
      "PositionY": -395,
      "Width": 1080,
      "Height": 1920
    }
  ],
  "Audio": {
    "ID": "",
    "Name": "",
    "Active": false
  }
}
```

- [ ] **Step 3: Final verification**

Run:
- `gofmt -w *.go`
- `go test ./...`
- `go vet ./...`
- `go build -o gowebwallpaper.exe .`

Expected: all commands complete successfully.

---

## Self-Review

- The plan covers strict monitor selection, topmost enforcement, tray monitor/audio settings, audio playback probing, config hygiene, and build verification.
- Audio device routing is intentionally best-effort because WebView2 does not expose a simple native output-device setter through the current Go wrapper. The implemented guarantee is: selected device is saved, JS attempts `setSinkId` on media elements, and the app logs whether playback was observed.
- No plan step depends on committing to git; the user asked to save the plan file and proceed with implementation.
