package scripts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func script(t *testing.T, root, name string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "scripts", name), args...)
	cmd.Dir = t.TempDir() // scripts must not depend on the caller's directory
	return cmd.CombinedOutput()
}

func fixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "checkout with spaces")
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"validate-release-version.sh", "build-linux-amd64.sh", "build-targets.sh", "package-release.sh", "print-install-commands.sh", "refresh-readme-install.sh"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, "scripts", name), string(data))
	}
	return root
}

func TestRefreshReadmeInstall(t *testing.T) {
	root := fixture(t)
	readme := `# saltrtui

before
<!-- release-installation:start -->
old installation content
<!-- release-installation:end -->
after
`
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := script(t, root, "refresh-readme-install.sh", "v1.2.3"); err != nil {
		t.Fatalf("refresh README: %v %s", err, out)
	}
	text, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"before", "after", "saltrtui_v1.2.3_linux_amd64.gz", "saltrtui_v1.2.3_linux_arm64.gz",
		"saltrtui_v1.2.3_darwin_amd64.gz", "saltrtui_v1.2.3_darwin_arm64.gz",
		"curl -fsSL", "| gunzip | sudo install -m 0755 /dev/stdin /usr/local/bin/saltrtui",
		"sudo mkdir -p /usr/local/bin", "saltrtui v1.2.3",
	} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("missing %q in refreshed README:\n%s", want, text)
		}
	}
	if strings.Contains(string(text), "old installation content") {
		t.Fatalf("old installation content remained:\n%s", text)
	}
	if _, err := script(t, root, "refresh-readme-install.sh", "dev"); err == nil {
		t.Fatal("development release accepted")
	}
}

func TestPrintInstallCommands(t *testing.T) {
	root := fixture(t)
	out, err := script(t, root, "print-install-commands.sh", "v1.2.3")
	if err != nil {
		t.Fatalf("generate commands: %v %s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"Linux amd64:",
		"https://github.com/reinier-vegter/saltrtui/releases/download/v1.2.3/saltrtui_v1.2.3_linux_amd64.gz",
		"curl -fL",
		"gzip -dc",
		"/usr/local/bin/saltrtui",
		"test -s",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output:\n%s", want, text)
		}
	}
	if strings.Count(text, "curl -fL") != 1 || strings.Count(text, "releases/download/v1.2.3/") != 1 {
		t.Fatalf("expected one literal install command:\n%s", text)
	}
	for _, forbidden := range []string{"sha256sum", "SHA256SUMS", "saltrtui_*", "<version>", "<filename>"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("generated command contains %q:\n%s", forbidden, text)
		}
	}
	for _, invalid := range []string{"dev", "v01.2.3", "v1.2.3-rc.1", "v1.2.3;touch bad"} {
		if _, err := script(t, root, "print-install-commands.sh", invalid); err == nil {
			t.Fatalf("invalid version %q accepted", invalid)
		}
	}

	// The generated command deliberately calls sudo, so its effects belong to
	// native disposable-destination validation rather than this unit test.
	/*
	home := filepath.Join(t.TempDir(), "home with spaces")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	curlStub := `#!/bin/sh
set -eu
test "$1" = -fL
shift
test -n "$1"
shift
test "$1" = -o
shift
cp "$INSTALL_ARCHIVE" "$1"
`
	write(t, filepath.Join(bin, "curl"), curlStub)
	program := filepath.Join(t.TempDir(), "saltrtui")
	write(t, program, "#!/bin/sh\nprintf 'saltrtui v1.2.3\\n'\n")
	archive := filepath.Join(t.TempDir(), "release.gz")
	compress := exec.Command("gzip", "-c", program)
	compressed, err := compress.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, compressed, 0644); err != nil {
		t.Fatal(err)
	}
	command := strings.Split(text, "\n")[1]
	if err := runInstallCommand(command, home, bin, archive); err != nil {
		t.Fatalf("run generated installer: %v", err)
	}
	installed := filepath.Join(home, ".local", "bin", "saltrtui")
	if out, err := exec.Command(installed, "--version").Output(); err != nil || string(out) != "saltrtui v1.2.3\n" {
		t.Fatalf("installed executable: %v %q", err, out)
	}
	info, err := os.Stat(installed)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("installed permissions: %v %v", info, err)
	}
	staging, _ := filepath.Glob(filepath.Join(home, ".local", "bin", ".saltrtui.*"))
	if len(staging) != 0 {
		t.Fatalf("temporary installation files leaked: %v", staging)
	}

	// A failed download/extraction must not replace the previous working binary.
	corrupt := filepath.Join(t.TempDir(), "corrupt.gz")
	if err := os.WriteFile(corrupt, []byte("not gzip"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runInstallCommand(command, home, bin, corrupt); err == nil {
		t.Fatal("corrupt archive installed successfully")
	}
	if out, err := exec.Command(installed, "--version").Output(); err != nil || string(out) != "saltrtui v1.2.3\n" {
		t.Fatalf("failed install replaced working executable: %v %q", err, out)
	}
	*/
}

func runInstallCommand(command, home, bin, archive string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "HOME="+home, "INSTALL_ARCHIVE="+archive,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := cmd.CombinedOutput()
	return err
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0755); err != nil {
		t.Fatal(err)
	}
}

func fakeGo(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	write(t, filepath.Join(bin, "go"), `#!/bin/sh
set -eu
if [ "$1" = version ]; then
    case "$3" in
      *darwin_arm64) printf 'build GOOS=darwin\nbuild GOARCH=arm64\nbuild GOARM64=v8.0\n' ;;
      *darwin_amd64) printf 'build GOOS=darwin\nbuild GOARCH=amd64\nbuild GOAMD64=v1\n' ;;
      *linux_arm64) printf 'build GOOS=linux\nbuild GOARCH=arm64\nbuild GOARM64=v8.0\n' ;;
      *) printf 'build GOOS=linux\nbuild GOARCH=amd64\nbuild GOAMD64=v1\n' ;;
    esac
    printf 'build CGO_ENABLED=0\n'
    exit 0
fi
test "$1" = build
test "$CGO_ENABLED/$GOWORK" = 0/off
case "$GOOS/$GOARCH" in linux/amd64|linux/arm64|darwin/amd64|darwin/arm64) ;; *) exit 1 ;; esac
version=
output=
while [ "$#" -gt 0 ]; do
    case "$1" in
        -ldflags=*) version=${1##*=} ;;
        -o) shift; output=$1 ;;
    esac
    shift
done
test -n "$version"
printf '#!/bin/sh\nprintf "saltrtui %s\\n"\n' "$version" > "$output"
chmod 755 "$output"
if [ "${FAIL_BUILD:-}" = yes ]; then exit 1; fi
`)
	write(t, filepath.Join(bin, "readelf"), `#!/bin/sh
set -eu
# The packaging test supplies a synthetic executable; its metadata has already
# been asserted through the fake Go tool above.
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func TestStableVersionValidation(t *testing.T) {
	root := fixture(t)
	for _, version := range []string{"v0.0.0", "v1.2.3", "v12.300.4"} {
		if out, err := script(t, root, "validate-release-version.sh", version); err != nil {
			t.Fatalf("valid %q: %v %s", version, err, out)
		}
	}
	for _, version := range []string{"", "dev", "1.2.3", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-rc.1", "v1.2.3+build", " v1.2.3", "v1.2.3\n", "v1.2.3\nv4.5.6", "v1.2.3; touch injected", "v١.2.3"} {
		if out, err := script(t, root, "validate-release-version.sh", version); err == nil {
			t.Fatalf("invalid %q accepted: %s", version, out)
		}
	}
	for _, args := range [][]string{nil, {"v1.2.3", "extra"}} {
		if _, err := script(t, root, "validate-release-version.sh", args...); err == nil {
			t.Fatalf("invalid argument count accepted: %v", args)
		}
	}
}

func TestBuildVersionAndFailurePreservation(t *testing.T) {
	root := fixture(t)
	fakeGo(t)
	for _, args := range [][]string{nil, {"v1.2.3"}} {
		if out, err := script(t, root, "build-linux-amd64.sh", args...); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
		want := "dev"
		if len(args) > 0 {
			want = args[0]
		}
		out, err := exec.Command(filepath.Join(root, "dist", "saltrtui-linux-amd64"), "--version").Output()
		if err != nil || string(out) != "saltrtui "+want+"\n" {
			t.Fatalf("embedded version: %v %q", err, out)
		}
	}
	path := filepath.Join(root, "dist", "saltrtui-linux-amd64")
	before, _ := os.ReadFile(path)
	t.Setenv("FAIL_BUILD", "yes")
	for _, args := range [][]string{{"v2.0.0"}, {"v01.0.0"}, {""}, {"dev", "extra"}} {
		if _, err := script(t, root, "build-linux-amd64.sh", args...); err == nil {
			t.Fatalf("failed/invalid build succeeded: %v", args)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) {
			t.Fatal("failed build replaced previous executable")
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, "dist", ".saltrtui-*"))
	if len(leftovers) != 0 {
		t.Fatalf("build staging leaked: %v", leftovers)
	}
}

func TestReleasePackaging(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 packaging only")
	}
	root := fixture(t)
	bin := fakeGo(t)
	if out, err := script(t, root, "package-release.sh", "v1.2.3"); err != nil {
		t.Fatalf("package: %v %s", err, out)
	}
	dir := filepath.Join(root, "dist", "release", "v1.2.3")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 5 {
		t.Fatalf("unexpected release files: %v %v", entries, err)
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	for _, asset := range []string{
		"saltrtui_v1.2.3_linux_amd64.gz",
		"saltrtui_v1.2.3_linux_arm64.gz",
		"saltrtui_v1.2.3_darwin_amd64.gz",
		"saltrtui_v1.2.3_darwin_arm64.gz",
	} {
		if !strings.Contains(string(manifest), "  "+asset+"\n") {
			t.Fatalf("missing %s from manifest: %s", asset, manifest)
		}
	}
	if strings.Count(string(manifest), "\n") != 4 {
		t.Fatalf("invalid archive manifest: %s", manifest)
	}
	if _, err := script(t, root, "package-release.sh", "v1.2.3"); err == nil {
		t.Fatal("existing release set overwritten")
	}
	if _, err := script(t, root, "package-release.sh", "dev"); err == nil {
		t.Fatal("development release accepted")
	}
	write(t, filepath.Join(bin, "gzip"), "#!/bin/sh\nexit 1\n")
	if _, err := script(t, root, "package-release.sh", "v2.0.0"); err == nil {
		t.Fatal("failed compression accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "release", "v2.0.0")); !os.IsNotExist(err) {
		t.Fatal("partial release set exposed")
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, "dist", "release", ".package.*"))
	if len(leftovers) != 0 {
		t.Fatalf("packaging staging leaked: %v", leftovers)
	}
}
