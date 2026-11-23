# ADR 007 — Panes de soporte en la desktop y TUI

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

El entorno necesita cuatro superficies nuevas para el flujo diario de depuración: correo saliente (**Mail**), volcados PHP (**Dumps**), sesión de Xdebug (**Debugger**) y registros (**Logs**). Además, los paneles genéricos *Overview* y *System* resultaban ambiguos y pasan a llamarse **General** y **About**. La desktop (Electron + React) y la TUI (Charmbracelet) deben ofrecer exactamente las mismas operaciones, sin duplicar lógica ni estados: las reglas ya viven en el `Dispatcher` del daemon. El web layer es el servidor Go integrado.

## Decisión

Añadir **Mail, Dumps, Debugger y Logs como panes del sidebar**, junto a General y About, tanto en la desktop como en la TUI. No se crean ventanas flotantes ni se instala una bandeja nativa del sistema.

- La desktop reorganiza el sidebar en once panes: General, Bases de datos, Runtimes, Servicios, Sitios, Web, Mail, Dumps, Debugger, Logs y About.
- La TUI replica los mismos panes en su navegación lateral con atajos `M`/`D`/`B` para Mail, Dumps y Debugger, sin colisionar con `m` (MariaDB), `d` (predeterminada) y `b` (crear base de datos).
- El backend expone tres acciones nuevas — `mail`, `dumps` y `debugger` — con un campo `op` en el payload, y amplía `logs-list` con `sources`. La CLI aún no las publica como subcomandos: se verifica con `lair <action>`/el bridge `POST /api`.
- Mail usa el SMTP catcher integrado en `127.0.0.1:2525`. Dumps inyecta el agente por `auto_prepend_file` en los pools FPM existentes y usa un token dedicado en `run/dumps.token` (`0600`). Debugger activa Xdebug bajo demanda reutilizando esos mismos pools, sin dejar un pool permanente.

## Alternativas

- **Ventanas flotantes** (una ventana por superficie en Electron, o pantallas separadas en la TUI): permite ver Mail y Logs a la vez, pero duplica el estado del daemon, los atajos y los puntos de sincronización; además la TUI no puede mostrar varias pantallas completas simultáneas con fiabilidad.
- **Bandeja nativa** (icono en el system tray que abra cada herramienta): añade dependencias por sistema operativo (AppIndicator, menu extra, taskbar de Windows), requiere permisos adicionales y deja la TUI sin equivalente; se descarta para mantener paridad desktop/TUI.
- **Reutilizar el pane System para todo**: evita nuevos panes, pero mezcla diagnóstico, correo y depuración en una sola vista y obliga a recargar el estado completo para cada cambio.
- **Instalar Mailcatcher como paquete externo**: ya no se instalan paquetes del sistema; el catcher propio en loopback cubre el caso sin gestionar dependencias de Ruby.

## Consecuencias

- Un único conjunto de acciones (`34` en el inventario compartido en el momento de esta decisión, `35` tras el [ADR 008](008-detalle-de-sitio-en-pestanas.md)) sirve a desktop, TUI y CLI; `app/scripts/check-actions.mjs` sigue validando la paridad en CI.
- Los atajos en mayúscula (`M`/`D`/`B`) conviven con los minúsculos heredados; la ayuda y la documentación deben dejar explícita la diferencia.
- El sidebar crece a once entradas: en terminales estrechas la TUI colapsa la navegación y la desktop usa el rail compacto del componente de sidebar.
- La captura de dumps es desactivable, el token del agente es independiente del token de la API y las acciones `clear` piden confirmación; estas superficies quedan documentadas en `docs/SECURITY.md`.
- Xdebug ya no deja un pool de depuración permanente: se activa solo durante la sesión y se reutilizan los pools del sitio.
- No hay cambios en el CI: los jobs Go y app existentes ya cubren estas superficies; no se publica ningún release nuevo ni se mueve el tag `v1.0.0`.
