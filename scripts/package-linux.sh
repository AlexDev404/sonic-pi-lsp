#!/usr/bin/env bash
# Packages the CLI for Linux: a .deb and a .tar.gz in dist/.
#
#   scripts/package-linux.sh VERSION [SUFFIX]
#
# Run scripts/build-server-linux.sh first. SUFFIX names the build (for
# example ubuntu-24.04), because the Ruby extensions are built for the
# distribution's own Ruby.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:?usage: package-linux.sh VERSION [SUFFIX]}"
SUFFIX="${2:-}"
DEBVER="${VERSION#v}"
ARCH="$(dpkg --print-architecture 2>/dev/null || echo amd64)"
DIST="$ROOT/dist"
WORK="$ROOT/build/package"
NAME="sonicpi-cli"
TAG="$DEBVER${SUFFIX:+_$SUFFIX}"

mkdir -p "$DIST"
rm -rf "$WORK"

echo "==> Go binaries"
(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X github.com/AlexDev404/sonic-pi-lsp/internal/version.Version=$VERSION" \
  -o build/bin/ ./cmd/sonicpi ./cmd/sonicpid)

echo "==> Stage"
"$ROOT/scripts/stage.sh" "$WORK/$NAME"
# The Ruby extensions were compiled for this Ruby, so run the server with it.
command -v ruby > "$WORK/$NAME/ruby.path"
"$WORK/$NAME/sonicpi" completion bash > "$WORK/$NAME/sonicpi.bash"
cp "$ROOT/packaging/linux/sonicpid.service" "$WORK/$NAME/"
cp "$ROOT/packaging/linux/install.sh" "$WORK/$NAME/"
cp "$ROOT/README.md" "$WORK/$NAME/" 2>/dev/null || true

echo "==> Tarball"
tar -C "$WORK" -czf "$DIST/${NAME}_${TAG}_linux_${ARCH}.tar.gz" "$NAME"

echo "==> Debian package"
PKG="$WORK/deb"
mkdir -p "$PKG/DEBIAN" "$PKG/usr/lib" "$PKG/usr/bin" "$PKG/usr/lib/systemd/user" \
  "$PKG/usr/share/bash-completion/completions" "$PKG/usr/share/doc/$NAME"
cp -r "$WORK/$NAME" "$PKG/usr/lib/$NAME"
rm -f "$PKG/usr/lib/$NAME/install.sh" "$PKG/usr/lib/$NAME/sonicpid.service" "$PKG/usr/lib/$NAME/sonicpi.bash"
mv "$PKG/usr/lib/$NAME/README.md" "$PKG/usr/share/doc/$NAME/" 2>/dev/null || true
ln -s "../lib/$NAME/sonicpi" "$PKG/usr/bin/sonicpi"
ln -s "../lib/$NAME/sonicpid" "$PKG/usr/bin/sonicpid"
cp "$ROOT/packaging/linux/sonicpid.service" "$PKG/usr/lib/systemd/user/"
cp "$WORK/$NAME/sonicpi.bash" "$PKG/usr/share/bash-completion/completions/sonicpi"
cp "$ROOT/packaging/linux/debian/postinst" "$ROOT/packaging/linux/debian/prerm" "$ROOT/packaging/linux/debian/postrm" "$PKG/DEBIAN/"

# Shared-library dependencies of the native parts (the audio engine,
# aubio_onset, the Ruby extensions), worked out by dpkg-shlibdeps.
DEPS=""
if command -v dpkg-shlibdeps >/dev/null; then
  BINS=$(find "$PKG/usr/lib/$NAME/sonic-pi" -type f \( -name '*.so' -o -perm -u+x \) -exec sh -c 'file -b "$1" | grep -q ELF' _ {} \; -print)
  if [ -n "$BINS" ]; then
    T="$WORK/shlibs"
    mkdir -p "$T/debian"
    printf 'Source: %s\n\nPackage: %s\nArchitecture: any\n' "$NAME" "$NAME" > "$T/debian/control"
    # shellcheck disable=SC2086
    DEPS=$(cd "$T" && dpkg-shlibdeps -O --ignore-missing-info $BINS 2>/dev/null | sed -n 's/^shlibs:Depends=//p')
  fi
fi
DEPS="ruby${DEPS:+, $DEPS}"
SIZE=$(du -sk "$PKG/usr" | cut -f1)
cat > "$PKG/DEBIAN/control" <<EOF
Package: $NAME
Version: $DEBVER
Architecture: $ARCH
Maintainer: sonic-pi-lsp <noreply@github.com>
Installed-Size: $SIZE
Depends: $DEPS
Recommends: pipewire-jack | jackd2, pipewire-alsa | alsa-utils
Section: sound
Priority: optional
Homepage: https://github.com/AlexDev404/sonic-pi-lsp
Description: Sonic Pi from the command line
 sonicpi plays Sonic Pi code from files (sonicpi song.rb), lints it
 (sonicpi lint), follows its log, and drives everything else the desktop
 app does - mixer, recording, MIDI, OSC, Link, buffers and settings -
 through a background service (sonicpid, a systemd user unit).
 .
 It bundles the Sonic Pi server and audio engine.
EOF
dpkg-deb --root-owner-group --build "$PKG" "$DIST/${NAME}_${TAG}_${ARCH}.deb"

ls -la "$DIST"
