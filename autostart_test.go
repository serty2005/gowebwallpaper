package main

import (
	"errors"
	"strings"
	"testing"
)

func TestAutostartEnableCreatesLogonTaskAndVerifiesIt(t *testing.T) {
	var calls []string
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Program Files\Go Web Wallpaper\gowebwallpaper.exe`, nil
		},
		run: func(name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			if name != "schtasks.exe" {
				t.Fatalf("unexpected command %q", name)
			}
			switch {
			case containsAll(args, "/Create", "/SC", "ONLOGON", "/TN", autostartTaskName, "/RL", "LIMITED", "/F"):
				if !containsArg(args, `/TR`) || !containsArg(args, `"C:\Program Files\Go Web Wallpaper\gowebwallpaper.exe"`) {
					t.Fatalf("create args do not quote executable path: %#v", args)
				}
				return []byte("SUCCESS"), nil
			case containsAll(args, "/Query", "/XML", "/TN", autostartTaskName):
				return []byte(`<Task><Settings><Enabled>true</Enabled></Settings><Actions><Exec><Command>C:\Program Files\Go Web Wallpaper\gowebwallpaper.exe</Command></Exec></Actions></Task>`), nil
			default:
				t.Fatalf("unexpected schtasks args: %#v", args)
			}
			return nil, nil
		},
	}

	if err := ensureAutostartEnabled(deps); err != nil {
		t.Fatalf("ensureAutostartEnabled returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected create and verify calls, got %#v", calls)
	}
}

func TestAutostartEnabledRequiresEnabledMatchingTask(t *testing.T) {
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Apps\gowebwallpaper.exe`, nil
		},
		run: func(name string, args ...string) ([]byte, error) {
			return []byte(`<Task><Settings><Enabled>true</Enabled></Settings><Actions><Exec><Command>C:\Apps\gowebwallpaper.exe</Command></Exec></Actions></Task>`), nil
		},
	}

	enabled, err := autostartEnabled(deps)
	if err != nil {
		t.Fatalf("autostartEnabled returned error: %v", err)
	}
	if !enabled {
		t.Fatal("expected matching enabled task")
	}
}

func TestAutostartEnabledReturnsFalseWhenTaskIsMissing(t *testing.T) {
	deps := autostartDeps{
		executable: func() (string, error) {
			return `C:\Apps\gowebwallpaper.exe`, nil
		},
		run: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("ERROR: The system cannot find the file specified.")
		},
	}

	enabled, err := autostartEnabled(deps)
	if err != nil {
		t.Fatalf("autostartEnabled returned error: %v", err)
	}
	if enabled {
		t.Fatal("expected missing task to be disabled")
	}
}

func containsAll(args []string, values ...string) bool {
	for _, value := range values {
		if !containsArg(args, value) {
			return false
		}
	}
	return true
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
