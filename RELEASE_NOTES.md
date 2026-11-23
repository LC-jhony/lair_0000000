# Lair v1.0.0

Primera versión etiquetada de **Lair**: entorno de desarrollo local para **PHP / Laravel nativo**, sin Docker ni máquinas virtuales. Un único binario Go contiene la CLI, el daemon, el dashboard de terminal y el punto de entrada de los shims. Todo se instala bajo una sola raíz (`$LAIR_HOME`, `$XDG_DATA_HOME/lair` o `~/.local/share/lair`) y los servicios corren como el usuario, escuchando solo en `127.0.0.1`. El enlace del servidor web es configurable: `bind 0.0.0.0` expone los sitios a la LAN, mientras que la API, el bridge de la app y el DNS permanecen en loopback.

> **Nota de republicación**: el tag `v1.0.0` no se mueve, pero el contenido del release se actualiza con este código. Los tamaños y hashes de las tablas siguientes corresponden a los artefactos originales del release; al publicar los nuevos binarios hay que usar los valores regenerados en `dist/SHA256SUMS` (el tag permanece intacto).

---

## Novedades

- **Carpeta `~/Lair`, migración e importación de Valet y DNS dnsmasq seguro**: la carpeta de proyectos por defecto pasa a ser `~/Lair` (**L mayúscula**, `ProjectDirs`/`DefaultProjectsDir`): se crea si falta y se registra como aparcada. `lair migrate-dirs` (auto al arrancar) migra los proyectos desde `~/lair` y `lair import-valet` lee `~/.valet/config.json` y `~/.valet/Sites` **en solo lectura**, sin instalar ni ejecutar Valet como root. Los puertos web por defecto son 80/443 con `CAP_NET_BIND_SERVICE`/`setcap`, y el DNS usa dnsmasq seguro en `127.0.0.1:53535` con `Domains=~test`, fallback automático y backup/rollback vía `revert-privileged.sh`. La página Sites no cambia. Decisión registrada en el **ADR 011**. El tag `v1.0.0` **no se mueve** y no se publica nada; esta entrada solo actualiza la documentación.
- **nginx eliminado; servidor Go integrado como único web layer**: se retira toda instrucción y configuración de nginx (paquete, `nginx.conf`, `sites/*.conf`, backups, `nginx -t`, `lair web test|regenerate|start|stop|restart|engine`, `/etc/nginx` y la variable `LAIR_NGINX_BIN`). Lair vuelve a servir los sitios `*.test` únicamente con su servidor Go embebido en `127.0.0.1`, con puertos 8080/8443 configurables, HTTPS con CA local y redirección HTTP→HTTPS. La acción `web` sigue existiendo con `status`/`configure`/`https-on`/`https-off` (posteriormente se retiran la CLI `lair web` y la pestaña Web; hoy se configura desde **Configuración de Lair** en Sitios). Sites sigue generando hosts virtuales en el servidor Go. La decisión se registra en el **ADR 010**. El tag `v1.0.0` **no se mueve** y no se publica nada; esta entrada solo actualiza la documentación.
- **Enable/disable de sitios, página de estado y vhost visible**: la acción `site` incorpora las operaciones `set-enabled` y `config`, y `SiteDetail` expone `enabled`: desactivar un sitio retira su host virtual del router web Go y reactivarlo lo recrea desde `config.json`, **sin escribir en `/etc` ni crear symlinks**. Cuando `root_state != ok` se sirve **`not-configured.html`** (página genérica, sin rutas ni secretos) en lugar de pasar la petición a PHP-FPM. La desktop añade **View generated vhost** en el Overview y la TUI la tecla **`V`** y un toggle enabled/disabled. El router web Go usa `Enabled`/`RootState` para decidir si publica el host virtual. El inventario sigue en **35 acciones** y la decisión se registra en el **ADR 009**. El tag `v1.0.0` **no se mueve** y no se publica nada.
- **Interfaz avanzada de Sites**: la página **Sites** incorpora un **detalle de sitio con siete pestañas** (General/Overview, Entorno, Base de datos, Correo, Colas, PHP y Node y Registros), servido por una nueva acción del `Dispatcher`, **`site`** (el inventario compartido pasa a **35 acciones**). La acción expone las operaciones `detail`, `env`, `env-set`, `domains` y `delete`/`unlink`, con `SiteDetail`, entradas de `.env` enmascaradas y alias por sitio (`domains`). Añade **dominios y alias** (host virtual y SNI), **puertos, enlace y dominio configurables** desde Configuración de Lair, **root detection** (`root_state` / `root_suggestions`) y **borrado opcional de la carpeta** con `delete_files: true` **y confirmación en la interfaz** (`unlink` sigue siendo no destructivo). Los secretos se enmascaran en el servidor y **nunca aparecen en `status`**: `env-set` solo informa del nombre de la clave. La TUI añade **`E`** (detalle: Overview + Environment) y **`X`** (borrar sitio con confirmación), sin colisionar con `e` (enlazar). Se mantiene el **servidor Go integrado como único web layer**, con hosts virtuales generados desde `config.json` y aplicados en caliente. La decisión se registra en el **ADR 008**. El tag `v1.0.0` **no se mueve** y no se publica nada; esta entrada solo actualiza la documentación.
- **Seis superficies de soporte en desktop y TUI**: los panes **Mail, Dumps, Debugger y Logs** se añaden al sidebar junto a los renombrados **General** (antes *Overview*/*Resumen*) y **About** (antes *System*/*Sistema*). Se decidió integrarlos como panes de la navegación: **no hay ventanas flotantes ni bandeja nativa**. El backend las sirve con tres acciones nuevas del `Dispatcher` — `mail` (catcher SMTP integrado en `127.0.0.1:2525`), `dumps` (captura de `dump()`/`dd()` por `auto_prepend_file` en los pools FPM, con token dedicado `0600`) y `debugger` (Xdebug bajo demanda reutilizando esos pools) — más la extensión de `logs-list` con `sources`. La TUI añade los atajos `M`/`D`/`B`, sin colisionar con `m`/`d`/`b` (MariaDB, predeterminada y crear BD). El inventario compartido queda en 34 acciones; el web layer es el servidor Go integrado.
- **Pestaña Sites rediseñada y configurable**: cada tarjeta muestra el nombre del proyecto y su host `<nombre>.test` como enlace abrible en el navegador (con el esquema/puerto de **Web**), insignias de HTTPS, PHP, Node y base de datos, interruptor de HTTPS por sitio y un diálogo de configuración para fijar versiones y BD. Incluye **Refresh projects** y confirmación antes de desvincular.
- **`lair new <nombre>` y detección automática de proyectos**: `lair new` es ahora el comando oficial para crear un proyecto Laravel (`lair project new` queda como alias). Tanto los proyectos creados por comando como los que aparezcan en `~/Lair` (por ejemplo con `laravel new` directo) se reconocen solos: cada carpeta con `artisan` o `composer.json` se registra como `<nombre>.test` y aparece en **Sites**. Los sitios enlazados explícitamente no se sobrescriben.
- **App nativa Lair (Electron + React)**: sustituye al panel web. `app/` contiene la ventana con sidebar y panes, componentes base con i18n en/en/es/cs, cliente del bridge HTTP y `FakeDaemon` con fixtures para desarrollar sin daemon. El daemon publica `POST /api` en `127.0.0.1:9797` con token en cabecera; la UI solo adopta su estado y usa jobs con polling.
- **MariaDB por defecto**: `lair install` instala MariaDB `11.4.7` automáticamente (los fallos de red no abortan el bootstrap). `lair services default mariadb|mysql|postgres` cambia la BD de nuevos proyectos.
- **Node LTS actual**: `lair node install` (o `lair node install lts`) instala `v26.13.1` “Lithium” con npm `11.20.0`, equivalente a `nvm install 26`. El catálogo firmado trae las seis plataformas con sus SHA-256 oficiales.
- **Dashboard de terminal**: `lair tui` / `lair dashboard` añade una interfaz interactiva con **Charmbracelet** (Bubble Tea, Bubbles y Lip Gloss), con atajos para Node LTS, PHP, MariaDB, catálogo y servicios. Por esto el binario ya no es “cero dependencias” en tiempo de compilación: las únicas dependencias externas son esas tres librerías de TUI y `mattn/go-isatty` para detectar TTY.
- **Catálogo firmado embebido**: una copia del manifiesto y su firma viajan dentro del binario, por lo que una instalación nueva funciona sin mirrors. Es un **catálogo de ejemplo de desarrollo**, con clave pública de desarrollo y clave privada fuera de Git; no es un feed de producción. El serial `1001` amplía el catálogo con PHP `8.1`–`8.5` (CLI y FPM), MariaDB `11.8.9` y PostgreSQL `17.12`/`18.7` para `linux-x64`; la desktop y la TUI ofrecen selectores de versión y motor conectados a la acción `versions` del daemon.
- **Lanzador de escritorio**: el `.deb` instala `lair.desktop` e `lair.svg`; el menú de aplicaciones muestra **Lair** y ejecuta `lair desktop`.
- **Paquetes `.deb`** para Linux `amd64` y `arm64`, con binario, docs, lanzador e icono.

### Correcciones

- **Documentación sincronizada con Sites avanzado**: README, `docs/CLI.md`, `docs/ARCHITECTURE.md` y `docs/SECURITY.md` describen ahora la página Sites y sus siete pestañas, la acción `site` (payloads/`op`), los puertos, el enlace y el dominio del servidor, los dominios/alias, el root detection, el borrado con confirmación y las teclas `E`/`X` de la TUI. Se añade el ADR 008 y se actualiza el inventario a 35 acciones. Markdown balanceado y `git diff --check` limpios; sin cambios en el tag ni en la publicación.
- **Documentación sincronizada con las nuevas superficies**: README, `docs/CLI.md`, `docs/ARCHITECTURE.md` y `docs/SECURITY.md` describen los seis panes, las acciones `mail`/`dumps`/`debugger`, el campo `sources` de `logs-list` y el ADR 007. El CI existente (jobs Go y app) ya cubre estos cambios con `go test ./...`, `go vet ./...`, `go test -race ./...`, los tests/typecheck/i18n/actions de la app y `npm run build`: **no hacen falta pasos nuevos ni publicar releases**. Según lo previsto, la CLI todavía no expone subcomandos dedicados para las tres acciones nuevas; se verifica con `lair <acción>`/el bridge hasta que se añadan.
- **Revisión final de `v1.0.0` (documentación, CI y seguridad)**: se corrigen los hallazgos de la revisión sin mover el tag ni publicar nada. **Seguridad**: el token del bridge viaja en `X-Lair-Token` (nunca en query ni fragmento), se aclara que `bind 0.0.0.0` expone la LAN y que el motor Go lo respeta, se restringe la CA y el SNI a sitios aparcados, se documentan los permisos y se elimina el filtrado del stderr de PHP-FPM en respuestas HTTP. **Timeouts e i18n**: se refuerzan los timeouts de red y se valida la paridad de claves en/es/cs con `npm run i18n:check`. **TUI**: paridad de acciones con la desktop, ayuda Glamour y comportamiento en pipes sin TTY. **Docs y calidad**: fases 1–6 unificadas, ADRs en `docs/adr/` con su índice, CI con jobs Go y app (Node 22), `.golangci.yml`, `make lint`, `make coverage` y `SHA256SUMS` verificado sin tolerar assets ausentes.
- **`n` en la TUI con catálogo vacío**: el motor vuelve a sembrar el manifiesto firmado embebido si el daemon arranca con el catálogo nulo o vacío (por ejemplo, un proceso anterior a una actualización). La TUI también pide al daemon que refresque y reintenta la acción una vez antes de mostrar el error. Node LTS se instala igual con `LAIR_HOME` nuevo y sin mirrors accesibles.
- **`node -v` / `npm -v` vacíos tras `lair node list`**: cualquier instalación lanzada desde la CLI, la TUI o la app nativa regenera ahora los shims y los fragmentos de PATH mediante la acción `ensure-shims` del daemon. `lair doctor --fix` repara el entorno desde cero, y la app usa `lair:repair-shims` como respaldo si el daemon en ejecución es anterior a esta versión.
- **Paridad desktop ↔ TUI**: la app nativa y la TUI exponen las mismas acciones del `Dispatcher` —predeterminadas, usar/desinstalar versión, crear BD, logs, cancelar trabajos, aparcar/enlazar/desvincular sitios, certificados, pin, isolate y reinicio—. La desktop añade formularios de instalación con versión/puerto y la TUI diálogos equivalentes (`p`, `n`, `m`, `I`). `app/scripts/check-actions.mjs` falla si el cliente desktop declara una acción que el daemon no conoce, y los tests ya no escriben en `~/.bashrc`/`~/.zshrc` reales.
- **Resolución de versiones y actualizaciones**: `lair install` y `ensure-database` ya funcionan con `LAIR_HOME` nuevo (versión vacía → última disponible), `lair node install latest` es alias de la última versión, `lair services update <id> [versión]` activa la versión exacta resuelta y puede sustituir un servicio en marcha. El instalador fusiona varios archivos verificados (CLI + FPM), extrae enlaces simbólicos encadenados y usa `argv[0]` con la ruta real para que los clientes `psql`/`mysql` y los envoltorios `npm`/`npx` funcionen desde los shims. `lair services create-db <id> <nombre>` ya interpreta correctamente el nombre.
- **Node gestionado por nvm a través de Lair**: la desktop y la TUI aceptan la sintaxis de nvm (`26`, `lts`, `22.16.0`, `node`). Toda instalación pasa por el nuevo comando `lair nvm install <versión>`: el daemon invoca el ejecutable de Lair, que carga `nvm.sh` en un `bash` con los argumentos como argv (sin interpolación). Las versiones instaladas se leen de `~/.nvm/versions/node`, los shims usan `lair nvm exec` y la salida de nvm se refleja en el job. No se ejecuta `curl | bash` automáticamente ni se cambia `nvm alias default`; Windows conserva el instalador del catálogo firmado.
- **Composer fijado en el catálogo**: `composer` `2.11.2` se descarga desde `https://getcomposer.org/download/2.11.2/composer.phar` y se acepta solo si coincide con el SHA-256 firmado. `lair composer install` y el botón de la tarjeta Composer vuelven a verificarlo; ya no depende de un `latest-stable` cambiante.
- **Consola Web con HTTPS configurable**: nueva sección **Web** en la desktop y pane equivalente en la TUI (`w`). El servidor Go integrado sirve un host virtual por sitio aparcado, con FastCGI a PHP-FPM y certificados de la CA local. Los puertos (8080/8443 por defecto), la dirección de enlace, el dominio y el interruptor de HTTPS son configurables; los cambios se aplican en caliente. No se instala ningún servidor web externo. Un único action `web` del daemon mantiene la paridad desktop/TUI/CLI.
- **Asistente de nuevo proyecto Laravel**: la desktop (**Sites → New project**), la TUI (`C` en Sitios) y la CLI (`lair project info` / `lair project new`) comparten una única acción `project` del daemon. Instala `laravel/installer` en un `COMPOSER_HOME` aislado y ejecuta `laravel new` con opciones no interactivas; admite SQLite, MariaDB, MySQL, PostgreSQL y SQL Server, con credenciales enviadas solo por `DB_*`. Cuando el motor es gestionado por Lair (MariaDB, MySQL o PostgreSQL), arranca el servicio si está parado y crea la BD antes de las migraciones. **SQL Server no es gestionado por Lair**: se ofrece como opción, pero Lair avisa y solo escribe las credenciales en `.env` (no arranca servicio ni crea la base de datos). El progreso, el registro en vivo y la cancelación reutilizan `job`, `logs` y `job-cancel`, y el sitio queda enlazado y opcionalmente protegido con HTTPS.

Detalles en `README.md` y `docs/CLI.md`.

---

## Campos del release

Esta versión publica **un asset por plataforma y arquitectura**: Linux, macOS y Windows, cada uno en `amd64` y `arm64`. Los seis comprimidos contienen `lair` (o `lair.exe`), `README.md` y `RELEASE_NOTES.md`; además, Linux incluye **dos paquetes `.deb`** (`amd64` y `arm64`) para instalar con `dpkg`/`apt`. Los binarios se compilaron con `CGO_ENABLED=0`, `-trimpath` y `-ldflags "-s -w"` (estáticos, sin dependencias de sistema; las librerías de la TUI quedan embebidas).

### Campos generales

| Campo | Valor | Descripción |
| --- | --- | --- |
| `tag` | `v1.0.0` | Etiqueta **anotada** de Git. Identifica esta versión de forma inmutable; no se reutiliza. |
| `target` | `main` | Commit etiquetado (la rama `main` en el momento del release); se comprueba con `git rev-parse v1.0.0^{commit}`. Incluye el commit inicial de GitHub integrado con la historia de Lair. |
| `title` | `Lair v1.0.0` | Título que se muestra en la página del release. |
| `body` | Este `RELEASE_NOTES.md` | Descripción publicada: novedades, campos de cada asset, instalación, verificación y seguridad. |
| `draft` | `false` | El release se publica de inmediato, no queda como borrador. |
| `prerelease` | `false` | Versión estable; no se marca como beta ni *release candidate*. |
| `assets` | 9 archivos | Seis comprimidos (dos por plataforma), dos `.deb` de Linux (`amd64` y `arm64`) y `SHA256SUMS` con los hashes oficiales. |
| `repository` | `LC-jhony/lair` | Repositorio de GitHub donde se crea el release. |

### Campos de cada asset (comunes a las tres plataformas)

| Campo | Descripción |
| --- | --- |
| `Asset` | Nombre del archivo adjunto al release; es lo que descarga el usuario. |
| `Sistema operativo` | Plataforma de destino: Linux, macOS o Windows. |
| `Arquitectura` | `amd64` (x86_64, Intel/AMD de 64 bits) o `arm64` (AArch64, Apple Silicon / ARM de 64 bits). |
| `Formato` | `.tar.gz` en Linux y macOS; `.zip` en Windows; además `.deb` en Linux. |
| `Binario` | `lair` en Linux y macOS; `lair.exe` en Windows. |
| `Tamaño` | Bytes del asset comprimido y su equivalente en MiB. |
| `SHA-256` | Hash del **archivo publicado**. Es la fuente de confianza: el host de descarga no aporta confianza. |
| `Verificación` | Comando que compara el hash local con `SHA256SUMS` antes de ejecutar nada. |
| `Instalación` | Pasos mínimos: descargar, descomprimir, instalar el binario en el PATH y ejecutar `lair install`. |
| `Notas` | Particularidades de la plataforma (Gatekeeper, PowerShell, plan privilegiado). |

---

### Linux

| Campo | amd64 (x86_64) | arm64 (AArch64) | Descripción |
| --- | --- | --- | --- |
| `Asset` | `lair_1.0.0_linux_amd64.tar.gz` | `lair_1.0.0_linux_arm64.tar.gz` | Archivo adjunto al release para esta arquitectura. |
| `Sistema operativo` | Linux | Linux | Binario estático: no requiere glibc ni dependencias del sistema. |
| `Arquitectura` | `amd64` | `arm64` | x86_64 (Intel/AMD) frente a AArch64 (ARM de 64 bits). |
| `Formato` | `.tar.gz` | `.tar.gz` | Se extrae con `tar -xzf`. |
| `Binario` | `lair` | `lair` | Ejecutable instalado en `$HOME/.local/bin/lair`. |
| `Tamaño` | 3 779 782 bytes (3.60 MiB) | 3 395 825 bytes (3.24 MiB) | Tamaño del comprimido publicado. |
| `SHA-256` | `2452676b3f160e59b1b2fb929c1d79d88ef3c6fffb37fd8096508ea79d5a7afa` | `2d398d9d3561c21de56d7a585da07bf2c7ba2ddbbd9e3ad85acd9de006a0c27e` | Hash oficial; comparar con `dist/SHA256SUMS`. |
| `Asset (.deb)` | `lair_1.0.0_amd64.deb` | `lair_1.0.0_arm64.deb` | Paquete Debian para instalar con `sudo dpkg -i`. |
| `Sistema operativo (.deb)` | Debian/Ubuntu y derivados | Debian/Ubuntu y derivados en ARM | El binario es estático; el paquete solo instala archivos. |
| `Arquitectura (.deb)` | `amd64` | `arm64` | Debe coincidir con `dpkg --print-architecture`. |
| `Formato (.deb)` | `.deb` | `.deb` | Paquete Debian 2.0 con `root:root`, `usr/bin/lair`, lanzador `.desktop` e icono SVG. |
| `Tamaño (.deb)` | 3 203 298 bytes (3.06 MiB) | 2 848 438 bytes (2.72 MiB) | Tamaño del paquete publicado. |
| `SHA-256 (.deb)` | `f03d0bd3eec78e23cf0f509305e7812eeb8d78da45c0c5a2643e4c194b9197ec` | `c0722b5917934cec02ca373efd1c5b24fa455c5cd36919d4f1a0f0d85d148ad8` | Hash oficial del `.deb`; comparar con `SHA256SUMS`. |
| `Verificación` | `sha256sum -c SHA256SUMS` | `sha256sum -c SHA256SUMS` | Valida los ocho assets de una sola vez. |
| `Instalación` | Ver bloques **Instalación en Linux** e **Instalación con .deb**. | Igual que amd64, cambiando el nombre del asset. | Comprimido en `$HOME/.local/bin` o paquete en `/usr/bin/lair`. |
| `Notas` | Plan privilegiado disponible: systemd, polkit, `systemd-resolved` y CA del sistema. | Igual que amd64; usar este asset en equipos ARM (Raspberry Pi, servidores ARM). | Linux es la única plataforma con `setup-privileged.sh`. |

**Instalación en Linux**

```sh
# 1. Descargar el asset correcto (amd64); para arm64 cambia el nombre del archivo
curl -fLO https://github.com/LC-jhony/lair/releases/download/v1.0.0/lair_1.0.0_linux_amd64.tar.gz
curl -fLO https://github.com/LC-jhony/lair/releases/download/v1.0.0/SHA256SUMS

# 2. Verificar la descarga antes de ejecutar
sha256sum -c SHA256SUMS

# 3. Extraer e instalar en el PATH del usuario
tar -xzf lair_1.0.0_linux_amd64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 0755 lair "$HOME/.local/bin/lair"
export PATH="$HOME/.local/bin:$PATH"

# 4. Arrancar Lair y aplicar el plan privilegiado (revisarlo antes de ejecutar)
"$HOME/.local/bin/lair" install
sudo "$HOME/.local/share/lair/setup-privileged.sh"
sudo "$HOME/.local/share/lair/trust-ca.sh" --add
sudo systemctl enable --now "lair@$USER"
```

Tras abrir una terminal nueva: `lair php install 8.4 && lair php use 8.4`, `lair node install 22 && lair node default 22` y `lair doctor`.

**Instalación con .deb en Linux (alternativa)**

```sh
# 1. Descargar el paquete de tu arquitectura y el SHA256SUMS
curl -fLO https://github.com/LC-jhony/lair/releases/download/v1.0.0/lair_1.0.0_amd64.deb
curl -fLO https://github.com/LC-jhony/lair/releases/download/v1.0.0/SHA256SUMS

# 2. Verificar antes de instalar
sha256sum -c SHA256SUMS

# 3. Instalar el paquete (instala /usr/bin/lair, docs, lanzador e icono)
sudo dpkg -i lair_1.0.0_amd64.deb

# 4. Arrancar Lair y aplicar el plan privilegiado (revisarlo antes)
lair install
sudo "$HOME/.local/share/lair/setup-privileged.sh"
sudo "$HOME/.local/share/lair/trust-ca.sh" --add
sudo systemctl enable --now "lair@$USER"

# 5. Muestra la conexión de la app nativa (o ábrela desde el menú de aplicaciones)
lair desktop
```

El icono aparece en el menú tras cerrar sesión y volver a entrar (o con `update-desktop-database`). En ARM cambia `amd64` por `arm64`. Comprueba tu arquitectura con `dpkg --print-architecture`. El `.deb` instala el binario en `/usr/bin/lair` (disponible para todos los usuarios); Lair guarda sus datos igualmente bajo `$LAIR_HOME` del usuario que lo ejecuta. Para desinstalar el paquete: `sudo dpkg -r lair`; recuerda ejecutar antes `lair uninstall` para limpiar el estado del usuario.

---

### macOS

| Campo | Apple Silicon (arm64) | Intel (amd64) | Descripción |
| --- | --- | --- | --- |
| `Asset` | `lair_1.0.0_darwin_arm64.tar.gz` | `lair_1.0.0_darwin_amd64.tar.gz` | Archivo adjunto al release para esta arquitectura. |
| `Sistema operativo` | macOS | macOS | Binario estático para macOS; sin Homebrew ni dependencias de sistema. |
| `Arquitectura` | `arm64` (Apple Silicon) | `amd64` (x86_64, Intel) | Elegir según el chip: M1/M2/M3/M4 → arm64; Intel → amd64. |
| `Formato` | `.tar.gz` | `.tar.gz` | Se extrae con `tar -xzf`. |
| `Binario` | `lair` | `lair` | Ejecutable instalado en `$HOME/.local/bin/lair`. |
| `Tamaño` | 3 512 828 bytes (3.35 MiB) | 3 830 452 bytes (3.65 MiB) | Tamaño del comprimido publicado. |
| `SHA-256` | `c35d21d1d17835abe7e636a517d5632bf1d9c3309386e7decc25bfa58adc994a` | `118828386b47568ffe5e024862666f50b46c25496628cba23add9b794aac0b15` | Hash oficial; comparar con `dist/SHA256SUMS`. |
| `Verificación` | `shasum -a 256 -c SHA256SUMS` | `shasum -a 256 -c SHA256SUMS` | Valida los ocho assets de una sola vez. |
| `Instalación` | Ver bloque **Instalación en macOS**. | Igual que arm64, cambiando el nombre del asset. | Descargar, verificar, extraer, instalar en `$HOME/.local/bin` y ejecutar `lair install`. |
| `Notas` | Asset por defecto en Macs con Apple Silicon. | Asset para Macs Intel. Si Gatekeeper bloquea el binario: `xattr -d com.apple.quarantine "$HOME/.local/bin/lair"`. | No se genera plan privilegiado; la CA se confía solo en el perfil de usuario. |

**Instalación en macOS**

```sh
# 1. Descargar el asset correcto (Apple Silicon); para Intel usa darwin_amd64
curl -fLO https://github.com/LC-jhony/lair/releases/download/v1.0.0/lair_1.0.0_darwin_arm64.tar.gz
curl -fLO https://github.com/LC-jhony/lair/releases/download/v1.0.0/SHA256SUMS

# 2. Verificar la descarga antes de ejecutar
shasum -a 256 -c SHA256SUMS

# 3. Extraer e instalar en el PATH del usuario
tar -xzf lair_1.0.0_darwin_arm64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 0755 lair "$HOME/.local/bin/lair"
export PATH="$HOME/.local/bin:$PATH"

# 4. Arrancar Lair y comprobar el entorno
"$HOME/.local/bin/lair" install
"$HOME/.local/bin/lair" doctor
```

Para hacer persistente el PATH, añade `export PATH="$HOME/.local/bin:$PATH"` a `~/.zshrc` (o `~/.bash_profile`). Si Gatekeeper bloquea el binario descargado: `xattr -d com.apple.quarantine "$HOME/.local/bin/lair"`.

---

### Windows

| Campo | amd64 (x86_64) | arm64 | Descripción |
| --- | --- | --- | --- |
| `Asset` | `lair_1.0.0_windows_amd64.zip` | `lair_1.0.0_windows_arm64.zip` | Archivo adjunto al release para esta arquitectura. |
| `Sistema operativo` | Windows 10/11 | Windows 10/11 ARM | Binario estático `lair.exe`; sin dependencias de sistema. |
| `Arquitectura` | `amd64` (x86_64) | `arm64` (AArch64) | Elegir según el procesador: la mayoría de PCs → amd64; Surface/ARM → arm64. |
| `Formato` | `.zip` | `.zip` | Se extrae con `Expand-Archive`. |
| `Binario` | `lair.exe` | `lair.exe` | Ejecutable instalado en `%LOCALAPPDATA%\Lair\lair.exe`. |
| `Tamaño` | 3 869 352 bytes (3.69 MiB) | 3 442 541 bytes (3.28 MiB) | Tamaño del comprimido publicado. |
| `SHA-256` | `b3dfd8a04698626e924059ca03ef974400bc2264444cf1f790158b4eb5705341` | `8b16041d99fba85a94516a6f3cd19cec8ea5455f689001a3b059fafa8915dc57` | Hash oficial; comparar con `dist/SHA256SUMS`. |
| `Verificación` | `Get-FileHash lair.zip -Algorithm SHA256` | `Get-FileHash lair.zip -Algorithm SHA256` | Comparar el valor con la línea correspondiente de `SHA256SUMS`. |
| `Instalación` | Ver bloque **Instalación en Windows (PowerShell)**. | Igual que amd64, cambiando el nombre del asset. | Descargar, verificar, `Expand-Archive` en `%LOCALAPPDATA%\Lair` y ejecutar `lair.exe install`. |
| `Notas` | Asset por defecto en PCs Windows. | Asset para Windows en ARM. | No se genera plan privilegiado; la CA se confía solo en el perfil de usuario. |

**Instalación en Windows (PowerShell)**

```powershell
# 1. Descargar el asset correcto (amd64); para ARM usa windows_arm64
Invoke-WebRequest -Uri "https://github.com/LC-jhony/lair/releases/download/v1.0.0/lair_1.0.0_windows_amd64.zip" -OutFile lair.zip
Invoke-WebRequest -Uri "https://github.com/LC-jhony/lair/releases/download/v1.0.0/SHA256SUMS" -OutFile SHA256SUMS

# 2. Verificar la descarga antes de ejecutar
Get-FileHash lair.zip -Algorithm SHA256
Get-Content SHA256SUMS

# 3. Extraer y añadir al PATH de la sesión
Expand-Archive lair.zip -DestinationPath "$env:LOCALAPPDATA\Lair" -Force
$env:Path = "$env:LOCALAPPDATA\Lair;$env:Path"

# 4. Arrancar Lair y comprobar el entorno
& "$env:LOCALAPPDATA\Lair\lair.exe" install
& "$env:LOCALAPPDATA\Lair\lair.exe" doctor
```

Para hacer persistente el PATH:

```powershell
[Environment]::SetEnvironmentVariable("Path", "$env:LOCALAPPDATA\Lair;$([Environment]::GetEnvironmentVariable('Path','User'))", "User")
```

> **Plan privilegiado:** systemd, polkit, `systemd-resolved` y la CA a nivel de sistema están implementados solo en Linux. En macOS y Windows funcionan la CLI, el daemon, el DNS en puertos altos, HTTPS con CA de usuario y los shims, pero no se genera ese plan; la CA puede confiarse solo en el perfil de usuario.

---

## Novedades por fases

> **Estado: fases 1–6 implementadas y testeadas.** El registro histórico por fase está en [`docs/PHASES.md`](docs/PHASES.md); las decisiones vigentes, en [`docs/adr/`](docs/adr/).

### Fase 1 — Catálogo firmado e instalación atómica

- Catálogo JSON con `schema`, `channel`, `serial`, `generated`, `expires` y paquetes por plataforma.
- Firma **Ed25519** en `.sig` base64; clave pública embebida con `go:embed`. Se rechazan firma inválida, canal distinto, schema desconocido, manifiesto expirado y serial menor.
- Múltiples URLs base de mirrors (`LAIR_MANIFEST_BASE_URLS`) con fallback en orden: la confianza depende de la firma, no del host.
- Caché del manifiesto con refresco en segundo plano tras 6 h y hook `OnChange` para reaccionar a cambios del catálogo.
- Descarga a `.part`, verificación **SHA-256 en streaming**, extracción con `safeJoin` (impide `../`), `Prepare` + `os.Rename` atómicos y marcador de build.
- `installer.Manager` con un job por paquete: fases, progreso, cancelación, logs en `log/<id>-install.log` y hooks `OnSuccess`.
- Builds marcados como *yanked* se rechazan y se reemplazan automáticamente (`Engine.ReplaceYanked`) al detectar una corrección vía `OnChange`.

### Fase 2 — PHP, Composer, Node y shims

- PHP **8.1–8.5 en paralelo** bajo `php/<minor>/`; `php.ini` se crea una vez y se preserva en actualizaciones.
- PHP-FPM por socket unix en `run/`, pool de depuración Xdebug separado y detección de extensiones requeridas desde `composer.json` / `.env`.
- `composer.phar` descargado solo si su SHA-256 coincide con el publicado; se ejecuta con el intérprete PHP correcto. URLs sustituibles por `LAIR_COMPOSER_DOWNLOAD_URL` / `LAIR_COMPOSER_CHECKSUM_URL`.
- Node con builds oficiales y resolución por proyecto: aislado → `.lairrc` (antes `.flockrc`) → `.nvmrc` / `.node-version` → default → más reciente. Alias ambiguos (`lts/*`, `node`, `stable`) se ignoran.
- Prefijo global de npm por major (`node/global/<major>/`) para que los paquetes globales sobrevivan a actualizaciones patch.
- Shims en `bin/` para `php`, `php-fpm`, `composer`, `node`, `npm`, `npx` y clientes de servicios. `lair exec` (oculto) usa `execve`, sin proceso intermediario.
- Perfiles bash, zsh, fish y `environment.d` idempotentes y reversibles, delimitados por `# >>> lair >>>` / `# <<< lair <<<`.

### Fase 3 — Servicios, supervisor y plataforma

- Specs puros para PostgreSQL 14–18, MySQL, MariaDB, MongoDB, Valkey/Redis, Meilisearch, Typesense y RustFS, con argumentos, entorno, init de datos, clientes y snippet para `.env`.
- Una sola versión activa por puerto; detección de conflictos con sugerencia de puerto alternativo. Validación adicional en MySQL/MariaDB para impedir que un config externo cambie bind, puerto o socket.
- Datos separados por “línea” (`services/<id>/data/<línea>/`; PostgreSQL usa el major) y conservados al cambiar de versión salvo `--delete-data`.
- Supervisor con backoff exponencial (1 s → 60 s), estado por servicio, cola de logs y señales al grupo de procesos. Sin deadlock al registrar logs.
- Plan privilegiado revisable: unidad `lair@<usuario>.service` (`User=%i`, `CAP_NET_BIND_SERVICE`, `NoNewPrivileges=yes`), regla polkit, drop-in de `systemd-resolved` (`DNS=127.0.0.1#53535`, `Domains=~test`) y helpers de CA con `--add` / `--remove`.
- Reversión completa con `revert-privileged.sh`; los scripts se conservan en `$HOME` si se usa `--delete-data`.

### Fase 4 — Daemon, DNS, HTTPS y CLI

- API local en unix socket con token de 256 bits (`0600`, comparación en tiempo constante). `internal/daemon/protocol.go` es la fuente única de acciones y payloads.
- DNS UDP propio en `127.0.0.1:53535`: responde `*.test` y reenvía el resto al upstream de `/etc/resolv.conf`.
- CA ECDSA P-256 local, certificados por sitio con SAN wildcard y soporte de confianza en sistema y navegadores.
- Capa web propia con `net/http`, TLS dinámico, router por Host, archivos estáticos en `public/` y cliente FastCGI mínimo hacia PHP-FPM; en la Fase 6 se convierte en el único web layer (no se instala ningún servidor web externo).
- `Engine` concentra la lógica compartida: instalación, sitios, exec, bootstrap, reemplazo de yanked y supervisión. Daemon y CLI son dos frentes del mismo motor; modo offline disponible.
- CLI completa: `install`, `uninstall`, `status`, `restart`, `update`, `php`, `node`, `services`, `park`, `link`, `unlink`, `secure`, `pin`, `isolate`, `logs`, `daemon`, `doctor` y `exec` oculto.
- Flags del daemon: `--foreground`, `--no-web`, `--no-dns`, `--no-api`, `--no-desktop`, `--desktop-port`, `--dns-port`, `--http-port`, `--https-port` y `--domain` (alias anteriores `--no-ui`, `--ui-port`).

### Fase 5 — App nativa y lanzador

- La SPA embebida se elimina. `internal/desktopapi` publica solo `POST /api` en `127.0.0.1:9797` con token en `X-Lair-Token`, comparación en tiempo constante y CORS limitado a orígenes locales/Electron. No se sirve HTML, CSS ni JS.
- `app/` contiene la app Electron + React + TypeScript: shell con sidebar de 200 px, componentes base reutilizables, tema claro/oscuro, catálogos en/es/cs, cliente HTTP y `FakeDaemon` con fixtures.
- `lair desktop` arranca el daemon si hace falta y lee `run/desktop.url` y `run/desktop.token` (`0600`). El proceso principal de Electron expone `window.lairDesktop.connect()` con `contextIsolation`.
- El `.deb` incluye `/usr/share/applications/lair.desktop` e `/usr/share/icons/hicolor/scalable/apps/lair.svg`.
- Se corrige además el router duplicado: el servidor HTTP y el `Engine` comparten ahora el mismo `Router`, de modo que los sitios aparcados se sirven realmente.

### Fase 6 — Web configurable, proyectos Laravel y calidad

- `internal/web` genera los hosts virtuales de los sitios aparcados en el servidor Go integrado, con FastCGI a PHP-FPM y certificados de la CA local (SNI restringido a sitios aparcados). Los cambios se aplican en caliente desde `config.json`, sin archivos de configuración externos que validar.
- Un único action `web` configura puertos (8080/8443 por defecto), HTTPS, `domain` y `bind` para desktop, TUI y CLI. El enlace por defecto es `127.0.0.1`; `0.0.0.0` expone la LAN y el servidor Go lo respeta.
- `internal/project` aporta la acción `project` compartida: `lair project info` / `lair project new` y los asistentes de desktop y TUI. `laravel/installer` se aísla en `COMPOSER_HOME`, credenciales por `DB_*` y limpieza de rutas. MariaDB/MySQL/PostgreSQL se gestionan de extremo a extremo; SQL Server queda como limitación documentada.
- `internal/nvm` carga `nvm.sh` con argv validado; toda gestión de Node pasa por `lair nvm ...`, sin `curl | bash` automático.
- Internacionalización (en/es/cs) con `npm run i18n:check`, TUI Charm v2 con ayuda Glamour y fixes de timeouts y stderr de PHP-FPM.
- Calidad: CI en `.github/workflows/ci.yml` (Go y app con Node 22, sin publicar releases), `.golangci.yml`, `make lint`, `make coverage`, ADRs en `docs/adr/` y documentación reconciliada (README, SECURITY, CLI, ARCHITECTURE y PHASES).

---

## Correcciones incluidas

- **`node -v` / `npm -v` no reconocidos:** `Bootstrap` ahora crea shims y PATH **antes** de descargar Composer. Composer y la CA son *best-effort* (avisos, no abortan), con timeout de 45 s. Cualquier `lair ... install` regenera shims y PATH al terminar.
- **Nuevo `lair doctor`:** diagnostica PATH, shims faltantes y versiones instaladas; imprime el `export PATH=...` exacto para la terminal actual.
- **`lair uninstall`:** `revert-privileged.sh` ya no reinstala la CA; invoca `<LAIR_HOME>/trust-ca.sh --remove` y `trust-ca-user.sh --remove`, con reversión NSS incluida.
- **Desinstalación completa:** `lair uninstall` ahora elimina PHP, `composer.phar`, Node, los binarios de todos los servicios (incluido MariaDB), shims, PATH, certificados, logs, caché y `config.json`; conserva las bases de datos salvo `--delete-data` y mantiene los scripts de reversión. Lair sirve los sitios únicamente con su **servidor Go integrado** y elimina su estado al desinstalar, sin instalar paquetes del sistema; tampoco instala Laravel Valet ni Mailcatcher, así que no se tocan paquetes del sistema.
- **Nombre del proyecto:** se consolidó `lair` en módulo Go (`github.com/lair-sh/lair`), binario, rutas, documentación y mensajes. Los restos del nombre anterior se renombraron a `.lairrc` y `lair.json`, manteniendo **compatibilidad**: `.flockrc` y `flock.json` siguen leyéndose, `.lairrc` tiene prioridad y el primer `lair pin` migra el contenido sin borrar el archivo antiguo.
- Referencias al antiguo diseño “Caddy embebido” actualizadas a “capa web embebida”.

---

## Documentación

- `README.md` reenfocado en **compilación/Makefile, configuración y uso del comando `lair`**: quick start, los 9 targets del Makefile (incluidos `lint` y `coverage`), cross-compilación, precedencia de `LAIR_HOME`, `config.json`, `.lairrc`, variables de entorno, perfiles, scripts privilegiados, referencia de comandos y flujos prácticos (Laravel nuevo, dos proyectos con PHP distinto, cambio de BD, instalación sin red), además de un mapa de módulos y troubleshooting.
- `docs/CLI.md`, `docs/ARCHITECTURE.md`, `docs/PHASES.md` y `docs/SECURITY.md` actualizados con `doctor`, las nuevas variables, el renombrado compatible, los puertos 8080/8443, el token en `X-Lair-Token`, el bind a LAN y los permisos. `docs/PHASES.md` registra ahora las fases 1–6 como implementadas.
- `docs/adr/` incorpora diez decisiones (catálogo firmado, CA local, bridge desktop, servidor Go integrado, acción `project`, uninstall/validación de rutas, panes de soporte en desktop/TUI, detalle de sitio en pestañas, hosts virtuales del servidor Go y servidor Go como único web layer). La calidad se refuerza con `.github/workflows/ci.yml`, `.golangci.yml`, `make lint`, `make coverage` y `coverage.out` ignorado por Git.

---

## Verificación

- `gofmt -l .`: sin salida.
- `go vet ./...`: sin hallazgos.
- `go test ./...`: todos los paquetes en `ok`, incluidos los tests de regresión de shims, bootstrap resiliente, doctor, reversiones de uninstall, dispatcher, yanked y compatibilidad `.flockrc` / `flock.json`.
- `go build ./...` y `go build -o lair ./cmd/lair`: correctos.
- Smoke tests: `lair status` offline, daemon en foreground, `node -v` → `v22.11.0` y `npm -v` → `10.9.0` a través del shim con PATH configurado, e `install` con red bloqueada creando los 17 shims y avisando de Composer.

> `go test -race` requiere `gcc`/cgo y no estaba disponible en el entorno de compilación; no bloquea esta versión.

---

## Seguridad

- La clave `keys/private.ed25519` es **solo de desarrollo** y no se versiona ni se embebe; se excluye mediante `.gitignore`.
- `certs/ca-key.pem` permite emitir certificados válidos en todo el sistema si la CA está instalada: trátala como secreto.
- Los scripts privilegiados no se ejecutan automáticamente: el usuario los revisa y los lanza con `sudo`.
- No hay sandbox: los servicios son procesos normales del usuario. El DNS propio es UDP mínimo sin DNSSEC.

---

## Instalación rápida

```sh
export PATH="$HOME/.local/go/bin:$PATH"
cd lair && make build
./lair install
sudo "$LAIR_HOME/setup-privileged.sh"
sudo "$LAIR_HOME/trust-ca.sh" --add
sudo systemctl enable --now lair@$USER
# abre una terminal nueva
lair php install 8.4 && lair php use 8.4
lair node install 22 && lair node default 22
node -v && npm -v && php -v && lair doctor
lair desktop                 # datos de conexión de la app nativa
```

---

## Publicación del release

Lair se publica en el repositorio **`LC-jhony/lair`** (antes `valet-deck`, renombrado conservando el historial). La rama `main`, el tag anotado `v1.0.0`, estos campos y estos assets están preparados localmente en `/home/jhony/Documentos/lair`.

Los artefactos se reconstruyen de forma reproducible con `GO=<go> scripts/build-dist.sh 1.0.0`: compila los seis binarios estáticos, genera los comprimidos, los dos `.deb` (con lanzador e icono) y `SHA256SUMS` verificado.

La publicación requiere credenciales de GitHub (un PAT con permiso `repo` o `gh auth login`). Hay dos formas:

**Opción A — script preparado (recomendado):**

```sh
cd /home/jhony/Documentos/lair
./scripts/create-release.sh
```

El script valida la autenticación, comprueba que el tag apunte a `HEAD`, sube `main` y el tag, y crea el release con el título, el cuerpo y los nueve assets descritos arriba. Si el release ya existe, primero elimina los assets antiguos con el mismo nombre y vuelve a subirlos (necesario tras regenerar `SHA256SUMS`).

**Opción B — comandos manuales:**

```sh
cd /home/jhony/Documentos/lair
git push origin main
git push origin v1.0.0
gh release create v1.0.0 \
  --repo LC-jhony/lair \
  --title "Lair v1.0.0" \
  --notes-file RELEASE_NOTES.md \
  --verify-tag \
  dist/SHA256SUMS \
  dist/lair_1.0.0_amd64.deb \
  dist/lair_1.0.0_arm64.deb \
  dist/lair_1.0.0_darwin_amd64.tar.gz \
  dist/lair_1.0.0_darwin_arm64.tar.gz \
  dist/lair_1.0.0_linux_amd64.tar.gz \
  dist/lair_1.0.0_linux_arm64.tar.gz \
  dist/lair_1.0.0_windows_amd64.zip \
  dist/lair_1.0.0_windows_arm64.zip
```

> `draft` y `prerelease` se omiten porque `gh` los deja en `false` por defecto, tal como indica la tabla de campos generales.
