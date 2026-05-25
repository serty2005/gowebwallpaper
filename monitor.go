package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const monitorInfoPrimary = 0x00000001

type rect struct {
	Left, Top, Right, Bottom int32
}

type monitorInfoEx struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
	SzDevice  [32]uint16
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	monitorEnumCallback     = syscall.NewCallback(enumDisplayMonitorCallback)
)

func getMonitors() []MonitorConfig {
	var monitors []MonitorConfig
	procEnumDisplayMonitors.Call(0, 0, monitorEnumCallbackValue(), uintptr(unsafe.Pointer(&monitors)))
	return monitors
}

func monitorEnumCallbackValue() uintptr {
	return monitorEnumCallback
}

func enumDisplayMonitorCallback(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
	monitors := (*[]MonitorConfig)(unsafe.Pointer(dwData))
	if monitors == nil {
		return 1
	}
	var mi monitorInfoEx
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return 1
	}
	*monitors = append(*monitors, MonitorConfig{
		Name:      windows.UTF16ToString(mi.SzDevice[:]),
		IsPrimary: mi.DwFlags&monitorInfoPrimary != 0,
		PositionX: int(mi.RcMonitor.Left),
		PositionY: int(mi.RcMonitor.Top),
		Width:     int(mi.RcMonitor.Right - mi.RcMonitor.Left),
		Height:    int(mi.RcMonitor.Bottom - mi.RcMonitor.Top),
	})
	return 1
}

func FindBestMonitor(target MonitorConfig, connected []MonitorConfig) (MonitorConfig, bool) {
	monitor, ok, _ := FindBestMonitorWithReason(target, connected)
	return monitor, ok
}

func FindBestMonitorWithReason(target MonitorConfig, connected []MonitorConfig) (MonitorConfig, bool, string) {
	for _, monitor := range connected {
		if monitor.Name != "" && monitor.Name == target.Name {
			return monitor, true, "exact-name"
		}
	}
	for _, monitor := range connected {
		if sameBounds(target, monitor) {
			return monitor, true, "exact-bounds"
		}
	}

	var sizeMatch MonitorConfig
	matches := 0
	for _, monitor := range connected {
		if monitor.Width == target.Width && monitor.Height == target.Height {
			sizeMatch = monitor
			matches++
		}
	}
	if matches == 1 {
		return sizeMatch, true, "unique-size"
	}
	if matches > 1 {
		return MonitorConfig{}, false, "ambiguous-size"
	}
	return MonitorConfig{}, false, "no-candidate"
}

func sameBounds(a, b MonitorConfig) bool {
	return a.PositionX == b.PositionX &&
		a.PositionY == b.PositionY &&
		a.Width == b.Width &&
		a.Height == b.Height
}

func resolveTargetMonitor(config *AppConfig, connected []MonitorConfig) (MonitorConfig, bool) {
	target, ok := activeMonitor(config)
	if !ok {
		return MonitorConfig{}, false
	}
	return FindBestMonitor(target, connected)
}

func resolveTargetMonitorSnapshot(target MonitorConfig, connected []MonitorConfig) (MonitorConfig, bool, string) {
	return FindBestMonitorWithReason(target, connected)
}

func formatMonitor(monitor MonitorConfig) string {
	return monitor.Name + " " +
		fmt.Sprintf("%dx%d@%d,%d primary=%t active=%t", monitor.Width, monitor.Height, monitor.PositionX, monitor.PositionY, monitor.IsPrimary, monitor.Active)
}

func formatMonitors(monitors []MonitorConfig) string {
	if len(monitors) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(monitors))
	for _, monitor := range monitors {
		parts = append(parts, formatMonitor(monitor))
	}
	return "[" + strings.Join(parts, "; ") + "]"
}
