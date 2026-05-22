package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

const logFileName = "gowebwallpaper.log"

var appLogFile *os.File

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
