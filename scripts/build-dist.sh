#!/usr/bin/env bash
# Construye todos los artefactos publicables de Lair: binarios, comprimidos,
# paquetes .deb (con lanzador .desktop e icono) y SHA256SUMS.
# Uso: scripts/build-dist.sh [versión]   (por defecto 1.0.0)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-1.0.0}"
# Prefer the notes for this release; fall back to the general changelog.
NOTES="RELEASE_NOTES.md"
if [ -f "RELEASE_NOTES_${VERSION}.md" ]; then
  NOTES="RELEASE_NOTES_${VERSION}.md"
fi
GO="${GO:-go}"
DIST="$ROOT/dist"
LDFLAGS="-s -w"

mkdir -p "$DIST"
export CGO_ENABLED=0

echo "==> comprobando herramientas"
command -v "$GO" >/dev/null || { echo "ERROR: go no encontrado" >&2; exit 1; }
command -v tar >/dev/null || { echo "ERROR: tar no encontrado" >&2; exit 1; }
command -v zip >/dev/null || { echo "ERROR: zip no encontrado" >&2; exit 1; }
HAVE_DEB=0
if command -v dpkg-deb >/dev/null 2>&1; then HAVE_DEB=1; else
  echo "aviso: dpkg-deb no disponible; no se generarán los .deb" >&2
fi

# La app de escritorio se empaqueta dentro del .deb para que un solo paquete
# abra la interfaz. Se genera con:
#   cd app && npm ci && npm run dist                    (amd64)
#   cd app && npx electron-builder --linux --arm64      (arm64)
# Cada arquitectura debe llevar su propio binario Electron.
desktop_dir() {
  case "$1" in
    amd64) printf '%s' "$ROOT/app/release/linux-unpacked" ;;
    arm64) printf '%s' "$ROOT/app/release/linux-arm64-unpacked" ;;
  esac
}

build_binary() {
  local os="$1" arch="$2" out="$3"
  echo "==> go build $os/$arch -> $(basename "$out")"
  GOOS="$os" GOARCH="$arch" "$GO" build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/lair
}

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

for os in linux darwin; do
  for arch in amd64 arm64; do
    bin="$DIST/lair-$os-$arch"
    [ "$os" = windows ] && bin="$bin.exe" || true
    build_binary "$os" "$arch" "$bin"
    pkg="$STAGE/$os-$arch"
    mkdir -p "$pkg"
    cp "$bin" "$pkg/lair"
    cp README.md "$pkg/"
    cp "$NOTES" "$pkg/RELEASE_NOTES.md"
    tar -czf "$DIST/lair_${VERSION}_${os}_${arch}.tar.gz" -C "$pkg" lair README.md RELEASE_NOTES.md
  done
done

for arch in amd64 arm64; do
  bin="$DIST/lair-windows-$arch.exe"
  build_binary windows "$arch" "$bin"
  pkg="$STAGE/windows-$arch"
  mkdir -p "$pkg"
  cp "$bin" "$pkg/lair.exe"
  cp README.md "$pkg/"
  cp "$NOTES" "$pkg/RELEASE_NOTES.md"
  (cd "$pkg" && zip -q "$DIST/lair_${VERSION}_windows_${arch}.zip" lair.exe README.md RELEASE_NOTES.md)
done

if [ "$HAVE_DEB" = 1 ]; then
  for arch in amd64 arm64; do
    root="$STAGE/deb-$arch"
    mkdir -p "$root/DEBIAN" "$root/usr/bin" "$root/usr/share/doc/lair" \
      "$root/usr/share/applications" "$root/usr/share/icons/hicolor/scalable/apps"
    install -m 0755 "$DIST/lair-linux-$arch" "$root/usr/bin/lair"
    install -m 0644 README.md "$root/usr/share/doc/lair/"
    install -m 0644 "$NOTES" "$root/usr/share/doc/lair/RELEASE_NOTES.md"
    cp LICENSE "$root/usr/share/doc/lair/copyright"
    chmod 0644 "$root/usr/share/doc/lair/copyright"
    install -m 0644 assets/lair.desktop "$root/usr/share/applications/lair.desktop"
    install -m 0644 assets/lair.svg "$root/usr/share/icons/hicolor/scalable/apps/lair.svg"
    install -m 0755 scripts/lair-desktop "$root/usr/bin/lair-desktop"
    desktop_unpacked="$(desktop_dir "$arch")"
    if [ -x "$desktop_unpacked/lair-desktop" ]; then
      mkdir -p "$root/usr/share/lair/desktop"
      cp -a "$desktop_unpacked/." "$root/usr/share/lair/desktop/"
      echo "==> app Electron $arch incluida en el paquete"
    else
      echo "aviso: falta Electron $arch en $desktop_unpacked; el .deb no incluirá la interfaz" >&2
    fi
    cat > "$root/DEBIAN/control" <<EOF
Package: lair
Version: $VERSION
Section: devel
Priority: optional
Architecture: $arch
Maintainer: LC-jhony <jhonyapm94@gmail.com>
Homepage: https://github.com/LC-jhony/lair
Description: native PHP/Laravel local development environment
 Lair is a single static Go binary providing a CLI, a local daemon, a native
 desktop app and shims for running PHP 8.1-8.5, Composer and Node natively,
 without Docker or VMs. It includes local DNS for *.test, HTTPS with a local
 CA, managed services and reversible PATH integration.
EOF
    dpkg-deb --root-owner-group --build "$root" "$DIST/lair_${VERSION}_${arch}.deb" >/dev/null
    echo "==> generado lair_${VERSION}_${arch}.deb"
  done
fi

echo "==> generando SHA256SUMS"
ASSETS=(
  "lair_${VERSION}_darwin_amd64.tar.gz"
  "lair_${VERSION}_darwin_arm64.tar.gz"
  "lair_${VERSION}_linux_amd64.tar.gz"
  "lair_${VERSION}_linux_arm64.tar.gz"
  "lair_${VERSION}_windows_amd64.zip"
  "lair_${VERSION}_windows_arm64.zip"
)
if [ "$HAVE_DEB" = 1 ]; then
  ASSETS+=("lair_${VERSION}_amd64.deb" "lair_${VERSION}_arm64.deb")
fi

# Fallar en cuanto falte un artefacto esperado: nunca publicar un SHA256SUMS
# incompleto (antes se silenciaban los errores con `2>/dev/null || true`).
for asset in "${ASSETS[@]}"; do
  [ -f "$DIST/$asset" ] || { echo "ERROR: falta el artefacto $asset" >&2; exit 1; }
done

(
  cd "$DIST"
  rm -f SHA256SUMS
  sha256sum "${ASSETS[@]}"
) > "$DIST/SHA256SUMS.new"
mv "$DIST/SHA256SUMS.new" "$DIST/SHA256SUMS"

echo "==> verificación final de SHA256SUMS"
(cd "$DIST" && sha256sum -c SHA256SUMS) || { echo "ERROR: verificación de hashes fallida" >&2; exit 1; }
lines="$(wc -l < "$DIST/SHA256SUMS" | tr -d ' ')"
[ "$lines" -eq "${#ASSETS[@]}" ] || {
  echo "ERROR: SHA256SUMS tiene $lines líneas; se esperaban ${#ASSETS[@]}" >&2
  exit 1
}
echo "==> $lines artefactos verificados correctamente"

echo "Listo. Artefactos en $DIST"
