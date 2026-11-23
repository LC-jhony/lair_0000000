# ADR 008 — Detalle de sitio en pestañas

- Estado: Aceptada (texto actualizado por el [ADR 010](010-servidor-go-como-unico-web-layer.md) en lo relativo al web layer)
- Fecha: 2026-11-18

## Contexto

La página **Sites** de la desktop mostraba tarjetas con acciones rápidas (HTTPS, configuración básica, desvincular), pero obligaba a abrir diálogos separados para cualquier dato avanzado del sitio. El flujo diario necesita consultar y editar, en un mismo lugar, los dominios y alias, los puertos, la raíz del proyecto, las versiones asignadas, la base de datos, el entorno y el host virtual generado.

La TUI debe ofrecer las mismas operaciones que la desktop sin duplicar lógica: las reglas ya viven en el `Dispatcher` del daemon. El servidor Go integrado publica los hosts virtuales en caliente a partir de `config.json`, sin archivos de configuración externos que validar.

## Decisión

Añadir un **detalle de sitio con siete pestañas** —General/Overview, Entorno, Base de datos, Correo, Colas, PHP y Node y Registros— dentro de la página Sites de la desktop, servido por una única acción del daemon, **`site`**, con un campo `op` en el payload. La TUI abre un **subconjunto** (Overview + Environment) con la tecla **`E`** (mayúscula, sin colisionar con `e` = enlazar) y añade **`X`** para borrar el sitio con confirmación, reutilizando la misma acción.

- `site` expone operaciones con `SiteRequest` (`op`, `name`, `key`, `value`, `domains`, `delete_files`): `detail` (vista completa), `env` (entradas de `.env` enmascaradas), `env-set` (actualiza una clave sin devolver su valor), `domains` (alias por sitio), `set-enabled` y `config` (definición del host virtual generado), y `delete`/`unlink` (borrado opcional de archivos con `delete_files: true`). `lair unlink` conserva su comportamiento no destructivo por defecto; la confirmación del borrado se pide en la interfaz, no en el protocolo.
- Los **puertos, el enlace y el dominio** del servidor se configuran con la acción `web` (`status`/`configure`/`https-on`/`https-off`) y se reflejan en `WebView`; no hay directivas de configuración generadas que inspeccionar ni copias que restaurar. Los dominios y alias se registran en el host virtual y en el SNI, siempre restringido a los nombres registrados.
- El entorno del sitio se **enmascara en el servidor** (`value` vacío cuando `secret: true`) y los secretos **nunca aparecen en `status`**: `env-set` solo informa del nombre de la clave. El borrado de archivos valida que la ruta registrada sea absoluta, esté fuera de `LAIR_HOME` y no resuelva a un enlace simbólico.

## Alternativas

- **Ventanas flotantes** (una ventana por dato del sitio en Electron, o pantallas modales separadas en la TUI): permite ver varios datos a la vez, pero duplica el estado del daemon, los atajos y los puntos de sincronización; la TUI no puede mostrar varias pantallas completas simultáneas con fiabilidad. Se descarta en favor de pestañas dentro del propio pane.
- **Diálogos independientes en la desktop sin acción `site`**: habría requerido varias acciones nuevas y lógica de presentación duplicada en la TUI, rompiendo la paridad que garantiza `app/scripts/check-actions.mjs`. Se descarta.
- **Generar archivos de configuración para un servidor externo**: añade un binario que Lair no instala, obliga a validar cada cambio y complica la reversión. Se descarta; el servidor Go aplica los cambios en caliente.

## Consecuencias

- El inventario compartido pasa de 34 a **35 acciones**; `site` se registra en `internal/daemon/protocol.go` junto a `mail`, `dumps` y `debugger`, y la lógica vive en `engine/site.go`. La app y la TUI solo consumen el protocolo.
- La desktop gana siete pestañas en el detalle de Sites y la TUI las teclas `E` (subconjunto) y `X` (borrar con confirmación); la ayuda y la documentación (`README.md`, `docs/CLI.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`) dejan explícita la diferencia con `e` (enlazar).
- Los hosts virtuales se publican en el servidor Go a partir de `config.json`, sin copias fechadas que gestionar; el registro y los archivos del proyecto se conservan.
- El borrado de carpetas queda protegido por validación de ruta y confirmación en la interfaz; los secretos nunca se devuelven en `status` y las claves se escriben en `.env` con validación de nombre y valor. Ambos puntos se reflejan en `docs/SECURITY.md`.
- No hay cambios en el CI: los jobs Go y app existentes cubren la nueva acción y la paridad desktop/TUI. **No se mueve el tag `v1.0.0` ni se publica ningún release**; los cambios se registran como entrada nueva en `RELEASE_NOTES.md`.
