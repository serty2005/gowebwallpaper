//go:build windows

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsStartupUI struct {
	mu            sync.Mutex
	statusPath    string
	statusScript  string
	statusCommand *exec.Cmd
	statusClosed  bool
}

func (ui *windowsStartupUI) Status(message string) {
	log.Printf("startup: %s", message)
	ui.writeStatus(message, nil)
}

func (ui *windowsStartupUI) Progress(message string, progress webView2DownloadProgress) {
	if progress.Total > 0 {
		percent := progress.Downloaded * 100 / progress.Total
		log.Printf("startup: %s: %d%% (%d/%d bytes)", message, percent, progress.Downloaded, progress.Total)
	} else {
		log.Printf("startup: %s: %d bytes", message, progress.Downloaded)
	}
	ui.writeStatus(message, &progress)
}

func (ui *windowsStartupUI) Notice(message string) {
	log.Printf("startup notice: %s", message)
	showMessageBox("Go Web Wallpaper", message)
}

func (ui *windowsStartupUI) PromptURL(currentURL string, firstRun bool, webView2Version string) (string, bool, error) {
	ui.Close()
	selected, err := runURLPromptPowerShell(currentURL, firstRun, webView2Version)
	if err != nil {
		showMessageBox("Go Web Wallpaper", "Unable to show startup URL dialog. The existing URL will be used.\n\n"+err.Error())
		return currentURL, true, nil
	}
	return selected, true, nil
}

func (ui *windowsStartupUI) Close() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.statusClosed {
		return
	}
	ui.statusClosed = true
	if ui.statusPath != "" {
		_ = os.WriteFile(ui.statusPath, []byte("__CLOSE__"), 0644)
	}
	if ui.statusCommand != nil && ui.statusCommand.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = ui.statusCommand.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-timeAfterOneSecond():
			_ = ui.statusCommand.Process.Kill()
		}
	}
	if ui.statusPath != "" {
		_ = os.Remove(ui.statusPath)
	}
	if ui.statusScript != "" {
		_ = os.Remove(ui.statusScript)
	}
}

func (ui *windowsStartupUI) writeStatus(message string, progress *webView2DownloadProgress) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.statusClosed {
		return
	}
	if err := ui.ensureStatusWindow(message, progress); err != nil {
		log.Printf("startup progress window unavailable: %v", err)
		return
	}
	if err := os.WriteFile(ui.statusPath, []byte(formatStartupStatus(message, progress)), 0644); err != nil {
		log.Printf("startup progress update failed: %v", err)
	}
}

func (ui *windowsStartupUI) ensureStatusWindow(message string, progress *webView2DownloadProgress) error {
	if ui.statusPath != "" {
		return nil
	}
	statusFile, err := os.CreateTemp("", "gowebwallpaper-status-*.txt")
	if err != nil {
		return err
	}
	ui.statusPath = statusFile.Name()
	if _, err := statusFile.WriteString(formatStartupStatus(message, progress)); err != nil {
		statusFile.Close()
		return err
	}
	if err := statusFile.Close(); err != nil {
		return err
	}

	scriptFile, err := os.CreateTemp("", "gowebwallpaper-status-*.ps1")
	if err != nil {
		return err
	}
	ui.statusScript = scriptFile.Name()
	if _, err := scriptFile.WriteString(startupStatusWindowScript()); err != nil {
		scriptFile.Close()
		return err
	}
	if err := scriptFile.Close(); err != nil {
		return err
	}

	command := exec.Command("powershell.exe", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", ui.statusScript, ui.statusPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		return err
	}
	ui.statusCommand = command
	return nil
}

func formatStartupStatus(message string, progress *webView2DownloadProgress) string {
	downloaded := int64(-1)
	total := int64(-1)
	if progress != nil {
		downloaded = progress.Downloaded
		total = progress.Total
	}
	return fmt.Sprintf("%s\n%d\n%d\n", strings.ReplaceAll(message, "\n", " "), downloaded, total)
}

func timeAfterOneSecond() <-chan time.Time {
	return time.After(time.Second)
}

func showMessageBox(title, message string) {
	titlePtr, titleErr := windows.UTF16PtrFromString(title)
	messagePtr, messageErr := windows.UTF16PtrFromString(message)
	if titleErr != nil || messageErr != nil {
		return
	}
	const (
		mbOK         = 0x00000000
		mbIconInfo   = 0x00000040
		mbSetForegnd = 0x00010000
		mbTaskModal  = 0x00002000
	)
	user32 := windows.NewLazySystemDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	messageBox.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), mbOK|mbIconInfo|mbSetForegnd|mbTaskModal)
}

func runURLPromptPowerShell(currentURL string, firstRun bool, webView2Version string) (string, error) {
	scriptFile, err := os.CreateTemp("", "gowebwallpaper-startup-*.ps1")
	if err != nil {
		return "", err
	}
	scriptPath := scriptFile.Name()
	defer os.Remove(scriptPath)

	if _, err := scriptFile.WriteString(startupURLPromptScript()); err != nil {
		scriptFile.Close()
		return "", err
	}
	if err := scriptFile.Close(); err != nil {
		return "", err
	}

	command := exec.Command("powershell.exe", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	firstRunValue := "0"
	if firstRun {
		firstRunValue = "1"
	}
	command.Env = append(os.Environ(),
		"GOWEBWALLPAPER_FIRST_RUN="+firstRunValue,
		"GOWEBWALLPAPER_WEBVIEW2_VERSION="+webView2Version,
	)
	command.Stdin = strings.NewReader(currentURL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func startupURLPromptScript() string {
	return `
$currentUrl = [Console]::In.ReadToEnd().Trim()
$firstRun = $env:GOWEBWALLPAPER_FIRST_RUN -eq '1'
$webView2Version = $env:GOWEBWALLPAPER_WEBVIEW2_VERSION

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = 'Go Web Wallpaper'
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.ClientSize = New-Object System.Drawing.Size(560, 210)
$form.TopMost = $true

$status = New-Object System.Windows.Forms.Label
$status.Location = New-Object System.Drawing.Point(16, 14)
$status.Size = New-Object System.Drawing.Size(528, 54)
if ($firstRun) {
  $status.Text = "First start: config was created. Set the page URL here, then use tray Start when ready."
} elseif (![string]::IsNullOrWhiteSpace($webView2Version)) {
  $status.Text = "Startup check complete. WebView2: $webView2Version. Confirm the page URL before the browser starts."
} else {
  $status.Text = "Set the web page URL. The browser window will restart if it is currently running."
}
$form.Controls.Add($status)

$urlLabel = New-Object System.Windows.Forms.Label
$urlLabel.Location = New-Object System.Drawing.Point(16, 82)
$urlLabel.Size = New-Object System.Drawing.Size(120, 20)
$urlLabel.Text = 'Web page URL'
$form.Controls.Add($urlLabel)

$urlBox = New-Object System.Windows.Forms.TextBox
$urlBox.Location = New-Object System.Drawing.Point(16, 106)
$urlBox.Size = New-Object System.Drawing.Size(528, 24)
$urlBox.Text = $currentUrl
$form.Controls.Add($urlBox)

$okButton = New-Object System.Windows.Forms.Button
$okButton.Location = New-Object System.Drawing.Point(344, 156)
$okButton.Size = New-Object System.Drawing.Size(96, 30)
$okButton.Text = 'Continue'
$okButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.AcceptButton = $okButton
$form.Controls.Add($okButton)

$cancelButton = New-Object System.Windows.Forms.Button
$cancelButton.Location = New-Object System.Drawing.Point(448, 156)
$cancelButton.Size = New-Object System.Drawing.Size(96, 30)
$cancelButton.Text = 'Cancel'
$cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$form.CancelButton = $cancelButton
$form.Controls.Add($cancelButton)

$result = $form.ShowDialog()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
  [Console]::Out.Write($urlBox.Text)
} else {
  [Console]::Out.Write($currentUrl)
}
`
}

func startupStatusWindowScript() string {
	return `
param([string]$StatusFile)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = 'Go Web Wallpaper'
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.ClientSize = New-Object System.Drawing.Size(520, 142)
$form.TopMost = $true

$label = New-Object System.Windows.Forms.Label
$label.Location = New-Object System.Drawing.Point(16, 16)
$label.Size = New-Object System.Drawing.Size(488, 36)
$label.Text = 'Starting...'
$form.Controls.Add($label)

$progress = New-Object System.Windows.Forms.ProgressBar
$progress.Location = New-Object System.Drawing.Point(16, 62)
$progress.Size = New-Object System.Drawing.Size(488, 22)
$progress.Minimum = 0
$progress.Maximum = 100
$progress.Style = [System.Windows.Forms.ProgressBarStyle]::Marquee
$progress.MarqueeAnimationSpeed = 30
$form.Controls.Add($progress)

$detail = New-Object System.Windows.Forms.Label
$detail.Location = New-Object System.Drawing.Point(16, 94)
$detail.Size = New-Object System.Drawing.Size(488, 28)
$detail.Text = ''
$form.Controls.Add($detail)

function Format-Size([Int64]$Bytes) {
  if ($Bytes -lt 0) { return '' }
  if ($Bytes -lt 1MB) { return "$([Math]::Round($Bytes / 1KB, 1)) KB" }
  return "$([Math]::Round($Bytes / 1MB, 1)) MB"
}

function Update-Status {
  if (!(Test-Path -LiteralPath $StatusFile)) { return }
  $content = Get-Content -LiteralPath $StatusFile -Raw -ErrorAction SilentlyContinue
  if ($null -eq $content) { return }
  if ($content.Trim() -eq '__CLOSE__') {
    $timer.Stop()
    $form.Close()
    return
  }

  $lines = $content -split "\r?\n"
  if ($lines.Length -gt 0 -and $lines[0].Trim().Length -gt 0) {
    $label.Text = $lines[0]
  }

  $downloaded = -1L
  $total = -1L
  if ($lines.Length -gt 1) { [void][Int64]::TryParse($lines[1], [ref]$downloaded) }
  if ($lines.Length -gt 2) { [void][Int64]::TryParse($lines[2], [ref]$total) }

  if ($total -gt 0 -and $downloaded -ge 0) {
    $percent = [Math]::Min(100, [Math]::Max(0, [int](($downloaded * 100) / $total)))
    $progress.Style = [System.Windows.Forms.ProgressBarStyle]::Continuous
    $progress.Value = $percent
    $detail.Text = "$percent%  ($(Format-Size $downloaded) / $(Format-Size $total))"
  } else {
    $progress.Style = [System.Windows.Forms.ProgressBarStyle]::Marquee
    if ($downloaded -gt 0) {
      $detail.Text = "$(Format-Size $downloaded)"
    } else {
      $detail.Text = 'Please wait...'
    }
  }
}

$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 250
$timer.Add_Tick({ Update-Status })
$timer.Start()
Update-Status
[void]$form.ShowDialog()
`
}
