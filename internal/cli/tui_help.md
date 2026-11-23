# LAIR · Dashboard TUI

Panel de control nativo para PHP, Node, bases de datos y servicios.
El render de esta ayuda dentro de la TUI se hace con **Glamour**; desde la
terminal puedes ver el mismo archivo con **Glow**:

```sh
glow internal/cli/tui_help.md
```

## Navegación

| Tecla | Acción |
| --- | --- |
| `tab` / `→` / `l` | Siguiente sección |
| `←` / `h` | Sección anterior |
| `1`–`9` | Ir directamente a la sección numerada del panel izquierdo |
| `↑` / `k` | Servicio anterior |
| `↓` / `j` | Servicio siguiente |
| `enter` / `space` | Activar o desactivar el servicio seleccionado |
| `r` | Refrescar el estado |
| `?` | Mostrar u ocultar esta ayuda |
| `q` / `esc` / `ctrl+c` | Salir |

Con la ayuda o un registro abiertos, `↑`/`↓` y la rueda del ratón desplazan el
panel derecho; `esc` cierra el registro o la ayuda. La selección del panel de
servicios se mantiene visible automáticamente al recorrer la lista.

## Servicios

Cada tarjeta muestra el estado real del servicio. El interruptor se cambia con
`enter` o `space` y aparece una confirmación antes de aplicar el cambio:

- **ACTIVO** → el daemon ejecuta `service-restart`.
- **DETENIDO** → el daemon ejecuta `service-stop`.

El panel izquierdo navega por las secciones; el panel derecho muestra las
tarjetas correspondientes. Las barras y los bordes redondeados se dibujan con
**Lip Gloss**, los componentes con **Bubbles** y el bucle con **Bubble Tea**.
En terminales estrechas las tarjetas de Resumen se apilan en lugar de
mostrarse una al lado de la otra.

## Runtimes, servicios y bases de datos

| Tecla | Acción |
| --- | --- |
| `n` | Elegir del catálogo o escribir una versión de Node para nvm (`26`, `lts`, `22.16.0`); la instalación se hace con `lair nvm install` |
| `p` | Elegir e instalar una versión concreta de PHP |
| `m` | Elegir motor y versión de base de datos |
| `I` | Instalar cualquier servicio del catálogo |
| `d` | Cambiar la versión/base de datos predeterminada |
| `y` | Usar una versión instalada de un servicio |
| `z` | Desinstalar una versión de servicio (con confirmación) |
| `b` | Crear una base de datos (sección Bases de datos) |
| `v` | Ver registros del servicio o de la lista de logs |
| `x` | Cancelar un trabajo en curso (sección Sistema) |

`x` y `b` son **contextuales**: `x` actúa en Sistema y `b` en Bases de datos.
En otras secciones esas teclas no inician ninguna acción.

## Sitios

| Tecla | Acción |
| --- | --- |
| `a` | Aparcar una ruta como sitio |
| `e` | Enlazar un sitio con nombre y ruta |
| `o` | Desvincular un sitio |
| `E` | Ver el detalle del sitio seleccionado (overview, entorno y logs) |
| `V` | Ver el vhost generado del sitio seleccionado (`site config`) |
| `t` | Alternar `enabled` del sitio desde la vista de detalle (`set-enabled`) |
| `X` | Borrar el sitio seleccionado; confirma si se borra también su carpeta |
| `c` | Emitir un certificado local |
| `C` | Crear un proyecto Laravel (nombre, carpeta, interfaz, herramientas y base de datos) |
| `g` | Fijar una versión en `.lairrc` |
| `i` | Aislar la versión de un sitio |

Con `↑`/`↓` se elige el sitio. `E` abre una vista de detalle con el Overview
(URL, rutas, PHP, Node, base de datos, HTTPS, estado de `root`, dominios y
problemas o notas) y el Environment, donde los valores secretos aparecen
enmascarados. Si `root_state` no es `ok`, el detalle muestra un aviso con las
sugerencias de corrección. Desde esa vista `v` abre los registros del sitio con
el selector de `logs-list` y el visor de `logs`; `V` muestra el vhost generado
por nginx (`site config`) en el mismo panel con desplazamiento. Un sitio
desactivado no tiene vhost: en ese caso el panel lo indica en lugar de mostrar
una configuración vacía. `t` (sólo en la vista de detalle) pide confirmación y
alterna `enabled` mediante `site set-enabled {name, enabled}`; el detalle se
recarga con el estado definitivo. `esc` o `retroceso` vuelve a Sitios. `X` pide
confirmación para desvincular el sitio y permite borrar también la carpeta del
proyecto (`delete_files`). `V` y `t` son contextuales de Sitios y no confunden
con `v` (logs), `E` (detalle) ni `X` (borrar).

El asistente `C` consulta `project info` al abrirse: sólo ofrece los gestores de
Node y las bases de datos disponibles en la máquina, avisa si falta el
instalador de Laravel y pide usuario y contraseña cuando la base de datos no es
SQLite.

## Carpetas de proyectos

| Tecla | Sección | Acción |
| --- | --- | --- |
| `F` | Resumen / Runtimes / Sistema | Gestionar las carpetas donde se crean los proyectos |

`F` es contextual de las secciones generales (Resumen, Runtimes y Sistema) y no
se solapa con las teclas de Sitios (`E`, `X`, `V`, `t`, ...). Al abrirlo se
consulta `project dirs` y se muestra la lista de carpetas. Desde el diálogo
puedes:

- **actualizar lista** — volver a consultar `project dirs`;
- **añadir carpeta** — envía `project dirs-add` con la ruta escrita;
- **quitar carpeta** — envía `project dirs-remove` con la ruta escrita;
- **importar carpetas de Laravel Valet** — envía `project import-valet`;
- **migrar la antigua ~/lair a ~/Lair** — envía `project migrate-dirs`.

La carpeta por defecto para nuevos proyectos es `~/Lair`. Tras cualquier
operación la lista se recarga automáticamente. Las rutas que empiecen por `~`
se expanden al directorio del usuario; Lair nunca crea proyectos dentro de su
propio directorio de datos.

## Correo, dumps y debugger

Estas tres secciones tienen estado propio: se cargan al entrar en ellas y con
`r`. Ninguna bloquea la interfaz; las respuestas del daemon llegan como mensajes
tipados al bucle de Bubble Tea. Sus teclas son **contextuales** y sólo actúan
cuando la sección correspondiente está en pantalla (al igual que `b` y `x`).
No confundir `M`/`D`/`B` con `m` (base de datos), `d` (predeterminada), `v`
(logs) ni `b` (crear BD), que son minúsculas y siguen su comportamiento habitual.

| Tecla | Sección | Acción |
| --- | --- | --- |
| `M` | Correo | Ver un mensaje, iniciar/detener el SMTP de pruebas o limpiar la bandeja |
| `D` | Dumps | Activar/desactivar la captura, alternar `show_in_page` o limpiar los eventos |
| `B` | Debugger | Activar/desactivar la detección y los breakpoints (puerto, modo, versiones de PHP y dominios) |

El pane de Correo muestra el estado del servidor SMTP (activo/detenido, puerto,
número de mensajes y entorno) y la lista de asuntos; `M` permite abrir el cuerpo
de un mensaje, que se cierra con `esc` o `retroceso`. El pane de Dumps muestra si
la captura está activa, el contador de eventos, los tipos habilitados
(`queries`, `logs`, `views`, `jobs`, `requests`) y los eventos recientes. El pane
de Debugger muestra detección y breakpoints, puerto, modo, la disponibilidad de
cada versión de PHP y los dominios configurados.

## Sistema

| Tecla | Acción |
| --- | --- |
| `u` | Actualizar el catálogo firmado |
| `s` | Reiniciar todos los servicios |
| `r` | Refrescar el estado |

Las acciones se registran en `~/.local/share/lair/log/tui.log` mediante
`charm.land/log/v2`; el archivo se cierra al salir del dashboard.

## Referencia rápida (`lair tui --help`)

El bloque entre marcadores es la fuente de la salida de `--help`, extraída de
este mismo archivo para que ambas ayudas no puedan divergir.

<!-- help:start -->
lair tui (alias: lair dashboard) - panel de control interactivo

Uso:
  lair tui
  lair dashboard

Teclas:
  tab/→/l       cambiar de sección
  ←/h           sección anterior
  1–9           ir a la sección numerada
  ↑/k · ↓/j     seleccionar servicio; desplazar ayuda/registros
  rueda ratón   desplazar el panel derecho
  enter/space   activar o desactivar el servicio seleccionado
  n             elegir del catálogo o escribir una versión de Node para nvm
  p             elegir e instalar una versión de PHP
  m             elegir motor y versión de base de datos
  I             instalar cualquier servicio del catálogo
  y             usar una versión instalada de un servicio
  z             desinstalar una versión de servicio
  b             crear una base de datos (contextual: Bases de datos)
  d             cambiar la versión/base de datos predeterminada
  M             correo: ver mensaje o iniciar/detener SMTP (contextual: Correo)
  D             dumps: captura, opciones o limpiar (contextual: Dumps)
  B             debugger: deteccion y breakpoints (contextual: Debugger)
  v             ver registros (servicio seleccionado o lista)
  x             cancelar un trabajo en curso (contextual: Sistema)
  a             aparcar una ruta como sitio
  e             enlazar un sitio
  o             desvincular un sitio
  E             detalle del sitio seleccionado (contextual: Sitios)
  V             ver el vhost generado del sitio (contextual: Sitios)
  t             alternar enabled del sitio en el detalle (contextual: Sitios)
  X             borrar el sitio y, si se confirma, su carpeta (contextual: Sitios)
  c             emitir certificado para un sitio
  C             crear un proyecto Laravel (asistente)
  F             gestionar carpetas de proyectos: lista, alta/baja, importar Valet, migrar ~/lair (contextual: Resumen/Runtimes/Sistema)
  g             fijar versión en .lairrc
  i             aislar la versión de un sitio
  u             actualizar el catálogo firmado
  r             actualizar el estado
  s             reiniciar los servicios
  ?             mostrar/ocultar ayuda completa
  q / esc / ctrl+c salir
<!-- help:end -->
