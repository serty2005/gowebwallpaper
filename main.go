package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/getlantern/systray"
)

func main() {
	if supervised, err := maybeRunSupervisor(); supervised {
		if err != nil {
			appendSupervisorLog("supervisor failed, running child inline: error=%v", err)
		} else {
			return
		}
	}
	if err := initFileLogging(); err != nil {
		_ = os.WriteFile(logFileName, []byte("failed to initialize file logging: "+err.Error()+"\n"), 0644)
	}
	defer closeFileLogging()
	defer recoverAndLogPanic("main")
	log.Printf("application starting: pid=%d version=%s go=%s args=%q", os.Getpid(), appVersion, runtime.Version(), os.Args)
	startSignalLogging()
	configureLoggingFromConfigFile()

	autoStart, err := runStartupFlow()
	if err != nil {
		if errors.Is(err, errRestarting) {
			log.Printf("application restart requested after startup flow")
			return
		}
		log.Fatalf("startup flow failed: %v", err)
	}
	runTrayApplication(autoStart)
	log.Printf("tray application returned")
}

func runTrayApplication(autoStart bool) {
	systray.Run(func() {
		defer recoverAndLogPanic("systray ready")
		onTrayReady(autoStart)
	}, func() {
		defer recoverAndLogPanic("systray exit")
		log.Printf("tray exit callback")
	})
}

func startSignalLogging() {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer recoverAndLogPanic("signal logger")
		for signal := range signals {
			log.Printf("process signal received: %v", signal)
		}
	}()
}

func onTrayReady(autoStart bool) {
	log.Printf("tray ready")
	systray.SetIcon(trayIconBytes())
	systray.SetTitle("Go Web Wallpaper")
	systray.SetTooltip("Go Web Wallpaper")

	controller := NewWallpaperController()
	startItem := systray.AddMenuItemCheckbox("Start", "Start or stop the topmost web window", false)
	urlItem := systray.AddMenuItem("URL", "Set web page URL")

	systray.AddSeparator()
	monitorMenu := systray.AddMenuItem("Monitor", "Select target monitor")
	monitorItems := buildMonitorMenu(monitorMenu, controller)

	audioMenu := systray.AddMenuItem("Audio output", "Select audio output")
	audioItems := buildAudioMenu(audioMenu, controller)

	autostartItem := systray.AddMenuItemCheckbox("Autostart", "Start Go Web Wallpaper when Windows starts", false)
	go func() {
		defer recoverAndLogPanic("autostart refresh goroutine")
		refreshAutostartMenuItem(autostartItem)
	}()

	systray.AddSeparator()
	exitItem := systray.AddMenuItem("Exit", "Exit")

	if autoStart {
		go func() {
			defer recoverAndLogPanic("auto start goroutine")
			log.Printf("auto start requested")
			if err := controller.Start(); err != nil {
				log.Printf("start failed: %v", err)
				startItem.Uncheck()
				return
			}
			startItem.Check()
		}()
	}

	go func() {
		defer recoverAndLogPanic("start menu goroutine")
		for range startItem.ClickedCh {
			if controller.IsRunning() {
				log.Printf("tray stop requested")
				controller.Stop()
				startItem.Uncheck()
				continue
			}
			log.Printf("tray start requested")
			if err := controller.Start(); err != nil {
				log.Printf("start failed: %v", err)
				startItem.Uncheck()
				continue
			}
			startItem.Check()
		}
	}()

	go func() {
		defer recoverAndLogPanic("url menu goroutine")
		for range urlItem.ClickedCh {
			config, err := loadConfig()
			if err != nil {
				log.Printf("load config for URL prompt failed: %v", err)
				continue
			}
			selected, err := runURLPromptPowerShell(config.URL, false, "")
			if err != nil {
				log.Printf("URL prompt failed: %v", err)
				continue
			}
			if err := controller.SetURL(selected); err != nil {
				log.Printf("URL update failed: %v", err)
			}
		}
	}()

	for _, entry := range monitorItems {
		entry := entry
		go func() {
			defer recoverAndLogPanic("monitor menu goroutine")
			for range entry.item.ClickedCh {
				log.Printf("tray monitor selected: %s", entry.name)
				checkOnly(entry.item, collectMonitorMenuItems(monitorItems))
				if err := controller.SetMonitor(entry.name); err != nil {
					log.Printf("monitor switch failed: %v", err)
					continue
				}
				hideDisconnectedMonitorItems(monitorItems)
			}
		}()
	}

	for _, entry := range audioItems {
		entry := entry
		go func() {
			defer recoverAndLogPanic("audio menu goroutine")
			for range entry.item.ClickedCh {
				log.Printf("tray audio selected: name=%q id=%q", entry.device.Name, entry.device.ID)
				checkOnly(entry.item, collectAudioMenuItems(audioItems))
				if err := controller.SetAudio(entry.device); err != nil {
					log.Printf("audio switch failed: %v", err)
				}
			}
		}()
	}

	go func() {
		defer recoverAndLogPanic("autostart menu goroutine")
		for range autostartItem.ClickedCh {
			wantEnabled := !autostartItem.Checked()
			autostartItem.Disable()
			var err error
			if wantEnabled {
				log.Printf("autostart enable requested")
				err = EnableAutostart()
			} else {
				log.Printf("autostart disable requested")
				err = DisableAutostart()
			}
			if err != nil {
				log.Printf("autostart update failed: %v", err)
			}
			refreshAutostartMenuItem(autostartItem)
			autostartItem.Enable()
		}
	}()

	go func() {
		defer recoverAndLogPanic("exit menu goroutine")
		<-exitItem.ClickedCh
		log.Printf("exit requested")
		controller.Stop()
		systray.Quit()
		os.Exit(0)
	}()
}

func configureLoggingFromConfigFile() {
	config, err := loadConfig()
	if err != nil {
		log.Printf("debug logging config preload skipped: %v", err)
		return
	}
	configureLoggingFromConfig(config)
	debugLogf("debug logging config preload output log=%q", config.Log)
}

func refreshAutostartMenuItem(item *systray.MenuItem) {
	enabled, err := AutostartEnabled()
	if err != nil {
		log.Printf("autostart check failed: %v", err)
		item.Uncheck()
		return
	}
	if enabled {
		item.Check()
		return
	}
	item.Uncheck()
}

type monitorMenuEntry struct {
	name    string
	item    *systray.MenuItem
	enabled bool
}

type audioMenuEntry struct {
	device AudioDevice
	item   *systray.MenuItem
}

type monitorMenuOption struct {
	Monitor MonitorConfig
	Title   string
	Checked bool
	Enabled bool
}

func buildMonitorMenu(parent *systray.MenuItem, controller *WallpaperController) []monitorMenuEntry {
	_ = controller
	config, err := loadConfig()
	if err != nil {
		log.Printf("load config for monitor menu: %v", err)
	}
	connected := getMonitors()
	options, activeConnectedName := buildMonitorMenuOptions(config, connected)
	if config != nil && activeConnectedName != "" {
		replaceConfigMonitors(config, connected, activeConnectedName)
		_ = saveConfig(config)
	}
	log.Printf("building monitor menu: connected=%d active=%s", len(connected), activeConnectedName)

	entries := make([]monitorMenuEntry, 0, len(options))
	for _, option := range options {
		item := parent.AddSubMenuItemCheckbox(option.Title, "Use this monitor", option.Checked)
		if !option.Enabled {
			item.Disable()
		}
		entries = append(entries, monitorMenuEntry{name: option.Monitor.Name, item: item, enabled: option.Enabled})
	}
	return entries
}

func buildMonitorMenuOptions(config *AppConfig, connected []MonitorConfig) ([]monitorMenuOption, string) {
	active, hasActive := activeMonitor(config)
	activeConnectedName := ""
	if hasActive {
		if resolved, ok := FindBestMonitor(active, connected); ok {
			activeConnectedName = resolved.Name
		}
	}

	options := make([]monitorMenuOption, 0, len(connected)+1)
	for _, monitor := range connected {
		options = append(options, monitorMenuOption{
			Monitor: monitor,
			Title:   formatMonitorMenuTitle(monitor, false),
			Checked: monitor.Name == activeConnectedName,
			Enabled: true,
		})
	}
	if hasActive && activeConnectedName == "" {
		options = append(options, monitorMenuOption{
			Monitor: active,
			Title:   formatMonitorMenuTitle(active, true),
			Checked: true,
			Enabled: false,
		})
	}
	return options, activeConnectedName
}

func formatMonitorMenuTitle(monitor MonitorConfig, disconnected bool) string {
	title := fmt.Sprintf("%s (%dx%d @ %d,%d)", monitor.Name, monitor.Width, monitor.Height, monitor.PositionX, monitor.PositionY)
	if monitor.IsPrimary {
		title += " [primary]"
	}
	if disconnected {
		title += " [disconnected]"
	}
	return title
}

func hideDisconnectedMonitorItems(entries []monitorMenuEntry) {
	for _, entry := range entries {
		if !entry.enabled {
			entry.item.Hide()
		}
	}
}

func buildAudioMenu(parent *systray.MenuItem, controller *WallpaperController) []audioMenuEntry {
	_ = controller
	config, err := loadConfig()
	if err != nil {
		log.Printf("load config for audio menu: %v", err)
	}
	selected := SelectedAudioDevice(config)
	devices := ListAudioDevices()
	log.Printf("building audio menu: devices=%d active=%t selected=%q", len(devices), selected.Active, selected.Name)
	entries := []audioMenuEntry{
		{device: AudioDevice{}, item: parent.AddSubMenuItemCheckbox("System default", "Use Windows default output", !selected.Active)},
	}
	for _, device := range devices {
		title := device.Name
		if title == "" {
			title = device.ID
		}
		checked := selected.Active && (selected.ID == device.ID || selected.Name == device.Name)
		item := parent.AddSubMenuItemCheckbox(title, "Try to route media elements to this output", checked)
		entries = append(entries, audioMenuEntry{device: device, item: item})
	}
	return entries
}

func collectMonitorMenuItems(entries []monitorMenuEntry) []*systray.MenuItem {
	items := make([]*systray.MenuItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entry.item)
	}
	return items
}

func collectAudioMenuItems(entries []audioMenuEntry) []*systray.MenuItem {
	items := make([]*systray.MenuItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entry.item)
	}
	return items
}

func checkOnly(active *systray.MenuItem, items []*systray.MenuItem) {
	for _, item := range items {
		if item == active {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func performDiagnosticRun() error {
	monitors := getMonitors()
	if len(monitors) == 0 {
		return fmt.Errorf("no monitors found")
	}
	log.Printf("diagnostic found monitors: %s", formatMonitors(monitors))
	config := &AppConfig{
		URL:      defaultURL,
		Monitors: monitors,
	}
	for i, monitor := range config.Monitors {
		if monitor.IsPrimary {
			config.Monitors[i].Active = true
			break
		}
	}
	if err := saveConfig(config); err != nil {
		return err
	}
	path, _ := getConfigPath()
	log.Printf("created config at %s", path)
	return nil
}
