package release

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const releaseDownloadURL = "https://github.com/reinier-vegter/saltrtui/releases/download"
const maxAssetSize = 64 << 20
const maxBinarySize = 128 << 20

// Installation describes the running standalone binary and the local alternative.
type Installation struct {
	Current  string
	Local    string
	Writable bool
}

// InspectInstallation resolves symlinks and checks directory replacement access.
func InspectInstallation() (Installation, error) {
	current, err := os.Executable()
	if err != nil {
		return Installation{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Installation{}, err
	}
	return inspectInstallation(current, home)
}

func inspectInstallation(current, home string) (Installation, error) {
	current, err := filepath.EvalSymlinks(current)
	if err != nil {
		return Installation{}, err
	}
	installation := Installation{Current: current, Local: filepath.Join(home, ".local", "bin", "saltrtui")}
	probe, err := os.CreateTemp(filepath.Dir(current), ".saltrtui-write-check-*")
	if err == nil {
		probe.Close()
		os.Remove(probe.Name())
		installation.Writable = true
	} else if !os.IsPermission(err) {
		return Installation{}, fmt.Errorf("check installation directory: %w", err)
	}
	return installation, nil
}

// Artifact keeps fetched data opaque until its integrity has been verified.
type Artifact struct {
	name            string
	manifest, asset []byte
}

// Fetch downloads assets without installing or executing anything.
func Fetch(ctx context.Context, version string) (Artifact, error) {
	client := &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return fmt.Errorf("refusing non-HTTPS release redirect")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many release redirects")
		}
		return nil
	}}
	return fetchArtifact(ctx, client, releaseDownloadURL, version, runtime.GOOS, runtime.GOARCH)
}

func fetchArtifact(ctx context.Context, client *http.Client, base, version, goos, arch string) (Artifact, error) {
	if !isReleaseVersion(version) || !strings.HasPrefix(version, "v") {
		return Artifact{}, fmt.Errorf("invalid release version %q", version)
	}
	if goos != "linux" || arch != "amd64" {
		return Artifact{}, fmt.Errorf("unsupported platform %s/%s; saltrtui only publishes linux/amd64", goos, arch)
	}
	name := "saltrtui_" + version + "_" + goos + "_" + arch + ".gz"
	url := base + "/" + version + "/"
	manifest, err := fetchBytes(ctx, client, url+"SHA256SUMS", 1<<20)
	if err != nil {
		return Artifact{}, fmt.Errorf("download checksums: %w", err)
	}
	asset, err := fetchBytes(ctx, client, url+name, maxAssetSize)
	if err != nil {
		return Artifact{}, fmt.Errorf("download binary: %w", err)
	}
	return Artifact{name: name, manifest: manifest, asset: asset}, nil
}

// Verify validates the checksum and extracts bounded executable bytes.
func Verify(artifact Artifact) ([]byte, error) {
	name, manifest, asset := artifact.name, artifact.manifest, artifact.asset
	var err error
	var expected []byte
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if expected != nil {
			return nil, fmt.Errorf("duplicate checksum for %s", name)
		}
		expected, err = hex.DecodeString(fields[0])
		if err != nil || len(expected) != sha256.Size {
			return nil, fmt.Errorf("invalid checksum for %s", name)
		}
	}
	if expected == nil {
		return nil, fmt.Errorf("checksum missing for %s", name)
	}
	actual := sha256.Sum256(asset)
	if !bytes.Equal(actual[:], expected) {
		return nil, fmt.Errorf("checksum mismatch for %s", name)
	}
	reader, err := gzip.NewReader(bytes.NewReader(asset))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer reader.Close()
	binary, err := readBounded(reader, maxBinarySize)
	if err != nil {
		return nil, fmt.Errorf("extract binary: %w", err)
	}
	if len(binary) == 0 {
		return nil, fmt.Errorf("release binary is empty")
	}
	return binary, nil
}

func fetchBytes(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "saltrtui-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", response.Status)
	}
	return readBounded(response.Body, limit)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download or extraction exceeds size limit")
	}
	return data, nil
}

// Install atomically replaces a regular standalone executable with verified bytes.
func Install(destination string, binary []byte) error {
	if !filepath.IsAbs(destination) || len(binary) == 0 {
		return fmt.Errorf("installation requires an absolute path and non-empty binary")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if err := checkDestination(destination); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".saltrtui-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(binary); err != nil {
		return err
	}
	if err = file.Chmod(0755); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = checkDestination(destination); err != nil {
		return err
	}
	return os.Rename(file.Name(), destination)
}

func checkDestination(destination string) error {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace non-regular or symlink destination %s", destination)
	}
	return nil
}

// The script is constant; destination is an argument, never interpolated code.
// Root reads verified bytes from a pipe, not a mutable user-owned source file.
const sudoInstallScript = `set -eu
target=$1
case "$target" in /*) ;; *) exit 1 ;; esac
[ ! -L "$target" ] && { [ ! -e "$target" ] || [ -f "$target" ]; }
tmp=$(/usr/bin/mktemp "${target}.update.XXXXXX")
trap '/bin/rm -f "$tmp"' EXIT
trap 'exit 1' HUP INT TERM
/bin/cat > "$tmp"
[ -s "$tmp" ]
/bin/chmod 0755 "$tmp"
[ ! -L "$target" ] && { [ ! -e "$target" ] || [ -f "$target" ]; }
/bin/mv -f "$tmp" "$target"
`

// SudoCommand elevates only atomic installation. sudo prompts through /dev/tty.
func SudoCommand(destination string, binary []byte) (*exec.Cmd, error) {
	if !filepath.IsAbs(destination) || len(binary) == 0 {
		return nil, fmt.Errorf("installation requires an absolute path and non-empty binary")
	}
	command := exec.Command("sudo", "--", "/bin/sh", "-c", sudoInstallScript, "saltrtui-install", destination)
	command.Stdin = bytes.NewReader(binary)
	return command, nil
}

// LocalPathGuidance checks this process's PATH without editing shell profiles.
func LocalPathGuidance(destination string) string {
	resolved, err := exec.LookPath("saltrtui")
	if err == nil {
		resolved, err = filepath.EvalSymlinks(resolved)
	}
	local, localErr := filepath.EvalSymlinks(destination)
	if err == nil && localErr == nil && resolved == local {
		return "PATH resolves saltrtui to the user-local installation. Restart saltrtui; reset your shell command cache if needed (hash -r in Bash)."
	}
	return "PATH does not resolve saltrtui to the user-local installation. Put $HOME/.local/bin first on PATH (export PATH=\"$HOME/.local/bin:$PATH\"), persist it in your shell profile, and reset your shell command cache (hash -r in Bash). You can also run the installed path directly."
}
