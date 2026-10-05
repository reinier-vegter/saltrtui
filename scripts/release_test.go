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
	for _, name := range []string{"validate-release-version.sh", "build-linux-amd64.sh", "package-release.sh"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, "scripts", name), string(data))
	}
	return root
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
    printf 'build GOOS=linux\nbuild GOARCH=amd64\nbuild CGO_ENABLED=0\n'
    exit 0
fi
test "$1" = build
test "$GOOS/$GOARCH/$CGO_ENABLED/$GOWORK" = linux/amd64/0/off
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
	if err != nil || len(entries) != 2 || entries[0].Name() != "SHA256SUMS" || entries[1].Name() != "saltrtui_v1.2.3_linux_amd64.gz" {
		t.Fatalf("unexpected release files: %v %v", entries, err)
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if !strings.HasSuffix(string(manifest), "  saltrtui_v1.2.3_linux_amd64.gz\n") {
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
