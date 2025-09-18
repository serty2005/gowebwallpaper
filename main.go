package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func main() {
	exists, err := configExists()
	if err != nil {
		log.Fatalf("Ошибка при проверке наличия конфига: %v", err)
	}

	if !exists {
		fmt.Println("Конфигурационный файл не найден. Запускаю диагностику...")
		performDiagnosticRun()
	} else {
		fmt.Println("Конфиг найден. Запускаю приложение...")
		runApplication()
	}
}

// runApplication создает окно и запускает приложение.
func runApplication() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Не удалось загрузить конфиг: %v", err)
	}

	var selectedMonitor *MonitorConfig
	for i, monitor := range config.Monitors {
		if monitor.Active {
			selectedMonitor = &config.Monitors[i]
			break
		}
	}

	if selectedMonitor == nil {
		if len(config.Monitors) > 0 {
			selectedMonitor = &config.Monitors[0]
			fmt.Println("Активный монитор не найден, используется первый в списке.")
		} else {
			log.Fatalf("В конфиге не найдено ни одного монитора.")
		}
	}

	fmt.Printf("Запуск на мониторе: %s (Primary: %t)\n", selectedMonitor.Name, selectedMonitor.IsPrimary)
	fmt.Printf("Координаты: (%d, %d), Размер: %dx%d\n", selectedMonitor.PositionX, selectedMonitor.PositionY, selectedMonitor.Width, selectedMonitor.Height)

	// Создаем webview с debug
	w := webview2.New(true)
	if w == nil {
		log.Fatalf("Не удалось создать окно webview2.")
	}
	defer w.Destroy()

	w.SetSize(selectedMonitor.Width, selectedMonitor.Height, webview2.HintNone)
	w.SetTitle("Go Web Wallpaper")
	w.Navigate(config.URL)

	// Получаем дескриптор окна (HWND) как windows.HWND
	hwnd := windows.HWND(w.Window())

	// Убираем рамку и заголовок окна
	ret, _, _ := procGetWindowLong.Call(uintptr(hwnd), uintptr(int(GWL_STYLE)))
	style := int(ret)
	ret, _, _ = procSetWindowLong.Call(uintptr(hwnd), uintptr(int(GWL_STYLE)), uintptr(int(style&^(WS_CAPTION|WS_THICKFRAME))))
	if ret == 0 {
		log.Fatalf("SetWindowLong failed")
	}

	// Используем безопасную функцию для установки точной позиции окна
	ret, _, _ = procSetWindowPos.Call(uintptr(hwnd), 0, uintptr(selectedMonitor.PositionX), uintptr(selectedMonitor.PositionY), uintptr(selectedMonitor.Width), uintptr(selectedMonitor.Height), uintptr(SWP_FRAMECHANGED|SWP_NOZORDER|SWP_NOACTIVATE))
	if ret == 0 {
		log.Fatalf("SetWindowPos failed")
	}

	w.Run()
}

// performDiagnosticRun ищет все мониторы и сохраняет их реальные координаты в config.json
func performDiagnosticRun() {
	monitors := getMonitors()
	if len(monitors) == 0 {
		log.Fatalln("Не удалось найти ни одного монитора.")
	}

	config := AppConfig{
		URL:      "http://localhost:3100/#/columns-fullscreen",
		Monitors: monitors,
	}

	for i, m := range config.Monitors {
		if m.IsPrimary {
			config.Monitors[i].Active = true
			break
		}
	}

	path, _ := getConfigPath()
	file, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Fatalf("Ошибка при сериализации конфига: %v", err)
	}

	err = os.WriteFile(path, file, 0644)
	if err != nil {
		log.Fatalf("Ошибка при записи конфига: %v", err)
	}

	fmt.Printf("Диагностика завершена. Файл '%s' создан.\n", path)
	fmt.Println("Пожалуйста, отредактируйте его, установив 'Active: true' для нужного монитора, и перезапустите приложение.")
}

// --- Блок для работы с Windows API ---

var (
	moduser32               = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayMonitors = moduser32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = moduser32.NewProc("GetMonitorInfoW")
	procGetWindowLong       = moduser32.NewProc("GetWindowLongW")
	procSetWindowLong       = moduser32.NewProc("SetWindowLongW")
	procSetWindowPos        = moduser32.NewProc("SetWindowPos")
)

const (
	MONITORINFOF_PRIMARY = 0x00000001
	GWL_STYLE            = 0xFFFFFFF0
	WS_CAPTION           = 0x00C00000
	WS_THICKFRAME        = 0x00040000
	SWP_FRAMECHANGED     = 0x0020
	SWP_NOZORDER         = 0x0004
	SWP_NOACTIVATE       = 0x0010
)

type RECT struct {
	Left, Top, Right, Bottom int32
}

type MONITORINFOEX struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
	SzDevice  [32]uint16
}

// getMonitors - эта функция остается без изменений
func getMonitors() []MonitorConfig {
	var monitors []MonitorConfig
	callback := syscall.NewCallback(func(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
		var mi MONITORINFOEX
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))

		monitors = append(monitors, MonitorConfig{
			Name:      windows.UTF16ToString(mi.SzDevice[:]),
			IsPrimary: mi.DwFlags&MONITORINFOF_PRIMARY != 0,
			PositionX: int(mi.RcMonitor.Left),
			PositionY: int(mi.RcMonitor.Top),
			Width:     int(mi.RcMonitor.Right - mi.RcMonitor.Left),
			Height:    int(mi.RcMonitor.Bottom - mi.RcMonitor.Top),
		})
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, callback, 0)
	return monitors
}
