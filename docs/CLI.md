# Referencia de CLI

La CLI detecta automáticamente el daemon: si `run/api.sock` responde y `run/api.token` es legible, usa la API; en caso contrario usa el mismo `engine` en proceso. Todos los comandos funcionan offline salvo los que necesitan descargas.

| Comando | Descripción |
|---|---|
| `lair install` | Bootstrap local (Composer verificado, shims, perfiles, CA), instalación inicial de PHP, Node LTS y SQLite con progreso, y generación de los scripts privilegiados (DNS/host virtual) para revisión. Sin catálogo/red avisa, pero no aborta. |
| `lair uninstall [--delete-data]` | Elimina PHP, Composer, Node, todos los servicios (MariaDB incluido), shims, PATH y CA local; genera el script de reversión. Las bases de datos se conservan salvo `--delete-data`. No instala ni elimina paquetes del sistema; solo limpia la configuración y los datos generados bajo `$LAIR_HOME`. |
| `lair status [--json]` | Estado de PHP, Node, servicios, sitios, jobs y defaults. |
| `lair doctor [--fix]` | Comprueba si `<LAIR_HOME>/bin` está en PATH, qué shims faltan y qué versiones hay instaladas. Con `--fix` regenera los shims y los fragmentos de PATH. |
| `lair restart` | Reinicia todos los servicios supervisados activos. |
| `lair update` | Fuerza el refresco del catálogo firmado. |
| `lair php install [version] [--from <dir\|.tar.gz\|.zip>]` | Instala una línea minor (p. ej. `8.4`) o una versión exacta; con `--from`, un build local detectado ejecutando el binario. |
| `lair php list` | Versiones instaladas; `*` marca la default. |
| `lair php use <minor>` / `lair use <minor>` | Cambia la default. |
| `lair php uninstall <minor>` | Quita un minor (nunca la default; conserva otros minor). |
| `lair node install [version] [--from <dir\|.tar.gz>]` / `list` / `default <major>` | Instala la última release publicada de la mayor (`22`, `22.11` y `22.11.0` resuelven igual). Sin versión instala la LTS más nueva del catálogo. |
| `lair node update <major>` / `uninstall <major>` | Actualiza a la última release de la mayor / elimina esa mayor. |
| `lair nvm install\|use\|ls\|current\|exec <versión> <comando>\|uninstall` | Node se instala y se gestiona **siempre a través de nvm**: `lair node install` llama a `lair nvm install <versión>`, que carga `~/.nvm/nvm.sh` con argv validado. Acepta `26`, `lts`, `lts/*`, `22.16.0` o `node`. `nvm exec` es usado por los shims `node`/`npm`/`npx`. Si nvm no está instalado, Lair avisa con la URL de instalación; no descarga scripts remotos por su cuenta. |
| `lair npm update` | Instala la última npm compatible con los `engines` del Node de la mayor por defecto. |
| `lair composer install\|update\|version` | Verifica/instala el phar fijado en el catálogo, lo re-descarga y verifica, o muestra su versión. |
| `lair ext [add <name>]` | Muestra los requisitos `ext-*` de `composer.json`/`composer.lock` o instala uno con el PHP del proyecto. |
| `lair job retry <id>` | Reintenta un job fallido con el mismo artefacto verificado. |
| Configuración de Lair (botón en Sitios) | Configura HTTPS, puertos, enlace y dominio del servidor Go que sirve `*.test` mediante la acción `web` del daemon; también muestra PHP/Node por defecto, carpetas de proyectos y estado de DNS/CA. Por defecto HTTP **80** y HTTPS **443**; para evitar `CAP_NET_BIND_SERVICE`/`setcap`, usa 8080/8443. |
| `lair project sync` | Vuelve a escanear las carpetas de proyectos y registra los sitios detectados. |
| `lair project migrate-dirs` (alias `lair migrate-dirs`) | Migra `~/lair` a `~/Lair` (mueve los proyectos y actualiza las rutas). No es automático: el arranque solo sincroniza las carpetas ya configuradas. |
| `lair project import-valet` (alias `lair import-valet`) | Importa en **solo lectura** `~/.valet/config.json` y `~/.valet/Sites`: añade las rutas a `project_dirs` y adopta el dominio; no modifica esos archivos ni instala/ejecuta Valet. Los sitios aparecen tras `lair project sync`. |
| `lair project info` | Muestra capacidades para crear proyectos: instalador Laravel, PHP, Composer, Node (nvm), drivers PDO, gestores Node, motores de BD y carpeta de proyectos. |
| `lair project dir [path]` | Muestra o fija la carpeta de proyectos por defecto. Por defecto `~/Lair` (**L mayúscula**); se rechaza cualquier ruta dentro de `$LAIR_HOME`. Atajo de `lair project dirs default`. |
| `lair project dirs [list\|add <path>\|remove <path>\|default <path>]` | Gestiona **ProjectDirs**: lista, añade, quita o fija la carpeta por defecto donde se escanean proyectos. |
| `lair migrate-dirs` | Migra `~/lair` a `~/Lair` de forma explícita; no se ejecuta automáticamente al arrancar. |
| `lair import-valet` | Importa en **solo lectura** la configuración de Valet (`project_dirs` + dominio), sin instalar ni ejecutar Valet. |
| `lair new <name> [options]` | Comando oficial para crear un proyecto Laravel; alias directo de `lair project new`. Sin `--parent` usa `~/Lair`. |
| `lair project new\|create <name> [options]` | Alias de compatibilidad de `lair new`. Crea un proyecto Laravel, prepara la BD, parchea `.env` y registra `<name>.test`. Sin `--parent` usa la carpeta configurada, nunca el directorio actual. Opciones: `--parent`, `--secure`, `--stack`, `--no-auth`, `--testing`, `--boost`, `--node-manager`, `--git`, `--database`, `--db-name`, `--db-user`, `--db-password`, `--db-host`, `--db-port`, `--no-create-db`, `--no-start`. La contraseña se pide sin eco si no se pasa (o `LAIR_DB_PASSWORD`). |
| `lair services [list]` (alias `lair service`) | Lista servicios, versión activa, puerto y estado. Válido también para el resto de subcomandos: `lair service install ...`, `lair service default ...`, etc. |
| `lair services install <id> [ver] [--port N]` | Descarga e inicializa una versión. No cambia la activa si ya hay una. |
| `lair services use <id> <version>` | Cambia la versión activa (los datos se conservan). |
| `lair services update <id> [ver]` | Instala, activa y reinicia. |
| `lair services uninstall <id> [ver] [--delete-data]` | Elimina la instalación; datos preservados por defecto. |
| `lair services create-db <id> <name>` | Ejecuta el comando de creación de BD del servicio. |
| `lair services default sqlite\|mariadb\|mysql\|postgres` | Fija la base de datos predeterminada (SQLite en una instalación nueva). |
| `lair services restart\|stop <id>` | Controla el supervisor de un servicio. |
| `lair park` | Registra el directorio actual como `<nombre>.test`. |
| `lair link <name> [path]` | Registra un sitio con host explícito. |
| `lair unlink <name>` | Quita un sitio del router sin tocar los archivos. Para borrar también la carpeta usa la acción `site` (`op: delete`, `delete_files: true`) o la tecla `X` de la TUI, con confirmación en la interfaz. |
| `lair secure [name]` | Emite certificado local para el sitio (y su wildcard). |
| `lair pin php\|node\|root <value>` | Escribe `.lairrc` en el proyecto (versionable). `root=` selecciona un data root alternativo. Los proyectos con `.flockrc` siguen funcionando y se migran al pinar. |
| `lair unpin php\|node\|root` | Quita el pin del proyecto sin borrar otras líneas. |
| `lair isolate php\|node <version\|--off>` / `unisolate` / `isolated` | Fija una versión para un sitio aparcado (config.json, prioridad máxima), la quita o la muestra. |
| `lair logs [name]` | Muestra un log o lista los disponibles. |
| `lair desktop` (alias `lair ui`) | Muestra los datos de conexión de la app nativa. Arranca el daemon si hace falta y lee `run/desktop.url` y `run/desktop.token`. |
| `lair tui` / `lair dashboard` | Dashboard interactivo. Usa el bundle **React/Ink + termcn** incrustado (diez panes, mismo i18n que la desktop) y cae al panel **Charm v2** (Bubble Tea, Bubbles, Lip Gloss, Huh, Glamour, Log y Harmonica; nueve panes) si Node 22+ no está disponible. `lair tui --help` muestra la ayuda markdown. Fuera de un TTY imprime el estado en texto plano y sale. |
| `lair daemon --foreground` (`-f`) | Ejecuta el daemon en primer plano. Flags: `--no-web`, `--no-dns`, `--no-api`, `--no-desktop`, `--desktop-port N`, `--dns-port`, `--http-port`, `--https-port`, `--domain` (alias: `--no-ui`, `--ui-port N`). Web escucha en 80/443 por defecto (usa `--http-port 8080 --https-port 8443` si no hay `CAP_NET_BIND_SERVICE`); DNS en `--dns-port 53535`. |

> La acción `web` del daemon expone las operaciones `status`, `configure`, `https-on` y `https-off`; la CLI, la desktop y la TUI son clientes de esa misma acción.

## ProjectDirs, migración e importación de Valet

El estado persistente incluye **`project_dirs`**, un **array** de carpetas escaneadas (la primera es la por defecto) y `projects_dir`, su vista derivada. **`DefaultProjectsDir` es `~/Lair`** (con L mayúscula), fuera de `$LAIR_HOME`:

```json
"project_dirs": ["/home/usuario/Lair", "/srv/otros-proyectos"],
"projects_dir": "/home/usuario/Lair"
```

- Si `~/Lair` no existe, Lair la crea y la registra como aparcada; sus proyectos aparecen en **Sites** como `<nombre>.test`. La página Sites no cambia.
- Si existe `~/lair`, usa `lair migrate-dirs` de forma explícita para mover los proyectos; el arranque del daemon solo sincroniza las carpetas configuradas. La carpeta antigua no se borra sin confirmación.
- `lair import-valet` lee `~/.valet/config.json` y `~/.valet/Sites` **en solo lectura**: añade esas rutas a `project_dirs` y adopta el dominio, sin escribir en esos archivos ni instalar/ejecutar Valet. Valet nunca se ejecuta como root; los sitios se registran tras `lair project sync`.

Estas operaciones son valores del campo `op` de la acción `project` del daemon (el inventario compartido tiene 49 acciones):

| `op` | Campos | Equivalente CLI |
| --- | --- | --- |
| `dirs-list` | — | `lair project dirs` / `lair project info` |
| `dirs-add` | `path` | `lair project dirs add <path>` |
| `dirs-remove` | `path` | `lair project dirs remove <path>` |
| `dirs-default` | `path` | `lair project dirs default <path>` / `lair project dir <path>` |
| `migrate-dirs` | — | `lair migrate-dirs` |
| `import-valet` | — | `lair import-valet` |

## Puertos y DNS

- **Web**: el servidor Go integrado enlaza `127.0.0.1:80` (HTTP) y `127.0.0.1:443` (HTTPS) por defecto. Para escuchar en puertos < 1024 sin root hace falta `CAP_NET_BIND_SERVICE` en la unidad `lair@<usuario>.service` o `sudo setcap 'cap_net_bind_service=+ep' "$(command -v lair)"`. Sin esa capacidad, usa Configuración de Lair en Sitios (8080/8443) o `--http-port`/`--https-port`. El daemon nunca corre como root.
- **DNS**: dnsmasq seguro en `127.0.0.1:53535` (nunca en el puerto 53 global) con drop-in de `systemd-resolved` (`DNS=127.0.0.1#53535`, `Domains=~test`). Solo `*.test` se reenvía al resolver local; si falla, hay **fallback automático** al DNS del sistema. El plan genera copia de seguridad de la configuración previa y `revert-privileged.sh` permite el rollback.
- **Verificación**:

```sh
resolvectl status | grep -A2 '~test'
ss -lunp | grep 53535
dig @127.0.0.1 -p 53535 mi-app.test
lair status && lair doctor
```

## Dashboard TUI

`lair tui` muestra un layout de dos paneles con bordes redondeados: navegación a la
izquierda y tarjetas de estado a la derecha. En la sección **Servicios**, cada
tarjeta tiene un interruptor; `enter` o `space` abre una confirmación (Huh) y
aplica `service-restart` para activar o `service-stop` para desactivar.

| Tecla | Acción |
| --- | --- |
| `1`–`9` / `tab` / `→` / `l` | Ir directo al pane / siguiente sección |
| `←` / `h` | Sección anterior |
| `↑` / `↓` | Seleccionar servicio |
| `enter` / `space` | Activar o desactivar el servicio seleccionado |
| `I` / `n` / `p` / `m` | Instalar servicio / Node LTS / PHP / MariaDB |
| `d` / `y` / `z` / `b` | Predeterminada / usar versión / desinstalar / crear base de datos |
| **`M` / `D` / `B`** | **Mail / Dumps / Debugger** (mayúsculas; no confundir con `m`/`d`/`b`) |
| `v` / `x` | Ver registros / cancelar trabajo |
| `a` / `e` / `o` / `c` / `C` / `g` / `i` | Aparcar / enlazar / desvincular / certificado / nuevo proyecto Laravel / fijar / aislar |
| **`E`** / **`X`** | **Detalle de sitio** (Overview + Environment, vía `site`) / **borrar sitio** con confirmación de carpeta |
| **`V`** / **`t`** | **Ver el vhost generado** del sitio seleccionado (`op: config`) / **alternar enabled/disabled** desde el detalle `E` (`op: set-enabled`, con confirmación) |
| **`F`** | **Project dirs**: listar/añadir/quitar/fijar carpetas de proyectos (`~/Lair` por defecto) y ejecutar `migrate-dirs` o `import-valet` (acciones `project`) |
| `u` / `s` / `r` | Catálogo / reiniciar servicios / refrescar |
| `?` | Ayuda markdown renderizada con Glamour |
| `q` / `esc` / `ctrl+c` | Salir |

Los panes del bundle React/Ink son **Resumen**, Runtimes, Bases de datos, Servicios, Sitios, **Correo**, **Dumps**, **Depurador**, **Logs** y **Sistema**. El fallback de Charmbracelet tiene **nueve**: no incluye Logs, y `v` consulta los registros desde cualquier pane. Al igual que en la desktop, son panes del sidebar: no hay ventanas flotantes ni bandeja nativa. `1`–`9` salta al pane y `I` abre la instalación de servicios; el pane **Logs** añade el filtro por `sources`. En el pane **Sitios**, la tecla **`E`** (mayúscula) abre un subconjunto del detalle de la desktop —Overview (URL, rutas, PHP, Node, BD, HTTPS, estado de `root`, dominios, avisos) y Environment (valores secretos enmascarados)— reutilizando la acción `site`; `v` abre los registros del sitio. No se confunde con `e` (enlazar sitio). La tecla **`X`** pide confirmación para desvincular el sitio y permite borrar también su carpeta (`delete_files: true`). La tecla **`V`** (mayúscula, distinta de `v` = registros) muestra el **virtual host generado** para el sitio seleccionado, con el mismo contenido que el botón **View generated vhost** del Overview en la desktop (`op: config`). La tecla **`t`** (solo desde el detalle `E`) alterna el estado **enabled/disabled** del sitio (con confirmación) llamando a `site` con `op: set-enabled`: un sitio desactivado no genera vhost y se retira del router web Go. La tecla **`F`** abre la gestión de **ProjectDirs** (carpetas de proyectos, `~/Lair` por defecto, con `migrate-dirs` e `import-valet`), sin alterar la página Sites.

La app desktop expone exactamente las mismas acciones; el cliente HTTP declara la
misma lista en `app/src/api/client.ts` y `scripts/check-actions.mjs` verifica que
cada acción exista en `internal/daemon/protocol.go`.

La ayuda completa vive en `internal/cli/tui_help.md` y puede verse fuera de la TUI
con `glow internal/cli/tui_help.md`. Las acciones se registran en
`<LAIR_HOME>/log/tui.log`. Para grabar una demo: `vhs scripts/tui-demo.tape`.

## Acciones de soporte: Mail, Dumps, Debugger y Logs

Estas operaciones son **acciones del daemon** y la CLI las expone como subcomandos reales: `lair mail ...`, `lair dumps ...` y `lair debugger ...`. El sobre `{action, payload}` sigue disponible para el bridge `POST /api` y el `Caller` en modo offline, pero no hace falta construirlo a mano.

| Acción | CLI | `op` del payload | Descripción |
|---|---|---|---|
| `mail` | `status\|start\|stop\|list\|open <id>\|raw <id>\|clear [inbox]\|env\|message <id>` | `status`, `start`, `stop`, `list`, `message`, `raw`, `clear`, `env` | Catcher SMTP integrado en `127.0.0.1:2525`. `list` devuelve los mensajes capturados; `env` imprime el bloque `.env`; `clear` requiere `confirm: true`. |
| `dumps` | `status\|capture-on\|capture-off\|list\|clear\|options\|ingest` | `status`, `capture-on`, `capture-off`, `list`, `clear`, `options`, `ingest` | Captura de `dump()`/`dd()` por `auto_prepend_file` en los pools FPM existentes. Acepta `site` opcional; `ingest` exige el token de `run/dumps.token`; `clear` requiere `confirm: true`. |
| `debugger` | `status\|detect-on\|detect-off\|breakpoints-on\|breakpoints-off` | `status`, `start`, `stop`, `enable`, `disable` | Xdebug bajo demanda reutilizando los pools FPM. Acepta `site` y `port` opcionales (9003 por defecto). |

Ejemplos de sobre enviado al bridge:

```json
{ "action": "mail", "payload": { "op": "status" } }
{ "action": "mail", "payload": { "op": "list" } }
{ "action": "mail", "payload": { "op": "clear", "confirm": true } }
{ "action": "dumps", "payload": { "op": "capture-on", "site": "mi-app.test" } }
{ "action": "dumps", "payload": { "op": "status" } }
{ "action": "dumps", "payload": { "op": "clear", "confirm": true } }
{ "action": "debugger", "payload": { "op": "status" } }
{ "action": "debugger", "payload": { "op": "start", "site": "mi-app.test", "port": 9003 } }
```

`logs-list` se amplió con `sources`: el payload puede filtrar por origen y la respuesta incluye el origen de cada entrada. El contenido se lee con la acción `logs` de siempre.

```json
{ "action": "logs-list", "payload": { "sources": ["php", "web", "services", "mail", "dumps", "debugger"] } }
```

```json
{ "items": [ { "name": "php-8.4.log", "source": "php" }, { "name": "mail.log", "source": "mail" } ] }
```

Los nombres de origen de los ejemplos son ilustrativos; la lista autoritativa de acciones y payloads es `internal/daemon/protocol.go`.

También están disponibles las formas abreviadas `lair logs [nombre]` para leer un registro y el filtro por `sources` desde el pane Logs de la TUI (`v`).

## Acción `site`: detalle avanzado de sitios

La página **Sites** de la desktop abre un detalle con **siete pestañas**: **General/Overview** (estado `enabled` con toggle, raíz detectada y su página de estado, dominios/alias, HTTPS, avisos y botón **View generated vhost**), **Entorno** (`.env` enmascarado y editable), **Base de datos**, **Correo** (SMTP del catcher), **Colas** (estado de servicios), **PHP y Node** (pin/isolate) y **Registros** (logs del sitio). Los puertos, el enlace y el dominio del servidor se consultan en el pane **Web** (acción `web`). Todo el detalle se sirve con una única acción del daemon, **`site`**, con el sobre `{ "action", "payload" }` del bridge `POST /api` (o el `Caller` en modo offline). El campo `op` selecciona la operación:

```json
{ "action": "site", "payload": { "op": "detail", "name": "mi-app.test" } }
{ "action": "site", "payload": { "op": "env", "name": "mi-app.test" } }
{ "action": "site", "payload": { "op": "env-set", "name": "mi-app.test", "key": "APP_ENV", "value": "local" } }
{ "action": "site", "payload": { "op": "domains", "name": "mi-app.test", "domains": ["www.mi-app.test", "mi-app.local"] } }
{ "action": "site", "payload": { "op": "set-enabled", "name": "mi-app.test", "enabled": false } }
{ "action": "site", "payload": { "op": "config", "name": "mi-app.test" } }
{ "action": "site", "payload": { "op": "delete", "name": "mi-app.test", "delete_files": true } }
```

| `op` | Payload | Respuesta |
| --- | --- | --- |
| `detail` (por defecto) | `name` | `detail`: `SiteDetail` con la vista completa del sitio, sin secretos. |
| `env` | `name` | `env`: entradas de `.env` (`key`, `value`, `secret`, `set`) con secretos enmascarados. |
| `env-set` | `name`, `key`, `value` | `output`: solo el nombre de la clave; el valor nunca se registra. |
| `domains` | `name`, `domains` | `domains`: lista normalizada; actualiza `config.json` y recarga sitios/web. |
| `set-enabled` | `name`, `enabled` | `detail`: vista actualizada con el nuevo `enabled`; aplica sitios/web. Con `enabled: false` se retira el host virtual del router. |
| `config` | `name` | `config`: texto del vhost generado (lo que muestra la TUI con `V`); vacío si el sitio está desactivado. |
| `delete` / `unlink` | `name`, `delete_files` | `output`: con `delete_files: true` borra también la carpeta. |

La fuente autoritativa del contrato es `SiteRequest` / `SiteResult` en `internal/engine/site.go`, reexportada por `internal/daemon/protocol.go`. `SiteDetail` expone `enabled` y `root_state`; el router web Go usa esos campos para decidir si publica el host virtual o sirve **`not-configured.html`** (página genérica: host y ruta esperada del front controller dentro del proyecto, sin rutas de `LAIR_HOME` ni secretos) en lugar de pasar la petición a PHP-FPM, y `op: config` devuelve la definición de ese host virtual. El borrado de archivos valida la ruta registrada (absoluta, fuera de `LAIR_HOME`, sin enlaces simbólicos) y exige confirmación en la interfaz (TUI `X`); `lair unlink` sigue siendo no destructivo. La TUI expone un subconjunto con **`E`** (Overview + Environment), **`V`** (vhost generado) y el toggle de enabled/disabled. Los hosts virtuales se generan en el servidor Go integrado a partir de `config.json`, sin archivos de configuración externos ni enlaces en `/etc` (decisión registrada en el [ADR 009](adr/009-virtual-hosts-en-servidor-go.md)). La CLI no publica un subcomando `lair site`, se verifica con el bridge `POST /api` o el `Caller` en modo offline.

## Bridge de la app nativa

No se sirve ninguna SPA. El daemon publica `POST /api` en `127.0.0.1:9797` (configurable con `--desktop-port N`) para la app Electron + React de `app/`.

- Autenticación: cabecera `X-Lair-Token` (comparación en tiempo constante). Sin token responde `401`; los tokens en query ya no se aceptan. Solo se permiten orígenes HTTP(S) de `localhost`, `127.0.0.1` y `::1`; el origen `null` se rechaza. Electron llama desde el proceso principal, sin cabecera `Origin`.
- Superficie: el `Dispatcher` registra **49 acciones**; el cliente desktop declara 43, verificadas por `scripts/check-actions.mjs`. Incluye `status`, `versions`, `update`, `bootstrap`, `first-run`, `ensure-shims`, `install-*`, `job`/`jobs`/`job-cancel`/`job-retry`, `logs`/`logs-list` (con `sources`), `php-use`/`php-uninstall`, `node-default`/`node-update`/`node-uninstall`, `composer-update`, `ext-*`, `npm-update`, `isolated`/`unpin`/`unisolate`, `service-*`, `create-db`/`db-default`/`ensure-database`, `park`/`link`/`unlink`/`secure`/`web`/`project`/`pin`/`isolate`/`site`, `mail`/`dumps`/`debugger` y `uninstall`. Fuera del cliente quedan `shutdown` y alguna acción interna de CLI/daemon. La app no calcula estados: adopta lo que devuelve el daemon.
- Archivos: `run/desktop.token` y `run/desktop.url`, ambos `0600` dentro de `run/` (`0700`). Se eliminan al apagar el daemon.
- `window.lairDesktop.connect()` (IPC de Electron, `contextIsolation`) entrega la URL y el token al renderer sin exponer filesystem.
- El bridge se desactiva con `lair daemon --foreground --no-desktop`. Para cambiar el puerto: reinicia el daemon con `--desktop-port N`; la app lee siempre `run/desktop.url`.

Para desarrollar la UI sin daemon, abre la app con `?fake=1` o compila con `VITE_FAKE_DAEMON=1`: se usa `FakeDaemon` con fixtures de `app/src/api/fixtures/`.

## Comando oculto `exec`

Los shims de `bin/` contienen:

```sh
exec "<lair>" exec <tool> "$@"
```

`lair exec` resuelve la versión según `isolated > .lairrc (legacy .flockrc) > (.nvmrc/.node-version) > default > newest`, construye el entorno y usa `execve` para reemplazar el proceso. Si falta la versión y stdin es un TTY, ofrece instalarla; si no hay TTY (p. ej. desde un script), falla con un mensaje claro.

Variables de entorno relevantes:

- `LAIR_HOME`: override del directorio de datos.
- `XDG_DATA_HOME`: base alternativa (`$XDG_DATA_HOME/lair`).
- `LAIR_MANIFEST_BASE_URLS`: lista de mirrors separada por comas. El primero, por defecto, es `https://raw.githubusercontent.com/LC-jhony/lair/main/catalog`; el binario trae además un catálogo firmado embebido con Node LTS `v26.13.1` y MariaDB `11.4.7` (ejemplo de desarrollo, **no un feed de producción**).
- `LAIR_COMPOSER_DOWNLOAD_URL` / `LAIR_COMPOSER_CHECKSUM_URL`: URLs alternativas para `composer.phar` y su checksum (el SHA-256 se sigue verificando).

## Shims y PATH

`lair install` genera los shims en `<LAIR_HOME>/bin` y añade ese directorio al PATH antes de cualquier descarga. Cualquier comando `lair ... install` vuelve a generarlos al terminar, también cuando la instalación se lanza desde la app nativa o la TUI. Si `node -v` no se reconoce, ejecuta `lair doctor --fix`: regenera shims y PATH e indica cómo activarlo en la terminal actual.
