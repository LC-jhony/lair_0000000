# Lair

Entorno de desarrollo local para **PHP / Laravel nativo** (sin Docker ni VM). Un solo binario Go reúne la CLI, el daemon y el punto de entrada de los shims: compilas una vez, configuras con dos variables y usas `lair` para todo.

> Estado: fases 1–6 implementadas y testeadas. Linux con `systemd-resolved` es la plataforma completa. El catálogo firmado incluido es **de ejemplo** (clave Ed25519 de desarrollo con la clave privada fuera de Git); no sirve para producción.

---

## En 30 segundos

```sh
# 1. compilar
export PATH="$HOME/.local/go/bin:$PATH"
cd lair && make build

# 2. configurar e instalar
export LAIR_HOME="$HOME/.local/share/lair"      # opcional: es el valor por defecto
./lair install
sudo "$LAIR_HOME/setup-privileged.sh"
sudo "$LAIR_HOME/trust-ca.sh" --add
sudo systemctl enable --now lair@$USER

# 3. usar
new shell                       # o: source ~/.bashrc
lair php install 8.4 && lair php use 8.4
lair node install                        # último LTS firmado: v26.13.1 (npm 11.20.0)
lair services install postgres 17        # MariaDB ya se instaló con `lair install`
node -v && npm -v && php -v
lair doctor                     # diagnóstico si algo no responde
```

---

## Índice

1. [Compilación y Makefile](#1-compilación-y-makefile)
2. [Configuración](#2-configuración)
3. [El comando `lair`](#3-el-comando-lair)
4. [Flujos de trabajo](#4-flujos-de-trabajo)
5. [Resolución de versiones](#5-resolución-de-versiones)
6. [Estructura de datos](#6-estructura-de-datos)
7. [Mapa de módulos](#7-mapa-de-módulos)
8. [Daemon, DNS y HTTPS](#8-daemon-dns-y-https)
9. [Seguridad](#9-seguridad)
10. [Solución de problemas](#10-solución-de-problemas)
11. [Más documentación](#11-más-documentación)

---

## 1. Compilación y Makefile

### Requisitos de compilación

Necesitas **Go 1.26.0+** (`go.mod` fija `go 1.26.0`; en este entorno: `~/.local/go`, versión 1.27.4) y acceso a `go mod download` la primera vez. El runtime de Lair usa la biblioteca estándar; las dependencias externas son las del fallback de la TUI (`charmbracelet/bubbletea`, `bubbles`, `lipgloss` y `mattn/go-isatty` para detectar TTY). La TUI principal es un bundle React/Ink + termcn que se compila con Node 22+ y se incrusta en el binario; el usuario final no necesita Go ni Node para compilar, solo Node 22+ para ejecutar `lair tui`. Para compilar sin red, usa la caché de módulos ya descargada.

Si Go está instalado en `~/.local/go`, añádelo al PATH de tu shell:

```sh
export PATH="$HOME/.local/go/bin:$PATH"
go version        # go version go1.27.4 linux/amd64
```

### El Makefile

```makefile
GO  ?= go
BIN := lair

all: test build
build:      $(GO) build -o $(BIN) ./cmd/lair
test:       $(GO) test ./...
test-race:  $(GO) test -race ./...
vet:        $(GO) vet ./...
lint:       golangci-lint run      # si no está instalado, usa $(GO) vet ./...
coverage:   $(GO) test -coverprofile=coverage.out ./...
fmt:        $(GO) fmt ./...
clean:      rm -f $(BIN)
```

| Target | Comando que ejecuta | Para qué sirve |
|---|---|---|
| `make build` | `go build -o lair ./cmd/lair` | Genera `./lair`: CLI + daemon + `exec` en un único ejecutable. Es el target que normalmente usarás. |
| `make test` | `go test ./...` | Toda la suite unitaria (manifiesto, versiones, extracción segura, servicios, shims, API, DNS, PKI, web, dispatcher, bootstrap, uninstall). |
| `make vet` | `go vet ./...` | Análisis estático; debe salir sin mensajes. |
| `make lint` | `golangci-lint run` (o `go vet ./...` si no está instalado) | Linters de `.golangci.yml`: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` y `misspell`. |
| `make coverage` | `go test -coverprofile=coverage.out ./...` | Genera `coverage.out` (ignorado por Git) y muestra el porcentaje total. |
| `make fmt` | `go fmt ./...` | Formatea todo el código con `gofmt`. |
| `make test-race` | `go test -race ./...` | Detecta carreras; **requiere cgo y un compilador C (`gcc`)**. Si no tienes gcc, usa `make test`. |
| `make all` | `test` + `build` | Valida y compila de un golpe (es el target por defecto: basta con `make`). |
| `make clean` | `rm -f lair` | Borra el binario generado. |

> **CI:** `.github/workflows/ci.yml` ejecuta en cada push y pull request los jobs `go` (`go test ./...`, `go vet ./...`, `go test -race ./...` y `go build`) y `app` (`npm ci`, `test`, `typecheck`, `i18n:check`, `actions:check` y `build`, con Node 22). El CI **no publica releases**; la publicación sigue siendo manual con `scripts/create-release.sh`.

### Personalizar la compilación

Las variables se pueden sobreescribir en la línea de comandos:

```sh
make GO="$HOME/.local/go/bin/go" BIN=./lair-dev build     # ruta explícita de Go / otro nombre
make GO=go test                                           # usar el go del PATH
make clean build                                          # reconstruir desde cero
```

Cross-compilación (el manifiesto usa claves `linux-x64`, `linux-arm64`, etc.):

```sh
GOOS=linux   GOARCH=amd64 make build BIN=./lair-linux-x64
GOOS=linux   GOARCH=arm64 make build BIN=./lair-linux-arm64
GOOS=darwin  GOARCH=arm64 make build BIN=./lair-darwin-arm64
CGO_ENABLED=0 make build      # binario estático; recomendado para distribuir
```

Tras compilar, comprueba el binario:

```sh
./lair help
./lair status --json
```

> Instala el binario en una ruta **estable** (por ejemplo `~/.local/bin/lair`). Los shims guardan la ruta absoluta del ejecutable con el que se generaron: si luego borras el binario temporal con el que hiciste `lair install`, los shims dejarán de funcionar. Si eso pasa, vuelve a ejecutar `lair install` con el binario definitivo.

---

## 2. Configuración

### 2.1 Raíz de datos (`LAIR_HOME` / `XDG_DATA_HOME`)

Todo lo que Lair gestiona vive bajo una sola raíz. La precedencia es:

| Prioridad | Variable | Resultado |
|---|---|---|
| 1 | `LAIR_HOME` | Se usa tal cual (si es relativa, se convierte en absoluta). |
| 2 | `XDG_DATA_HOME` | `$XDG_DATA_HOME/lair` |
| 3 | — | `~/.local/share/lair` |

```sh
export LAIR_HOME="$HOME/.local/share/lair"     # valor por defecto; puedes omitirlo
export LAIR_HOME="/datos/lair"                  # raíz alternativa (disco, tests, etc.)
export XDG_DATA_HOME="$HOME/.local/share"       # equivalente estándar XDG
```

Comprueba la raíz que se está usando en cualquier momento:

```sh
lair status    # primera línea: "Lair home: ..."
lair doctor    # imprime home, bin, PATH y shims
```

### 2.2 Configuración persistente (`config.json`)

`<LAIR_HOME>/config.json` (permisos `0600`) es el estado de la instalación. **En producción solo lo escribe el daemon**; la CLI lo usa directamente únicamente en modo offline/bootstrap. Las escrituras son atómicas.

```json
{
  "version": 1,
  "installed": {
    "php/8.4": "N3",
    "node/22.11.0": "N1",
    "postgres/17.2": "N2"
  },
  "defaults": {
    "php": "8.4",
    "node": "22.11.0",
    "postgres": "17.2"
  },
  "ports": { "postgres": 5432 },
  "project_dirs": {
    "default": "/home/usuario/Lair",
    "dirs": ["/home/usuario/Lair"]
  },
  "sites": {
    "mi-app.test": {
      "path": "/home/usuario/Lair/mi-app",
      "php": "8.4",
      "node": "22"
    }
  }
}
```

| Campo | Significado | Se modifica con |
|---|---|---|
| `installed` | `"<paquete>/<versión>" → build` instalado. | `lair php/node/services install` |
| `defaults` | Versión activa de `php`, `node` y cada servicio. | `lair php use`, `lair node default`, `lair services use` |
| `ports` | Puerto asignado a cada servicio (uno activo por puerto). | `lair services install --port N` |
| `project_dirs` | **ProjectDirs**: carpeta por defecto (`~/Lair`) y lista de carpetas que se escanean en busca de proyectos. | `lair project dirs`, `lair migrate-dirs`, `lair import-valet` |
| `sites` | Sitios aparcados y sus versiones aisladas. | `lair park`, `lair link`, `lair isolate` |

`config.json` no se edita a mano: usa los comandos para que el daemon mantenga el estado coherente.

### 2.3 Configuración por proyecto (`.lairrc`)

`.lairrc` es un archivo de **texto versionable** que fija versiones para un proyecto. Se escribe con `lair pin` y lo lee el shim antes de elegir versión:

```ini
# .lairrc
php=8.4
node=22
```

```sh
lair pin php 8.4
lair pin node 22
git add .lairrc && git commit -m "Fijar versiones de Lair"
```

> **Compatibilidad:** los proyectos que todavía tengan `.flockrc` (nombre del diseño original) siguen funcionando: se lee como alternativa, tiene menos prioridad que `.lairrc`, y el primer `lair pin` migra su contenido al archivo nuevo sin borrar el antiguo. Lo mismo ocurre con el marcador `flock.json` de instalaciones PHP antiguas, que ahora se escribe como `lair.json`.

También se respetan `.nvmrc` y `.node-version` para Node (en ese orden, después de `.lairrc`). Los alias ambiguos como `lts/*` o `node` se ignoran deliberadamente. Para fijar una versión solo en un sitio aparcado, sin versionarla, usa `lair isolate php 8.3` (o `lair isolate php --off` para quitarla).

### 2.4 Variables de entorno

| Variable | Uso |
|---|---|
| `LAIR_HOME` | Raíz de datos (ver 2.1). |
| `XDG_DATA_HOME` | Base alternativa (`$XDG_DATA_HOME/lair`). |
| `LAIR_MANIFEST_BASE_URLS` | Mirrors del catálogo firmado separados por comas; se prueban en orden. Por defecto incluye `https://raw.githubusercontent.com/lair-sh/lair/main/catalog`. La confianza es de la firma, no del host. |
| `LAIR_COMPOSER_DOWNLOAD_URL` | URL alternativa de `composer.phar` (se sigue verificando el SHA-256). |
| `LAIR_COMPOSER_CHECKSUM_URL` | URL alternativa del `.sha256` de Composer (solo si no hay checksum en el catálogo). |
| `LAIR_COMPOSER_SHA256` | SHA-256 alternativo para `composer.phar` en pruebas/espejos. |

Las dos últimas son útiles en entornos cerrados o con mirror interno:

```sh
export LAIR_MANIFEST_BASE_URLS="https://cdn.lair.sh/stable,https://mirror.interno/lair/stable"
export LAIR_COMPOSER_DOWNLOAD_URL="https://mirror.interno/composer.phar"
export LAIR_COMPOSER_CHECKSUM_URL="https://mirror.interno/composer.phar.sha256"
lair update && lair install
```

### 2.5 PATH y perfiles de shell

`lair install` escribe un bloque delimitado por `# >>> lair >>>` / `# <<< lair <<<` en:

- `~/.bashrc` y `~/.zshrc` (guarda `[ -d "$bin" ]` para no contaminar PATH si se borra la raíz);
- `~/.config/fish/conf.d/lair.fish`;
- `~/.config/environment.d/lair.conf`.

Es idempotente y reversible: `lair uninstall` elimina exactamente esos bloques y respeta cualquier otra edición tuya. Para aplicarlo sin reiniciar:

```sh
source ~/.bashrc     # bash
source ~/.zshrc      # zsh
```

Si `node -v` no se reconoce, ejecuta `lair doctor --fix`: regenera los shims y los fragmentos de PATH, y te dice si falta el `export PATH="..."` exacto.

### 2.6 Scripts privilegiados (configuración del sistema)

`lair install` genera tres scripts que **tú debes revisar y ejecutar una vez**; Lair nunca los lanza automáticamente:

| Script | Se ejecuta con | Qué configura |
|---|---|---|
| `<LAIR_HOME>/setup-privileged.sh` | `sudo` | Unidad `lair@<usuario>.service`, regla polkit, `CAP_NET_BIND_SERVICE` (o `setcap` del binario) para los puertos 80/443, servicio **dnsmasq** en `127.0.0.1:53535` y drop-in de `systemd-resolved` (`DNS=127.0.0.1#53535`, `Domains=~test`), con copia de seguridad y reversión. |
| `<LAIR_HOME>/trust-ca.sh --add` | `sudo` | CA local en el store del sistema. |
| `<LAIR_HOME>/trust-ca-user.sh --add` | tu usuario | CA en Firefox/Chromium (requiere `certutil`). |

```sh
sudo "$LAIR_HOME/setup-privileged.sh"
sudo "$LAIR_HOME/trust-ca.sh" --add
"$LAIR_HOME/trust-ca-user.sh" --add
sudo systemctl enable --now lair@$USER
```

> El plan privilegiado instala dnsmasq **solo en `127.0.0.1:53535`** (nunca en el puerto 53 global) y escribe el drop-in de `systemd-resolved` con `Domains=~test`, de modo que solo `*.test` se reenvía al resolver local. Antes de tocar nada guarda una copia de la configuración previa; `revert-privileged.sh` la restaura si la resolución falla o si decides volver atrás. Lair **no ejecuta Valet como root** y el daemon corre como el usuario.

`lair uninstall` **elimina todo lo que Lair instaló**: shims y entradas de PATH, PHP, `composer.phar`, Node, los binarios de **todos los servicios (incluido MariaDB)**, la CA local, certificados, logs, caché y `config.json`. Por defecto conserva las bases de datos en `services/<id>/data/`; `--delete-data` también las borra. Genera `revert-privileged.sh` (y conserva los helpers con `--remove`) para deshacer los cambios privilegiados; con `--delete-data`, esos scripts se guardan en `$HOME` para que no desaparezcan al borrar la raíz.

> Lair sirve los sitios con **su servidor Go integrado** en `127.0.0.1`, sin paquetes del sistema: no hay servidor web externo, no se instala ninguno y no se escribe en `/etc`. No instala Laravel Valet, Mailcatcher ni bandeja nativa del sistema; el catcher SMTP es propio de Lair (ver [§3.12](#312-superficies-de-soporte-mail-dumps-debugger-y-logs)) y el uninstall no toca paquetes del sistema. Los hosts virtuales los genera el router web Go a partir de `config.json`, con los dominios/alias registrados y HTTPS con la CA local. El estado Web informa de los puertos, el enlace, el dominio y la redirección HTTP→HTTPS. Detalle en [§3.6](#36-sitios-y-https).

### 2.7 Catálogo firmado de ejemplo

Lair no consulta un índice editable: consume un **manifiesto firmado con Ed25519**. El binario lleva una copia embebida (`internal/assets/catalog.json` + `.sig`) y el repositorio publica la misma pareja en `catalog/`; por eso una instalación nueva funciona aunque `cdn.lair.sh` o `lair-builds.github.io` estén caídos. Un manifiesto del mirror con serial mayor sustituye al embebido; uno más antiguo se ignora.

> **Este catálogo es un ejemplo de desarrollo, no un feed de producción.** La clave pública está en `keys/public.ed25519` y la privada (`keys/private.ed25519`, `0600`) está excluida de Git. Para producción hay que generar un par nuevo, publicar solo la pública en el binario y firmar los manifiestos con la privada en un entorno controlado.

Incluye Node.js `v26.13.1` LTS “Lithium” (npm `11.20.0`, equivalente a `nvm install 26`) para Linux/macOS/Windows en `x64`/`arm64`; PHP `8.1.34`, `8.2.32`, `8.3.32`, `8.4.23` y `8.5.8` (CLI + FPM estáticos, `linux-x64`) de `static-php-cli`; MariaDB `11.4.7` y `11.8.9`; y PostgreSQL `17.12.0` y `18.7.0` para `linux-x64`; Composer `2.11.2` (`composer.phar` oficial verificado por SHA-256). Todos los SHA-256 se verifican antes de extraer nada. MySQL no se incluye: los binarios oficiales requieren `libaio.so.1` del sistema y no arrancan en una instalación limpia.

**Node se instala y se gestiona con nvm, siempre a través de Lair.** El daemon no llama a nvm directamente: ejecuta `lair nvm install <versión>`, que es quien carga `~/.nvm/nvm.sh`. El panel Runtimes (y la TUI con `n`) acepta la sintaxis real de nvm: `26`, `lts`, `22.16.0` o `node`. Las versiones se leen de `~/.nvm/versions/node` y los shims (`node`, `npm`, `npx`) resuelven mediante `lair nvm exec`, respetando `.nvmrc`. Puedes usar el comando directamente:

```bash
lair nvm install 26
lair nvm install lts/*
lair nvm install 22.16.0
lair nvm ls
lair nvm current
```

Si nvm no está instalado, Lair avisa con la URL de instalación; no descarga ni ejecuta scripts remotos por su cuenta. En Windows (donde nvm no existe) se mantiene el instalador del catálogo firmado.

Formato (un build por `paquete → versión → plataforma`):

```json
{
  "schema": 1,
  "channel": "stable",
  "serial": 1001,
  "packages": {
    "node": {
      "v26.13.1": {
        "linux-x64": {
          "build": "N1",
          "url": "https://nodejs.org/dist/v26.13.1/node-v26.13.1-linux-x64.tar.gz",
          "sha256": "518674de2b621728a5e4486f5fb3b7d2900bb3cc4aa97969edaa463db99f9778",
          "size": 62904977,
          "published": "2026-11-17T00:00:00Z",
          "lts": "Lithium"
        }
      }
    }
  }
}
```

`url` admite `https://`, `http://` y `file://` (útil con mirrors internos); las relativas se resuelven contra `LAIR_MANIFEST_BASE_URLS`. El campo `lts` es el que permite a `lair node install` elegir el último LTS. Los campos opcionales `files` y `components` permiten publicar builds cuyo layout no coincide con el de Lair: `files` mueve miembros concretos del archivo (`php` → `bin/php`) y `components` fusiona otros archivos verificados en el mismo destino (`php-fpm` → `sbin/php-fpm`). Para volver a firmar el catálogo tras editarlo:

```sh
go run ./cmd/catalog-sign -in catalog/manifest.json -out catalog/manifest.json.sig -key keys/private.ed25519
cp catalog/manifest.json internal/assets/catalog.json
cp catalog/manifest.json.sig internal/assets/catalog.json.sig
```

La firma cubre los bytes exactos del JSON, así que cualquier edición sin volver a firmar se rechaza antes de parsearse. Tanto la desktop como la TUI leen estas mismas listas mediante la acción `versions` del daemon.

---

## 3. El comando `lair`

El binario es la **única** entrada: `lair <comando> [subcomando] [args] [flags]`. Si el daemon está activo, la CLI habla con él por un socket unix; si no, usa el mismo motor en proceso (modo offline). No hay dos implementaciones de la lógica.

### 3.1 Instalación y diagnóstico

| Comando | Descripción |
|---|---|
| `lair install` | Bootstrap local (shims, PATH, CA, Composer verificado), instalación de **MariaDB como base de datos por defecto** y generación de los scripts privilegiados. Si no hay red o catálogo, avisa pero no aborta. |
| `lair uninstall [--delete-data]` | Elimina PHP, Composer, Node, todos los servicios, shims, PATH y CA local; conserva las bases de datos salvo `--delete-data`. También genera la reversión privilegiada. |
| `lair status [--json]` | Muestra home, estado del daemon, PHP, Node, servicios, puertos, sitios y jobs. `--json` para scripts. |
| `lair doctor [--fix]` | Comprueba PATH, shims y versiones; imprime cómo arreglarlo. Con `--fix` regenera shims y PATH. **Primer comando si algo no responde.** |
| `lair restart` | Reinicia todos los servicios supervisados activos. |
| `lair update` | Fuerza el refresco del catálogo firmado. |
| `lair logs [nombre]` | Muestra un log (`postgres`, `node-22-install`, ...) o lista los disponibles si no hay argumento. |
| `lair help` | Ayuda en pantalla. |

### 3.2 PHP

| Comando | Descripción |
|---|---|
| `lair php list` | Versiones instaladas; `*` marca la default. |
| `lair php install [versión]` | Instala una línea minor (`8.4`) o versión exacta. Sin versión, instala la más reciente del catálogo. |
| `lair php use <minor>` | Cambia la default global. |

```sh
lair php install 8.3
lair php install 8.4 && lair php use 8.4
php -v && composer -V
```

### 3.3 Node

| Comando | Descripción |
|---|---|
| `lair node list` | Versiones instaladas; `*` marca la default. |
| `lair node install [versión]` | Instala con nvm mediante `lair nvm install <versión>`: sin versión (o `lts` / `latest-lts`) instala el último **LTS**; también acepta `26`, `node`, `latest` o `22.16.0`. La salida de nvm aparece en el job. |
| `lair composer install` | Instala/verifica `tools/composer.phar` usando la versión y el SHA-256 del catálogo firmado (actualmente `2.11.2`, descargado de getcomposer.org). |
| Configuración de Lair (botón en Sitios) | Muestra y configura el servidor Go: enlace, puertos, HTTPS, dominio y redirección HTTP→HTTPS. También permite elegir PHP/Node por defecto, ver las carpetas de proyectos y el estado de DNS y CA, e importar carpetas de Valet. |
| Configuración de Lair | Activa o desactiva HTTPS para los sitios `*.test`. |
| Configuración de Lair | Configura puertos, enlace y dominio del servidor Go. |
| `lair node default <versión>` | Fija la default (acepta major, p. ej. `22`). |

> La acción `web` del daemon mantiene la paridad entre la CLI, la desktop y la TUI y expone las operaciones `status`, `configure`, `https-on` y `https-off`.

```sh
lair node install                 # LTS actual: v26.13.1
lair node default v26.13.1
node -v && npm -v && npx --version
```

> Si `node -v` falla justo después de instalar, es un problema de PATH/shims, no de la instalación. Ver [solución](#10-solución-de-problemas).

### 3.4 Servicios

Forma general: `lair services <acción> <id> [versión] [flags]`.

| Acción | Ejemplo | Descripción |
|---|---|---|
| `list` (o sin acción) | `lair services list` | Estado de todos los servicios. |
| `install` | `lair services install postgres 17 --port 5432` | Descarga e inicializa; no cambia la activa si ya hay una. |
| `use` | `lair services use postgres 17.2` | Cambia la versión activa conservando los datos. |
| `update` | `lair services update postgres` | Instala la última, activa y reinicia. |
| `uninstall` | `lair services uninstall postgres 17.2 --delete-data` | Elimina la instalación; sin `--delete-data` conserva los datos. |
| `create-db` | `lair services create-db postgres laravel` | Crea una base de datos. |
| `default` | `lair services default mariadb\|mysql\|postgres` | Elige la BD predeterminada de nuevos proyectos (MariaDB por defecto). |
| `restart` / `stop` | `lair services restart postgres` | Controla el supervisor. |

IDs disponibles: `postgres`, `mysql`, `mariadb`, `mongodb`, `redis`, `valkey`, `meilisearch`, `typesense`, `rustfs`.

Los servicios de búsqueda y almacenamiento usan los puertos 7700 (Meilisearch), 8108 (Typesense) y 9000 con consola en 9001 (RustFS). Sus credenciales son fijas y **solo para desarrollo**: `TYPESENSE_API_KEY=lair` y `rustfsadmin`/`rustfsadmin`. Typesense no tiene build para Windows y aparece como no disponible allí. PostgreSQL usa siempre el puerto 5432: no se elige al instalar ni se cambia después.

El catálogo firmado publica compilaciones para `mariadb`, `postgres`, `mongodb`, `valkey` 9.2.0 y `redis` 7.4.6 en Linux x64/arm64. Valkey y Redis comparten el puerto 6379: instala el segundo con `--port 6380`. La interfaz muestra todas las tarjetas y marca las que no tienen compilación para la plataforma actual («Not available on …»), en lugar de ocultarlas sin explicación.

SQLite no es un servicio supervisable: es una base de datos por archivo. Lair crea `database/database.sqlite` dentro de cada proyecto (opción `sqlite` en `lair new`) y la pestaña Services muestra una tarjeta informativa; no hay proceso ni puerto que iniciar. El CLI `sqlite3` 3.54.1 sí se instala desde el catálogo (`lair services install sqlite3`) y queda disponible como shim (`lair exec sqlite3 --version`). El catálogo publica el CLI para Linux x64 y macOS/Windows x64+arm64; Linux arm64 requiere compilarlo, así que aparece como no disponible en esa plataforma.

Valkey y Redis no tienen binario oficial para Windows (solo WSL/Docker), por lo que en Windows aparecen como no disponibles en lugar de ofrecer una instalación que no funcionaría. SQL Server no se implementa deliberadamente: exige root o Docker y no encaja en el modelo de servicios locales de Lair.

El catálogo permite canales `stable`, `beta` y `experimental` mediante `LAIR_CHANNEL`; cada canal tiene su propio `manifest[-canal].json`, firma, caché y contador de serial, y no acepta un manifiesto caducado ni con serial menor. `.github/workflows/binaries.yml` verifica las URLs de los artefactos, incrementa el serial, actualiza la caducidad y vuelve a firmar el catálogo con la clave privada del secreto `LAIR_SIGNING_KEY`.

Los clientes (`psql`, `mysql`, `redis-cli`, `valkey-cli`, `mongosh`, ...) también son shims e inyectan automáticamente host, puerto y usuario del servicio activo:

```sh
lair services install postgres 17
lair services create-db postgres laravel
psql -d laravel            # apunta a 127.0.0.1:5432 sin configurar nada
```

### 3.5 Proyectos Laravel

Lair puede crear un proyecto Laravel completo con una sola acción del daemon
(`project`), compartida por la desktop, la TUI y la CLI:

| Comando | Descripción |
|---|---|
| `lair project info` | Muestra instalador Laravel, versiones de PHP/Composer/Node, drivers PDO, gestores Node, motores de BD disponibles y la carpeta de proyectos. |
| `lair project dir [ruta]` | Muestra o fija la carpeta por defecto donde se crean los proyectos. Por defecto `~/Lair` (**L mayúscula**); nunca se usa el directorio actual ni `$LAIR_HOME`. Es un atajo de `lair project dirs default`. |
| `lair project dirs [list\|add <ruta>\|remove <ruta>\|default <ruta>]` | Gestiona **ProjectDirs**: lista, añade, quita o fija la carpeta por defecto. Las rutas deben ser absolutas y estar fuera de `$LAIR_HOME`. |
| `lair migrate-dirs` | Migra la carpeta anterior `~/lair` a `~/Lair`: mueve los proyectos y registra sus sitios como aparcados. También se ejecuta automáticamente al arrancar si detecta `~/lair`. |
| `lair import-valet` | Importa, **en solo lectura**, `~/.valet/config.json` y `~/.valet/Sites`: registra como sitios aparcados las rutas y dominios que allí aparecen, sin modificar esos archivos ni instalar o ejecutar Valet. |
| `lair new <nombre> [opciones]` | **Comando oficial** para crear un proyecto Laravel. Prepara la BD, escribe `.env` y registra `<nombre>.test`. Sin `--parent`, usa `~/Lair`. |
| `lair project new <nombre> [opciones]` | Alias de `lair new`, mantenido por compatibilidad. |

Cualquier proyecto que aparezca en la carpeta configurada (`~/Lair` por defecto) se detecta automáticamente: la pestaña **Sites** lo muestra como `<nombre>.test` aunque se haya creado con `laravel new` directamente. Se reconocen las carpetas que contengan `artisan` o `composer.json`; los sitios enlazados explícitamente no se sobrescriben. |

```sh
lair project new blog --stack=livewire --database=mariadb --db-name=blog_dev --secure
# La contraseña se pide sin eco; también puede pasar en LAIR_DB_PASSWORD.
```

El asistente de la desktop (**Sites → New project**) y la TUI (`C` en Sitios)
ofrecen las mismas opciones: nombre, carpeta padre (campo de texto), HTTPS,
interfaz, autenticación, framework de pruebas, gestor Node, Git y base de datos.
`laravel/installer` se instala en un `COMPOSER_HOME` aislado de Lair la primera
vez; nunca se toca el Composer global del usuario. Las credenciales viajan por
variables de entorno `DB_*` y nunca aparecen en argumentos ni en los registros
del job. El progreso, el log en vivo y la cancelación reutilizan `job`, `logs` y
`job-cancel`.

Cuando la base de datos es gestionada por Lair (MariaDB, MySQL o PostgreSQL),
el asistente arranca el servicio si está parado y crea la BD antes de que
Laravel ejecute las migraciones. SQL Server no es gestionado por Lair: en ese
caso Lair avisa y solo escribe las credenciales en `.env`.

#### ProjectDirs, `migrate-dirs` e `import-valet`

`config.json` guarda un bloque **`project_dirs`** (tipo **`ProjectDirs`**) con la carpeta por defecto y la lista de carpetas que se escanean:

```json
"project_dirs": {
  "default": "/home/usuario/Lair",
  "dirs": ["/home/usuario/Lair"]
}
```

- **`DefaultProjectsDir` es `~/Lair`** (con L mayúscula), siempre fuera de `$LAIR_HOME`. Si la carpeta no existe, Lair la crea y la registra como aparcada: sus proyectos aparecen en **Sites** como `<nombre>.test` sin que cambie la página.
- Si existe la carpeta anterior `~/lair`, Lair migra automáticamente los proyectos a `~/Lair` y reemplaza sus registros; `lair migrate-dirs` fuerza la migración cuando se necesita. El contenido se conserva y la carpeta antigua no se borra sin confirmación.
- `lair import-valet` lee `~/.valet/config.json` y `~/.valet/Sites` **solo para leer**: añade los sitios aparcados con sus rutas y dominios, sin escribir en esos archivos, sin instalar Valet y sin ejecutarlo como root.
- Las mismas operaciones están en el daemon dentro de la acción `project`, campo `op`: `dirs-list`, `dirs-add`, `dirs-remove`, `dirs-default`, `migrate-dirs` e `import-valet`. En la TUI se agrupan bajo la tecla **`F`**. La acción `site` y el detalle de **Sites** no cambian.

### 3.6 Sitios y HTTPS

| Comando | Descripción |
|---|---|
| `lair park` | Registra el directorio actual como `<nombre>.test`. |
| `lair link <nombre> [ruta]` | Registra un sitio con host explícito (por defecto, el directorio actual). |
| `lair unlink <nombre>` | Quita el sitio del router sin borrar sus archivos. Para borrar también la carpeta usa la acción `site` (`op: delete`, `delete_files: true`) o la tecla `X` de la TUI, siempre tras confirmación. |
| `lair secure [nombre]` | Emite certificado para el sitio y su wildcard con la CA local. Sin nombre, detecta el sitio del directorio actual. |
| `lair isolate php\|node <versión\|--off>` | Fija o quita una versión solo para ese sitio (config.json, prioridad máxima). |

La pestaña **Sites** de la desktop muestra una tarjeta por proyecto con el **nombre del proyecto** y su **host virtual** (`<nombre>.test`) como enlace abrible en el navegador, usando el esquema y puerto configurados en **Web**. Cada tarjeta permite activar/desactivar **HTTPS** del sitio (emite o retira el certificado local), abrir el **detalle del sitio** (las siete pestañas descritas abajo) y quitar el sitio. El botón **Refresh projects** vuelve a escanear la carpeta de proyectos; los cambios se guardan en `config.json` y se aplican al host virtual.

Cada sitio tiene un estado **enabled**. Desactivarlo (`site` con `op: set-enabled` y `enabled: false`) no borra el proyecto ni su registro: Lair retira su host virtual del router web Go. Reactivarlo (`enabled: true`) vuelve a publicarlo desde `config.json`. El toggle está en el detalle de **Sites** de la desktop y en la TUI; desde el Overview se puede pulsar **View generated vhost** (tecla **`V`** en la TUI) para inspeccionar exactamente el contenido generado, que también se obtiene con `op: config`.

Cuando el **root detection** no devuelve `ok` —raíz ausente, carpeta vacía o `index.php` en la raíz en lugar de `public/index.php`— el vhost sirve la página de estado **`not-configured.html`** en lugar de pasar la petición a PHP-FPM. Es una página genérica con el host solicitado, la ruta esperada del front controller **dentro de cada proyecto** (`<raíz>/public/index.php`) y sugerencias (`root_state`, `root_suggestions`); no expone rutas internas de `LAIR_HOME`, tokens ni valores del `.env`.

> **¿Por qué no hay configuración en `/etc`?** El enfoque clásico de los servidores web escribe cada sitio en un árbol del sistema y lo activa con enlaces simbólicos. Lair no sigue ese patrón: el daemon corre como el usuario y no tiene privilegios en caliente, y los hosts virtuales los genera el servidor Go integrado en memoria a partir de `config.json`. Activar o desactivar un sitio cambia su estado en el router, sin crear ni quitar enlaces ni archivos en `/etc`. Así no se toca la configuración de la distro, no se instala ningún paquete web y no hay configuración externa que validar. La decisión y las alternativas están en el [ADR 009](docs/adr/009-virtual-hosts-en-servidor-go.md).

#### Detalle de sitio: siete pestañas

Al abrir un sitio se muestra un detalle con **siete pestañas**, alimentado por la acción `site` del daemon (no hay lógica duplicada en la desktop):

| Pestaña | Qué muestra |
|---|---|
| **General (Overview)** | Framework, tipo, URL, estado **enabled** (con toggle), **raíz detectada** (`root_state`, `root_suggestions`) y su página de estado, **dominios/alias**, HTTPS y avisos (`problem` / `notice`); botón **View generated vhost**. |
| **Entorno** | Entradas del `.env` del sitio: los secretos llegan con `value` vacío y `secret: true`, `set` indica si la clave existe; permite editar valores con `env-set`. |
| **Base de datos** | Driver, host, puerto y nombre de la BD; usuario y contraseña enmascarada; permite cambiar el driver y crear bases de datos. |
| **Correo (Mail)** | Configuración SMTP para el catcher integrado (`127.0.0.1:2525`) y acceso al pane Mail. |
| **Colas (Queues)** | Estado de los servicios (solo lectura) y acceso a los registros. |
| **PHP y Node** | Versión resuelta y su origen (`php_source`, `php_file`, `node_source`, `node_file`), con botones de pin e isolate por sitio. |
| **Registros (Logs)** | Logs del sitio filtrados por origen (`logSources` + `logs`). |

> Los **dominios/alias** y el **root detection** se muestran en General y se editan con la acción `site` (`op: domains`). El estado **enabled** se alterna con `op: set-enabled`: un sitio desactivado no genera vhost. El botón **View generated vhost** (TUI `V`, `op: config`) muestra el host virtual generado, que no contiene secretos. Cuando `root_state != ok` se sirve `not-configured.html` en lugar de pasar la petición a PHP. Los **puertos, el enlace y el dominio** del servidor se consultan en el pane **Web** y en la acción `web`. La carpeta de un sitio **nunca se borra sin confirmar en la interfaz**: `delete` solo elimina archivos con `delete_files: true`, y el motor valida que la ruta registrada sea absoluta, esté fuera de `LAIR_HOME` y no sea un enlace simbólico. `lair unlink` sigue siendo no destructivo. Las variables de entorno se enmascaran en el servidor y los secretos nunca se devuelven en `status`.

#### Acción `site` del daemon

Como `mail`, `dumps` y `debugger`, `site` usa el sobre `{ "action": "site", "payload": { ... } }` del bridge `POST /api` (o el `Caller` en modo offline). El campo `op` selecciona la operación:

```json
{ "action": "site", "payload": { "op": "detail", "name": "mi-app.test" } }
{ "action": "site", "payload": { "op": "env", "name": "mi-app.test" } }
{ "action": "site", "payload": { "op": "env-set", "name": "mi-app.test", "key": "APP_ENV", "value": "local" } }
{ "action": "site", "payload": { "op": "domains", "name": "mi-app.test", "domains": ["www.mi-app.test", "mi-app.local"] } }
{ "action": "site", "payload": { "op": "set-enabled", "name": "mi-app.test", "enabled": false } }
{ "action": "site", "payload": { "op": "config", "name": "mi-app.test" } }
{ "action": "site", "payload": { "op": "delete", "name": "mi-app.test", "delete_files": true } }
```

| `op` | Campos | Respuesta (`SiteResult`) |
|---|---|---|
| `detail` (por defecto) | `name` | `detail`: vista completa del sitio (sin secretos) para el detalle de siete pestañas. |
| `env` | `name` | `env`: entradas de `.env` con los valores secretos enmascarados. |
| `env-set` | `name`, `key`, `value` | `output`: solo el nombre de la clave; el valor nunca se registra. |
| `domains` | `name`, `domains` | `domains`: lista normalizada; actualiza `config.json` y recarga sitios/web. |
| `set-enabled` | `name`, `enabled` | `detail`: vista actualizada con el nuevo `enabled`; aplica sitios/web. Con `enabled: false` se retira el host virtual del router. |
| `config` | `name` | `config`: texto del vhost generado; vacío si el sitio está desactivado. |
| `delete` / `unlink` | `name`, `delete_files` | `output`: `"sitio eliminado"`. Con `delete_files: true` borra también la carpeta. |

La fuente autoritativa del contrato es `SiteRequest` / `SiteResult` en `internal/engine/site.go`, reexportado por `internal/daemon/protocol.go`. El `detail` incluye `enabled` y `root_state`; el router web Go usa esos campos para decidir si publica el host virtual o sirve `not-configured.html`. Los errores están tipados (`ErrInvalidSiteName`, `ErrSiteNotFound`, `ErrInvalidEnvKey`, `ErrUnsafeSitePath`) para que los clientes los clasifiquen sin depender del texto. Los hosts virtuales se derivan de `config.json` y se aplican en caliente al servidor Go integrado, sin archivos de configuración externos.

```sh
cd ~/proyectos/mi-app
lair park                  # https://mi-app.test
lair secure mi-app.test
lair isolate php 8.3       # este sitio usa 8.3 aunque la default sea 8.4
lair isolate php --off
lair unlink mi-app.test    # quita el sitio, conserva los archivos
```

### 3.7 Pines de proyecto

```sh
lair pin php 8.4           # escribe php=8.4 en .lairrc del directorio actual
lair pin node 22           # añade node=22
```

### 3.8 Daemon

```sh
lair daemon --foreground   # primer plano, para desarrollo
```

Flags: `--no-web`, `--no-dns`, `--no-api`, `--no-desktop`, `--desktop-port N`, `--dns-port`, `--http-port`, `--https-port`, `--domain` ( `--no-ui` y `--ui-port N` siguen aceptados como alias). Los puertos web por defecto son **80/443** (con `CAP_NET_BIND_SERVICE` o `setcap`); `--http-port`/`--https-port` permiten usar 8080/8443 u otros sin capacidad. El DNS permanece en `--dns-port 53535` por defecto. Sin `--foreground`, implica que el daemon lo gestiona systemd (`lair@<usuario>.service`).

```sh
lair daemon --foreground --no-web --no-dns
lair daemon --foreground --desktop-port 9800
```

### 3.9 App nativa y lanzador (`lair desktop`)

No hay interfaz web. El daemon expone un **bridge HTTP local** para la app nativa de Lair (Electron + React, en `app/`), en `127.0.0.1:9797` por defecto. `lair desktop` (alias `lair ui`) arranca el daemon si hace falta e imprime los datos de conexión; la app los lee directamente.

```sh
lair desktop
```

- El bridge publica `POST /api` con `{action, payload}` y cabecera `X-Lair-Token`. No sirve HTML, CSS ni JS; solo llama al mismo `Dispatcher` que usa la CLI.
- El token se escribe en `$LAIR_HOME/run/desktop.token` (`0600`) y la URL base en `$LAIR_HOME/run/desktop.url` (`0600`). El token nunca viaja en la URL: se rechaza el fallback por query y se acepta solo desde orígenes locales/Electron (`null`, `localhost`, `127.0.0.1`).
- El proceso principal de Electron lee esos archivos y expone `window.lairDesktop.connect()` al renderer con `contextIsolation`; el renderer no tiene acceso a filesystem.
- La app adopta el estado que devuelve el daemon, usa jobs con polling y no instala nada por sí misma. Cada texto visible es una clave de `app/lang/<código>/app.json` (en, es, cs).
- El bridge se desactiva con `lair daemon --foreground --no-desktop` y su puerto se cambia con `--desktop-port N`.
- El sidebar organiza once panes: **General** (antes *Overview*), Bases de datos, Runtimes, Servicios, Sitios, **Mail**, **Dumps**, **Debugger**, **Logs** y **About** (antes *System*). Los cuatro nuevos son panes del sidebar, **no ventanas flotantes ni una bandeja nativa**: se decidió mantener todo dentro de la misma ventana para no duplicar estado ni atajos.
- El paquete `.deb` instala `/usr/share/applications/lair.desktop` e `/usr/share/icons/hicolor/scalable/apps/lair.svg`. Tras instalar, cierra sesión y vuelve a entrar (o ejecuta `update-desktop-database`) para que el icono aparezca en el menú.

### 3.10 Dashboard de terminal (`lair tui`)

Si prefieres la terminal, `lair tui` (alias `lair dashboard`) abre un panel interactivo construido con **React/Ink y [termcn](https://termcn.dev)**: los mismos diez panes de la navegación lateral que la app de escritorio (**Overview**, Runtimes, Databases, Services, Sites, Mail, Dumps, Debugger, Logs y System), con los mismos textos, i18n (es/en/cs) y acciones del daemon. El bundle se compila en `app/tui/` con componentes de termcn (Card, Badge, Sidebar, KeyValue, Toggle, StatusMessage…) y se incrusta en el binario Go (`internal/assets/tui.mjs`), así que `lair tui` no necesita `node_modules`. Requiere Node 22+ (usa el PATH o el gestionado por nvm); si no está disponible cae al panel anterior de **Charmbracelet** (Bubble Tea, Bubbles, Lip Gloss, Huh, Glamour). Si la shell no ofrece un TTY real (IDE, runners), `lair tui` abre la TUI dentro de un PTY con `script(1)` para que Ink pueda activar el modo raw; en Windows sin `script(1)` muestra el estado en texto plano. Para reconstruirlo tras cambiar la TUI: `cd app && npm run tui:build`. Verifica tipos con `npm run tui:typecheck`.

Además de `n` (Node LTS), `p` (PHP), `m` (MariaDB), `u` (catálogo), `s` (servicios), `r` (refrescar) y `q` (salir), ofrece `d` (predeterminada), `y` (usar versión), `z` (desinstalar), `b` (crear BD), `v` (logs), `x` (cancelar trabajo), `a`/`e`/`o` (aparcar/enlazar/desvincular), `c` (certificado), `g` (pin) e `i` (aislar). Las mayúsculas **`M` (Mail), `D` (Dumps), `B` (Debugger), `E` (detalle de sitio) y `X` (borrar sitio)** abren las superficies avanzadas; no confundirlas con las minúsculas `m` (MariaDB), `d` (default), `b` (crear base de datos) y `e` (enlazar sitio). La tecla **`E`** muestra un subconjunto del detalle de sitio de la desktop —Overview (URL, rutas, PHP, Node, BD, HTTPS, estado de `root`, dominios y avisos) y Environment (valores secretos enmascarados)— reutilizando la acción `site`; `v` abre los registros del sitio. La tecla **`X`** pide confirmación para desvincular el sitio y permite borrar también su carpeta (`delete_files: true`). La tecla **`V`** (mayúscula) muestra el **vhost generado** del sitio seleccionado y la tecla **`t`** (desde el detalle `E`) alterna su estado **enabled/disabled** (con confirmación, `op: set-enabled`): al desactivar, Lair retira el host virtual del router web Go. La tecla `v` sigue abriendo registros y el pane **Logs** añade el filtro por `sources`.

```sh
lair tui          # o: lair dashboard
```

Cuando stdin no es un TTY (CI, pipes), imprime el mismo estado en texto plano y termina, en lugar de dejar una pantalla completa colgada. Usa el mismo `Dispatcher` que la app nativa y la CLI, así que no hay acciones exclusivas de ninguna interfaz; `app/scripts/check-actions.mjs` verifica en CI que cada acción del cliente desktop exista en el daemon. Las instalaciones lanzadas desde la TUI o la app regeneran los shims y el PATH al terminar, de modo que `node -v`/`npm -v` funcionan sin pasos extra.

### 3.11 `exec` (interno, no lo llames a mano)

Cada shim contiene `exec "<lair>" exec <tool> "$@"`. El subcomando oculto `exec` resuelve la versión para el directorio actual y usa `execve`, sin proceso intermediario. Si falta la versión y hay TTY, ofrece instalarla; desde un script, falla con un mensaje claro. Es la pieza que hace que `node`, `php` y `psql` "simplemente funcionen".

### 3.12 Superficies de soporte: Mail, Dumps, Debugger y Logs

Cuatro panes comparten el `Dispatcher` del daemon: ninguno implementa lógica propia en la desktop ni en la TUI. El backend las expone como **tres acciones nuevas** (`mail`, `dumps`, `debugger`) más la extensión de `logs-list` con `sources`. Todas se invocan con el sobre `{ "action", "payload" }` del bridge `POST /api` (o con `lair <acción>` cuando la CLI las exponga; mientras tanto: `verify with lair <action>`).

#### Mail — catcher SMTP

El pane Mail lista el correo saliente capturado por el **SMTP catcher integrado**, que escucha solo en `127.0.0.1:2525` (no se instala el paquete Mailcatcher). Apunta tu `.env` a él:

```env
MAIL_MAILER=smtp
MAIL_HOST=127.0.0.1
MAIL_PORT=2525
MAIL_USERNAME=lair
MAIL_PASSWORD=null
MAIL_SCHEME=null
```

El catcher acepta `AUTH PLAIN` y `AUTH LOGIN` con **cualquier credencial**: la contraseña se ignora y el **usuario elige la bandeja** (normalizado a minúsculas, p. ej. `Mi App` → `mi-app`); sin autenticar se usa `default`. Acepta hasta 1000 destinatarios y 25 MB por mensaje, sin TLS ni relay: nada sale del equipo. Los mensajes se guardan con rename atómico en `<root>/mail/<bandeja>/` y se pueden consultar por bandeja con el campo `inbox`.

Acciones (campo `op` del payload de `mail`): `status`, `start`, `stop`, `list`, `message`, `raw`, `clear`. CLI: `lair mail list`, `lair mail open <id>`, `lair mail raw <id>`, `lair mail clear [inbox]`.

```json
{ "action": "mail", "payload": { "op": "status" } }
{ "action": "mail", "payload": { "op": "list" } }
{ "action": "mail", "payload": { "op": "clear", "confirm": true } }
```

`status` informa si el catcher está activo y cuántos mensajes hay; `list` devuelve los mensajes con `id`, `from`, `to`, `subject`, `fecha` y `size`; `clear` pide confirmación explícita. El cuerpo de cada mensaje se consulta desde el pane al seleccionarlo.

#### Dumps — captura de `dump()` / `dd()`

El pane Dumps recoge el volcado de variables de las peticiones PHP. El agente se inyecta con **`auto_prepend_file` en los pools FPM existentes** de cada sitio (los mismos que usa el sitio; no se crea un pool nuevo) y envía cada volcado al daemon con un **token dedicado** en `run/dumps.token` (`0600`). La captura se puede desactivar en caliente.

Acciones (campo `op` del payload de `dumps`): `status`, `capture-on`, `capture-off`, `list`, `clear`. El campo opcional `site` restringe la operación a un sitio aparcado.

```json
{ "action": "dumps", "payload": { "op": "capture-on", "site": "mi-app.test" } }
{ "action": "dumps", "payload": { "op": "status" } }
{ "action": "dumps", "payload": { "op": "list" } }
{ "action": "dumps", "payload": { "op": "clear", "confirm": true } }
```

`status` muestra si la captura está activa, el token en uso y el número de volcados; `list` devuelve `id`, `site`, `hora`, `size` y una vista previa. `capture-off` desactiva el `auto_prepend_file` sin reiniciar PHP-FPM; `clear` pide confirmación.

#### Debugger — Xdebug bajo demanda

El pane Debugger muestra el estado de Xdebug y permite activarlo solo mientras se depura. **Reutiliza los pools FPM del sitio**: no deja un pool de depuración permanente escuchando. El puerto por defecto del cliente es `9003`.

Acciones (campo `op` del payload de `debugger`): `status`, `start`, `stop`, `enable`, `disable`; `site` y `port` son opcionales.

```json
{ "action": "debugger", "payload": { "op": "status" } }
{ "action": "debugger", "payload": { "op": "start", "site": "mi-app.test", "port": 9003 } }
{ "action": "debugger", "payload": { "op": "stop", "site": "mi-app.test" } }
```

`status` indica si Xdebug está cargado, el puerto y la sesión activa; `start`/`enable` y `stop`/`disable` son los extremos de la misma operación bajo demanda.

#### Logs — `sources` en `logs-list`

`logs-list` acepta ahora un campo `sources` para filtrar por origen y devuelve el origen de cada entrada, de modo que el pane Logs pueda alternar entre registros de PHP, web, servicios, Mail, Dumps y Debugger:

```json
{ "action": "logs-list", "payload": { "sources": ["php", "web", "services", "mail", "dumps", "debugger"] } }
```

```json
{ "items": [ { "name": "php-8.4.log", "source": "php" }, { "name": "mail.log", "source": "mail" } ] }
```

Los nombres de origen son ilustrativos; la lista autoritativa de acciones y payloads está en `internal/daemon/protocol.go`. El contenido de un registro se sigue leyendo con `{ "action": "logs", "payload": { "name": "..." } }`.

### 3.13 Inventario de acciones del daemon (35)

El `Dispatcher` registra **35 acciones compartidas** por desktop, TUI y CLI; `uninstall` y `shutdown` son internas de la CLI/daemon y no cuentan en este inventario.

| Área | Acciones |
|---|---|
| Estado y catálogo | `status`, `versions`, `update`, `bootstrap`, `ensure-shims` |
| Instalación y jobs | `install-php`, `install-node`, `install-service`, `job`, `jobs`, `job-cancel` |
| Registros | `logs`, `logs-list` (con `sources`) |
| Versiones y servicios | `php-use`, `node-default`, `service-use`, `service-restart`, `service-stop`, `service-uninstall`, `ensure-database`, `create-db`, `db-default` |
| Sitios y configuración | `park`, `link`, `unlink`, `secure`, `web`, `project`, `pin`, `isolate`, `restart`, **`site`** (detalle, alias, enable/disable, vhost generado, puertos, root con página de estado y borrado con confirmación) |
| Superficies de soporte | `mail`, `dumps`, `debugger` |

La acción `site` completa el inventario y sigue el mismo patrón de campo `op` que `mail`, `dumps` y `debugger`; sus operaciones (`detail`, `env`, `env-set`, `domains`, `set-enabled`, `config`, `delete`/`unlink`) **no añaden acciones**: el inventario sigue siendo de 35. Su detalle está en [§3.6](#36-sitios-y-https). `app/scripts/check-actions.mjs` verifica en CI que cada acción declarada por el cliente desktop existe en el daemon.

---

## 4. Flujos de trabajo

### 4.1 Proyecto Laravel nuevo

```sh
make build
./lair install
sudo "$LAIR_HOME/setup-privileged.sh"
sudo "$LAIR_HOME/trust-ca.sh" --add
sudo systemctl enable --now lair@$USER

lair php install 8.4 && lair php use 8.4
lair node install                       # LTS v26.13.1
lair services default mariadb           # ya instalada por `lair install`

# Todo en un paso: instalador, BD, .env y sitio *.test
lair project new mi-app --parent ~/proyectos --database=mariadb --secure

cd ~/proyectos/mi-app
lair pin php 8.4 && lair pin node v26.13.1
php -v && composer -V && node -v && npm -v
lair doctor
```

### 4.2 Dos proyectos con PHP distinto

```sh
cd ~/proyectos/legacy && lair pin php 8.1 && php -v   # 8.1
cd ~/proyectos/nuevo  && lair pin php 8.4 && php -v   # 8.4
```

### 4.3 Cambiar de versión de base de datos sin perder datos

```sh
lair services install postgres 18      # instala, no activa
lair services use postgres 18          # activa; los datos se conservan
lair services restart postgres
```

### 4.4 Instalación sin red

```sh
lair install                  # crea shims y PATH; Composer avisa pero no aborta
lair status                   # las versiones ya instaladas siguen funcionando
lair doctor                   # verifica que PATH y shims estén bien
```

---

## 5. Resolución de versiones

El shim elige versión con esta precedencia (de mayor a menor prioridad):

| Herramienta | Orden |
|---|---|
| PHP | `isolate` (sitio) → `.lairrc` (`php=`, antes `.flockrc`) → default global → última instalada |
| Node | `isolate` → `.lairrc` (`node=`, antes `.flockrc`) → `.nvmrc` → `.node-version` → default global → última instalada |
| Servicios | `defaults[id]` del `config.json` (una versión activa por puerto) |

`version.ExactOrLatest` acepta `22` (última de la major), `22.11.0` (exacta o última de la major) y `8.4` (última de la minor).

---

## 6. Estructura de datos

```
<LAIR_HOME>/
├── bin/                     shims ejecutables (0755) y binario lair
├── tools/                   composer.phar verificado
├── php/<minor>/             bin/, sbin/php-fpm, etc/php.ini, etc/pool.d/, run/, lair.json
├── node/<version>/bin/      node, npm, npx
├── node/global/<major>/     prefijo global de npm por major
├── services/<id>/<version>-build<N>/    binarios
├── services/<id>/data/<línea>/          datos (postgres usa el major)
├── log/                     logs de instalación y servicios
├── run/                     sockets unix y token de la API (0700)
├── certs/                   CA, clave (0600) y certificados por sitio
├── web/pages/               not-configured.html, página de estado si root_state != ok (0644)
├── cache/binaries/          caché de artifacts
├── config.json              estado persistente (solo daemon)
├── setup-privileged.sh / trust-ca.sh / trust-ca-user.sh / revert-privileged.sh
```

---

## 7. Mapa de módulos

La lógica compartida está en `internal/engine`; el daemon y la CLI son dos frentes del mismo motor. Detalle completo en [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

| Paquete | Responsabilidad | API principal |
|---|---|---|
| `cmd/lair` | Único `main`: CLI, daemon y `exec`. | `main()` |
| `app` | Rutas e identidad. | `Resolve()`, `Platform()` |
| `assets` | Clave pública, catálogo de ejemplo firmado y logos embebidos (`go:embed`). | `PublicKey()`, `ExampleCatalog()` |
| `version` | Parser semver y resolución. | `Parse`, `ExactOrLatest` |
| `manifest` | Catálogo firmado, serial, caché 6 h, `OnChange`, marcas LTS. | `Catalog.Refresh/Load`, `Verify`, `LTSOf` |
| `download` + `installer` | Descarga `.part`, SHA-256, `safeJoin`, jobs. | `Manager.Start`, `Job.Status/Cancel` |
| `config` | Store atómico de `config.json`. | `Store.Update/View` |
| `php` | Versiones paralelas, php.ini, FPM, Xdebug. | `EnsureINI`, `WriteFPM`, `Env` |
| `composer` | Phar verificado por SHA-256. | `Installer.Ensure`, `Command` |
| `node` | Builds oficiales, resolución, prefijo npm. | `ResolveVersion`, `Env` |
| `shims` | Wrappers y perfiles reversibles. | `Generate`, `EnsureProfiles` |
| `services` | Specs puros, datos, puertos. | `Build`, `DataPath`, `CheckPort` |
| `supervisor` | Backoff, logs, grupos de procesos. | `Supervisor.Run/Update` |
| `platform` | systemd, polkit, resolved, CA, plan privilegiado. | `BuildPlan`, `InstallTrust` |
| `dns` / `pki` / `fastcgi` / `web` | `*.test`, TLS local, FPM, router HTTP. | `Server`, `IssueSite`, `Router.Match` |
| `daemon` | API unix+token y protocolo de acciones. | `Server`, `Client`, `Dispatcher`, `Runtime` |
| `engine` | Orquestación compartida; detalle de sitio en `engine/site.go`. | `InstallPHP/Node/Service`, `LatestLTS`, `EnsureDefaultDatabase`, `Status`, `ResolveTool`, `Bootstrap`, `SiteControl` (detalle, entorno enmascarado, dominios, enable/disable, vhost generado y borrado seguro) |
| `project` | Asistente de proyecto Laravel compartido por desktop, TUI y CLI. | `Detect`, `RunLaravel`, `LaravelArgs` |
| `desktopapi` | Bridge HTTP local para la app nativa; `X-Lair-Token`, sin estáticos. | `New`, `ServeHTTP` |
| `nvm` | Carga de `nvm.sh` con argv validado. | `nvm.Install`, `nvm.Exec` |
| `mail` | SMTP catcher integrado en `127.0.0.1:2525`; bandeja de mensajes para el pane Mail. | `Control`, `List`, `Clear` |
| `dumps` | Captura de `dump()`/`dd()` mediante `auto_prepend_file` en los pools FPM y token dedicado. | `Control`, `List`, `Clear` |
| `cli` | Comandos, modo offline, `exec`, `doctor`, TUI Charmbracelet. | `Run`, `runExec`, `runTUI` |

El debugger vive en `engine`: activa Xdebug bajo demanda reutilizando los pools FPM del sitio, sin dejar un pool permanente. El detalle avanzado de sitios (detalle completo, entorno enmascarado, dominios/alias, root detection y borrado seguro) vive en `engine/site.go` y se expone mediante la acción `site`; los hosts virtuales del servidor Go se derivan de `config.json` y se publican en el estado Web. Las acciones `mail`, `dumps`, `debugger` y `site` se registran junto al resto en `internal/daemon/protocol.go`.

---

## 8. Daemon, DNS y HTTPS

```
mi-app.test:80 / :443
  → systemd-resolved (DNS=127.0.0.1#53535, Domains=~test; fallback automático)
  → dnsmasq seguro 127.0.0.1:53535 (solo *.test) → 127.0.0.1
  → servidor web Go integrado (router por Host)
  → host virtual generado desde config.json (dominios/alias)
  → estáticos en public/   OR   FastCGI → socket de PHP-FPM
```

| Componente | Dirección |
|---|---|
| API local | `run/api.sock` + `run/api.token` (`0600`) |
| Bridge de la app nativa | `127.0.0.1:9797/api` + `run/desktop.url` / `run/desktop.token` (`0600`) |
| DNS `*.test` | dnsmasq en `127.0.0.1:53535` (puerto 53 global no usado; vía drop-in de `systemd-resolved`) |
| HTTP / HTTPS (por defecto) | `127.0.0.1:80` / `127.0.0.1:443` (requiere `CAP_NET_BIND_SERVICE` o `setcap`; configurable a 8080/8443) |
| SMTP catcher (Mail) | `127.0.0.1:2525` (solo loopback) |
| Agente de dumps | `run/dumps.token` (`0600`), inyección por `auto_prepend_file` en pools FPM |
| Xdebug (Debugger) | `127.0.0.1:9003` bajo demanda, reutilizando los pools FPM del sitio |

Los puertos, la dirección de enlace, el dominio y HTTPS se configuran desde **Configuración de Lair**, el botón de la pestaña **Sitios** de la desktop; cuando está activo, Lair emite certificados con la CA local para cada sitio y el servidor Go sirve TLS con redirección HTTP→HTTPS. No hay configuración externa que validar ni paquetes que instalar: el router web Go aplica los cambios en caliente. Los dominios y alias por sitio se registran en el host virtual y en el SNI, siempre restringido a los nombres registrados. Un sitio con `enabled: false` **no genera host virtual**: se retira del router y reactivarlo lo recrea desde `config.json`. El vhost generado se puede inspeccionar con **View generated vhost** (TUI `V`, `op: config`) y no contiene secretos. Si `root_state != ok`, el servidor Go sirve `not-configured.html` en lugar de pasar la petición a PHP-FPM; la página solo muestra el host y la ruta esperada del front controller dentro del proyecto, sin exponer rutas internas de `LAIR_HOME`, tokens ni secretos. Detalle y decisiones en [§3.6](#36-sitios-y-https) y en el [ADR 009](docs/adr/009-virtual-hosts-en-servidor-go.md).

El daemon **no corre como root** y Lair **no ejecuta Valet como root**: la unidad usa `User=%i` y `NoNewPrivileges=yes`. Los puertos web por defecto son **80/443**, por lo que el binario necesita `CAP_NET_BIND_SERVICE`: lo concede el plan privilegiado en la unidad (`AmbientCapabilities=CAP_NET_BIND_SERVICE`) o, si lo prefieres, `sudo setcap 'cap_net_bind_service=+ep' "$(command -v lair)"`. Si no quieres dar esa capacidad, usa puertos altos (8080/8443) desde Configuración de Lair o los flags `--http-port`/`--https-port` del daemon. Los servicios siguen ejecutándose como el usuario y solo escuchan en loopback.

El DNS usa **dnsmasq en `127.0.0.1:53535`**, nunca el puerto 53 global. El drop-in de `systemd-resolved` fija `DNS=127.0.0.1#53535` y `Domains=~test`, así que solo `*.test` se envía al resolver local; si el resolved o dnsmasq fallan, hay **fallback automático** al DNS del sistema y el plan guarda una copia de seguridad de la configuración previa con reversión mediante `revert-privileged.sh`. Verificación rápida:

```sh
resolvectl status | grep -A2 '~test'     # DNS=127.0.0.1#53535 para ~test
ss -lunp | grep 53535                    # dnsmasq escucha en loopback
dig @127.0.0.1 -p 53535 mi-app.test      # responde 127.0.0.1
lair status                              # puertos 80/443 y estado del DNS
lair doctor                              # diagnóstico general
```

> **Atención con `bind`:** el valor por defecto es `127.0.0.1`, pero el campo *Bind* de Configuración de Lair con valor `0.0.0.0` expone los sitios a **toda la LAN**, no solo a tu equipo. El servidor Go integrado respeta este valor y enlaza la dirección elegida tanto en HTTP como en HTTPS. Usa `0.0.0.0` solo en redes de confianza y protégelo con un firewall: la CA local y los certificados `*.test` no son identidad pública. Volver a `127.0.0.1` restaura el comportamiento aislado. La API del daemon, el bridge de la app nativa y el DNS siguen enlazando siempre en loopback.

---

## 9. Seguridad

- La confianza de los binarios depende **solo de la firma Ed25519** embebida, nunca del mirror.
- El resolver local es **dnsmasq en `127.0.0.1:53535`**, no un DNS global en el puerto 53: `Domains=~test` limita el reenvío a `*.test`, con fallback automático al DNS del sistema y copia de seguridad/reversión del drop-in previo.
- `lair import-valet` lee `~/.valet/config.json` y `~/.valet/Sites` **en solo lectura**; Lair no instala, modifica ni ejecuta Valet, y nunca lo ejecuta como root.
- Para enlazar 80/443 sin root se usa `CAP_NET_BIND_SERVICE` en la unidad o `setcap` sobre el binario; también puedes volver a 8080/8443 si no quieres conceder esa capacidad.
- Descarga a `.part` + SHA-256 en streaming + `safeJoin`; builds *yanked* rechazados y reemplazados automáticamente.
- `run/` es `0700`; los tokens de la API y del bridge de la app nativa son `0600` y se comparan en tiempo constante. El token del bridge viaja en la cabecera **`X-Lair-Token`**, nunca en la URL (ni en query ni en fragmento); se rechazan los fallbacks por query y solo se aceptan orígenes locales/Electron.
- El agente de dumps usa un **token dedicado** en `run/dumps.token` (`0600`), distinto del token de la API. El SMTP catcher escucha solo en `127.0.0.1:2525`; la captura de dumps se puede desactivar (`capture-off`), Xdebug se activa solo bajo demanda y las operaciones `clear` piden confirmación. Detalle en [`docs/SECURITY.md`](docs/SECURITY.md).
- El detalle de sitio **enmascara las variables de entorno** (los secretos llegan con `value` vacío y `secret: true`) y los secretos **nunca aparecen en `status`** ni en los payloads de la acción `site`: `env-set` solo informa del nombre de la clave. El borrado de la carpeta de un sitio requiere `delete_files: true` **y confirmación en la interfaz** (TUI `X`); `unlink` sigue siendo no destructivo. El motor además rechaza rutas relativas, dentro de `LAIR_HOME` o enlaces simbólicos. Los hosts virtuales se generan en memoria desde `config.json`, sin archivos de configuración externos. Detalle en [`docs/SECURITY.md`](docs/SECURITY.md).
- El host virtual generado solo contiene rutas, sockets, certificados, dominios/alias y puertos: **ningún secreto ni valor del `.env`**. Desactivar un sitio retira su host virtual del router, y la página de estado `not-configured.html` (cuando `root_state != ok`) solo muestra el host y la ruta esperada del front controller **dentro del proyecto**: no expone rutas de `LAIR_HOME`, tokens ni variables de entorno. No se escribe en `/etc`, así que no se amplía la superficie del sistema. Detalle en [`docs/SECURITY.md`](docs/SECURITY.md).
- `certs/ca-key.pem` permite emitir certificados válidos en **todo el sistema** si la CA está instalada: trátalo como secreto.
- Los scripts privilegiados no se ejecutan automáticamente; el usuario los revisa y los lanza con `sudo`.
- No hay sandbox: los servicios son procesos normales del usuario. Detalle en [`docs/SECURITY.md`](docs/SECURITY.md).

---

## 10. Solución de problemas

| Síntoma | Causa | Solución |
|---|---|---|
| `node: command not found` tras `lair node install` | Shims/PATH no configurados. | `lair doctor --fix` → abrir terminal nueva o `source ~/.bashrc`. |
| `npm` usa otro Node | Otra versión antes en PATH. | `echo $PATH`: `<LAIR_HOME>/bin` debe ir primero. |
| `lair install` tarda mucho | Composer sin red. | Ya no bloquea: crea los shims primero y avisa. Reintenta con red. |
| Shims apuntan a un binario borrado | `lair install` se hizo con un binario temporal. | Instala `lair` en ruta estable y vuelve a ejecutar `lair install`. |
| `permission denied` en el puerto web | Falta `CAP_NET_BIND_SERVICE` para 80/443 o hay otro proceso enlazado. | Concede la capacidad con `sudo setcap 'cap_net_bind_service=+ep' "$(command -v lair)"` o usa puertos altos desde Configuración de Lair (8080/8443). |
| `*.test` no resuelve | Drop-in de `systemd-resolved` o dnsmasq no aplicados; fallback activo. | `sudo "$LAIR_HOME/setup-privileged.sh"`, `resolvectl status | grep ~test` y `ss -lunp | grep 53535`. Para volver atrás, `sudo "$LAIR_HOME/revert-privileged.sh"` restaura la copia de seguridad. |
| Los proyectos siguen en `~/lair` o no aparecen en Sites | Migración pendiente o carpeta no registrada. | `lair migrate-dirs` y `lair project dirs list`; comprueba que `~/Lair` exista y esté aparcada con `lair status`. |
| Quieres importar un Valet existente | Hay un `~/.valet/config.json` / `~/.valet/Sites`. | `lair import-valet` (solo lectura; no instala ni ejecuta Valet). |
| Certificado no confiable | Falta confianza en navegador. | `"$LAIR_HOME/trust-ca-user.sh" --add` y reinicia el navegador. |
| `no catalog loaded` | Sin caché y sin red. | `lair update` con red; lo ya instalado sigue funcionando offline. |
| `make test-race` falla | Falta `gcc`/cgo. | Usa `make test`. |

---

## 11. Más documentación

- [`RELEASE_NOTES.md`](RELEASE_NOTES.md) — changelog detallado de la versión `v1.0.0`.
- [`docs/CLI.md`](docs/CLI.md) — referencia completa de comandos y variables de entorno.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — interfaces y flujos internos.
- [`docs/PHASES.md`](docs/PHASES.md) — estado de las fases 1–6 (todas implementadas) y decisiones por fase.
- [`docs/SECURITY.md`](docs/SECURITY.md) — nota de riesgos y superficie privilegiada.
- [`docs/php.md`](docs/php.md) — instalación, configuración y extensiones de PHP.
- [`docs/node.md`](docs/node.md) — instalación de Node por mayor y prefijos globales de npm.
- [`docs/project-versions.md`](docs/project-versions.md) — pin/aislamiento y resolución de versiones por proyecto.
- [`docs/adr/`](docs/adr/) — decisiones de arquitectura (catálogo firmado, CA local, bridge desktop, servidor Go integrado, acción `project`, uninstall/validación de rutas, panes de soporte en desktop/TUI, detalle de sitio en pestañas, hosts virtuales del servidor Go, [servidor Go como único web layer](docs/adr/010-servidor-go-como-unico-web-layer.md) y [carpeta `~/Lair` y DNS dnsmasq seguro con fallback](docs/adr/011-carpeta-lair-y-dns-dnsmasq-seguro-con-fallback.md)).

Logos: `assets/logo-dark.svg` (contenedor negro, L roja degradada) y `assets/logo-light.svg` (contenedor blanco, L negra), con copias embebidas en `internal/assets/` para `go:embed`.
