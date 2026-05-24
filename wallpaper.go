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
	debugLogf("WallpaperController.Start input")
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
		debugLogf("WallpaperController.Start output error=%v", err)
		return err
	}
	log.Printf("wallpaper started")
	debugLogf("WallpaperController.Start output running=true")
	return nil
}

func (c *WallpaperController) Stop() {
	debugLogf("WallpaperController.Stop input")
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
			defer recoverAndLogPanic("webview destroy dispatch")
			debugLogf("WallpaperController.Stop dispatch Destroy input")
			w.Destroy()
			debugLogf("WallpaperController.Stop dispatch Destroy output")
		})
	}
	debugLogf("WallpaperController.Stop output hadWindow=%t", w != nil)
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

func (c *WallpaperController) SetURL(url string) error {
	log.Printf("set URL requested")
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if !setConfigURL(config, url) {
		log.Printf("URL unchanged")
		return nil
	}
	if err := saveConfig(config); err != nil {
		return err
	}
	if shouldRestartAfterConfigChange(c.IsRunning()) {
		log.Printf("URL saved, restarting")
		return c.Restart()
	}
	log.Printf("URL saved while stopped")
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
	startedSent := false
	defer func() {
		if recovered := recover(); recovered != nil {
			logRecoveredPanic("webview goroutine", recovered)
			if !startedSent {
				started <- fmt.Errorf("webview goroutine panic: %v", recovered)
			}
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	runStartedAt := time.Now()
	debugLogf("runWebView input")

	config, err := loadConfig()
	if err != nil {
		log.Printf("load config before webview failed: %v", err)
		started <- err
		startedSent = true
		return
	}
	if target, ok := activeMonitor(config); ok {
		log.Printf("webview target from config: %s", formatMonitor(target))
	} else {
		log.Printf("webview target from config: none")
	}
	configureLoggingFromConfig(config)
	debugLogf("runWebView config url=%q audioActive=%t log=%q", config.URL, config.Audio.Active, config.Log)
	target, ok := activeMonitor(config)
	if !ok {
		err := fmt.Errorf("no active monitor in config")
		log.Printf("target monitor wait failed: %v", err)
		started <- err
		startedSent = true
		return
	}
	monitor, err := waitForTargetMonitor(ctx, config, 20*time.Second)
	if err != nil {
		log.Printf("target monitor wait failed: %v", err)
		started <- err
		startedSent = true
		return
	}
	log.Printf("target monitor resolved: target=%s connected=%s", formatMonitor(target), formatMonitor(monitor))

	for {
		w, hwnd, err := c.createWebViewWindow(config, monitor)
		if err != nil {
			if !startedSent {
				started <- err
				startedSent = true
			} else {
				log.Printf("webview resume failed: %v", err)
				c.clearIdleCancel()
			}
			return
		}

		c.mu.Lock()
		c.webview = w
		c.hwnd = hwnd
		c.running = true
		c.mu.Unlock()

		monitorMissing := make(chan struct{}, 1)
		go c.enforceLoop(ctx, w, hwnd, target, monitorMissing)
		if !startedSent {
			started <- nil
			startedSent = true
		}

		w.Run()
		log.Printf("webview run loop exited after %s", time.Since(runStartedAt).Round(time.Second))
		debugLogf("runWebView window output duration=%s", time.Since(runStartedAt))

		c.mu.Lock()
		if c.webview == w {
			c.webview = nil
			c.hwnd = 0
			c.running = false
		}
		c.mu.Unlock()

		if ctx.Err() != nil {
			c.mu.Lock()
			if c.webview == nil {
				c.cancel = nil
				c.running = false
			}
			c.mu.Unlock()
			debugLogf("runWebView output cancelled duration=%s", time.Since(runStartedAt))
			return
		}

		select {
		case <-monitorMissing:
			log.Printf("waiting for target monitor to return: %s", formatMonitor(target))
			monitor, err = waitForTargetMonitor(ctx, &AppConfig{Monitors: []MonitorConfig{target}}, 24*time.Hour)
			if err != nil {
				log.Printf("target monitor wait after disappearance stopped: %v", err)
				c.clearIdleCancel()
				return
			}
			log.Printf("target monitor returned: %s", formatMonitor(monitor))
		default:
			log.Printf("webview exited without monitor disappearance; not restarting automatically")
			c.clearIdleCancel()
			return
		}
	}
}

func (c *WallpaperController) clearIdleCancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.webview == nil {
		c.cancel = nil
		c.running = false
	}
}

func (c *WallpaperController) createWebViewWindow(config *AppConfig, monitor MonitorConfig) (webview2.WebView, windows.HWND, error) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug: debugLoggingEnabled(),
		WindowOptions: webview2.WindowOptions{
			Title:  "Go Web Wallpaper",
			Width:  uint(monitor.Width),
			Height: uint(monitor.Height),
		},
	})
	if w == nil {
		log.Printf("webview creation returned nil")
		debugLogf("createWebViewWindow output error=webview nil")
		return nil, 0, fmt.Errorf("failed to create WebView2 window")
	}
	debugLogf("webview created debug=%t", debugLoggingEnabled())

	if err := w.Bind("goWebWallpaperAudioStatus", c.receiveAudioStatus); err != nil {
		log.Printf("audio status bind failed: %v", err)
		w.Destroy()
		return nil, 0, err
	}
	log.Printf("audio probe configured: active=%t name=%q id=%q", config.Audio.Active, config.Audio.Name, config.Audio.ID)
	w.Init(buildAudioProbeScript(config.Audio))
	w.SetSize(monitor.Width, monitor.Height, webview2.HintFixed)
	w.SetTitle("Go Web Wallpaper")

	hwnd := windows.HWND(w.Window())
	if err := makeWindowBorderless(hwnd); err != nil {
		log.Printf("make window borderless failed: %v", err)
		w.Destroy()
		return nil, 0, err
	}
	if err := forceWindowTopmost(hwnd, monitorBounds(monitor)); err != nil {
		log.Printf("force topmost failed: %v", err)
		w.Destroy()
		return nil, 0, err
	}
	log.Printf("window positioned topmost: hwnd=%v bounds=%+v", hwnd, monitorBounds(monitor))
	debugLogf("window setup output hwnd=%v bounds=%+v", hwnd, monitorBounds(monitor))

	w.Navigate(config.URL)
	log.Printf("webview navigating: %s", config.URL)
	debugLogf("webview Navigate input url=%q", config.URL)
	return w, hwnd, nil
}

func waitForTargetMonitor(ctx context.Context, config *AppConfig, timeout time.Duration) (MonitorConfig, error) {
	debugLogf("waitForTargetMonitor input timeout=%s", timeout)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	attempt := 0
	for {
		attempt++
		connected := getMonitors()
		target, hasTarget := activeMonitor(config)
		if shouldLogMonitorSearchAttempt(attempt) {
			debugLogf("monitor search attempt=%d connected=%s hasTarget=%t target=%s", attempt, formatMonitors(connected), hasTarget, formatMonitor(target))
		}
		if !hasTarget {
			if shouldLogMonitorSearchAttempt(attempt) {
				log.Printf("monitor search attempt %d: no active monitor in config; connected=%s", attempt, formatMonitors(connected))
			}
		} else if monitor, ok, reason := FindBestMonitorWithReason(target, connected); ok {
			if shouldLogMonitorSearchAttempt(attempt) {
				log.Printf("monitor search attempt %d: matched target=%s as=%s reason=%s", attempt, formatMonitor(target), formatMonitor(monitor), reason)
			}
			return monitor, nil
		} else {
			if shouldLogMonitorSearchAttempt(attempt) {
				log.Printf("monitor search attempt %d: no match target=%s reason=%s connected=%s", attempt, formatMonitor(target), reason, formatMonitors(connected))
			}
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

func shouldLogMonitorSearchAttempt(attempt int) bool {
	return attempt == 1 || attempt%60 == 0
}

func (c *WallpaperController) enforceLoop(ctx context.Context, w webview2.WebView, hwnd windows.HWND, target MonitorConfig, monitorMissing chan<- struct{}) {
	defer recoverAndLogPanic("enforce loop")
	debugLogf("enforceLoop input hwnd=%v target=%s", hwnd, formatMonitor(target))
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			debugLogf("enforceLoop output cancelled error=%v attempts=%d", ctx.Err(), attempt)
			return
		case <-tick.C:
			attempt++
			connected := getMonitors()
			monitor, ok, reason := resolveTargetMonitorSnapshot(target, connected)
			if !ok {
				log.Printf("target monitor disappeared: target=%s reason=%s connected=%s", formatMonitor(target), reason, formatMonitors(connected))
				select {
				case monitorMissing <- struct{}{}:
				default:
				}
				w.Dispatch(func() {
					defer recoverAndLogPanic("monitor missing destroy dispatch")
					w.Destroy()
				})
				return
			}
			desired := monitorBounds(monitor)
			if attempt == 1 || attempt%60 == 0 {
				debugLogf("enforceLoop heartbeat tick=%d desired=%+v matchReason=%s", attempt, desired, reason)
			}
			w.Dispatch(func() {
				defer recoverAndLogPanic("enforce dispatch")
				current, err := readWindowBounds(hwnd)
				if err == nil && !windowNeedsRepair(current, desired) {
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
	debugLogf("receiveAudioStatus input payload=%s", payload)
	c.mu.Lock()
	c.lastAudioStatus = payload
	c.mu.Unlock()
	if strings.Contains(payload, `"event":"playing"`) ||
		strings.Contains(payload, `"event":"sink-applied"`) ||
		strings.Contains(payload, `"event":"sink-failed"`) {
		log.Printf("audio status: %s", payload)
	}
	debugLogf("receiveAudioStatus output stored=true")
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
