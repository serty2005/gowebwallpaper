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
	"runtime"
	"strings"
	"time"

	"github.com/jchv/go-webview2/webviewloader"
)

// Microsoft fwlinks from the WebView2 Runtime download page.
const webView2StandaloneX64URL = "https://go.microsoft.com/fwlink/?linkid=2124701"
const webView2StandaloneX86URL = "https://go.microsoft.com/fwlink/?linkid=2099617"
const webView2StandaloneARM64URL = "https://go.microsoft.com/fwlink/?linkid=2099616"

var errRestarting = errors.New("application restart requested")

type startupUI interface {
	Status(message string)
	Progress(message string, progress webView2DownloadProgress)
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
	ui := &windowsStartupUI{}
	defer ui.Close()
	return prepareStartup(startupDeps{
		ui:                   ui,
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
	if firstRun {
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

func (logOnlyStartupUI) Progress(message string, progress webView2DownloadProgress) {
	if progress.Total > 0 {
		log.Printf("startup: %s (%d/%d bytes)", message, progress.Downloaded, progress.Total)
		return
	}
	log.Printf("startup: %s (%d bytes)", message, progress.Downloaded)
}

func (logOnlyStartupUI) Notice(message string) {
	log.Printf("startup notice: %s", message)
}

func (logOnlyStartupUI) PromptURL(currentURL string, _ bool, _ string) (string, bool, error) {
	return currentURL, true, nil
}

type webView2RuntimeDeps struct {
	getInstalledVersion func() (string, error)
	installerPackage    func() (webView2InstallerPackage, error)
	downloadInstaller   func(webView2InstallerPackage, func(webView2DownloadProgress)) (string, error)
	installInstaller    func(path string) error
	restartSelf         func() error
}

func defaultWebView2RuntimeDeps() webView2RuntimeDeps {
	return webView2RuntimeDeps{
		getInstalledVersion: webviewloader.GetInstalledVersion,
		installerPackage:    currentWebView2StandaloneInstallerPackage,
		downloadInstaller:   downloadWebView2Installer,
		installInstaller:    installWebView2Installer,
		restartSelf:         restartCurrentExecutable,
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

	pkg, err := deps.installerPackage()
	if err != nil {
		return "", fmt.Errorf("select WebView2 installer failed: %w", err)
	}

	ui.Notice("Microsoft Edge WebView2 Runtime is not installed. The app will download the full standalone installer, install it, then restart.")
	ui.Status("Downloading WebView2 runtime standalone installer")
	installer, err := deps.downloadInstaller(pkg, func(progress webView2DownloadProgress) {
		ui.Progress("Downloading WebView2 runtime standalone installer", progress)
	})
	if err != nil {
		return "", fmt.Errorf("download WebView2 installer failed: %w", err)
	}
	defer os.Remove(installer)

	ui.Status("Installing WebView2 runtime")
	if err := deps.installInstaller(installer); err != nil {
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

type webView2InstallerPackage struct {
	URL          string
	FileName     string
	Architecture string
}

type webView2DownloadProgress struct {
	Downloaded int64
	Total      int64
}

func currentWebView2StandaloneInstallerPackage() (webView2InstallerPackage, error) {
	return webView2StandaloneInstallerPackage(runtime.GOARCH)
}

func webView2StandaloneInstallerPackage(goarch string) (webView2InstallerPackage, error) {
	switch goarch {
	case "amd64":
		return webView2InstallerPackage{
			URL:          webView2StandaloneX64URL,
			FileName:     "MicrosoftEdgeWebView2RuntimeInstallerX64.exe",
			Architecture: "x64",
		}, nil
	case "386":
		return webView2InstallerPackage{
			URL:          webView2StandaloneX86URL,
			FileName:     "MicrosoftEdgeWebView2RuntimeInstallerX86.exe",
			Architecture: "x86",
		}, nil
	case "arm64":
		return webView2InstallerPackage{
			URL:          webView2StandaloneARM64URL,
			FileName:     "MicrosoftEdgeWebView2RuntimeInstallerARM64.exe",
			Architecture: "ARM64",
		}, nil
	default:
		return webView2InstallerPackage{}, fmt.Errorf("unsupported architecture %q", goarch)
	}
}

func downloadWebView2Installer(pkg webView2InstallerPackage, reportProgress func(webView2DownloadProgress)) (string, error) {
	client := http.Client{Timeout: 5 * time.Minute}
	response, err := client.Get(pkg.URL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected HTTP status: %s", response.Status)
	}

	path := filepath.Join(os.TempDir(), pkg.FileName)
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := copyWithDownloadProgress(file, response.Body, response.ContentLength, reportProgress); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func copyWithDownloadProgress(dst io.Writer, src io.Reader, total int64, reportProgress func(webView2DownloadProgress)) error {
	buffer := make([]byte, 64*1024)
	var downloaded int64
	lastPercent := int64(-1)
	report := func() {
		if reportProgress == nil {
			return
		}
		if total <= 0 {
			reportProgress(webView2DownloadProgress{Downloaded: downloaded, Total: total})
			return
		}
		percent := downloaded * 100 / total
		if percent == lastPercent && downloaded != total {
			return
		}
		lastPercent = percent
		reportProgress(webView2DownloadProgress{Downloaded: downloaded, Total: total})
	}
	report()
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			written, writeErr := dst.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			downloaded += int64(written)
			report()
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func installWebView2Installer(path string) error {
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
