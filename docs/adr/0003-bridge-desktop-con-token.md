# ADR 003 — Bridge desktop con token en cabecera

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

La SPA embebida se sustituye por una app nativa (Electron + React en `app/`). El proceso principal necesita hablar con el `Dispatcher` del daemon sin servir HTML en el binario y sin exponer un puerto de administración en la red. La versión anterior ponía el token en el fragmento de la URL (`#token=...`), lo que dejaba secretos en historiales y facilitaba filtrados por referrer o logs.

## Decisión

`internal/desktopapi` publica únicamente `POST /api` en `127.0.0.1:9797` (configurable con `--desktop-port`). La autenticación exige la cabecera **`X-Lair-Token`**, comparada en tiempo constante; se rechazan tokens en query o fragmento. El CORS se limita a orígenes locales/Electron (`null`, `localhost`, `127.0.0.1`, `::1`). El token se escribe en `run/desktop.token` (`0600`) y la URL base en `run/desktop.url` (`0600`), dentro de `run/` (`0700`). El proceso principal de Electron lee ambos archivos y los entrega al renderer por IPC con `contextIsolation`: el renderer no tiene filesystem. `lair desktop` (alias `lair ui`) muestra los datos de conexión y arranca el daemon si hace falta.

## Alternativas

- Mantener el token en el fragmento: se filtra fácilmente y no es una cabecera autenticable por el servidor.
- Confiar solo en `127.0.0.1` sin token: cualquier proceso local podría invocar acciones privilegiadas.
- Integrar la app directamente con `run/api.sock`: acoplaría el renderer al formato de la API interna y rompería la separación de procesos.

## Consecuencias

- No se sirve HTML/CSS/JS desde el daemon; el paquete `.deb` sigue incluyendo lanzador e icono.
- La app adopta el estado del daemon y usa jobs con polling; no hay acciones exclusivas de una interfaz.
- `app/scripts/check-actions.mjs` verifica en CI que cada acción del cliente existe en `internal/daemon/protocol.go`.
- Desactivar con `--no-desktop` elimina el bridge; cambiar el puerto requiere reiniciar el daemon con `--desktop-port N`.
