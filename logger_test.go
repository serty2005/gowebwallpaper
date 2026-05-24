package main

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogPathForExecutableUsesExecutableDirectory(t *testing.T) {
	exePath := filepath.Join("C:", "apps", "gowebwallpaper", "gowebwallpaper.exe")

	path := logPathForExecutable(exePath)

	want := filepath.Join("C:", "apps", "gowebwallpaper", "gowebwallpaper.log")
	if path != want {
		t.Fatalf("expected %q, got %q", want, path)
	}
}

func TestDebugLogfWritesOnlyWhenEnabled(t *testing.T) {
	var output bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	previousDebug := debugLoggingEnabled()
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
		setDebugLogging(previousDebug)
	})
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")

	setDebugLogging(false)
	debugLogf("hidden input=%q output=%q", "in", "out")
	if output.Len() != 0 {
		t.Fatalf("expected debug log to be suppressed, got %q", output.String())
	}

	setDebugLogging(true)
	debugLogf("visible input=%q output=%q", "in", "out")

	got := output.String()
	if !strings.Contains(got, "debug: visible input=\"in\" output=\"out\"") {
		t.Fatalf("expected debug log with inputs and outputs, got %q", got)
	}
}

func TestConfigureLoggingFromConfigTogglesDebugMode(t *testing.T) {
	previousDebug := debugLoggingEnabled()
	t.Cleanup(func() {
		setDebugLogging(previousDebug)
	})

	configureLoggingFromConfig(&AppConfig{Log: logLevelDebug})
	if !debugLoggingEnabled() {
		t.Fatal("expected debug logging to be enabled")
	}

	configureLoggingFromConfig(&AppConfig{})
	if debugLoggingEnabled() {
		t.Fatal("expected debug logging to be disabled")
	}

	configureLoggingFromConfig(nil)
	if debugLoggingEnabled() {
		t.Fatal("expected nil config to disable debug logging")
	}
}

func TestRecoverAndLogPanicRecordsStack(t *testing.T) {
	var output bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})
	log.SetOutput(&output)
	log.SetFlags(0)

	func() {
		defer recoverAndLogPanic("test scope")
		panic("boom")
	}()

	got := output.String()
	if !strings.Contains(got, "panic recovered in test scope: boom") {
		t.Fatalf("expected panic message, got %q", got)
	}
	if !strings.Contains(got, "goroutine") {
		t.Fatalf("expected stack trace, got %q", got)
	}
}
