#!/bin/sh
# Removes the mvm binary, its store ($MVM_HOME, default ~/.mvm, which holds
# the downloaded Meteor releases and the meteor shim) and the lines install.sh
# added to your shell profile. ~/.meteor is not touched.
#   curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/uninstall.sh | sh
# Pass -y to skip the confirmation (needed when no terminal is attached):
#   curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/uninstall.sh | sh -s -- -y
set -eu

DIR=${MVM_INSTALL_DIR:-$HOME/.local/bin}
HOME_DIR=${MVM_HOME:-$HOME/.mvm}
BEGIN='# >>> mvm >>>'
END='# <<< mvm <<<'

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

profiles=
for p in "${PROFILE:-}" "${ZDOTDIR:-$HOME}/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" \
         "$HOME/.profile" "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish"; do
  [ -n "$p" ] && [ -f "$p" ] && grep -qxF "$BEGIN" "$p" || continue
  case " $profiles " in *" $p "*) continue ;; esac
  profiles="$profiles $p"
done

targets=
[ -e "$DIR/mvm" ] && targets="$targets $DIR/mvm"
[ -e "$HOME_DIR" ] && targets="$targets $HOME_DIR"
if [ -z "$targets$profiles" ]; then
  echo "mvm: nothing to remove ($DIR/mvm and $HOME_DIR not found)"
  exit 0
fi

echo "mvm: this will remove:"
for t in $targets; do echo "  $t"; done
for p in $profiles; do echo "  mvm lines in $p"; done
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
for p in $profiles; do
  # Drop the block and the blank line install.sh put before it; cat keeps
  # the profile's permissions and any symlink (dotfile managers).
  tmp=$(mktemp)
  awk -v b="$BEGIN" -v e="$END" '
    $0 == b { skip = 1; blank = 0; next }
    skip { if ($0 == e) skip = 0; next }
    blank { print ""; blank = 0 }
    $0 == "" { blank = 1; next }
    { print }
    END { if (blank) print "" }
  ' "$p" >"$tmp" && cat "$tmp" >"$p"
  rm -f "$tmp"
done
echo "mvm uninstalled; open a new terminal to refresh PATH"
