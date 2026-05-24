package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
)

const logFileName = "gowebwallpaper.log"
const logLevelDebug = "debug"

var appLogFile *os.File
var debugLogging atomic.Bool

func initFileLogging() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	path := logPathForExecutable(exePath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	appLogFile = file
	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("logging initialized: %s", path)
	return nil
}

func configureLoggingFromConfig(config *AppConfig) {
	enabled := config != nil && strings.EqualFold(strings.TrimSpace(config.Log), logLevelDebug)
	previous := debugLoggingEnabled()
	setDebugLogging(enabled)
	if previous != enabled {
		log.Printf("debug logging enabled=%t", enabled)
	}
}

func setDebugLogging(enabled bool) {
	debugLogging.Store(enabled)
}

func debugLoggingEnabled() bool {
	return debugLogging.Load()
}

func debugLogf(format string, args ...any) {
	if !debugLoggingEnabled() {
		return
	}
	log.Printf("debug: "+format, args...)
}

func recoverAndLogPanic(scope string) {
	if recovered := recover(); recovered != nil {
		logRecoveredPanic(scope, recovered)
	}
}

func logRecoveredPanic(scope string, recovered any) {
	log.Printf("panic recovered in %s: %v\n%s", scope, recovered, debug.Stack())
}

func logPathForExecutable(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), logFileName)
}

func closeFileLogging() {
	if appLogFile == nil {
		return
	}
	_ = appLogFile.Close()
	appLogFile = nil
	log.SetOutput(io.Discard)
}
