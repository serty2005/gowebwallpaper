package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	gwlStyle       = 0xFFFFFFF0
	wsCaption      = 0x00C00000
	wsThickFrame   = 0x00040000
	wsMinimizeBox  = 0x00020000
	wsMaximizeBox  = 0x00010000
	wsSysMenu      = 0x00080000
	swpFrameChange = 0x0020
	swpNoActivate  = 0x0010
	swpShowWindow  = 0x0040
)

var (
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procGetWindowRect     = user32.NewProc("GetWindowRect")
	procIsWindow          = user32.NewProc("IsWindow")
	hwndTopmost           = uintptr(^uintptr(0))
)

type WindowBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

func monitorBounds(monitor MonitorConfig) WindowBounds {
	return WindowBounds{
		X:      monitor.PositionX,
		Y:      monitor.PositionY,
		Width:  monitor.Width,
		Height: monitor.Height,
	}
}

func makeWindowBorderless(hwnd windows.HWND) error {
	style, _, err := procGetWindowLongPtrW.Call(uintptr(hwnd), uintptr(gwlStyle))
	if style == 0 && err != windows.ERROR_SUCCESS {
		return fmt.Errorf("GetWindowLongPtrW failed: %w", err)
	}
	style &^= uintptr(wsCaption | wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsSysMenu)
	ret, _, err := procSetWindowLongPtrW.Call(uintptr(hwnd), uintptr(gwlStyle), style)
	if ret == 0 && err != windows.ERROR_SUCCESS {
		return fmt.Errorf("SetWindowLongPtrW failed: %w", err)
	}
	return nil
}

func forceWindowTopmost(hwnd windows.HWND, bounds WindowBounds) error {
	if !isWindow(hwnd) {
		return fmt.Errorf("window handle is no longer valid")
	}
	ret, _, err := procSetWindowPos.Call(
		uintptr(hwnd),
		hwndTopmost,
		intToUintptr(bounds.X),
		intToUintptr(bounds.Y),
		uintptr(bounds.Width),
		uintptr(bounds.Height),
		uintptr(swpFrameChange|swpNoActivate|swpShowWindow),
	)
	if ret == 0 {
		return fmt.Errorf("SetWindowPos failed: %w", err)
	}
	return nil
}

func readWindowBounds(hwnd windows.HWND) (WindowBounds, error) {
	var r rect
	ret, _, err := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return WindowBounds{}, fmt.Errorf("GetWindowRect failed: %w", err)
	}
	return WindowBounds{
		X:      int(r.Left),
		Y:      int(r.Top),
		Width:  int(r.Right - r.Left),
		Height: int(r.Bottom - r.Top),
	}, nil
}

func windowNeedsRepair(current, desired WindowBounds) bool {
	return current != desired
}

func isWindow(hwnd windows.HWND) bool {
	ret, _, _ := procIsWindow.Call(uintptr(hwnd))
	return ret != 0
}

func intToUintptr(value int) uintptr {
	return uintptr(int64(value))
}
