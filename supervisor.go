package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const supervisorChildEnv = "GOWEBWALLPAPER_SUPERVISED"
const supervisorDisabledEnv = "GOWEBWALLPAPER_DISABLE_SUPERVISOR"

func maybeRunSupervisor() (bool, error) {
	config, err := loadConfig()
	if err != nil {
		return false, nil
	}
	if !shouldRunSupervisor(config, os.Getenv(supervisorChildEnv), os.Getenv(supervisorDisabledEnv)) {
		return false, nil
	}
	return true, runSupervisorProcess()
}

func shouldRunSupervisor(config *AppConfig, supervisedValue, disabledValue string) bool {
	if strings.TrimSpace(supervisedValue) != "" || strings.TrimSpace(disabledValue) != "" {
		return false
	}
	return config != nil && strings.EqualFold(strings.TrimSpace(config.Log), logLevelDebug)
}

func supervisedChildEnv(env []string) []string {
	next := make([]string, 0, len(env)+2)
	for _, entry := range env {
		if strings.HasPrefix(entry, supervisorChildEnv+"=") || strings.HasPrefix(entry, "GOTRACEBACK=") {
			continue
		}
		next = append(next, entry)
	}
	next = append(next, supervisorChildEnv+"=1")
	next = append(next, "GOTRACEBACK=all")
	return next
}

func runSupervisorProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	startedAt := time.Now()
	appendSupervisorLog("supervisor starting child: parentPid=%d exe=%q args=%q", os.Getpid(), exe, os.Args[1:])

	command := exec.Command(exe, os.Args[1:]...)
	command.Env = supervisedChildEnv(os.Environ())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdout, err := command.StdoutPipe()
	if err != nil {
		appendSupervisorLog("supervisor child stdout pipe failed: error=%v", err)
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		appendSupervisorLog("supervisor child stderr pipe failed: error=%v", err)
		return err
	}
	if err := command.Start(); err != nil {
		appendSupervisorLog("supervisor child start failed: error=%v", err)
		return err
	}
	appendSupervisorLog("supervisor child started: childPid=%d", command.Process.Pid)

	var outputDone sync.WaitGroup
	outputDone.Add(2)
	go func() {
		defer outputDone.Done()
		copySupervisorOutput("stdout", stdout, appendSupervisorLog)
	}()
	go func() {
		defer outputDone.Done()
		copySupervisorOutput("stderr", stderr, appendSupervisorLog)
	}()

	err = command.Wait()
	outputDone.Wait()
	exitCode := -1
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	appendSupervisorLog("supervisor child exited: childPid=%d exitCode=%d duration=%s error=%v", command.Process.Pid, exitCode, time.Since(startedAt).Round(time.Second), err)
	return nil
}

func copySupervisorOutput(source string, reader io.Reader, logf func(string, ...any)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		logf("supervisor child %s: %s", source, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logf("supervisor child %s read failed: %v", source, err)
	}
}

func appendSupervisorLog(format string, args ...any) {
	exePath, err := os.Executable()
	path := logFileName
	if err == nil {
		path = logPathForExecutable(exePath)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("supervisor log open failed: %v", err)
		return
	}
	defer file.Close()
	timestamp := time.Now().Format("2006/01/02 15:04:05.000000")
	_, _ = fmt.Fprintf(file, timestamp+" "+format+"\n", args...)
}
