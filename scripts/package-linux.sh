#!/usr/bin/env bash
# Packages the Linux desktop build as .deb and AppImage.
#   scripts/package-linux.sh <version> <app-binary> <helper-binary> <icon.png> <out-dir>
set -euo pipefail

VERSION="${1#v}"
APP_BIN="$2"
HELPER_BIN="$3"
ICON="$4"
OUT="$5"
ARCH=amd64
mkdir -p "$OUT"

desktop_entry() {
  cat <<EOF
[Desktop Entry]
Name=ExitLagFree
Comment=Self-hosted game traffic relay
Exec=$1
Icon=exitlagfree
Terminal=false
Type=Application
Categories=Network;Game;
EOF
}

# .deb: app and helper live together in /opt so the app finds the helper.
DEB="$(mktemp -d)"
install -Dm755 "$APP_BIN" "$DEB/opt/exitlagfree/ExitLagFree"
install -Dm755 "$HELPER_BIN" "$DEB/opt/exitlagfree/exitlag-helper"
install -Dm644 "$ICON" "$DEB/usr/share/icons/hicolor/512x512/apps/exitlagfree.png"
mkdir -p "$DEB/usr/bin" "$DEB/usr/share/applications" "$DEB/DEBIAN"
ln -s /opt/exitlagfree/ExitLagFree "$DEB/usr/bin/exitlagfree"
desktop_entry /opt/exitlagfree/ExitLagFree > "$DEB/usr/share/applications/exitlagfree.desktop"
cat > "$DEB/DEBIAN/control" <<EOF
Package: exitlagfree
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Depends: libgtk-3-0, libwebkit2gtk-4.1-0 | libwebkit2gtk-4.0-37, policykit-1 | pkexec
Maintainer: ExitLagFree contributors
Description: Self-hosted game traffic relay client
 Routes selected game traffic through your own VPS over WireGuard.
EOF
cat > "$DEB/DEBIAN/prerm" <<'EOF'
#!/bin/sh
[ -x /usr/local/lib/exitlagfree/exitlag-helper ] && /usr/local/lib/exitlagfree/exitlag-helper uninstall || true
EOF
chmod 755 "$DEB/DEBIAN/prerm"
dpkg-deb --root-owner-group --build "$DEB" "$OUT/exitlagfree_${VERSION}_${ARCH}.deb"
rm -rf "$DEB"

# AppImage
APPDIR="$(mktemp -d)/ExitLagFree.AppDir"
install -Dm755 "$APP_BIN" "$APPDIR/usr/bin/ExitLagFree"
install -Dm755 "$HELPER_BIN" "$APPDIR/usr/bin/exitlag-helper"
install -Dm644 "$ICON" "$APPDIR/exitlagfree.png"
desktop_entry ExitLagFree > "$APPDIR/exitlagfree.desktop"
cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/ExitLagFree" "$@"
EOF
chmod 755 "$APPDIR/AppRun"
TOOL="$(mktemp -d)/appimagetool"
curl -fsSL -o "$TOOL" https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage
chmod +x "$TOOL"
ARCH=x86_64 APPIMAGE_EXTRACT_AND_RUN=1 "$TOOL" "$APPDIR" "$OUT/ExitLagFree-${VERSION}-x86_64.AppImage"
