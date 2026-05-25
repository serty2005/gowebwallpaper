package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestShouldRunSupervisorOnlyForUnsupervisedDebugConfig(t *testing.T) {
	if !shouldRunSupervisor(&AppConfig{Log: logLevelDebug}, "", "") {
		t.Fatal("expected debug config to enable supervisor")
	}
	if shouldRunSupervisor(&AppConfig{Log: logLevelDebug}, "1", "") {
		t.Fatal("did not expect already supervised child to start another supervisor")
	}
	if shouldRunSupervisor(&AppConfig{Log: logLevelDebug}, "", "1") {
		t.Fatal("did not expect disabled supervisor to run")
	}
	if shouldRunSupervisor(&AppConfig{}, "", "") {
		t.Fatal("did not expect normal logging config to enable supervisor")
	}
}

func TestSupervisedChildEnvAddsMarkerAndTraceback(t *testing.T) {
	env := supervisedChildEnv([]string{"A=B", "GOTRACEBACK=single"})

	if len(env) != 3 {
		t.Fatalf("expected filtered env plus diagnostics, got %#v", env)
	}
	if env[1] != supervisorChildEnv+"=1" {
		t.Fatalf("expected supervisor marker, got %#v", env)
	}
	if env[2] != "GOTRACEBACK=all" {
		t.Fatalf("expected GOTRACEBACK=all, got %#v", env)
	}
}

func TestCopySupervisorOutputPrefixesEachLine(t *testing.T) {
	var lines []string

	copySupervisorOutput("stderr", strings.NewReader("panic: boom\nstack line\n"), func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	})

	if len(lines) != 2 {
		t.Fatalf("expected two captured lines, got %#v", lines)
	}
	if lines[0] != "supervisor child stderr: panic: boom" {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
	if lines[1] != "supervisor child stderr: stack line" {
		t.Fatalf("unexpected second line: %q", lines[1])
	}
}
