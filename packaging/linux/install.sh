#!/usr/bin/env bash
# Installs sonicpi from the release tarball, for distributions without the
# .deb. Run from the unpacked sonicpi-cli directory:
#
#   ./install.sh            for this user: ~/.local/lib/sonicpi-cli, ~/.local/bin
#   sudo ./install.sh       for everyone:  /opt/sonicpi-cli, /usr/local/bin
#
# Then it registers the service as a systemd user unit and starts it.
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ "$(id -u)" = 0 ]; then
  DEST=/opt/sonicpi-cli
  BIN=/usr/local/bin
  UNITDIR=/etc/systemd/user
else
  DEST="$HOME/.local/lib/sonicpi-cli"
  BIN="$HOME/.local/bin"
  UNITDIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
fi

echo "Installing to $DEST"
rm -rf "$DEST"
mkdir -p "$DEST" "$BIN" "$UNITDIR"
cp -r "$SRC/sonicpi" "$SRC/sonicpid" "$SRC/lint" "$SRC/sonic-pi" "$DEST/"
[ -f "$SRC/ruby.path" ] && cp "$SRC/ruby.path" "$DEST/"
ln -sf "$DEST/sonicpi" "$BIN/sonicpi"
ln -sf "$DEST/sonicpid" "$BIN/sonicpid"
sed "s|/usr/bin/sonicpid|$DEST/sonicpid|" "$SRC/sonicpid.service" > "$UNITDIR/sonicpid.service"

if [ "$(id -u)" = 0 ]; then
  mkdir -p /var/log/sonic-pi && chmod 1777 /var/log/sonic-pi
  systemctl --global enable sonicpid.service 2>/dev/null || true
  echo "Enabled the sonicpid user service for every user (it starts at their next login)."
elif systemctl --user daemon-reload 2>/dev/null; then
  systemctl --user enable --now sonicpid.service
  echo "Started the sonicpid user service."
else
  echo "systemd's user instance is not available: sonicpi will start the service when it needs it."
fi

case ":$PATH:" in
  *":$BIN:"*) ;;
  *) echo "Add $BIN to your PATH to run sonicpi." ;;
esac
echo "Done. Try: sonicpi doctor"
