# Referencia de CLI

La CLI detecta automáticamente el daemon: si `run/api.sock` responde y `run/api.token` es legible, usa la API; en caso contrario usa el mismo `engine` en proceso. Todos los comandos funcionan offline salvo los que necesitan descargas.

| Comando | Descripción |
|---|---|
| `lair install` | Bootstrap local (Composer verificado, shims, perfiles, CA), instalación de MariaDB por defecto y generación de los scripts privilegiados. Sin catálogo/red avisa, pero no aborta. |
| `lair uninstall [--delete-data]` | Elimina PHP, Composer, Node, todos los servicios (MariaDB incluido), shims, PATH y CA local; genera el script de reversión. Las bases de datos se conservan salvo `--delete-data`. No toca nginx, Laravel Valet ni Mailcatcher (no los instala Lair). |
| `lair status [--json]` | Estado de PHP, Node, servicios, sitios, jobs y defaults. |
| `lair doctor` | Comprueba si `<LAIR_HOME>/bin` está en PATH, qué shims faltan y qué versiones hay instaladas. Imprime el `export PATH=...` necesario. |
| `lair restart` | Reinicia todos los servicios supervisados activos. |
| `lair update` | Fuerza el refresco del catálogo firmado. |
| `lair php install [version]` | Instala una línea minor (p. ej. `8.4`) o una versión exacta. |
| `lair php list` | Versiones instaladas; `*` marca la default. |
| `lair php use <minor>` | Cambia la default. |
| `lair node install [version]` / `list` / `default <version>` | Equivalente para Node. Sin versión (o `lts`) instala el último LTS firmado: `v26.13.1` (npm `11.20.0`). |
| `lair services [list]` | Lista servicios, versión activa, puerto y estado. |
| `lair services install <id> [ver] [--port N]` | Descarga e inicializa una versión. No cambia la activa si ya hay una. |
| `lair services use <id> <version>` | Cambia la versión activa (los datos se conservan). |
| `lair services update <id> [ver]` | Instala, activa y reinicia. |
| `lair services uninstall <id> [ver] [--delete-data]` | Elimina la instalación; datos preservados por defecto. |
| `lair services create-db <id> <name>` | Ejecuta el comando de creación de BD del servicio. |
| `lair services default mariadb\|mysql\|postgres` | Fija la base de datos predeterminada (MariaDB en una instalación nueva). |
| `lair services restart\|stop <id>` | Controla el supervisor de un servicio. |
| `lair park` | Registra el directorio actual como `<nombre>.test`. |
| `lair link <name> [path]` | Registra un sitio con host explícito. |
| `lair unlink <name>` | Quita un sitio del router sin tocar los archivos. |
| `lair secure [name]` | Emite certificado local para el sitio (y su wildcard). |
| `lair pin php\|node <version>` | Escribe `.lairrc` en el proyecto (versionable). Los proyectos con `.flockrc` siguen funcionando y se migran al pinar. |
| `lair isolate php\|node <version\|--off>` | Fija una versión para un sitio aparcado (config.json, prioridad máxima). |
| `lair logs [name]` | Muestra un log o lista los disponibles. |
| `lair ui` | Abre el panel web embebido en el navegador. Arranca el daemon si hace falta y lee la URL tokenizada de `run/ui.url`. |
| `lair tui` / `lair dashboard` | Dashboard interactivo con Charmbracelet (Bubble Tea / Lip Gloss). Fuera de un TTY imprime el estado en texto plano y sale. |
| `lair daemon --foreground` | Ejecuta el daemon en primer plano. Flags: `--no-web`, `--no-dns`, `--no-api`, `--no-ui`, `--ui-port N`, `--dns-port`, `--http-port`, `--https-port`, `--domain`. |

## Panel web embebido

El daemon sirve una SPA en `127.0.0.1:9797` (configurable con `--ui-port N`). No es una aplicación Electron: el HTML, el CSS y el JS viajan dentro del binario y el lanzador `.desktop` solo ejecuta `lair ui`.

- Autenticación: la URL contiene `#token=<hex>`. La SPA mueve el token a `sessionStorage` y lo envía como `X-Lair-Token` en cada `POST /api`. El bridge rechaza peticiones sin token con `401` y usa comparación en tiempo constante.
- Superficie: panel de estado, instalación/activación de PHP y Node, vista **Bases de datos** (instalar MariaDB/MySQL/PostgreSQL, crear BD, elegir predeterminada, usar versión, reiniciar/parar), sitios (`park`, `link`, `unlink`, `secure`, `pin`, `isolate`), servicios, logs, actualización del catálogo, `doctor`, reinicio, apagado y desinstalación.
- Archivos: `run/ui.token` y `run/ui.url`, ambos `0600` dentro de `run/` (`0700`). Se eliminan al apagar el daemon.
- El panel y la CLI comparten el mismo `Dispatcher`, por lo que no hay acciones "de UI" distintas: cualquier cambio se refleja en `lair status`.

Si el daemon está activo pero se inició con `--no-ui`, `lair ui` avisa en lugar de abrir una URL obsoleta. Para cambiar el puerto: reinicia el daemon con `--ui-port N`; el lanzador lee siempre `run/ui.url`, así que no necesita configuración adicional.

## Comando oculto `exec`

Los shims de `bin/` contienen:

```sh
exec "<lair>" exec <tool> "$@"
```

`lair exec` resuelve la versión según `isolated > .lairrc (legacy .flockrc) > (.nvmrc/.node-version) > default > newest`, construye el entorno y usa `execve` para reemplazar el proceso. Si falta la versión y stdin es un TTY, ofrece instalarla; si no hay TTY (p. ej. desde un script), falla con un mensaje claro.

Variables de entorno relevantes:

- `LAIR_HOME`: override del directorio de datos.
- `XDG_DATA_HOME`: base alternativa (`$XDG_DATA_HOME/lair`).
- `LAIR_MANIFEST_BASE_URLS`: lista de mirrors separada por comas. El primero, por defecto, es `https://raw.githubusercontent.com/lair-sh/lair/main/catalog`; el binario trae además un catálogo firmado embebido con Node LTS `v26.13.1` y MariaDB `11.4.7` (ejemplo de desarrollo, **no un feed de producción**).
- `LAIR_COMPOSER_DOWNLOAD_URL` / `LAIR_COMPOSER_CHECKSUM_URL`: URLs alternativas para `composer.phar` y su checksum (el SHA-256 se sigue verificando).

## Shims y PATH

`lair install` genera los shims en `<LAIR_HOME>/bin` y añade ese directorio al PATH antes de cualquier descarga. Cualquier comando `lair ... install` vuelve a generarlos al terminar. Si `node -v` no se reconoce, ejecuta `lair doctor`: indicará si faltan shims o si el directorio no está en PATH, y mostrará cómo corregirlo sin reiniciar la terminal.
