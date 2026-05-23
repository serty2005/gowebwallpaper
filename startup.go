package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jchv/go-webview2/webviewloader"
)

const webView2BootstrapperURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

var errRestarting = errors.New("application restart requested")

type startupUI interface {
	Status(message string)
	Notice(message string)
	PromptURL(currentURL string, firstRun bool, webView2Version string) (string, bool, error)
}

type startupDeps struct {
	ui                   startupUI
	configExists         func() (bool, error)
	performDiagnosticRun func() error
	loadConfig           func() (*AppConfig, error)
	saveConfig           func(*AppConfig) error
	ensureWebView2       func(startupUI) (string, error)
}

func runStartupFlow() (bool, error) {
	return prepareStartup(startupDeps{
		ui:                   windowsStartupUI{},
		configExists:         configExists,
		performDiagnosticRun: performDiagnosticRun,
		loadConfig:           loadConfig,
		saveConfig:           saveConfig,
		ensureWebView2: func(ui startupUI) (string, error) {
			return ensureWebView2Runtime(defaultWebView2RuntimeDeps(), ui)
		},
	})
}

func prepareStartup(deps startupDeps) (bool, error) {
	if deps.ui == nil {
		deps.ui = logOnlyStartupUI{}
	}

	deps.ui.Status("Checking WebView2 runtime")
	webView2Version, err := deps.ensureWebView2(deps.ui)
	if err != nil {
		return false, err
	}

	existed, err := deps.configExists()
	if err != nil {
		return false, fmt.Errorf("config check failed: %w", err)
	}
	if !existed {
		deps.ui.Status("Creating initial config")
		if err := deps.performDiagnosticRun(); err != nil {
			return false, fmt.Errorf("diagnostic run failed: %w", err)
		}
	}

	config, err := deps.loadConfig()
	if err != nil {
		return false, fmt.Errorf("load config failed: %w", err)
	}

	firstRun := !existed
	requestedURL, ok, err := deps.ui.PromptURL(config.URL, firstRun, webView2Version)
	if err != nil {
		return false, fmt.Errorf("startup URL prompt failed: %w", err)
	}
	if ok {
		config.URL = normalizeStartupURL(requestedURL, config.URL)
		if err := deps.saveConfig(config); err != nil {
			return false, fmt.Errorf("save config failed: %w", err)
		}
	}

	return existed, nil
}

func normalizeStartupURL(prompted, current string) string {
	next := strings.TrimSpace(prompted)
	if next == "" {
		return current
	}
	return next
}

type logOnlyStartupUI struct{}

func (logOnlyStartupUI) Status(message string) {
	log.Printf("startup: %s", message)
}

func (logOnlyStartupUI) Notice(message string) {
	log.Printf("startup notice: %s", message)
}

func (logOnlyStartupUI) PromptURL(currentURL string, _ bool, _ string) (string, bool, error) {
	return currentURL, true, nil
}

type webView2RuntimeDeps struct {
	getInstalledVersion  func() (string, error)
	downloadBootstrapper func(downloadURL string) (string, error)
	installBootstrapper  func(path string) error
	restartSelf          func() error
}

func defaultWebView2RuntimeDeps() webView2RuntimeDeps {
	return webView2RuntimeDeps{
		getInstalledVersion:  webviewloader.GetInstalledVersion,
		downloadBootstrapper: downloadWebView2Bootstrapper,
		installBootstrapper:  installWebView2Bootstrapper,
		restartSelf:          restartCurrentExecutable,
	}
}

func ensureWebView2Runtime(deps webView2RuntimeDeps, ui startupUI) (string, error) {
	version, err := deps.getInstalledVersion()
	if err != nil {
		return "", fmt.Errorf("check WebView2 runtime failed: %w", err)
	}
	if version != "" {
		ui.Status("WebView2 runtime found: " + version)
		return version, nil
	}

	ui.Notice("Microsoft Edge WebView2 Runtime is not installed. The app will download and install it, then restart.")
	ui.Status("Downloading WebView2 runtime bootstrapper")
	bootstrapper, err := deps.downloadBootstrapper(webView2BootstrapperURL)
	if err != nil {
		return "", fmt.Errorf("download WebView2 bootstrapper failed: %w", err)
	}
	defer os.Remove(bootstrapper)

	ui.Status("Installing WebView2 runtime")
	if err := deps.installBootstrapper(bootstrapper); err != nil {
		return "", fmt.Errorf("install WebView2 runtime failed: %w", err)
	}

	version, err = deps.getInstalledVersion()
	if err != nil {
		return "", fmt.Errorf("verify WebView2 runtime failed: %w", err)
	}
	if version == "" {
		return "", errors.New("WebView2 runtime installation finished, but runtime is still not detected")
	}

	ui.Notice("WebView2 Runtime was installed. The app will restart now.")
	ui.Status("Restarting application")
	if err := deps.restartSelf(); err != nil {
		return "", fmt.Errorf("restart application failed: %w", err)
	}
	return version, errRestarting
}

func downloadWebView2Bootstrapper(downloadURL string) (string, error) {
	client := http.Client{Timeout: 5 * time.Minute}
	response, err := client.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected HTTP status: %s", response.Status)
	}

	path := filepath.Join(os.TempDir(), "MicrosoftEdgeWebview2Setup.exe")
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, response.Body); err != nil {
		return "", err
	}
	return path, nil
}

func installWebView2Bootstrapper(path string) error {
	command := exec.Command(path, "/silent", "/install")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func restartCurrentExecutable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(exe, os.Args[1:]...)
	command.Dir = mustExecutableDir(exe)
	return command.Start()
}

func mustExecutableDir(exe string) string {
	dir := filepath.Dir(exe)
	if dir == "" {
		return "."
	}
	return dir
}

func shouldRestartAfterConfigChange(running bool) bool {
	return running
}
