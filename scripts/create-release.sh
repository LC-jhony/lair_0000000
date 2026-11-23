#!/usr/bin/env bash
# Crea un release de Lair en LC-jhony/lair con todos los campos y assets.
# Uso: scripts/create-release.sh [versión]   (por defecto 1.0.0)
# Requiere GitHub CLI autenticado: gh auth login (o GITHUB_TOKEN en el entorno).
# Con SKIP_BUILD=1 no reconstruye dist/ y usa los artefactos existentes.
set -euo pipefail

readonly REPO="LC-jhony/lair"
readonly VERSION="${1:-1.0.0}"
readonly TAG="v$VERSION"
readonly TITLE="Lair v$VERSION"
readonly NOTES="RELEASE_NOTES_${VERSION}.md"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [ "${SKIP_BUILD:-0}" != 1 ]; then
  echo "==> Construyendo artefactos $VERSION"
  scripts/build-dist.sh "$VERSION"
fi
NOTES_FILE="$NOTES"
if [ ! -f "$NOTES_FILE" ]; then
  NOTES_FILE="RELEASE_NOTES.md"
fi

ASSETS=(
  "dist/SHA256SUMS"
  "dist/lair_${VERSION}_amd64.deb"
  "dist/lair_${VERSION}_arm64.deb"
  "dist/lair_${VERSION}_darwin_amd64.tar.gz"
  "dist/lair_${VERSION}_darwin_arm64.tar.gz"
  "dist/lair_${VERSION}_linux_amd64.tar.gz"
  "dist/lair_${VERSION}_linux_arm64.tar.gz"
  "dist/lair_${VERSION}_windows_amd64.zip"
  "dist/lair_${VERSION}_windows_arm64.zip"
)

if ! command -v gh >/dev/null 2>&1; then
  echo "ERROR: se necesita GitHub CLI (gh). Instálalo y ejecuta 'gh auth login'." >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "ERROR: GitHub CLI no está autenticado. Ejecuta 'gh auth login' o exporta GITHUB_TOKEN." >&2
  exit 1
fi

if [ "$(git rev-parse "$TAG^{commit}")" != "$(git rev-parse HEAD)" ]; then
  echo "ERROR: el tag $TAG no apunta a HEAD. Recrea el tag antes de publicar." >&2
  exit 1
fi

for asset in "${ASSETS[@]}"; do
  if [ ! -f "$asset" ]; then
    echo "ERROR: falta el asset $asset" >&2
    exit 1
  fi
done

echo "==> Verificando hashes de los assets"
(cd dist && sha256sum -c SHA256SUMS)

echo "==> Subiendo rama y tag a $REPO"
git push origin main
git push origin "$TAG"

if gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  echo "==> El release ya existe: actualizando notas y reemplazando assets"
  gh release edit "$TAG" --repo "$REPO" --title "$TITLE" --notes-file "$NOTES_FILE"
  for asset in "${ASSETS[@]}"; do
    name="$(basename "$asset")"
    if gh release view "$TAG" --repo "$REPO" --json assets --jq ".assets[].name" | grep -qx "$name"; then
      gh release delete-asset "$TAG" "$name" --repo "$REPO" --yes
    fi
  done
  gh release upload "$TAG" "${ASSETS[@]}" --repo "$REPO" --clobber
else
  echo "==> Creando el release $TAG"
  gh release create "$TAG" \
    --repo "$REPO" \
    --title "$TITLE" \
    --notes-file "$NOTES_FILE" \
    --verify-tag \
    "${ASSETS[@]}"
fi

echo "Listo: https://github.com/$REPO/releases/tag/$TAG"
