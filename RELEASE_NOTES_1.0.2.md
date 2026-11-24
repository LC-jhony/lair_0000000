# Lair v1.0.2

Correcciones de creación de proyectos, catálogo firmado y servicios. Mantiene
compatibilidad con `v1.0.1`; no cambia el esquema de `config.json`.

## Proyectos Laravel

- **Corregido `Application already exists!` al crear proyectos SQLite.** Lair
  creaba `database/database.sqlite` antes de `laravel new`, por lo que el
  instalador rechazaba la carpeta y el job fallaba siempre. El archivo se crea
  ahora **después** del instalador, como respaldo (`internal/engine/project.go`).
- **Contraseña de base de datos vacía permitida.** El root local de MariaDB
  instalado por Lair no tiene contraseña; la UI, la TUI y el daemon ya no la
  exigen (`internal/project/types.go`, `app/src/panes/ProjectDialog.tsx`,
  `internal/cli/tui_actions.go`).
- **Valores por defecto coherentes por motor:** PostgreSQL usa `postgres`
  (antes `root`), el puerto se toma del servicio instalado y el asistente
  adopta el motor configurado (`default_database`). Verificado con MariaDB y
  SQLite usando el instalador real.
- **`lair new` y `lair project new`:** el nombre ya no se confunde con el valor
  de una opción (`lair new --database mariadb blog` funciona), y `--help` en
  cualquier posición muestra ayuda sin crear nada.
- `lair import-valet --help` y `lair migrate-dirs --help` muestran ayuda sin
  modificar `config.json`.
- `lair project info` informa del motor configurado; el aviso de Composer
  explica que el instalador se instala en un `COMPOSER_HOME` aislado.
- **No hace falta `composer global require laravel/installer`:** Lair lo
  instala automáticamente en `$LAIR_HOME/tools/laravel-global` la primera vez
  que creas un proyecto.

## Catálogo firmado

- El mirror por defecto apunta al repositorio real
  (`raw.githubusercontent.com/LC-jhony/lair/main/catalog`). Se retiraron
  `cdn.lair.sh` y `lair-builds.github.io`, que no existen.
- Los errores de `fetch` informan de **todos** los mirrors fallidos, no solo
  del último. El catálogo embebido firmado sigue siendo el fallback offline.

## Servicios

- Redis y Valkey comparten el puerto 6379: el asistente sugiere el siguiente
  libre (por ejemplo 6380) y el daemon lo elige automáticamente cuando no se
  pasa `--port`. Se libera el puerto al desinstalar la última versión.
- Corregido el duplicado de la tarjeta SQLite en la pestaña Servicios.
- MySQL ya no se muestra como servicio instalable (no hay build firmado;
  MariaDB es el motor compatible). El driver PDO `mysql` sigue disponible para
  proyectos Laravel.

## Documentación

- README, `docs/CLI.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`,
  `docs/node.md`, `docs/project-versions.md`, `app/README.md` y varios ADR:
  `project_dirs` como array, CORS con origen `null` rechazado, comandos
  obsoletos, rutas de nvm, importación de Valet y publicación de releases.
- Nueva sección «De la app de escritorio a la terminal» y casos nuevos en
  «Solución de problemas».

## Verificación

- `go test ./...` y `go vet ./...` correctos.
- `npm test` (92 pruebas), `npm run typecheck`, `i18n:check` y `actions:check`
  correctos.
- Creación real con Laravel Installer 5.33.0: SQLite y MariaDB con contraseña
  vacía, base de datos y migraciones completas.
- `lair update` devuelve «El catálogo firmado ya está actualizado» (HTTP 200).

Los hashes de los artefactos publicados están en `SHA256SUMS` del release.
