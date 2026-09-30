#!/bin/sh
# Installs the latest mvm release into $MVM_INSTALL_DIR (default ~/.local/bin).
#   curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/install.sh | sh
set -eu

REPO=MarcoTribuz/mvm
DIR=${MVM_INSTALL_DIR:-$HOME/.local/bin}

case "$(uname -s)" in
  Linux) os=linux ;;
  *) echo "mvm: unsupported OS $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "mvm: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

version=${MVM_VERSION:-$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p')}
[ -n "$version" ] || { echo "mvm: cannot determine latest version" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
base="https://github.com/$REPO/releases/download/v$version"
file="mvm_${version}_${os}_${arch}.tar.gz"
curl -fsSL -o "$tmp/$file" "$base/$file"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
(cd "$tmp" && grep " $file\$" checksums.txt | sha256sum -c -)

mkdir -p "$DIR"
tar -xzf "$tmp/$file" -C "$tmp" mvm
install -m 0755 "$tmp/mvm" "$DIR/mvm"
echo "mvm $version installed to $DIR/mvm"
case ":$PATH:" in *":$DIR:"*) ;; *) echo "add $DIR to your PATH" ;; esac
