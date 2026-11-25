# Lair v1.0.4

Esta entrega cierra la migración del dashboard de terminal a Go/Charm e incorpora
las mejoras de funcionamiento, seguridad y superficies identificadas al comparar
Lair con Lerd. El binario sigue siendo único, nativo y sin contenedores.

## Cambios principales

### Dashboard de terminal

- El dashboard pasa de React/Ink a una TUI Go con Bubble Tea v2, Lip Gloss y Huh.
- Se retiran `app/tui/`, los shims TUI y el script de demo; no quedan referencias
  rotas en `package.json`, CI ni i18n.
- Mail y Dumps cargan ahora con `op: list` (antes `status`, que no devuelve
  mensajes ni eventos y dejaba ambos panes vacíos).

### Compilación y releases

- `lair version` (o `--version`, `-V`) muestra versión, commit y fecha del binario.
- `make build` y `scripts/build-dist.sh` compilan con `-buildmode=pie -trimpath`
  e inyectan los metadatos con `-ldflags -X`.
- `go.mod` fija `toolchain go1.27.4` y `build-dist.sh` se niega a publicar desde
  un árbol sucio (`LAIR_ALLOW_DIRTY=1` solo para pruebas locales).
- CI ejecuta `make build`, comprueba `lair version` y añade `govulncheck`
  no bloqueante.
- La clave pública Ed25519 del catálogo puede sustituirse en compilación con
  `LAIR_PUBLIC_KEY_HEX`; no hay override en tiempo de ejecución.

### Seguridad y fiabilidad

- El motor exige `confirm: true` para vaciar correo y dumps; la CLI lo envía con
  `--yes`/`-y` y la app tras el diálogo de confirmación.
- `lair doctor --dry-run` muestra la reparación sin aplicarla.
- El daemon limita `GOMAXPROCS` a 4 y fija un techo de heap (256 MiB por defecto,
  configurable con `LAIR_DAEMON_MEMORY_LIMIT_MIB`).
- Los servicios TypeSense y RustFS permiten sobreescribir sus credenciales de
  ejemplo por variables de entorno.

### Bases de datos

- `lair db snapshots|snapshot|restore|import|export` con SQLite, PostgreSQL y
  MariaDB/MySQL; sidecar JSON con SHA-256, retención configurable y permisos
  `0700`/`0600`.
- `restore` e `import` exigen confirmación y crean un snapshot automático previo.
- Borrar archivos de un sitio o datos de un servicio con snapshots activados
  también prescribe un snapshot.
- Las rutas de importación se validan: solo bajo `snapshots/` o el home del
  usuario, sin enlaces simbólicos.

### Worktrees de git

- `lair worktree list|sync|provision|prune` registra ramas como sitios
  `<rama>.test`, con hash corto contra colisiones y dominios bajo 63 caracteres.
- Los sitios creados llevan marca `Worktree`: Lair nunca secuestra un sitio
  enlazado a mano ni borra bases de datos al podar ramas.
- El escaneo automático al arrancar el daemon es opcional (`worktrees.auto`,
  desactivado por defecto).

### Runtimes y observabilidad

- `lair php detect [dir]` recomienda la minor instalada que satisface
  `.php-version` o `require.php` de `composer.json`.
- El agente PHP captura `QueryExecuted` con duración y un id de petición
  inyectado por FastCGI.
- `dumps list` incorpora `insights` (consultas lentas, N+1 por fingerprint y
  agregados) y `timeline` por petición. Se corrigió el truncado que conservaba
  los eventos más antiguos pese a ordenarlos de más reciente a más antiguo.

### MCP

- `lair mcp` expone un servidor Model Context Protocol sobre stdio (JSON-RPC
  2.0, una mensaje por línea) con seis herramientas de solo lectura:
  `lair_status`, `lair_versions`, `lair_jobs`, `lair_logs`,
  `lair_services_health` y `lair_site_data`.
- Documentado en `docs/mcp.md`, con aviso de que hereda el token local del
  daemon y puede leer estado, logs y entorno de los sitios.

## Verificación

- `go test ./...`, `go vet ./...` y `gofmt` sin errores.
- App Electron: `npm test` (99 tests), `typecheck`, `i18n:check`, `actions:check`
  y `build`.
- REA sobre el binario publicado: `pie=enabled`, `nx=enabled`,
  `stripped=enabled` y metadatos `BuildVersion/BuildCommit/BuildDate`.
- Servidor MCP probado con `initialize` y `tools/list`.

## Notas

- La clave incluida en el catálogo de ejemplo sigue siendo la de desarrollo;
  sustitúyela en compilación con `LAIR_PUBLIC_KEY_HEX` antes de distribuir este
  binario a usuarios finales.
- Los worktrees y los snapshots están desactivados por defecto donde escriben
  estado nuevo; actívalos de forma explícita.
