package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const selfUpdateReleaseTag = "latest"

var appVersion = "dev"
var appGitHubRepository = "serty2005/gowebwallpaper"

var selfUpdateAssetPattern = regexp.MustCompile(`^webwallpaper-(\d+\.\d+\.\d+)\.exe$`)

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type selfUpdateAsset struct {
	Name        string
	Version     string
	DownloadURL string
}

type selfUpdateDeps struct {
	currentVersion  string
	repository      string
	client          *http.Client
	executable      func() (string, error)
	processID       func() int
	downloadRelease func(client *http.Client, repository string) (githubRelease, error)
	downloadAsset   func(client *http.Client, asset selfUpdateAsset, reportProgress func(webView2DownloadProgress)) (string, error)
	installUpdate   func(executablePath, downloadedPath string, pid int) error
}

func defaultSelfUpdateDeps() selfUpdateDeps {
	return selfUpdateDeps{
		currentVersion:  appVersion,
		repository:      appGitHubRepository,
		client:          &http.Client{Timeout: 2 * time.Minute},
		executable:      os.Executable,
		processID:       os.Getpid,
		downloadRelease: fetchLatestGithubRelease,
		downloadAsset:   downloadSelfUpdateAsset,
		installUpdate:   installDownloadedSelfUpdate,
	}
}

func ensureSelfUpdate(deps selfUpdateDeps, ui startupUI) error {
	if deps.currentVersion == "" || strings.EqualFold(deps.currentVersion, "dev") {
		ui.Status("Application update check disabled for development build")
		return nil
	}
	if strings.TrimSpace(deps.repository) == "" {
		return errors.New("GitHub repository is not configured")
	}
	if deps.client == nil {
		deps.client = &http.Client{Timeout: 2 * time.Minute}
	}

	release, err := deps.downloadRelease(deps.client, deps.repository)
	if err != nil {
		return fmt.Errorf("fetch GitHub release %q failed: %w", selfUpdateReleaseTag, err)
	}
	asset, ok := selectSelfUpdateAsset(release, deps.currentVersion)
	if !ok {
		ui.Status("Application is up to date")
		return nil
	}

	ui.Notice("A new Go Web Wallpaper version is available. The app will download the update, replace itself, and restart.")
	ui.Status("Downloading application update " + asset.Version)
	downloadedPath, err := deps.downloadAsset(deps.client, asset, func(progress webView2DownloadProgress) {
		ui.Progress("Downloading application update "+asset.Version, progress)
	})
	if err != nil {
		return fmt.Errorf("download application update failed: %w", err)
	}

	executablePath, err := deps.executable()
	if err != nil {
		_ = os.Remove(downloadedPath)
		return fmt.Errorf("locate executable failed: %w", err)
	}

	ui.Status("Installing application update " + asset.Version)
	if err := deps.installUpdate(executablePath, downloadedPath, deps.processID()); err != nil {
		_ = os.Remove(downloadedPath)
		return fmt.Errorf("install application update failed: %w", err)
	}
	return errRestarting
}

func fetchLatestGithubRelease(client *http.Client, repository string) (githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repository, selfUpdateReleaseTag)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "gowebwallpaper-self-update/"+appVersion)

	response, err := client.Do(request)
	if err != nil {
		return githubRelease{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("unexpected HTTP status: %s", response.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func selectSelfUpdateAsset(release githubRelease, currentVersion string) (selfUpdateAsset, bool) {
	if !strings.EqualFold(release.TagName, selfUpdateReleaseTag) {
		return selfUpdateAsset{}, false
	}
	if _, err := parseSemanticVersion(currentVersion); err != nil {
		return selfUpdateAsset{}, false
	}

	var selected selfUpdateAsset
	for _, asset := range release.Assets {
		version := versionFromSelfUpdateAssetName(asset.Name)
		if version == "" || asset.BrowserDownloadURL == "" {
			continue
		}
		compare, err := compareSemanticVersions(version, currentVersion)
		if err != nil || compare <= 0 {
			continue
		}
		if selected.Version != "" {
			best, err := compareSemanticVersions(version, selected.Version)
			if err != nil || best <= 0 {
				continue
			}
		}
		selected = selfUpdateAsset{
			Name:        asset.Name,
			Version:     version,
			DownloadURL: asset.BrowserDownloadURL,
		}
	}
	return selected, selected.Version != ""
}

func versionFromSelfUpdateAssetName(name string) string {
	matches := selfUpdateAssetPattern.FindStringSubmatch(name)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func compareSemanticVersions(left, right string) (int, error) {
	leftParts, err := parseSemanticVersion(left)
	if err != nil {
		return 0, err
	}
	rightParts, err := parseSemanticVersion(right)
	if err != nil {
		return 0, err
	}
	for i := range leftParts {
		switch {
		case leftParts[i] > rightParts[i]:
			return 1, nil
		case leftParts[i] < rightParts[i]:
			return -1, nil
		}
	}
	return 0, nil
}

func parseSemanticVersion(version string) ([3]int, error) {
	var parsed [3]int
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return parsed, fmt.Errorf("invalid semantic version %q", version)
	}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return parsed, fmt.Errorf("invalid semantic version %q: %w", version, err)
		}
		parsed[i] = value
	}
	return parsed, nil
}

func downloadSelfUpdateAsset(client *http.Client, asset selfUpdateAsset, reportProgress func(webView2DownloadProgress)) (string, error) {
	request, err := http.NewRequest(http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "gowebwallpaper-self-update/"+appVersion)

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected HTTP status: %s", response.Status)
	}

	file, err := os.CreateTemp("", "gowebwallpaper-update-*.exe")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := copyWithDownloadProgress(file, response.Body, response.ContentLength, reportProgress); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func installDownloadedSelfUpdate(executablePath, downloadedPath string, pid int) error {
	scriptPath, err := writeSelfUpdateScript()
	if err != nil {
		return err
	}

	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden",
		"-File", scriptPath,
		"-ProcessId", strconv.Itoa(pid),
		"-Source", downloadedPath,
		"-Target", executablePath,
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Start()
}

func writeSelfUpdateScript() (string, error) {
	file, err := os.CreateTemp("", "gowebwallpaper-self-update-*.ps1")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.WriteString(file, selfUpdatePowerShellScript()); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func selfUpdatePowerShellScript() string {
	return `param(
  [int]$ProcessId,
  [string]$Source,
  [string]$Target
)

$ErrorActionPreference = 'Stop'

try {
  Wait-Process -Id $ProcessId -Timeout 30
} catch {
}

Start-Sleep -Milliseconds 300
Copy-Item -LiteralPath $Source -Destination $Target -Force
Remove-Item -LiteralPath $Source -Force -ErrorAction SilentlyContinue
Start-Process -FilePath $Target -WorkingDirectory (Split-Path -Parent $Target)
Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
`
}
