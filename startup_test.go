package main

import (
	"bytes"
	"errors"
	"testing"
)

type fakeStartupUI struct {
	statuses        []string
	notices         []string
	progress        []webView2DownloadProgress
	promptURL       string
	promptOK        bool
	promptCalls     int
	promptFirstRuns []bool
}

func (ui *fakeStartupUI) Status(message string) {
	ui.statuses = append(ui.statuses, message)
}

func (ui *fakeStartupUI) Notice(message string) {
	ui.notices = append(ui.notices, message)
}

func (ui *fakeStartupUI) Progress(message string, progress webView2DownloadProgress) {
	ui.statuses = append(ui.statuses, message)
	ui.progress = append(ui.progress, progress)
}

func (ui *fakeStartupUI) PromptURL(currentURL string, firstRun bool, webView2Version string) (string, bool, error) {
	ui.promptCalls++
	ui.promptFirstRuns = append(ui.promptFirstRuns, firstRun)
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
		ensureSelfUpdate: func(startupUI) error {
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
	if ui.promptCalls != 1 {
		t.Fatalf("expected URL prompt once on first run, got %d", ui.promptCalls)
	}
	if len(ui.promptFirstRuns) != 1 || !ui.promptFirstRuns[0] {
		t.Fatalf("expected first-run flag in URL prompt, got %#v", ui.promptFirstRuns)
	}
	if config.URL != "https://example.test/wallpaper" {
		t.Fatalf("expected trimmed prompted URL, got %q", config.URL)
	}
}

func TestStartupFlowExistingConfigSkipsURLPromptAndAutoStarts(t *testing.T) {
	ui := &fakeStartupUI{promptURL: "https://example.test/live", promptOK: true}
	config := &AppConfig{URL: "http://old.test"}
	createdConfig := false
	savedConfig := false

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
			savedConfig = true
			config = next
			return nil
		},
		ensureSelfUpdate: func(startupUI) error {
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
	if ui.promptCalls != 0 {
		t.Fatalf("did not expect URL prompt for existing config, got %d calls", ui.promptCalls)
	}
	if savedConfig {
		t.Fatal("did not expect config save when URL prompt is skipped")
	}
	if config.URL != "http://old.test" {
		t.Fatalf("expected existing URL to stay unchanged, got %q", config.URL)
	}
}

func TestStartupFlowRestartsWhenSelfUpdateInstallsUpdate(t *testing.T) {
	ui := &fakeStartupUI{}
	webViewChecked := false

	_, err := prepareStartup(startupDeps{
		ui: ui,
		configExists: func() (bool, error) {
			t.Fatal("did not expect config check after update restart")
			return false, nil
		},
		performDiagnosticRun: func() error {
			t.Fatal("did not expect diagnostic run after update restart")
			return nil
		},
		loadConfig: func() (*AppConfig, error) {
			t.Fatal("did not expect config load after update restart")
			return nil, nil
		},
		saveConfig: func(*AppConfig) error {
			t.Fatal("did not expect config save after update restart")
			return nil
		},
		ensureSelfUpdate: func(startupUI) error {
			return errRestarting
		},
		ensureWebView2: func(startupUI) (string, error) {
			webViewChecked = true
			return "123.0.0.0", nil
		},
	})

	if !errors.Is(err, errRestarting) {
		t.Fatalf("expected errRestarting, got %v", err)
	}
	if webViewChecked {
		t.Fatal("did not expect WebView2 check after update restart")
	}
}

func TestEnsureWebView2RuntimeDownloadsInstallsAndRestartsWhenMissing(t *testing.T) {
	ui := &fakeStartupUI{}
	versions := []string{"", "124.0.0.0"}
	pkg := webView2InstallerPackage{
		URL:      "https://example.test/webview2-x64.exe",
		FileName: "MicrosoftEdgeWebView2RuntimeInstallerX64.exe",
	}
	downloaded := false
	installed := false
	restarted := false

	_, err := ensureWebView2Runtime(webView2RuntimeDeps{
		getInstalledVersion: func() (string, error) {
			next := versions[0]
			versions = versions[1:]
			return next, nil
		},
		installerPackage: func() (webView2InstallerPackage, error) {
			return pkg, nil
		},
		downloadInstaller: func(downloadPackage webView2InstallerPackage, reportProgress func(webView2DownloadProgress)) (string, error) {
			if downloadPackage != pkg {
				t.Fatalf("unexpected download package: %#v", downloadPackage)
			}
			downloaded = true
			reportProgress(webView2DownloadProgress{Downloaded: 25, Total: 100})
			reportProgress(webView2DownloadProgress{Downloaded: 100, Total: 100})
			return `C:\Temp\MicrosoftEdgeWebView2RuntimeInstallerX64.exe`, nil
		},
		installInstaller: func(path string) error {
			if path == "" {
				t.Fatal("expected installer path")
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
	if len(ui.progress) != 2 {
		t.Fatalf("expected download progress to be reported, got %d updates", len(ui.progress))
	}
}

func TestEnsureWebView2RuntimeDoesNothingWhenInstalled(t *testing.T) {
	ui := &fakeStartupUI{}
	calledDownload := false

	_, err := ensureWebView2Runtime(webView2RuntimeDeps{
		getInstalledVersion: func() (string, error) {
			return "124.0.0.0", nil
		},
		installerPackage: func() (webView2InstallerPackage, error) {
			t.Fatal("did not expect installer package selection")
			return webView2InstallerPackage{}, nil
		},
		downloadInstaller: func(webView2InstallerPackage, func(webView2DownloadProgress)) (string, error) {
			calledDownload = true
			return "", nil
		},
		installInstaller: func(string) error {
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
		t.Fatal("did not expect installer download when runtime is installed")
	}
}

func TestWebView2StandaloneInstallerPackageUsesArchitectureSpecificURL(t *testing.T) {
	tests := []struct {
		goarch   string
		url      string
		fileName string
	}{
		{
			goarch:   "amd64",
			url:      webView2StandaloneX64URL,
			fileName: "MicrosoftEdgeWebView2RuntimeInstallerX64.exe",
		},
		{
			goarch:   "386",
			url:      webView2StandaloneX86URL,
			fileName: "MicrosoftEdgeWebView2RuntimeInstallerX86.exe",
		},
		{
			goarch:   "arm64",
			url:      webView2StandaloneARM64URL,
			fileName: "MicrosoftEdgeWebView2RuntimeInstallerARM64.exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got, err := webView2StandaloneInstallerPackage(tt.goarch)
			if err != nil {
				t.Fatalf("webView2StandaloneInstallerPackage returned error: %v", err)
			}
			if got.URL != tt.url || got.FileName != tt.fileName {
				t.Fatalf("unexpected package: %#v", got)
			}
		})
	}
}

func TestCopyWithDownloadProgressReportsStartAndCompletion(t *testing.T) {
	var dst bytes.Buffer
	var progress []webView2DownloadProgress

	err := copyWithDownloadProgress(&dst, bytes.NewBufferString("runtime"), 7, func(next webView2DownloadProgress) {
		progress = append(progress, next)
	})

	if err != nil {
		t.Fatalf("copyWithDownloadProgress returned error: %v", err)
	}
	if dst.String() != "runtime" {
		t.Fatalf("unexpected copied content: %q", dst.String())
	}
	if len(progress) < 2 {
		t.Fatalf("expected at least start and completion progress, got %d updates", len(progress))
	}
	if progress[0] != (webView2DownloadProgress{Downloaded: 0, Total: 7}) {
		t.Fatalf("unexpected initial progress: %#v", progress[0])
	}
	last := progress[len(progress)-1]
	if last != (webView2DownloadProgress{Downloaded: 7, Total: 7}) {
		t.Fatalf("unexpected final progress: %#v", last)
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
