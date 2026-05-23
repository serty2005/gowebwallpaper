package main

import (
	"errors"
	"testing"
)

type fakeStartupUI struct {
	statuses  []string
	notices   []string
	promptURL string
	promptOK  bool
}

func (ui *fakeStartupUI) Status(message string) {
	ui.statuses = append(ui.statuses, message)
}

func (ui *fakeStartupUI) Notice(message string) {
	ui.notices = append(ui.notices, message)
}

func (ui *fakeStartupUI) PromptURL(currentURL string, firstRun bool, webView2Version string) (string, bool, error) {
	if ui.promptURL == "" {
		return currentURL, ui.promptOK, nil
	}
	return ui.promptURL, ui.promptOK, nil
}

func TestStartupFlowCreatesConfigPromptsURLAndDoesNotAutoStartOnFirstRun(t *testing.T) {
	ui := &fakeStartupUI{promptURL: " https://example.test/wallpaper ", promptOK: true}
	config := &AppConfig{URL: defaultURL}
	createdConfig := false
	savedConfig := false

	autoStart, err := prepareStartup(startupDeps{
		ui: ui,
		configExists: func() (bool, error) {
			return false, nil
		},
		performDiagnosticRun: func() error {
			createdConfig = true
			return nil
		},
		loadConfig: func() (*AppConfig, error) {
			if !createdConfig {
				t.Fatal("expected config to be created before loading it")
			}
			return config, nil
		},
		saveConfig: func(next *AppConfig) error {
			savedConfig = true
			config = next
			return nil
		},
		ensureWebView2: func(startupUI) (string, error) {
			return "123.0.0.0", nil
		},
	})

	if err != nil {
		t.Fatalf("prepareStartup returned error: %v", err)
	}
	if autoStart {
		t.Fatal("expected first run to stay stopped until tray Start is clicked")
	}
	if !createdConfig {
		t.Fatal("expected missing config to be created")
	}
	if !savedConfig {
		t.Fatal("expected prompted URL to be saved")
	}
	if config.URL != "https://example.test/wallpaper" {
		t.Fatalf("expected trimmed prompted URL, got %q", config.URL)
	}
}

func TestStartupFlowExistingConfigSavesPromptedURLAndAutoStarts(t *testing.T) {
	ui := &fakeStartupUI{promptURL: "https://example.test/live", promptOK: true}
	config := &AppConfig{URL: "http://old.test"}
	createdConfig := false

	autoStart, err := prepareStartup(startupDeps{
		ui: ui,
		configExists: func() (bool, error) {
			return true, nil
		},
		performDiagnosticRun: func() error {
			createdConfig = true
			return nil
		},
		loadConfig: func() (*AppConfig, error) {
			return config, nil
		},
		saveConfig: func(next *AppConfig) error {
			config = next
			return nil
		},
		ensureWebView2: func(startupUI) (string, error) {
			return "123.0.0.0", nil
		},
	})

	if err != nil {
		t.Fatalf("prepareStartup returned error: %v", err)
	}
	if !autoStart {
		t.Fatal("expected existing config to auto-start after startup UI")
	}
	if createdConfig {
		t.Fatal("did not expect diagnostic config creation for existing config")
	}
	if config.URL != "https://example.test/live" {
		t.Fatalf("expected prompted URL to be saved, got %q", config.URL)
	}
}

func TestEnsureWebView2RuntimeDownloadsInstallsAndRestartsWhenMissing(t *testing.T) {
	ui := &fakeStartupUI{}
	versions := []string{"", "124.0.0.0"}
	downloaded := false
	installed := false
	restarted := false

	_, err := ensureWebView2Runtime(webView2RuntimeDeps{
		getInstalledVersion: func() (string, error) {
			next := versions[0]
			versions = versions[1:]
			return next, nil
		},
		downloadBootstrapper: func(downloadURL string) (string, error) {
			if downloadURL != webView2BootstrapperURL {
				t.Fatalf("unexpected download URL: %s", downloadURL)
			}
			downloaded = true
			return `C:\Temp\MicrosoftEdgeWebview2Setup.exe`, nil
		},
		installBootstrapper: func(path string) error {
			if path == "" {
				t.Fatal("expected bootstrapper path")
			}
			installed = true
			return nil
		},
		restartSelf: func() error {
			restarted = true
			return nil
		},
	}, ui)

	if !errors.Is(err, errRestarting) {
		t.Fatalf("expected errRestarting, got %v", err)
	}
	if !downloaded || !installed || !restarted {
		t.Fatalf("expected download/install/restart, got downloaded=%t installed=%t restarted=%t", downloaded, installed, restarted)
	}
	if len(ui.notices) == 0 {
		t.Fatal("expected user-facing install notice")
	}
}

func TestEnsureWebView2RuntimeDoesNothingWhenInstalled(t *testing.T) {
	ui := &fakeStartupUI{}
	calledDownload := false

	_, err := ensureWebView2Runtime(webView2RuntimeDeps{
		getInstalledVersion: func() (string, error) {
			return "124.0.0.0", nil
		},
		downloadBootstrapper: func(string) (string, error) {
			calledDownload = true
			return "", nil
		},
		installBootstrapper: func(string) error {
			t.Fatal("did not expect installer to run")
			return nil
		},
		restartSelf: func() error {
			t.Fatal("did not expect restart")
			return nil
		},
	}, ui)

	if err != nil {
		t.Fatalf("ensureWebView2Runtime returned error: %v", err)
	}
	if calledDownload {
		t.Fatal("did not expect bootstrapper download when runtime is installed")
	}
}

func TestNormalizeStartupURLKeepsCurrentWhenPromptIsBlank(t *testing.T) {
	got := normalizeStartupURL("   ", "https://current.test")

	if got != "https://current.test" {
		t.Fatalf("expected current URL, got %q", got)
	}
}

func TestShouldRestartAfterConfigChangeOnlyWhenRunning(t *testing.T) {
	if !shouldRestartAfterConfigChange(true) {
		t.Fatal("expected running wallpaper to restart after config change")
	}
	if shouldRestartAfterConfigChange(false) {
		t.Fatal("expected stopped wallpaper to stay stopped after config change")
	}
}
