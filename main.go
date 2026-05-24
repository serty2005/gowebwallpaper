package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/getlantern/systray"
)

func main() {
	if err := initFileLogging(); err != nil {
		_ = os.WriteFile(logFileName, []byte("failed to initialize file logging: "+err.Error()+"\n"), 0644)
	}
	defer closeFileLogging()
	log.Printf("application starting")

	autoStart, err := runStartupFlow()
	if err != nil {
		if errors.Is(err, errRestarting) {
			log.Printf("application restart requested after startup flow")
			return
		}
		log.Fatalf("startup flow failed: %v", err)
	}
	runTrayApplication(autoStart)
}

func runTrayApplication(autoStart bool) {
	systray.Run(func() {
		onTrayReady(autoStart)
	}, func() {})
}

func onTrayReady(autoStart bool) {
	log.Printf("tray ready")
	systray.SetIcon(trayIconBytes())
	systray.SetTitle("Go Web Wallpaper")
	systray.SetTooltip("Go Web Wallpaper")

	controller := NewWallpaperController()
	startItem := systray.AddMenuItemCheckbox("Start", "Start or stop the topmost web window", false)

	systray.AddSeparator()
	monitorMenu := systray.AddMenuItem("Monitor", "Select target monitor")
	monitorItems := buildMonitorMenu(monitorMenu, controller)

	audioMenu := systray.AddMenuItem("Audio output", "Select audio output")
	audioItems := buildAudioMenu(audioMenu, controller)

	autostartMenu := systray.AddMenuItem("Autostart", "Start Go Web Wallpaper when Windows starts")
	autostartEnabledItem := autostartMenu.AddSubMenuItemCheckbox("Enabled", "Enable or disable Windows logon startup", false)
	go refreshAutostartMenuItem(autostartEnabledItem)

	systray.AddSeparator()
	exitItem := systray.AddMenuItem("Exit", "Exit")

	if autoStart {
		go func() {
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

	for _, entry := range monitorItems {
		entry := entry
		go func() {
			for range entry.item.ClickedCh {
				log.Printf("tray monitor selected: %s", entry.name)
				checkOnly(entry.item, collectMonitorMenuItems(monitorItems))
				if err := controller.SetMonitor(entry.name); err != nil {
					log.Printf("monitor switch failed: %v", err)
				}
			}
		}()
	}

	for _, entry := range audioItems {
		entry := entry
		go func() {
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
		for range autostartEnabledItem.ClickedCh {
			wantEnabled := !autostartEnabledItem.Checked()
			autostartEnabledItem.Disable()
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
			refreshAutostartMenuItem(autostartEnabledItem)
			autostartEnabledItem.Enable()
		}
	}()

	go func() {
		<-exitItem.ClickedCh
		log.Printf("exit requested")
		controller.Stop()
		systray.Quit()
		os.Exit(0)
	}()
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
	name string
	item *systray.MenuItem
}

type audioMenuEntry struct {
	device AudioDevice
	item   *systray.MenuItem
}

func buildMonitorMenu(parent *systray.MenuItem, controller *WallpaperController) []monitorMenuEntry {
	_ = controller
	config, err := loadConfig()
	if err != nil {
		log.Printf("load config for monitor menu: %v", err)
	}
	active, _ := activeMonitor(config)
	connected := getMonitors()
	if config != nil {
		replaceConfigMonitors(config, connected, active.Name)
		_ = saveConfig(config)
	}
	log.Printf("building monitor menu: connected=%d active=%s", len(connected), active.Name)

	entries := make([]monitorMenuEntry, 0, len(connected))
	for _, monitor := range connected {
		title := fmt.Sprintf("%s (%dx%d @ %d,%d)", monitor.Name, monitor.Width, monitor.Height, monitor.PositionX, monitor.PositionY)
		if monitor.IsPrimary {
			title += " [primary]"
		}
		item := parent.AddSubMenuItemCheckbox(title, "Use this monitor", monitor.Name == active.Name)
		entries = append(entries, monitorMenuEntry{name: monitor.Name, item: item})
	}
	return entries
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
