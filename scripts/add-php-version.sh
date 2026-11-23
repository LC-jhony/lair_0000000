#!/bin/sh
# Publica una nueva versión de PHP en el catálogo firmado de Lair.
#
# Uso:
#   scripts/add-php-version.sh <version> <plataforma> <url-cli> <url-fpm> [clave]
#
# Ejemplo:
#   scripts/add-php-version.sh 8.6.0 linux-x64 \
#     https://dl.static-php.dev/static-php-cli/common/php-8.6.0-cli-linux-x86_64.tar.gz \
#     https://dl.static-php.dev/static-php-cli/common/php-8.6.0-fpm-linux-x86_64.tar.gz
#
# La clave por defecto es keys/private.ed25519 (raw de 64 bytes, solo desarrollo).
# En CI usa el secreto LAIR_SIGNING_KEY. Las URLs DEBEN existir: el script
# descarga cada artefacto, calcula SHA-256 y tamaño, y se niega a publicar
# entradas que no se puedan verificar.
set -eu

version="${1:-}"
platform="${2:-}"
cli_url="${3:-}"
fpm_url="${4:-}"
key="${5:-keys/private.ed25519}"

if [ -z "$version" ] || [ -z "$platform" ] || [ -z "$cli_url" ] || [ -z "$fpm_url" ]; then
  echo "usage: $0 <version> <platform> <cli-url> <fpm-url> [key]" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

python3 - "$version" "$platform" "$cli_url" "$fpm_url" "$tmp" <<'PY'
import hashlib, json, sys, urllib.request
version, platform, cli_url, fpm_url, tmp = sys.argv[1:6]

def fetch(url, name):
    req = urllib.request.Request(url, headers={'User-Agent': 'lair-catalog-publisher'})
    with urllib.request.urlopen(req, timeout=120) as r:
        body = r.read()
    if len(body) < 1024:
        raise SystemExit(f'{url}: artefacto demasiado pequeño')
    path = f'{tmp}/{name}'
    open(path, 'wb').write(body)
    return hashlib.sha256(body).hexdigest(), len(body)

cli_sha, cli_size = fetch(cli_url, 'cli')
fpm_sha, fpm_size = fetch(fpm_url, 'fpm')

for path in ('catalog/manifest.json', 'internal/assets/catalog.json'):
    with open(path, encoding='utf-8') as f:
        manifest = json.load(f)
    php = manifest['packages'].setdefault('php', {}).setdefault(version, {})
    php[platform] = {
        'build': 'N1',
        'url': cli_url,
        'sha256': cli_sha,
        'size': cli_size,
        'published': __import__('datetime').datetime.now(__import__('datetime').timezone.utc)
                     .isoformat().replace('+00:00', 'Z'),
        'prerelease': any(x in version.lower() for x in ('rc', 'beta', 'alpha', 'dev')),
        'files': [{'from': 'php', 'to': 'bin/php'}],
        'components': [{
            'url': fpm_url, 'sha256': fpm_sha, 'size': fpm_size,
            'files': [{'from': 'php-fpm', 'to': 'sbin/php-fpm'}],
        }],
    }
    manifest['serial'] = int(manifest.get('serial', 0)) + 1
    with open(path, 'w', encoding='utf-8') as f:
        json.dump(manifest, f, indent=2, ensure_ascii=False)
        f.write('\n')
    print(f'{path}: {version}/{platform} sha={cli_sha[:12]} size={cli_size}')
PY

python3 - "$key" <<'PY'
import base64, sys
raw = open(sys.argv[1], 'rb').read()
assert len(raw) == 64, 'la clave debe tener 64 bytes'
der = bytes.fromhex('302e020100300506032b657004220420') + raw[:32]
pem = '-----BEGIN PRIVATE KEY-----\n' + base64.encodebytes(der).decode() + '-----END PRIVATE KEY-----\n'
open('/tmp/lair-catalog-key.pem', 'w').write(pem)
PY

for pair in "catalog/manifest.json catalog/manifest.json.sig" \
            "internal/assets/catalog.json internal/assets/catalog.json.sig"; do
  set -- $pair
  openssl pkeyutl -sign -inkey /tmp/lair-catalog-key.pem -rawin -in "$1" -out "$tmp/sig.bin"
  base64 -w0 "$tmp/sig.bin" > "$2"
  printf '\n' >> "$2"
done
echo "catálogo actualizado y firmado; serial=$(python3 -c "import json;print(json.load(open('$root/catalog/manifest.json'))['serial'])")"
