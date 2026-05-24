package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const autostartTaskName = "Go Web Wallpaper"

type autostartDeps struct {
	executable func() (string, error)
	run        func(name string, args ...string) ([]byte, error)
}

type scheduledTaskXML struct {
	Settings struct {
		Enabled string `xml:"Enabled"`
	} `xml:"Settings"`
	Actions struct {
		Execs []scheduledTaskExec `xml:"Exec"`
	} `xml:"Actions"`
}

type scheduledTaskExec struct {
	Command   string `xml:"Command"`
	Arguments string `xml:"Arguments"`
}

func defaultAutostartDeps() autostartDeps {
	return autostartDeps{
		executable: os.Executable,
		run: func(name string, args ...string) ([]byte, error) {
			command := exec.Command(name, args...)
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			return command.CombinedOutput()
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
	output, err := deps.run("schtasks.exe",
		"/Create",
		"/TN", autostartTaskName,
		"/SC", "ONLOGON",
		"/TR", quoteTaskRunPath(exe),
		"/RL", "LIMITED",
		"/F",
	)
	if err != nil {
		return fmt.Errorf("create autostart task failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	enabled, err := autostartEnabled(deps)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("autostart task was created but verification failed")
	}
	return nil
}

func disableAutostart(deps autostartDeps) error {
	output, err := deps.run("schtasks.exe", "/Delete", "/TN", autostartTaskName, "/F")
	if err != nil && !looksLikeMissingTask(output, err) {
		return fmt.Errorf("delete autostart task failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func autostartEnabled(deps autostartDeps) (bool, error) {
	exe, err := deps.executable()
	if err != nil {
		return false, err
	}
	output, err := deps.run("schtasks.exe", "/Query", "/TN", autostartTaskName, "/XML")
	if err != nil {
		return false, nil
	}
	task, err := parseScheduledTaskXML(output)
	if err != nil {
		return false, err
	}
	if !task.Enabled() {
		return false, nil
	}
	for _, action := range task.Actions.Execs {
		if sameExecutablePath(exe, action.Command) {
			return true, nil
		}
	}
	return false, nil
}

func parseScheduledTaskXML(output []byte) (scheduledTaskXML, error) {
	var task scheduledTaskXML
	if err := xml.Unmarshal(output, &task); err != nil {
		return scheduledTaskXML{}, fmt.Errorf("parse autostart task XML failed: %w", err)
	}
	return task, nil
}

func (task scheduledTaskXML) Enabled() bool {
	enabled := strings.TrimSpace(task.Settings.Enabled)
	return enabled == "" || strings.EqualFold(enabled, "true")
}

func quoteTaskRunPath(path string) string {
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

func looksLikeMissingTask(output []byte, err error) bool {
	text := strings.ToLower(string(output) + " " + err.Error())
	return strings.Contains(text, "cannot find") ||
		strings.Contains(text, "does not exist") ||
		strings.Contains(text, "cannot find the file")
}
