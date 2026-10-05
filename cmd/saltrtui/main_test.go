package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionExitsBeforeRuntime(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Salt is unavailable and no terminal is needed.
	previous := version
	t.Cleanup(func() { version = previous })
	for _, value := range []string{"dev", "v1.2.3"} {
		version = value
		for _, args := range [][]string{{"--version"}, {"-version", "-config-dir", "/unavailable", "-no-alt-screen"}} {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 0 || stdout.String() != "saltrtui "+value+"\n" || stderr.Len() != 0 {
				t.Fatalf("code/output mismatch: %d %q %q", code, stdout.String(), stderr.String())
			}
		}
	}
}

func TestHelpAndInvalidFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "-version") || !strings.Contains(stderr.String(), "-config-dir") {
		t.Fatalf("help: %d %s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"--unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid flag exit: %d", code)
	}
}
