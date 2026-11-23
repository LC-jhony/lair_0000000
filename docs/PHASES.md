# Notas por fases

> **Estado: fases 1–6 implementadas y testeadas.** Este documento es un registro histórico de decisiones por fase; no describe trabajo pendiente. Para el detalle publicado de cada entrega, consulta la sección «Novedades por fases» de [`RELEASE_NOTES.md`](../RELEASE_NOTES.md). Las decisiones de arquitectura vigentes están en [`adr/`](adr/).

## Fase 1 — Catálogo y descarga verificada

- `internal/manifest`: manifiesto `schema/channel/serial/generated/expires/packages`, `.sig` base64, clave pública embebida, rechazo de firma inválida, canal distinto, esquema desconocido, expirado y serial menor por canal.
- `internal/download`: URLs base con fallback, descarga a `.part`, SHA-256 en streaming, extracción con `safeJoin` y verificación de ejecutables esperados.
- `internal/installer`: un job por id, fases `downloading`/`installing`, progreso, cancelación, logs y hook `OnSuccess`.
- Builds *yanked* no se ofrecen en `Versions()` ni se instalan; `NeedsReplacement` detecta el build corregido.

Tests: verificación de manifiesto, resolución de versiones, extracción segura.

## Fase 2 — PHP, Composer, Node y shims

- PHP 8.1–8.5 en paralelo bajo `php/<minor>/`; `php.ini` se preserva si ya existe; FPM por socket unix; Xdebug en pool separado; marcador `lair.json` (se sigue leyendo el legacy `flock.json`).
- Composer: `composer.phar` se acepta solo si su SHA-256 coincide con `getcomposer.org/.../composer.phar.sha256`. Se ejecuta siempre como `<php> tools/composer.phar ...` con `argv[0]` absoluto.
- Node: builds oficiales por manifiesto; resolución `isolated > .lairrc (legacy .flockrc) > .nvmrc/.node-version > default > newest`; prefijo global de npm por major.
- Shims: `exec "<binario>" exec <tool> "$@"`; perfiles bash/zsh/fish/`environment.d` con marcadores idempotentes y reversibles.

Tests: resolución de PHP/Node, verificación de Composer, generación de perfiles y shims.

## Fase 3 — Servicios y supervisor

- Specs para PostgreSQL 14–18, MySQL, MariaDB, MongoDB, Valkey/Redis, Meilisearch, Typesense y RustFS. Todos escuchan en `127.0.0.1`; MySQL/MariaDB validan `mysqld-auto.cnf` para impedir que un config externo mueva el puerto o el bind.
- Datos en `services/<id>/data/<línea>/`; PostgreSQL separa por major. Conservación por defecto; `--delete-data` es explícito.
- `create-db` y clients shimeados (`psql`, `mysql`, `redis-cli`, ...).
- Supervisor: backoff exponencial, último error, cola de log de 50 líneas, comparación de Specs, parada limpia y señales al grupo de procesos.

Nota: se corrigió un deadlock en `Supervisor.Run`: registraba logs llamando a `RecordLog` con el mutex ya tomado; ahora usa `appendTail` inline.

Tests: Specs de servicios, init de datos, puertos, supervisor con runner falso.

## Fase 4 — Daemon, DNS y HTTPS

- `internal/daemon`: API JSON sobre socket unix con token `0600`; acciones tipadas en `protocol.go`; `Runtime` arranca API + DNS + web + supervisores y atiende señales.
- `internal/dns`: DNS UDP en `127.0.0.1:53535`; responde A/AAAA para `*.test` y reenvía el resto a los nameservers de `/etc/resolv.conf`.
- `internal/pki`: CA ECDSA P-256 de 10 años con clave `0600`; certificados por sitio con SANs base + wildcard.
- `internal/web`: router por Host (incluye subdominios), archivos estáticos en `public/`, proxy FastCGI, TLS dinámico y detección de conflictos en los puertos configurados (8080/8443 por defecto).
- `internal/fastcgi`: cliente mínimo RESPONDER sobre unix socket.
- `internal/platform`: unidad `lair@<usuario>.service`, regla polkit solo para la unidad propia, drop-in de `systemd-resolved`, trust store y plan privilegiado renderizado como script único.
- `internal/cli`: todos los comandos requeridos, modo daemon/offline, `exec` oculto con prompt TTY y render de los scripts de privilegios/reversión.

Tests: API con token (incluye rechazo de unauthorized), DNS codec/servidor, PKI, FastCGI, router web, generación del plan privilegiado, dispatcher y CLI.

## Fase 5 — App nativa y lanzador

- Se elimina la SPA embebida (`internal/ui`). `internal/desktopapi` publica solo `POST /api` en `127.0.0.1:9797`: token en la cabecera `X-Lair-Token` (comparación en tiempo constante), sin query ni fragmento, CORS restringido a orígenes locales/Electron y sin HTML/CSS/JS.
- `app/` contiene la app Electron + React + TypeScript: shell con sidebar, componentes base, tema claro/oscuro, i18n en/es/cs, cliente del bridge y `FakeDaemon` con fixtures. El renderer recibe URL y token por IPC (`contextIsolation`), sin filesystem.
- `lair desktop` (alias `lair ui`) arranca el daemon si hace falta y lee `run/desktop.url` y `run/desktop.token` (`0600`). El `.deb` instala el lanzador `.desktop` y el icono.
- Se mantiene la paridad de acciones con la CLI y la TUI; `app/scripts/check-actions.mjs` falla si el cliente declara una acción ausente en `internal/daemon/protocol.go`.
- Tests: autenticación, CORS, llamadas al `Dispatcher` y ausencia de interfaz estática.

## Fase 6 — Web configurable, proyectos Laravel y calidad

- `internal/web` genera los hosts virtuales de los sitios aparcados en el servidor Go integrado, con FastCGI a PHP-FPM y certificados de la CA local (SNI restringido a sitios aparcados). Los cambios se aplican en caliente desde `config.json`, sin archivos de configuración externos que validar; **no se instala ningún servidor web**.
- Un único action `web` configura puertos HTTP/HTTPS (8080/8443 por defecto), HTTPS, `domain` y `bind` para desktop, TUI y CLI. El servidor Go respeta `bind`: `0.0.0.0` expone la LAN, `127.0.0.1` es el aislado por defecto.
- `internal/project` implementa la acción `project` compartida: `lair project info` / `lair project new` y los asistentes de desktop y TUI. `laravel/installer` se aísla en `COMPOSER_HOME`; credenciales por `DB_*`, nunca en argv ni en logs. SQL Server no es gestionado por Lair: se documenta la limitación (solo se escriben credenciales en `.env`).
- `internal/nvm` carga `nvm.sh` con argv validado; toda gestión de Node pasa por `lair nvm ...`, sin `curl | bash` automático.
- Internacionalización completa (en/es/cs) con `npm run i18n:check`, TUI Charm v2 con ayuda Glamour y grabación VHS, y correcciones de timeouts y filtrado de stderr de PHP-FPM.
- Calidad: CI en `.github/workflows/ci.yml` (Go + app con Node 22, sin publicar releases), `.golangci.yml`, `make lint` y `make coverage`, ADRs en `docs/adr/` y documentación (README, SECURITY, CLI, ARCHITECTURE) reconciliada.

Tests: enrutado web y hosts virtuales, web settings (bind, puertos y dominio), acción `project`, nvm con argv, i18n y paridad de acciones.
