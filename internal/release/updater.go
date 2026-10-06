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
	"syscall"
)

const releaseDownloadURL = "https://github.com/reinier-vegter/saltrtui/releases/download"
const maxAssetSize = 64 << 20
const maxBinarySize = 128 << 20

// Installation describes the running standalone binary and the local alternative.
type Installation struct {
	Current        string
	Writable       bool
	Eligible       bool
	SudoAllowed    bool
	OriginalDigest string
	Reason         string
}

// InspectInstallation resolves symlinks and checks directory replacement access.
func InspectInstallation() (Installation, error) {
	current, err := os.Executable()
	if err != nil {
		return Installation{}, err
	}
	return inspectInstallation(current, "")
}

func inspectInstallation(current, home string) (Installation, error) {
	current, err := filepath.EvalSymlinks(current)
	if err != nil {
		return Installation{}, err
	}
	installation := Installation{Current: current}
	info, err := os.Lstat(current)
	if err != nil {
		return Installation{}, err
	}
	if !info.Mode().IsRegular() || filepath.Base(current) != "saltrtui" {
		installation.Reason = "the running executable is not a standalone saltrtui file"
		return installation, nil
	}
	parent := filepath.Dir(current)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	parentInfo, parentErr := os.Stat(parent)
	if err != nil || parentErr != nil || resolvedParent != parent || parentInfo.Mode().Perm()&0o022 != 0 {
		installation.Reason = "the installation path is not safe for in-place replacement"
		return installation, nil
	}
	digest, err := fileDigest(current)
	if err != nil {
		return Installation{}, fmt.Errorf("digest running executable: %w", err)
	}
	installation.OriginalDigest = digest
	installation.Eligible = true
	probe, err := os.CreateTemp(filepath.Dir(current), ".saltrtui-write-check-*")
	if err == nil {
		probe.Close()
		os.Remove(probe.Name())
		installation.Writable = true
	} else if !os.IsPermission(err) {
		return Installation{}, fmt.Errorf("check installation directory: %w", err)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid == 0 && info.Mode().Perm()&0o022 == 0 {
		installation.SudoAllowed = true
	}
	return installation, nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
	return InstallChecked(destination, "", binary)
}

// InstallChecked preserves the reviewed executable until the verified payload
// is ready for an atomic replacement.
func InstallChecked(destination, expectedDigest string, binary []byte) error {
	if !filepath.IsAbs(destination) || len(binary) == 0 {
		return fmt.Errorf("installation requires an absolute path and non-empty binary")
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
	if expectedDigest != "" {
		actual, err := fileDigest(destination)
		if err != nil || actual != expectedDigest {
			return fmt.Errorf("running executable changed since update review")
		}
	}
	if err := os.Rename(file.Name(), destination); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("replacement committed but parent sync unavailable: %w", err)
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("replacement committed but parent sync unavailable: %w", err)
	}
	return nil
}

// ReplacementCommitted reports whether an error occurred after the atomic
// rename commit point. Callers must not claim the previous executable survived.
func ReplacementCommitted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "replacement committed")
}

// InstallFromReader is the only elevated helper operation. It verifies the
// reviewed byte count and digest before reusing the normal atomic installer.
func InstallFromReader(destination, originalDigest, payloadDigest string, length int64, reader io.Reader) error {
	if length <= 0 || length > maxBinarySize {
		return fmt.Errorf("invalid installer payload length")
	}
	binary, err := readBounded(io.LimitReader(reader, length+1), length)
	if err != nil {
		return err
	}
	if int64(len(binary)) != length {
		return fmt.Errorf("installer payload length mismatch")
	}
	hash := sha256.Sum256(binary)
	if hex.EncodeToString(hash[:]) != payloadDigest {
		return fmt.Errorf("installer payload digest mismatch")
	}
	return InstallChecked(destination, originalDigest, binary)
}

func checkDestination(destination string) error {
	parent := filepath.Dir(destination)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || resolvedParent != parent {
		return fmt.Errorf("refusing symlinked or unavailable installation directory %s", parent)
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("refusing unsafe installation directory %s", parent)
	}
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return fmt.Errorf("refusing to create a new update destination %s", destination)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace non-regular or symlink destination %s", destination)
	}
	return nil
}

// SudoCommand invokes the inspected root-owned executable as a narrowly scoped
// helper. Root receives payload bytes only through stdin.
func SudoCommand(helper, destination, expectedDigest string, binary []byte) (*exec.Cmd, error) {
	if !filepath.IsAbs(helper) || !filepath.IsAbs(destination) || expectedDigest == "" || len(binary) == 0 {
		return nil, fmt.Errorf("installation requires an absolute path and non-empty binary")
	}
	payloadHash := sha256.Sum256(binary)
	command := exec.Command("/usr/bin/sudo", "--", helper, "--install-stdin", destination, expectedDigest, hex.EncodeToString(payloadHash[:]), fmt.Sprintf("%d", len(binary)))
	command.Stdin = bytes.NewReader(binary)
	return command, nil
}
