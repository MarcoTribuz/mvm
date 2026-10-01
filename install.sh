#!/bin/sh
# Installs the latest mvm release into $MVM_INSTALL_DIR (default ~/.local/bin),
# creates the meteor shim and adds PATH + shell completion to your profile.
#   curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/install.sh | sh
# PROFILE=/path/to/rc picks the profile; PROFILE=/dev/null leaves profiles alone.
set -eu

REPO=MarcoTribuz/mvm
DIR=${MVM_INSTALL_DIR:-$HOME/.local/bin}
HOME_DIR=${MVM_HOME:-$HOME/.mvm}
BEGIN='# >>> mvm >>>'
END='# <<< mvm <<<'

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
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
if command -v sha256sum >/dev/null 2>&1; then sha256="sha256sum"; else sha256="shasum -a 256"; fi
(cd "$tmp" && grep " $file\$" checksums.txt | $sha256 -c -)

mkdir -p "$DIR"
tar -xzf "$tmp/$file" -C "$tmp" mvm
install -m 0755 "$tmp/mvm" "$DIR/mvm"
echo "mvm $version installed to $DIR/mvm"
MVM_HOME="$HOME_DIR" "$DIR/mvm" init >/dev/null

# Pick the profile of the user's login shell, like nvm does.
shell=$(basename "${SHELL:-sh}")
if [ -n "${PROFILE:-}" ]; then
  profile=$PROFILE
else
  case "$shell" in
    zsh) profile=${ZDOTDIR:-$HOME}/.zshrc ;;
    bash) if [ "$os" = darwin ]; then profile=$HOME/.bash_profile; else profile=$HOME/.bashrc; fi ;;
    fish) profile=${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish ;;
    *) profile=$HOME/.profile ;;
  esac
fi
case "$profile" in
  fish|*.fish) kind=fish ;;
  *zshrc|*.zsh*) kind=zsh ;;
  *bashrc|*bash_profile) kind=bash ;;
  *) kind=sh ;;
esac

# Write paths under $HOME as "$HOME/..." so the profile stays readable.
rel() { case "$1" in "$HOME"/*) printf '%s' "\$HOME${1#"$HOME"}" ;; *) printf '%s' "$1" ;; esac; }

block() {
  bin=$(rel "$DIR") shim=$(rel "$HOME_DIR/bin")
  echo "$BEGIN"
  if [ "$kind" = fish ]; then
    echo "set -gx PATH \"$bin\" \"$shim\" \$PATH"
    echo "mvm completion fish | source"
  else
    echo "export PATH=\"$bin:$shim:\$PATH\""
    [ "$kind" = zsh ] && echo 'if (( $+functions[compdef] )); then source <(mvm completion zsh); fi'
    [ "$kind" = bash ] && echo 'eval "$(mvm completion bash)"'
  fi
  echo "$END"
}

if [ "$profile" = /dev/null ]; then
  echo "PROFILE=/dev/null: add $DIR and $HOME_DIR/bin to your PATH yourself"
elif [ -f "$profile" ] && grep -qxF "$BEGIN" "$profile"; then
  echo "$profile already sets up mvm"
else
  mkdir -p "$(dirname "$profile")"
  { echo; block; } >>"$profile"
  echo "added mvm PATH and completion to $profile"
  echo "open a new terminal, or run: . \"$profile\""
fi
