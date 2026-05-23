package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

type WallpaperController struct {
	mu              sync.Mutex
	webview         webview2.WebView
	hwnd            windows.HWND
	cancel          context.CancelFunc
	running         bool
	lastAudioStatus string
}

func NewWallpaperController() *WallpaperController {
	return &WallpaperController{}
}

func (c *WallpaperController) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *WallpaperController) Start() error {
	c.mu.Lock()
	if c.running {
		log.Printf("start ignored: wallpaper already running")
		c.mu.Unlock()
		return nil
	}
	log.Printf("starting wallpaper")
	ctx, cancel := context.WithCancel(context.Background())
	c.running = true
	c.cancel = cancel
	c.mu.Unlock()

	started := make(chan error, 1)
	go c.runWebView(ctx, started)

	if err := <-started; err != nil {
		c.mu.Lock()
		cancel()
		c.running = false
		c.cancel = nil
		c.webview = nil
		c.hwnd = 0
		c.mu.Unlock()
		log.Printf("wallpaper start failed: %v", err)
		return err
	}
	log.Printf("wallpaper started")
	return nil
}

func (c *WallpaperController) Stop() {
	c.mu.Lock()
	w := c.webview
	cancel := c.cancel
	c.webview = nil
	c.hwnd = 0
	c.cancel = nil
	c.running = false
	c.mu.Unlock()

	log.Printf("stopping wallpaper: hadWindow=%t", w != nil)
	if cancel != nil {
		cancel()
	}
	if w != nil {
		w.Dispatch(func() {
			w.Destroy()
		})
	}
}

func (c *WallpaperController) Restart() error {
	log.Printf("restarting wallpaper")
	c.Stop()
	time.Sleep(300 * time.Millisecond)
	return c.Start()
}

func (c *WallpaperController) SetMonitor(name string) error {
	log.Printf("set monitor requested: %s", name)
	config, err := loadConfig()
	if err != nil {
		return err
	}
	connected := getMonitors()
	log.Printf("available monitors for selection: %s", formatMonitors(connected))
	replaceConfigMonitors(config, connected, name)
	if err := saveConfig(config); err != nil {
		return err
	}
	if shouldRestartAfterConfigChange(c.IsRunning()) {
		log.Printf("monitor saved, restarting: %s", name)
		return c.Restart()
	}
	log.Printf("monitor saved while stopped: %s", name)
	return nil
}

func (c *WallpaperController) SetAudio(device AudioDevice) error {
	log.Printf("set audio requested: name=%q id=%q", device.Name, device.ID)
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if device.ID == "" && device.Name == "" {
		ClearAudioDevice(config)
	} else {
		SelectAudioDevice(config, device)
	}
	if err := saveConfig(config); err != nil {
		return err
	}
	if shouldRestartAfterConfigChange(c.IsRunning()) {
		log.Printf("audio saved, restarting: active=%t name=%q id=%q", config.Audio.Active, config.Audio.Name, config.Audio.ID)
		return c.Restart()
	}
	log.Printf("audio saved while stopped: active=%t name=%q id=%q", config.Audio.Active, config.Audio.Name, config.Audio.ID)
	return nil
}

func (c *WallpaperController) runWebView(ctx context.Context, started chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	config, err := loadConfig()
	if err != nil {
		log.Printf("load config before webview failed: %v", err)
		started <- err
		return
	}
	if target, ok := activeMonitor(config); ok {
		log.Printf("webview target from config: %s", formatMonitor(target))
	} else {
		log.Printf("webview target from config: none")
	}
	monitor, err := waitForTargetMonitor(ctx, config, 20*time.Second)
	if err != nil {
		log.Printf("target monitor wait failed: %v", err)
		started <- err
		return
	}
	log.Printf("target monitor resolved: %s", formatMonitor(monitor))

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "Go Web Wallpaper",
			Width:  uint(monitor.Width),
			Height: uint(monitor.Height),
		},
	})
	if w == nil {
		log.Printf("webview creation returned nil")
		started <- fmt.Errorf("failed to create WebView2 window")
		return
	}

	if err := w.Bind("goWebWallpaperAudioStatus", c.receiveAudioStatus); err != nil {
		log.Printf("audio status bind failed: %v", err)
		w.Destroy()
		started <- err
		return
	}
	log.Printf("audio probe configured: active=%t name=%q id=%q", config.Audio.Active, config.Audio.Name, config.Audio.ID)
	w.Init(buildAudioProbeScript(config.Audio))
	w.SetSize(monitor.Width, monitor.Height, webview2.HintFixed)
	w.SetTitle("Go Web Wallpaper")

	hwnd := windows.HWND(w.Window())
	if err := makeWindowBorderless(hwnd); err != nil {
		log.Printf("make window borderless failed: %v", err)
		w.Destroy()
		started <- err
		return
	}
	if err := forceWindowTopmost(hwnd, monitorBounds(monitor)); err != nil {
		log.Printf("force topmost failed: %v", err)
		w.Destroy()
		started <- err
		return
	}
	log.Printf("window positioned topmost: hwnd=%v bounds=%+v", hwnd, monitorBounds(monitor))

	w.Navigate(config.URL)
	log.Printf("webview navigating: %s", config.URL)

	c.mu.Lock()
	c.webview = w
	c.hwnd = hwnd
	c.running = true
	c.mu.Unlock()

	go c.enforceLoop(ctx, w, hwnd)
	started <- nil

	w.Run()
	log.Printf("webview run loop exited")

	c.mu.Lock()
	if c.webview == w {
		c.webview = nil
		c.hwnd = 0
		c.cancel = nil
		c.running = false
	}
	c.mu.Unlock()
}

func waitForTargetMonitor(ctx context.Context, config *AppConfig, timeout time.Duration) (MonitorConfig, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	attempt := 0
	for {
		attempt++
		connected := getMonitors()
		target, hasTarget := activeMonitor(config)
		if !hasTarget {
			log.Printf("monitor search attempt %d: no active monitor in config; connected=%s", attempt, formatMonitors(connected))
		} else if monitor, ok, reason := FindBestMonitorWithReason(target, connected); ok {
			log.Printf("monitor search attempt %d: matched target=%s as=%s reason=%s", attempt, formatMonitor(target), formatMonitor(monitor), reason)
			return monitor, nil
		} else {
			log.Printf("monitor search attempt %d: no match target=%s reason=%s connected=%s", attempt, formatMonitor(target), reason, formatMonitors(connected))
		}
		select {
		case <-ctx.Done():
			log.Printf("monitor search cancelled after %d attempts", attempt)
			return MonitorConfig{}, ctx.Err()
		case <-deadline.C:
			log.Printf("monitor search timed out after %d attempts", attempt)
			return MonitorConfig{}, fmt.Errorf("target monitor was not found")
		case <-tick.C:
		}
	}
}

func (c *WallpaperController) enforceLoop(ctx context.Context, w webview2.WebView, hwnd windows.HWND) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			config, err := loadConfig()
			if err != nil {
				log.Printf("load config during enforcement: %v", err)
				continue
			}
			monitor, ok := resolveTargetMonitor(config, getMonitors())
			if !ok {
				log.Printf("target monitor disappeared")
				continue
			}
			desired := monitorBounds(monitor)
			w.Dispatch(func() {
				current, err := readWindowBounds(hwnd)
				if err == nil && !windowNeedsRepair(current, desired) {
					_ = forceWindowTopmost(hwnd, desired)
					return
				}
				if err != nil {
					log.Printf("window bounds read failed, repairing anyway: %v", err)
				} else {
					log.Printf("window repair needed: current=%+v desired=%+v", current, desired)
				}
				if err := makeWindowBorderless(hwnd); err != nil {
					log.Printf("borderless repair failed: %v", err)
				}
				if err := forceWindowTopmost(hwnd, desired); err != nil {
					log.Printf("topmost repair failed: %v", err)
				}
			})
		}
	}
}

func (c *WallpaperController) receiveAudioStatus(payload string) error {
	c.mu.Lock()
	c.lastAudioStatus = payload
	c.mu.Unlock()
	if strings.Contains(payload, `"event":"playing"`) ||
		strings.Contains(payload, `"event":"sink-applied"`) ||
		strings.Contains(payload, `"event":"sink-failed"`) {
		log.Printf("audio status: %s", payload)
	}
	return nil
}

func buildAudioProbeScript(audio AudioConfig) string {
	selectedID, _ := json.Marshal(audio.ID)
	selectedName, _ := json.Marshal(audio.Name)
	active := "false"
	if audio.Active {
		active = "true"
	}
	return fmt.Sprintf(`(function() {
  const selectedId = %s;
  const selectedName = %s;
  const routingActive = %s;
  const seen = new WeakSet();

  function send(event, data) {
    try {
      if (window.goWebWallpaperAudioStatus) {
        window.goWebWallpaperAudioStatus(JSON.stringify(Object.assign({event: event}, data || {})));
      }
    } catch (_) {}
  }

  async function pickSink() {
    if (!routingActive || !navigator.mediaDevices || !navigator.mediaDevices.enumerateDevices) {
      return null;
    }
    const devices = await navigator.mediaDevices.enumerateDevices();
    const outputs = devices.filter(d => d.kind === 'audiooutput');
    send('devices', {count: outputs.length, labels: outputs.map(d => d.label || d.deviceId)});
    let match = outputs.find(d => d.deviceId === selectedId);
    if (!match && selectedName) {
      const needle = selectedName.toLowerCase();
      match = outputs.find(d => (d.label || '').toLowerCase() === needle) ||
              outputs.find(d => (d.label || '').toLowerCase().indexOf(needle) >= 0 ||
                                  needle.indexOf((d.label || '').toLowerCase()) >= 0);
    }
    return match || null;
  }

  async function wireMedia(el) {
    if (!el || seen.has(el)) return;
    seen.add(el);
    el.muted = false;
    el.addEventListener('playing', () => send('playing', {tag: el.tagName, muted: el.muted, volume: el.volume}));
    el.addEventListener('error', () => send('media-error', {tag: el.tagName}));
    if (!routingActive) return;
    if (!el.setSinkId) {
      send('sink-unsupported', {tag: el.tagName});
      return;
    }
    try {
      const sink = await pickSink();
      if (!sink) {
        send('sink-not-found', {tag: el.tagName, selectedName: selectedName});
        return;
      }
      await el.setSinkId(sink.deviceId);
      send('sink-applied', {tag: el.tagName, label: sink.label || sink.deviceId});
    } catch (err) {
      send('sink-failed', {tag: el.tagName, message: String(err && err.message || err)});
    }
  }

  function scan() {
    document.querySelectorAll('audio,video').forEach(wireMedia);
  }

  document.addEventListener('DOMContentLoaded', scan);
  window.addEventListener('load', scan);
  new MutationObserver(scan).observe(document.documentElement || document, {childList: true, subtree: true});
  setInterval(scan, 3000);
  send('probe-ready', {routingActive: routingActive, selectedName: selectedName});
})();`, string(selectedID), string(selectedName), active)
}
