# PHP en Lair

Lair instala PHP sin privilegios bajo `<root>/php/<minor>/`, donde `<root>` es
`LAIR_HOME`, `XDG_DATA_HOME/lair`, `~/.local/share/lair`, `%AppData%\Lair` en
Windows o `~/Library/Application Support/Lair` en macOS. Solo se publica una
release por minor (8.1–8.5) y las versiones conviven.

## Instalación y Actualización

```sh
lair php install 8.4          # última release publicada del minor
lair php list
lair php use 8.4              # default del sistema
lair php uninstall 8.2        # nunca permite quitar la default
```

El pipeline es: descarga del catálogo firmado → SHA-256 → extracción en
*staging* con protección contra `path traversal` → ejecución de `bin/php -v`
para comprobar que el paquete contiene realmente la versión anunciada →
`rename` atómico. Una versión incorrecta o un SHA alterado no dejan residuos.

Antes del swap se restauran los archivos del usuario (`php.ini`, `conf.d/`,
`conf.d-cli/`, `conf.d-xdebug/`, `lair.json`). **Actualizar conserva
`php.ini`**; desinstalar borra el minor completo.

Instalación local (sin catálogo, se detecta la versión ejecutando el binario):

```sh
lair php install 8.3 --from ./php-build.tar.gz
lair php install --from ./php-directory
```

## Configuración

- `php.ini`: se crea una sola vez y es del usuario (512M, 60 s, errores
  development, `output_buffering=1048576`, `variables_order=EGPCS`, opcache y
  el bundle de CA en `openssl.cafile`/`curl.cainfo`).
- `conf.d/00-lair.ini`: regenerado por Lair; contiene los defaults de la app y,
  en builds con extensiones compartidas, `extension_dir` y las `extension=`
  correspondientes. Nunca sobrescribe lo que definiste en `php.ini`.
- `conf.d-cli` (pcov) y `conf.d-xdebug` (Xdebug) se añaden al
  `PHP_INI_SCAN_DIR` solo en los contextos oportunos.
- Entorno al ejecutar: `PHPRC`, `PHP_INI_SCAN_DIR` (`;` en Windows, `:` en
  Unix) y `OPENSSL_CONF` en Windows.

## Servidor

Unix usa PHP-FPM con un socket por sitio en `run/` y un `php-fpm.conf` por
minor. Windows usa `php-cgi.exe` con seis workers por versión en puertos
loopback derivados del minor (más tres workers de debug en `puerto+1000`).

## Extensiones

```sh
lair ext                       # ext-* en composer.json y composer.lock
lair ext add redis             # ejecuta composer require ext-redis:*
```

En Windows cada minor necesita un pack `php-exts` firmado: DLLs en `ext/`,
librerías junto a los ejecutables, licencias en `LICENSES/` y
`bin/openssl.cnf`. Un minor sin pack no se ofrece. Windows además requiere el
[VC++ Redistributable x64](https://aka.ms/vs/17/release/vc_redist.x64.exe);
Lair avisa con el enlace si falta.
