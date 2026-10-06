#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$#" -ne 1 ]; then
    echo 'Usage: refresh-readme-install.sh vMAJOR.MINOR.PATCH' >&2
    exit 2
fi
version=$1
"$root/scripts/validate-release-version.sh" "$version"

readme="$root/README.md"
start='<!-- release-installation:start -->'
end='<!-- release-installation:end -->'
if ! grep -Fqx "$start" "$readme" || ! grep -Fqx "$end" "$readme"; then
    echo 'README installation markers are missing' >&2
    exit 1
fi

block=$(mktemp)
output=$(mktemp)
trap 'rm -f "$block" "$output"' EXIT HUP INT TERM

asset() {
    os=$1
    arch=$2
    printf 'https://github.com/reinier-vegter/saltrtui/releases/download/%s/saltrtui_%s_%s_%s.gz' "$version" "$version" "$os" "$arch"
}

install_block() {
    os=$1
    arch=$2
    url=$(asset "$os" "$arch")
    cat <<EOF
\`\`\`sh
sudo mkdir -p /usr/local/bin
test ! -L /usr/local/bin/saltrtui
curl -fsSL '$url' | gunzip | sudo install -m 0755 /dev/stdin /usr/local/bin/saltrtui
\`\`\`
EOF
}

cat > "$block" <<EOF
$start

The current stable release is [$version](https://github.com/reinier-vegter/saltrtui/releases/tag/$version). Choose the archive for your operating system and architecture. These commands require \`curl\`, \`gunzip\`, and \`sudo\`.

### Linux

**Intel/AMD 64-bit (\`x86_64\`):** [saltrtui_${version}_linux_amd64.gz]($(asset linux amd64))

$(install_block linux amd64)

**ARM 64-bit (\`aarch64\` / \`arm64\`):** [saltrtui_${version}_linux_arm64.gz]($(asset linux arm64))

$(install_block linux arm64)

### macOS

**Intel Mac:** [saltrtui_${version}_darwin_amd64.gz]($(asset darwin amd64))

$(install_block darwin amd64)

**Apple Silicon Mac (M-series):** [saltrtui_${version}_darwin_arm64.gz]($(asset darwin arm64))

$(install_block darwin arm64)

Confirm command resolution and the installed release:

\`\`\`sh
command -v saltrtui
saltrtui --version
\`\`\`

The command should resolve to \`/usr/local/bin/saltrtui\` and report \`saltrtui $version\`. If it does not, put \`/usr/local/bin\` on \`PATH\`, clear the shell command cache (for example, \`hash -r\` in Bash), and check again. Do not overwrite a package-managed executable; use its distribution channel or install the standalone release in a safe location instead.

$end
EOF

awk -v start="$start" -v end="$end" -v block="$block" '
    $0 == start {
        while ((getline line < block) > 0) print line
        close(block)
        replacing = 1
        next
    }
    $0 == end && replacing {
        replacing = 0
        next
    }
    !replacing { print }
' "$readme" > "$output"
mv "$output" "$readme"
