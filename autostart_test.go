package main

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestAutostartEnableWritesRunRegistryValueAndVerifiesIt(t *testing.T) {
	values := map[string]string{}
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Program Files\Go Web Wallpaper\gowebwallpaper.exe`, nil
		},
		readRunValue: func(name string) (string, error) {
			value, ok := values[name]
			if !ok {
				return "", registry.ErrNotExist
			}
			return value, nil
		},
		writeRunValue: func(name, value string) error {
			values[name] = value
			return nil
		},
		deleteRunValue: func(name string) error {
			delete(values, name)
			return nil
		},
	}

	if err := ensureAutostartEnabled(deps); err != nil {
		t.Fatalf("ensureAutostartEnabled returned error: %v", err)
	}
	if values[autostartRunValueName] != `"C:\Program Files\Go Web Wallpaper\gowebwallpaper.exe"` {
		t.Fatalf("unexpected Run value: %q", values[autostartRunValueName])
	}
}

func TestAutostartEnabledRequiresMatchingCurrentExecutableRunValue(t *testing.T) {
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Apps\gowebwallpaper.exe`, nil
		},
		readRunValue: func(name string) (string, error) {
			if name != autostartRunValueName {
				t.Fatalf("unexpected Run value name %q", name)
			}
			return `"C:\Apps\gowebwallpaper.exe"`, nil
		},
	}

	enabled, err := autostartEnabled(deps)
	if err != nil {
		t.Fatalf("autostartEnabled returned error: %v", err)
	}
	if !enabled {
		t.Fatal("expected matching Run value")
	}
}

func TestAutostartEnabledReturnsFalseWhenRunValuePointsElsewhere(t *testing.T) {
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Apps\gowebwallpaper.exe`, nil
		},
		readRunValue: func(name string) (string, error) {
			return `C:\Other\gowebwallpaper.exe`, nil
		},
	}

	enabled, err := autostartEnabled(deps)
	if err != nil {
		t.Fatalf("autostartEnabled returned error: %v", err)
	}
	if enabled {
		t.Fatal("expected non-matching Run value to be disabled")
	}
}

func TestAutostartEnabledReturnsFalseWhenRunValueIsMissing(t *testing.T) {
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Apps\gowebwallpaper.exe`, nil
		},
		readRunValue: func(name string) (string, error) {
			return "", registry.ErrNotExist
		},
	}

	enabled, err := autostartEnabled(deps)
	if err != nil {
		t.Fatalf("autostartEnabled returned error: %v", err)
	}
	if enabled {
		t.Fatal("expected missing Run value to be disabled")
	}
}

func TestDisableAutostartIgnoresMissingRunValue(t *testing.T) {
	deps := autostartDeps{
		deleteRunValue: func(name string) error {
			if name != autostartRunValueName {
				t.Fatalf("unexpected Run value name %q", name)
			}
			return registry.ErrNotExist
		},
	}

	if err := disableAutostart(deps); err != nil {
		t.Fatalf("disableAutostart returned error: %v", err)
	}
}

func TestDisableAutostartReportsDeleteErrors(t *testing.T) {
	wantErr := errors.New("access denied")
	deps := autostartDeps{
		deleteRunValue: func(name string) error {
			return wantErr
		},
	}

	err := disableAutostart(deps)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected delete error, got %v", err)
	}
}
