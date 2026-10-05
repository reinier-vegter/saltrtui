package release

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func checksumLine(name string, data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)
}

func TestFetchArtifactRejectsInvalidVersionsAndPlatforms(t *testing.T) {
	client := &http.Client{}
	if _, err := fetchArtifact(context.Background(), client, "https://example.invalid", "v1.2.3", "windows", "amd64"); err == nil {
		t.Fatal("expected unsupported platform rejection")
	}
	if _, err := fetchArtifact(context.Background(), client, "https://example.invalid", "latest", "linux", "amd64"); err == nil {
		t.Fatal("expected invalid version rejection")
	}
}

func TestFetchAndVerifyRoundTrip(t *testing.T) {
	archive := gzipBytes(t, []byte("fake-binary-contents"))
	name := "saltrtui_v1.2.3_linux_amd64.gz"
	sums := checksumLine(name, archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.2.3/SHA256SUMS":
			w.Write([]byte(sums))
		case "/v1.2.3/" + name:
			w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	artifact, err := fetchArtifact(context.Background(), server.Client(), server.URL, "v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	binary, err := Verify(artifact)
	if err != nil || string(binary) != "fake-binary-contents" {
		t.Fatalf("verify = %q, %v", binary, err)
	}
}

func TestVerifyRejectsMismatchedAndMissingChecksums(t *testing.T) {
	archive := gzipBytes(t, []byte("payload"))
	name := "saltrtui_v1.2.3_linux_amd64.gz"
	if _, err := Verify(Artifact{name: name, manifest: []byte("deadbeef  " + name + "\n"), asset: archive}); err == nil {
		t.Fatal("expected checksum mismatch rejection")
	}
	if _, err := Verify(Artifact{name: name, manifest: []byte(""), asset: archive}); err == nil {
		t.Fatal("expected missing checksum rejection")
	}
	duplicate := checksumLine(name, archive) + checksumLine(name, archive)
	if _, err := Verify(Artifact{name: name, manifest: []byte(duplicate), asset: archive}); err == nil {
		t.Fatal("expected duplicate checksum rejection")
	}
}

func TestInstallAtomicReplacementAndSymlinkRefusal(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "saltrtui")
	if err := os.WriteFile(destination, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Install(destination, []byte("new-binary")); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "new-binary" {
		t.Fatalf("installed contents = %q, %v", contents, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary file was not cleaned up: %v", entries)
	}

	link := filepath.Join(dir, "linked")
	if err := os.Symlink(destination, link); err != nil {
		t.Fatal(err)
	}
	if err := Install(link, []byte("x")); err == nil {
		t.Fatal("expected symlink destination to be refused")
	}
}

func TestInstallRejectsRelativeOrEmptyInput(t *testing.T) {
	if err := Install("relative/path", []byte("x")); err == nil {
		t.Fatal("expected relative destination rejection")
	}
	if err := Install(filepath.Join(t.TempDir(), "saltrtui"), nil); err == nil {
		t.Fatal("expected empty binary rejection")
	}
}

func TestInspectInstallationWritability(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "saltrtui")
	if err := os.WriteFile(current, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	installation, err := inspectInstallation(current, home)
	if err != nil || !installation.Writable || installation.Local != filepath.Join(home, ".local", "bin", "saltrtui") {
		t.Fatalf("installation = %#v, %v", installation, err)
	}
}

func TestSudoCommandNeverInvokedInTests(t *testing.T) {
	command, err := SudoCommand("/usr/local/bin/saltrtui", []byte("verified"))
	if err != nil {
		t.Fatal(err)
	}
	if command.Path == "" || command.Args[0] != "sudo" {
		t.Fatalf("unexpected command: %#v", command)
	}
	// This test never runs command.Run(); it only checks construction.
}
