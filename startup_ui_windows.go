//go:build windows

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsStartupUI struct{}

func (windowsStartupUI) Status(message string) {
	log.Printf("startup: %s", message)
}

func (windowsStartupUI) Notice(message string) {
	log.Printf("startup notice: %s", message)
	showMessageBox("Go Web Wallpaper", message)
}

func (windowsStartupUI) PromptURL(currentURL string, firstRun bool, webView2Version string) (string, bool, error) {
	selected, err := runURLPromptPowerShell(currentURL, firstRun, webView2Version)
	if err != nil {
		showMessageBox("Go Web Wallpaper", "Unable to show startup URL dialog. The existing URL will be used.\n\n"+err.Error())
		return currentURL, true, nil
	}
	return selected, true, nil
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

	command := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
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
} else {
  $status.Text = "Startup check complete. WebView2: $webView2Version. Confirm the page URL before the browser starts."
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
