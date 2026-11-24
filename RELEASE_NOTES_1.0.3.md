# Lair v1.0.3

Corrección posterior a v1.0.2. No cambia el esquema de `config.json`.

## Proyectos

- **El driver `mysql` funciona con MariaDB.** El catálogo firmado no publica
  build de MySQL, por lo que `lair new --database=mysql` fallaba con
  `mysql has no active version`. Ahora, si no hay MySQL instalado pero sí
  MariaDB, Lair usa el servicio MariaDB para crear la base de datos y ajusta
  el puerto; `.env` conserva `DB_CONNECTION=mysql` (compatible con MariaDB).
- **Mensaje de validación de nombre de base de datos más claro** en la
  interfaz: debe empezar por letra o guion bajo y usar solo letras, números y
  guiones bajos (sin guiones). El valor por defecto `<nombre>_dev` ya cumple.

## Verificación

- `go test ./...`, `npm test` (92 pruebas), typecheck, `i18n:check` y
  `actions:check` correctos.
- Creados y migrados proyectos reales con SQLite, MariaDB (contraseña vacía)
  y el driver `mysql` sobre MariaDB. Bases y sitios de prueba eliminados.

Nota: v1.0.2 ya corrigió el error `Application already exists!` de SQLite; si
sigues viéndolo, reinicia el daemon para que cargue el binario nuevo
(`pkill -x lair && lair ui`).

Los hashes de los artefactos están en `SHA256SUMS` del release.
