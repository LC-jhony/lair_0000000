# TUI de Lair (`lair tui`)

Dashboard de terminal a pantalla completa, implementado en `internal/tui/` con
Charm v2 (Bubble Tea, Lip Gloss, Bubbles, Huh) y bubblezone para ratón. Se abre
con `lair tui` o `lair dashboard`; `lair tui --help` imprime la ayuda resumida y
fuera de un TTY devuelve un error claro.

## Principios

- **Un solo origen de datos.** La TUI no lee `config.json` ni llama al engine
  directamente. Todo pasa por `daemon.Caller` (cliente del daemon o
  `Dispatcher` en modo offline), el mismo contrato que usan la CLI y la desktop.
- **`Snapshot` inmutable.** `loadSnapshot()` reconstruye el estado completo
  desde las acciones `status`, `jobs`, `services-health`, `logs-list`,
  `project`, `site`, `site-data`, `queue`, `mail`, `dumps` y `debugger`. Las
  vistas son proyecciones puras; el estado de UI (cursor, pestaña, filtro) vive
  aparte en el `Model`.
- **Nada bloquea el event loop.** La carga y las acciones corren en `tea.Cmd`;
  un tick de 3 s y un `p.Send` tras cada acción mantienen la vista viva.
- **Sin acciones destructivas.** Quedan fuera `uninstall`, `php-uninstall`,
  `node-uninstall`, `service-uninstall`, `unlink/delete` y `job-cancel`.

## Estructura

| Archivo | Responsabilidad |
|---|---|
| `tui.go` | `Run`, comprobación de TTY, `tea.Program` |
| `model.go` | Model raíz, `Update`, `View`, teclas globales, ratón |
| `snapshot.go` | `Snapshot` y `loadSnapshot` |
| `events.go` | mensajes y comandos asíncronos |
| `theme.go`, `chrome.go` | paleta ANSI adaptativa, layout, hint line |
| `sidebar.go`, `dashboard.go` | navegación y pantalla inicial |
| `siteview.go`, `serviceview.go`, `databases.go`, `runtimes.go` | páginas |
| `settings.go`, `coreview.go`, `debug.go`, `logs_render.go` | ajustes, procesos core, debug y logs |
| `palette.go`, `help.go`, `toast.go` | overlays y notificaciones |
| `actions.go` | formularios Huh y acciones del daemon |

## Páginas y teclas

Sidebar: Dashboard, Runtimes, Databases, Services, Sites, Mail, Dumps,
Debugger, Logs y System; debajo los sitios agrupados por project dir y los
servicios instalados.

| Tecla | Acción |
|---|---|
| `ctrl+p` | paleta de comandos (búsqueda fuzzy, nunca destructiva) |
| `↑↓` / `j k` | mover en la lista enfocada |
| `enter` | abrir sitio/servicio o ejecutar la selección |
| `tab` / `esc` | alternar sidebar y área principal |
| `\` | mostrar sidebar superpuesta en pantallas estrechas |
| `/` | filtrar · `o` ordenar |
| `1`–`5` | pestañas de la página |
| `i` | instalar PHP/Node/servicio (selector con catálogo firmado) |
| `p` | crear proyecto Laravel (asistente) |
| `n` | crear base de datos |
| `e` / `h` / `c` | dominios / HTTPS / vhost del sitio seleccionado |
| `l` / `v` | cargar logs |
| `s` / `d` | reiniciar servicio / fijar versión o BD por defecto |
| `[` `]` | cambiar lente de debug o fuente de logs |
| `R` | refrescar · `?` ayuda · `:` comando · `q`/`ctrl+c` salir |

Responsive: sidebar completa desde 110 columnas, compacta desde 96, oculta por
debajo (con `\` para superponerla); menos de 60x12 muestra un aviso de
redimensionado. El ratón permite clic en filas y en la paleta.

## Verificación

```sh
go test ./internal/tui/   # render en 60x14, 100x30 y 160x50 + lógica de teclas
go test ./...
```
