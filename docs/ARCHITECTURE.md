# Arquitectura

## Estructura de paquetes

```
cmd/lair                  único binario: CLI, daemon y hidden `exec`
internal/
  app/                    identidad, rutas (LAIR_HOME/XDG), plataforma del manifiesto
  assets/                 clave pública Ed25519 y logos embebidos (go:embed)
  manifest/               catálogo firmado, verificación, control de serial y caché
  download/               descarga con hash en streaming, .part, extracción segura
  installer/              jobs (fases, progreso, cancelación, logs), hooks OnSuccess
  config/                 store atómico de config.json (0600); ProjectDirs (DefaultProjectsDir = ~/Lair)
  php/                    versiones en paralelo, php.ini, marcadores, FPM/Xdebug
  composer/               phar verificado contra getcomposer.org
  node/                   builds oficiales, resolución por proyecto, prefijos npm
  shims/                  wrappers en bin/ y snippets de perfil de shell
  services/               Specs puros, init de datos, create-db, detección de puertos
  supervisor/             loop de supervisión, backoff, grupos de procesos, logs
  platform/               unit systemd, polkit, dnsmasq seguro :53535, drop-in resolved, setcap/CAP_NET_BIND_SERVICE, backup/rollback, CA
  dns/                    resolución *.test -> loopback, resto se reenvía (dnsmasq en :53535 con fallback automático)
  pki/                    CA local y certificados por sitio
  fastcgi/                cliente FastCGI mínimo (RESPONDER) hacia PHP-FPM
  web/                    router por Host, archivos estáticos, proxy FastCGI, TLS dinámico; Site con dominios y estado de root
  mail/                   SMTP catcher en 127.0.0.1:2525; bandeja de mensajes (acción `mail`)
  dumps/                  captura de dump()/dd() por auto_prepend_file + token dedicado (acción `dumps`)
  daemon/                 API unix+token, protocolo de acciones, dispatcher y runtime
  desktopapi/             bridge HTTP local para la app nativa: POST /api, X-Lair-Token, CORS local
  project/                asistente Laravel compartido (acción `project`): ProjectDirs, migrate-dirs e import-valet (solo lectura de ~/.valet)
  nvm/                    carga de nvm.sh y ejecución con argv validado
  engine/                 orquestación compartida; control de Xdebug bajo demanda (acción `debugger`) y detalle de sitios en site.go (acción `site`)
  cli/                    comandos, conexión daemon/offline, resolución de shims
  tui/                    dashboard full-screen (Charm v2), Snapshot vía daemon.Caller
  version/                parser semver y resolución exacta/major
```

## Interfaces clave

| Paquete | Tipo | Responsabilidad |
|---|---|---|
| `manifest` | `Catalog` | `Refresh`, `LoadFromCache`, `Start`, `Subscribe` (hook `OnChange`) |
| `installer` | `Manager`, `Job`, `Request` | `Start(ctx, Request)`, `Status`, `Cancel`, `OnSuccess` |
| `config` | `Store` | `Update(fn)`, `View()` con escritura atómica |
| `config` | `Config.ProjectDirs`, `Config.ProjectsDir` | `project_dirs` (array) y su vista `projects_dir` en `config.json`; `DefaultProjectsDir(home)` = `~/Lair` (se crea y se aparca si falta) |
| `services` | `Spec` | Contrato puro del proceso: `Executable`, `Args`, `Env`, `DataDir`, `ClientTools` |
| `supervisor` | `Runner`, `Process`, `Supervisor` | `Start`, `Wait`, `Signal`, `Run`, `Update`, `State` |
| `platform` | `PrivilegedPlan` | `BuildPlan(cfg)`, `Script()`, `InstallTrust(ca)`; instala dnsmasq en `127.0.0.1:53535`, drop-in de resolved con `Domains=~test`, `CAP_NET_BIND_SERVICE`/`setcap` para 80/443 y backup/rollback |
| `dns` | `Server` | `Resolve`, `LocalName`, `ListenAndServe` |
| `web` | `Server`, `Router`, `Site` | `Match(host)`, `ServeHTTP`, `Start`, `Shutdown`; `Site` incorpora `Domains`, `RootState`, `RootSuggestions`, `Type`, `Framework`, `PHPVersion`, `PHPSource`, `PHPFile`, `NodeSource`, `NodeFile`, `URL`, `Problem` y `Notice` además de `Name`, `Root`, `PHPSocket`, `Secure`, `PHP`, `Node` y `Database` |
| `daemon` | `Server`, `Client`, `Caller`, `Dispatcher` | `Handle`, `Serve`, `Call` |
| `mail` | funciones de control | Catcher SMTP en `127.0.0.1:2525`: `status`, `start`, `stop`, `list`, `clear` sobre la bandeja local |
| `dumps` | funciones de control | `status`, `capture-on`, `capture-off`, `list`, `clear`; agente inyectado por `auto_prepend_file` en pools FPM con token `0600` |
| `desktopapi` | `Server` | `New(caller, token)`, `ServeHTTP` — solo `POST /api`, token en `X-Lair-Token`. La app Electron realiza las llamadas en el proceso main (`lair:call`) para que el token nunca llegue al renderer; `Origin: null` se rechaza |
| `project` | `CreateRequest`, `Info` | `Detect(resolver, paths)`, `RunLaravel(...)` — misma acción para desktop y CLI |
| `nvm` | funciones | `ValidateArgs`, `Install`, `Exec` — carga `nvm.sh` sin interpolación de shell |
| `engine` | `Engine` | `InstallPHP`, `InstallNode`, `InstallService`, `Status`, `ResolveTool`, `Bootstrap`; `DebuggerControl` activa Xdebug bajo demanda reutilizando los pools FPM; `SiteControl` (`site.go`) construye el detalle, lee/escribe `.env` enmascarado, fija dominios, alterna `enabled`, devuelve el vhost generado y valida el borrado de archivos |
| `config` | Store de sitios | `sites` en `config.json` guarda host, ruta, versiones y los alias por sitio; el borrado de archivos es una operación explícita de la acción `site` |

### Nuevos campos de `web.Site`

La vista de sitio que usan el router y la desktop (`internal/web/router.go`) mantiene el contrato estable de enrutado (`Name`, `Root`, `PHPSocket`, `Secure`, `PHP`, `Node`, `Database`) y añade campos descriptivos tolerantes, rellenados por el engine desde `config.json`, el `.env` y el filesystem:

| Campo | Contenido |
|---|---|
| `Enabled` | Estado del sitio. Con `false` **no se genera host virtual**: el router web Go no publica el sitio y lo retira de la tabla de enrutado. Se alterna con `site` (`op: set-enabled`). |
| `Domains []string` | Dominios y **alias por sitio** además de `Name`. Se registran en el host virtual del servidor Go y amplían el SNI, siempre restringido a los nombres registrados. |
| `RootState` / `RootSuggestions []string` | Resultado del **root detection**: estado de la raíz configurada y sugerencias encontradas (`public/`, `artisan`, `composer.json`). |
| `Type` / `Framework` | Tipo de proyecto y framework detectados. |
| `PHPVersion` / `PHPSource` / `PHPFile` | Versión resuelta de PHP, origen de la resolución y archivo que la fijó. |
| `NodeSource` / `NodeFile` | Origen y archivo de la versión de Node resuelta. |
| `URL` / `Problem` / `Notice` | URL pública del sitio y avisos o incidencias para la interfaz. |

`SiteDetail` (`engine/site.go`) es la proyección completa devuelta por `op: detail`: incluye además `enabled` y `root_state` (con `root_suggestions`, `problem` y `notice`), `database_host`, `database_port` y `database_name`. El entorno se expone por separado con `op: env` (`SiteEnvEntry`: `key`, `value`, `secret`, `set`), con los valores secretos enmascarados en el servidor; nunca salen del daemon en `status` ni en `detail`. La interfaz puede pedir **un** valor concreto con `op: env`, `key` y `reveal: true` (petición explícita del usuario, p. ej. el botón ojo de la contraseña); la respuesta lleva solo `value` y no se registra en logs.

### Layout del host virtual y página de estado

El router web Go (`internal/web/router.go`) construye los hosts virtuales a partir de la vista de sitio: además de `Name`, `Root`, `PHPSocket`, `Cert`, `Key` y `Domains`, incorpora **`Enabled`**, **`RootState`** y el directorio de páginas (`<LAIR_HOME>/web/pages`). La acción `site` (`op: config`) devuelve la definición del host virtual generado, la misma que usa el router; la desktop lo muestra con el botón **View generated vhost**. El vhost contiene solo rutas, sockets, certificados, dominios y puertos: ningún secreto ni valor del `.env`.

- **Sin configuración en `/etc`**: el servidor Go integrado mantiene los hosts virtuales en memoria, derivados de `config.json`, y aplica los cambios en caliente sin privilegios. Activar o desactivar un sitio **no** crea ni borra enlaces ni archivos en el sistema: `Enabled` decide si el host se publica en el router. Así no se mezcla con la configuración de la distro y no hay configuración externa que validar. Decisión en el [ADR 009](adr/009-virtual-hosts-en-servidor-go.md).
- **Página de estado**: cuando `RootState != ok` (raíz ausente, vacía o con `index.php` en la raíz), el **servidor Go integrado** sirve una página de estado generada en `web.Server` en lugar de pasar la petición a PHP-FPM. Muestra el host y la ruta esperada del front controller (`<raíz>/public/index.php`); no incluye rutas internas de `LAIR_HOME`, tokens ni variables de entorno. Los archivos bajo `web/nginx/` de `engine/vhost.go` son solo una vista previa del vhost, no una fuente de enrutado.

## ProjectDirs, importación de Valet y DNS

`config.json` incorpora **`project_dirs`** (array de rutas; la primera es la por defecto) y `projects_dir` (vista derivada). `DefaultProjectsDir` devuelve **`~/Lair`** (L mayúscula), siempre fuera de `$LAIR_HOME`. En el arranque, `SyncProjectDirs` crea `~/Lair` si falta y registra como aparcados los proyectos que encuentre (`artisan` o `composer.json`); la migración de `~/lair` requiere `lair migrate-dirs`.

```json
"project_dirs": ["/home/usuario/Lair", "/srv/otros-proyectos"],
"projects_dir": "/home/usuario/Lair"
```

- `migrate-dirs` (`project` con `op: migrate-dirs`; CLI `lair migrate-dirs`): mueve los proyectos de `~/lair` a `~/Lair`, conserva los archivos y actualiza los sitios registrados; la carpeta antigua no se borra sin confirmación.
- `import-valet` (`op: import-valet`; CLI `lair import-valet`): lee `~/.valet/config.json` y `~/.valet/Sites` **solo en lectura** y registra las rutas y dominios como sitios aparcados. Lair no escribe en esos archivos, no instala Valet y nunca lo ejecuta como root.
- Las operaciones de carpetas (`dirs-list`, `dirs-add`, `dirs-remove`, `dirs-default`, `migrate-dirs`, `import-valet`) comparten la acción `project`, por lo que no añaden acciones al inventario de **49**.
- La página **Sites**, `SiteDetail` y el layout del host virtual descritos más arriba no cambian: los proyectos migrados o importados se integran en el mismo router web Go a partir de `config.json`.

**Puertos y DNS.** El web layer enlaza `127.0.0.1:80` y `127.0.0.1:443` por defecto. `setup-privileged.sh` concede `CAP_NET_BIND_SERVICE` a la unidad (`AmbientCapabilities=CAP_NET_BIND_SERVICE`, con `User=%i` y `NoNewPrivileges=yes`; el daemon nunca corre como root) y ofrece `setcap` sobre el binario como alternativa. Sin esa capacidad, usa Configuración de Lair en Sitios (8080/8443) o los flags `--http-port`/`--https-port`. En Linux el DNS lo sirve un servicio **dnsmasq** configurado únicamente en `127.0.0.1:53535` (nunca en el puerto 53 global); el drop-in de `systemd-resolved` fija `DNS=127.0.0.1#53535` y `Domains=~test`, de modo que solo `*.test` se reenvía al resolver local. Si resolved o dnsmasq fallan, el sistema vuelve al upstream habitual (**fallback automático**); el plan guarda la configuración previa y `revert-privileged.sh` realiza el rollback. En macOS no hay systemd: el plan escribe `/etc/resolver/test` y el **servidor DNS embebido de Lair** (`internal/dns`, multiplataforma) responde en `127.0.0.1:53535`. En Windows se usa un bloque gestionado en el archivo `hosts` y workers `php-cgi` por puerto loopback, sin sockets UNIX.

## Flujo de una instalación

```
CLI/daemon -> Engine.InstallX
   -> manifest.Get + VerifyArtifact (sha256, no yanked)
   -> installer.Manager.Start
        -> Downloader.DownloadToProgress (.part + sha256 en streaming)
        -> download.Install: staging (safeJoin, sin symlinks) -> Prepare
        -> AfterExtract (pack de extensiones de Windows) -> VerifyStaging (php -v)
        -> Preserve (php.ini, conf.d*) -> os.Rename atómico
        -> archivo `build` -> config.json -> OnSuccess (php.ini, prefijos npm)
   -> Job.Status (downloading -> installing -> done; Retry only if retryable)
```

## Flujo de una petición HTTP

```
*.test -> systemd-resolved (DNS=127.0.0.1#53535, Domains=~test)
   -> dnsmasq seguro 127.0.0.1:53535 (fallback automático) -> 127.0.0.1
   -> servidor web Go integrado (80/443 por defecto)
   -> TLS con certificado emitido por la CA local (SNI restringido a sitios aparcados)
   -> Router.Match(Host)
   -> archivo estático en public/  OR  FastCGI -> socket FPM del sitio
```

El enlace es `127.0.0.1` por defecto. Configuración de Lair con `bind 0.0.0.0` expone los sitios a la LAN y el servidor Go integrado respeta ese valor en HTTP y HTTPS; la API, el bridge de la app y el DNS permanecen en loopback. El servidor Go integrado es el **único web layer**: no hay servidor web externo que instalar ni configurar.

## Flujo del agente PHP (Dumps y Debugger)

Las superficies Dumps y Debugger no levantan servicios nuevos: reutilizan los **pools FPM que ya sirven cada sitio aparcado**.

```
petición al sitio (*.test:80/443, servidor Go integrado)
  -> PHP-FPM (pool del sitio, sin pool paralelo permanente)
     -> auto_prepend_file (agente de internal/dumps)  -- captura dump()/dd()
        -> POST local autenticado con run/dumps.token (0600)
           -> daemon: acción `dumps` -> pane Dumps
     -> Xdebug solo si el engine lo activó bajo demanda (acción `debugger`)
        -> cliente en 127.0.0.1:9003 -> pane Debugger
```

- `capture-on` escribe/activa el `auto_prepend_file` en la configuración del pool; `capture-off` lo desactiva sin dejar el agente cargado. El token del agente es distinto del token de la API y se guarda con `0600` en `run/`.
- Xdebug se habilita solo mientras se depura: `debugger detect-on` prepara el pool existente y `detect-off` lo desactiva (`breakpoints-on|off` controla los puntos de rotura). No queda un pool de depuración permanente.
- El correo sale por el SMTP catcher integrado (`internal/mail`) en `127.0.0.1:2525`, que vuelca los mensajes a la bandeja consultada por la acción `mail`. Es un componente propio de Lair: no se instala Mailcatcher ni Caddy.

## Flujo del detalle de sitio y publicación de hosts virtuales

La acción `site` (campo `op`) es la única fuente del detalle de siete pestañas de la desktop. La lógica de sitio vive en `engine/site.go` y el router web Go publica los hosts virtuales a partir de `config.json`:

```
desktop (7 pestañas, View generated vhost, toggle enabled/disabled)
  -> POST /api { action: "site", payload: { op, name, key, value, domains, enabled, delete_files } }
     -> Dispatcher -> engine.SiteControl
        -> op detail        -> SiteDetail (config + .env + filesystem; enabled, root_state; sin secretos)
        -> op env           -> SiteEnvEntry[] (value vacío cuando secret, set indica existencia)
        -> op env-set       -> PatchEnvFile; output = nombre de la clave (nunca el valor)
        -> op domains       -> config.json (sites[name].domains) -> reloadSites -> dominios del host virtual + SNI
        -> op set-enabled   -> config.json (sites[name].enabled) -> reloadSites
                            -> enabled: false => el router retira el host virtual
        -> op config        -> definición del host virtual generado (o vacío si desactivado)
        -> op delete/unlink -> valida ruta (absoluta, fuera de LAIR_HOME, sin symlink)
                            -> delete_files: true borra la carpeta (confirmación en la interfaz)

web (acción `web`: status / configure / https-on / https-off):
  -> config.json (bind, http_port, https_port, https, domain)
  -> servidor web Go integrado -> aplica enlace, puertos, HTTPS y redirección HTTP→HTTPS en caliente
  -> TLS con la CA local -> SNI restringido a los sitios aparcados
```

- Los cambios de Web se aplican en caliente al servidor Go integrado: `WebControl` persiste la configuración y reinicia los listeners (`Server.Reconfigure`) cuando cambian puertos o `bind`; el dominio y los alias se aplican al siguiente handshake TLS. No hay configuración externa que validar. `0.0.0.0` continúa siendo una decisión explícita del usuario.
- Los dominios y alias por sitio se reflejan en el host virtual y en el SNI de los certificados de la CA local; el SNI sigue restringido a los nombres registrados.
- El borrado de la carpeta es opcional: `delete_files: true` más confirmación en la interfaz. `lair unlink` sin esa opción no toca los archivos. El motor rechaza rutas relativas, dentro de `LAIR_HOME` o enlaces simbólicos; los secretos y el entorno se enmascaran y no aparecen en `status`.
- El servidor Go integrado es el **único web layer** (decisión del [ADR 010](adr/010-servidor-go-como-unico-web-layer.md)). Un sitio con `Enabled: false` no publica host virtual y con `RootState != ok` se sirve `not-configured.html`; el layout de hosts virtuales se documenta en el [ADR 009](adr/009-virtual-hosts-en-servidor-go.md).

## Comunicación CLI ↔ daemon

`internal/daemon/protocol.go` define los nombres de acción y los payloads. El daemon registra un `Dispatcher` en el `Server` de la API; la CLI crea un `Client` si `run/api.sock` responde y, si no, usa el mismo `Dispatcher` contra un `Engine` local. Así no hay dos implementaciones de la lógica de negocio.

El inventario compartido se deriva de las constantes `Action*` en `internal/daemon/protocol.go` (ya no se documenta un número fijo). `mail`, `dumps`, `debugger` y `site` siguen el patrón de un campo `op` en el payload, y `logs-list` acepta `sources`. `uninstall` y `shutdown` son internas del `Dispatcher`. `app/scripts/check-actions.mjs` comprueba que las acciones declaradas por el cliente existan en el daemon (y que no falte ninguna acción de daemon salvo un conjunto explícito de acciones solo-CLI), sin fijar un contador obsoleto. Los panes de la desktop solo consumen este protocolo; la lógica vive en `engine`.

El token se genera con `crypto/rand`, se escribe en `run/api.token` con `0600` y se compara con `subtle.ConstantTimeCompare`.

## Decisiones de implementación

- **Sin dependencias de sistema en el daemon.** La lógica Go usa la biblioteca estándar; las únicas librerías externas son Charm v2 (Bubble Tea, Bubbles, Lip Gloss, Huh y bubblezone) para `lair tui`, y `github.com/charmbracelet/x/term` y `go-isatty` para detectar TTY. El daemon se compila en un binario único y reproducible. La administración gráfica vive aparte, en `app/` (Electron + React), y se empaqueta para Linux, macOS y Windows con `npm run dist`.
- **Sin UI web.** Se eliminó el SPA embebido. `internal/desktopapi` expone únicamente `POST /api` en loopback con token en cabecera; el proceso principal de Electron lee `run/desktop.token` y `run/desktop.url` y los pasa al renderer por IPC. Las reglas de negocio siguen estando solo en el `Dispatcher`.
- **Capa web embebida.** En lugar de distribuir un proxy externo, se implementa la capa necesaria en Go (`net/http` + `GetCertificate` + FastCGI). Cubre router por host, estáticos y proxy a FPM.
- **Privilegios únicos y previos.** Ninguna acción privilegiada se ejecuta en caliente: se genera un script que el usuario revisa y ejecuta una vez. El drop-in de systemd usa `AmbientCapabilities=CAP_NET_BIND_SERVICE` (o `setcap` sobre el binario) para los puertos 80/443, sin ejecutar el daemon ni Valet como root. El DNS se apoya en dnsmasq en `127.0.0.1:53535` y un drop-in de `systemd-resolved` con `Domains=~test`; el plan guarda copia de seguridad y permite rollback, con fallback automático al resolver del sistema.
- **Un solo writer de config.** En producción el daemon posee el `Store`; la CLI solo lo usa en modo offline/bootstrap.

## Decisiones de arquitectura (ADRs)

Las decisiones con más impacto están documentadas con formato Contexto / Decisión / Alternativas / Consecuencias en [`docs/adr/`](adr/):

1. [Catálogo firmado con Ed25519](adr/0001-catalogo-firmado-ed25519.md)
2. [CA local y HTTPS](adr/0002-ca-local-y-https.md)
3. [Bridge desktop con token](adr/0003-bridge-desktop-con-token.md)
4. [Servidor Go integrado](adr/0004-servidor-go-embebido.md)
5. [Acción `project` para Laravel](adr/0005-accion-project-para-laravel.md)
6. [Uninstall destructivo y validación de rutas](adr/0006-uninstall-destructivo-y-validacion-de-rutas.md)
7. [Panes de soporte en la desktop y TUI](adr/007-panes-de-soporte-en-desktop-y-tui.md)
8. [Detalle de sitio en pestañas](adr/008-detalle-de-sitio-en-pestanas.md)
9. [Hosts virtuales del servidor Go](adr/009-virtual-hosts-en-servidor-go.md)
10. [Servidor Go como único web layer](adr/010-servidor-go-como-unico-web-layer.md)
11. [Carpeta `~/Lair` y DNS dnsmasq seguro con fallback](adr/011-carpeta-lair-y-dns-dnsmasq-seguro-con-fallback.md)
