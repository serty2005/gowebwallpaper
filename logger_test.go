package main

import (
	"path/filepath"
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
