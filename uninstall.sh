#!/bin/sh
# Removes the mvm binary and its store ($MVM_HOME, default ~/.mvm, which holds
# the downloaded Meteor releases and the meteor shim). ~/.meteor is not touched.
#   curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/uninstall.sh | sh
# Pass -y to skip the confirmation (needed when no terminal is attached):
#   curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/uninstall.sh | sh -s -- -y
set -eu

DIR=${MVM_INSTALL_DIR:-$HOME/.local/bin}
HOME_DIR=${MVM_HOME:-$HOME/.mvm}

yes=
for arg in "$@"; do
  case "$arg" in
    -y|--yes) yes=1 ;;
    *) echo "mvm: unknown option $arg" >&2; exit 2 ;;
  esac
done

case "$HOME_DIR" in
  ""|/|"$HOME"|"$HOME/") echo "mvm: refusing to remove MVM_HOME=$HOME_DIR" >&2; exit 1 ;;
esac

targets=
[ -e "$DIR/mvm" ] && targets="$targets $DIR/mvm"
[ -e "$HOME_DIR" ] && targets="$targets $HOME_DIR"
if [ -z "$targets" ]; then
  echo "mvm: nothing to remove ($DIR/mvm and $HOME_DIR not found)"
  exit 0
fi

echo "mvm: this will remove:"
for t in $targets; do echo "  $t"; done
if [ -z "$yes" ]; then
  if ! { : </dev/tty; } 2>/dev/null; then
    echo "mvm: no terminal to confirm; re-run with -y" >&2
    exit 1
  fi
  printf "continue? [y/N] "
  read -r answer </dev/tty
  case "$answer" in
    y|Y|yes|YES) ;;
    *) echo "mvm: aborted"; exit 1 ;;
  esac
fi

rm -f "$DIR/mvm"
rm -rf "$HOME_DIR"
echo "mvm uninstalled"
echo "remove the mvm PATH lines (e.g. $HOME_DIR/bin, added after 'mvm init') from your shell profile"
