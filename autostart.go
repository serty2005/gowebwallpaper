package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	autostartRunKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartRunValueName = "Go Web Wallpaper"
)

type autostartDeps struct {
	executable     func() (string, error)
	readRunValue   func(name string) (string, error)
	writeRunValue  func(name, value string) error
	deleteRunValue func(name string) error
}

func defaultAutostartDeps() autostartDeps {
	return autostartDeps{
		executable: os.Executable,
		readRunValue: func(name string) (string, error) {
			key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKeyPath, registry.QUERY_VALUE)
			if err != nil {
				return "", err
			}
			defer key.Close()
			value, _, err := key.GetStringValue(name)
			return value, err
		},
		writeRunValue: func(name, value string) error {
			key, _, err := registry.CreateKey(registry.CURRENT_USER, autostartRunKeyPath, registry.SET_VALUE)
			if err != nil {
				return err
			}
			defer key.Close()
			return key.SetStringValue(name, value)
		},
		deleteRunValue: func(name string) error {
			key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKeyPath, registry.SET_VALUE)
			if err != nil {
				return err
			}
			defer key.Close()
			return key.DeleteValue(name)
		},
	}
}

func AutostartEnabled() (bool, error) {
	return autostartEnabled(defaultAutostartDeps())
}

func EnableAutostart() error {
	return ensureAutostartEnabled(defaultAutostartDeps())
}

func DisableAutostart() error {
	return disableAutostart(defaultAutostartDeps())
}

func ensureAutostartEnabled(deps autostartDeps) error {
	exe, err := deps.executable()
	if err != nil {
		return err
	}
	if strings.Contains(exe, `"`) {
		return fmt.Errorf("executable path contains an unsupported quote: %s", exe)
	}
	if err := deps.writeRunValue(autostartRunValueName, quoteRunPath(exe)); err != nil {
		return fmt.Errorf("write autostart Run value failed: %w", err)
	}
	enabled, err := autostartEnabled(deps)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("autostart Run value was written but verification failed")
	}
	return nil
}

func disableAutostart(deps autostartDeps) error {
	if err := deps.deleteRunValue(autostartRunValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete autostart Run value failed: %w", err)
	}
	return nil
}

func autostartEnabled(deps autostartDeps) (bool, error) {
	exe, err := deps.executable()
	if err != nil {
		return false, err
	}
	value, err := deps.readRunValue(autostartRunValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return sameExecutablePath(exe, value), nil
}

func quoteRunPath(path string) string {
	return `"` + path + `"`
}

func sameExecutablePath(expected, actual string) bool {
	expected = strings.Trim(strings.TrimSpace(expected), `"`)
	actual = strings.Trim(strings.TrimSpace(actual), `"`)
	if expected == "" || actual == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(expected), filepath.Clean(actual))
}
