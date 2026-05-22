package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// Windows API константы и типы
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

func main() {
	exists, err := configExists()
	if err != nil {
		log.Fatalf("Ошибка при проверке наличия конфига: %v", err)
	}

	if !exists {
		fmt.Println("Конфигурационный файл не найден. Запускаю диагностику...")
		performDiagnosticRun()
		return
	}

	// Запускаем приложение в фоновом режиме с иконкой в трее
	runTrayApplication()
}

// runTrayApplication создает приложение с иконкой в системном трее
func runTrayApplication() {
	done := make(chan bool, 1)

	go func() {
		defer func() {
			done <- true
		}()
		systray.Run(onTrayReady, onTrayExit)
	}()

	<-done
}

// onTrayReady настраивает меню в трее при запуске
func onTrayReady() {
	// Создаем основные пункты меню
	startItem := systray.AddMenuItem("Запустить обои", "Запустить приложение на выбранном мониторе")
	stopItem := systray.AddMenuItem("Остановить", "Остановить отображение обоев")
	stopItem.Disable()

	systray.AddSeparator()

	// Подменю выбора монитора
	monitorMenu := systray.AddMenuItem("Выбрать монитор", "Выбрать монитор для отображения обоев")

	// Заполняем список мониторов
	monitors := getMonitors()
	monitorItems := make(map[string]*systray.MenuItem)

	for _, monitor := range monitors {
		monitorName := fmt.Sprintf("%s (%dx%d)", monitor.Name, monitor.Width, monitor.Height)
		if monitor.IsPrimary {
			monitorName += " [Основной]"
		}

		item := monitorMenu.AddSubMenuItem(monitorName, fmt.Sprintf("Переключиться на монитор %s", monitor.Name))
		monitorItems[monitor.Name] = item
	}

	systray.AddSeparator()
	settingsItem := systray.AddMenuItem("Настройки", "Открыть настройки")
	aboutItem := systray.AddMenuItem("О программе", "Информация о программе")
	quitItem := systray.AddMenuItem("Выход", "Выход из программы")

	// Переменные для управления состоянием
	var currentWebview interface{}

	// Обработчики меню
	for {
		select {
		case <-startItem.ClickedCh:
			if currentWebview != nil {
				continue // Уже запущено
			}

			// Загружаем активный монитор из конфига
			config, _ := loadConfig()
			targetMonitor := findActiveMonitor(config, monitors)

			if targetMonitor == nil {
				fmt.Println("Не найден активный монитор в конфигурации")
				continue
			}

			// Запускаем веб-вью на фоне
			go func() {
				currentWebview = startWallpaperOnMonitor(targetMonitor, config.URL)
				if currentWebview != nil {
					startItem.Disable()
					stopItem.Enable()
					fmt.Println("Обои запущены")
				} else {
					fmt.Println("Ошибка запуска обоев")
				}
			}()

		case <-stopItem.ClickedCh:
			if currentWebview != nil {
				if w, ok := currentWebview.(interface{ Destroy() }); ok {
					w.Destroy()
				}
				currentWebview = nil
				startItem.Enable()
				stopItem.Disable()
				fmt.Println("Обои остановлены")
			}

		case <-quitItem.ClickedCh:
			if currentWebview != nil {
				if w, ok := currentWebview.(interface{ Destroy() }); ok {
					w.Destroy()
				}
			}
			systray.Quit()
			return

		case <-aboutItem.ClickedCh:
			fmt.Println("Go Web Wallpaper v1.0")

		case <-settingsItem.ClickedCh:
			go openSettingsDialog()

		default:
			// Обработка выбора мониторов
			for monitorName, item := range monitorItems {
				select {
				case <-item.ClickedCh:
					err := setActiveMonitor(monitorName)
					if err != nil {
						fmt.Printf("Ошибка: %v\n", err)
					} else {
						fmt.Printf("Монитор %s установлен как активный\n", monitorName)

						// Если обои запущены, перезапускаем их на новом мониторе
						if currentWebview != nil {
							if w, ok := currentWebview.(interface{ Destroy() }); ok {
								w.Destroy()
							}
							currentWebview = nil

							// Запускаем на новом мониторе
							config, _ := loadConfig()
							var targetMonitor *MonitorConfig
							for _, m := range monitors {
								if m.Name == monitorName {
									targetMonitor = &m
									break
								}
							}

							if targetMonitor != nil {
								go func() {
									currentWebview = startWallpaperOnMonitor(targetMonitor, config.URL)
								}()
							}
						}
					}
				default:
					// Продолжаем обработку
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// onTrayExit выполняется при выходе из приложения
func onTrayExit() {
	// Очистка ресурсов при выходе
}

// startWallpaperOnMonitor запускает отображение обоев на указанном мониторе
func startWallpaperOnMonitor(monitor *MonitorConfig, url string) interface{} {
	// Ожидаем появления монитора
	var foundMonitor *MonitorConfig
	for foundMonitor == nil {
		fmt.Println("Проверка подключенных мониторов...")
		connectedMonitors := getMonitors()

		// Ищем совпадение по разрешению
		for i, connected := range connectedMonitors {
			if connected.Width == monitor.Width && connected.Height == monitor.Height {
				fmt.Printf("Целевой монитор найден: %s\n", connected.Name)
				foundMonitor = &connectedMonitors[i]
				break
			}
		}

		if foundMonitor == nil {
			fmt.Println("Монитор не найден. Следующая проверка через 5 секунд...")
			time.Sleep(5 * time.Second)
		}
	}

	// Ждем стабилизации
	fmt.Println("Монитор подключен. Ожидание 3 секунды для стабилизации...")
	time.Sleep(3 * time.Second)

	fmt.Printf("Запуск на мониторе: %s (Primary: %t)\n", foundMonitor.Name, foundMonitor.IsPrimary)
	fmt.Printf("Актуальные координаты: (%d, %d), Размер: %dx%d\n",
		foundMonitor.PositionX, foundMonitor.PositionY, foundMonitor.Width, foundMonitor.Height)

	// Создаем webview
	w := webview2.New(true)
	if w == nil {
		log.Fatalf("Не удалось создать окно webview2.")
		return nil
	}

	w.SetSize(foundMonitor.Width, foundMonitor.Height, webview2.HintNone)
	w.SetTitle("Go Web Wallpaper")
	w.Navigate(url)

	// Получаем дескриптор окна
	hwnd := windows.HWND(w.Window())

	// Убираем рамку и заголовок окна
	ret, _, _ := procGetWindowLong.Call(uintptr(hwnd), uintptr(int(GWL_STYLE)))
	style := int(ret)
	ret, _, _ = procSetWindowLong.Call(uintptr(hwnd), uintptr(int(GWL_STYLE)),
		uintptr(int(style&^(WS_CAPTION|WS_THICKFRAME))))
	if ret == 0 {
		log.Printf("SetWindowLong failed")
		w.Destroy()
		return nil
	}

	// Устанавливаем позицию окна
	ret, _, _ = procSetWindowPos.Call(uintptr(hwnd), 0,
		uintptr(foundMonitor.PositionX), uintptr(foundMonitor.PositionY),
		uintptr(foundMonitor.Width), uintptr(foundMonitor.Height),
		uintptr(SWP_FRAMECHANGED|SWP_NOZORDER|SWP_NOACTIVATE))
	if ret == 0 {
		log.Printf("SetWindowPos failed")
		w.Destroy()
		return nil
	}

	// Запускаем webview в отдельной горутине
	go w.Run()

	return w
}

// findActiveMonitor находит активный монитор в конфигурации
func findActiveMonitor(config *AppConfig, monitors []MonitorConfig) *MonitorConfig {
	var targetMonitor *MonitorConfig
	for i, monitor := range config.Monitors {
		if monitor.Active {
			targetMonitor = &config.Monitors[i]
			break
		}
	}

	if targetMonitor == nil {
		if len(config.Monitors) > 0 {
			targetMonitor = &config.Monitors[0]
			fmt.Println("Активный монитор не найден, используется первый в списке.")
		} else {
			log.Fatalf("В конфиге не найдено ни одного монитора.")
		}
	}

	// Находим соответствующий монитор в системе
	for _, m := range monitors {
		if m.Width == targetMonitor.Width && m.Height == targetMonitor.Height {
			return &m
		}
	}

	return nil
}

// setActiveMonitor устанавливает активный монитор в конфигурации
func setActiveMonitor(monitorName string) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Сбрасываем все мониторы
	for i := range config.Monitors {
		config.Monitors[i].Active = false
	}

	// Устанавливаем выбранный монитор как активный
	for i := range config.Monitors {
		if config.Monitors[i].Name == monitorName {
			config.Monitors[i].Active = true
			break
		}
	}

	// Сохраняем конфигурацию
	return saveConfig(config)
}

// saveConfig сохраняет конфигурацию в файл
func saveConfig(config *AppConfig) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	file, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, file, 0644)
}

// openSettingsDialog открывает диалог настроек (заглушка)
func openSettingsDialog() {
	// TODO: Реализовать GUI диалог настроек
	// Пока показываем сообщение
	fmt.Println("Диалог настроек будет добавлен в следующей версии")
}

// performDiagnosticRun проводит диагностику мониторов
func performDiagnosticRun() {
	monitors := getMonitors()
	if len(monitors) == 0 {
		log.Fatalln("Не удалось найти ни одного монитора.")
	}

	config := AppConfig{
		URL:      "http://localhost:3100/#/columns-fullscreen",
		Monitors: monitors,
	}

	// Устанавливаем основной монитор как активный
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
	fmt.Println("Конфигурация сохранена. Запустите приложение для работы с иконкой в трее.")
}

// getMonitors получает список всех подключенных мониторов
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
