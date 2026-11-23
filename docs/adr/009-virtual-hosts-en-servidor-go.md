# ADR 009 — Hosts virtuales del servidor Go

- Estado: Aceptada (texto actualizado por el [ADR 010](010-servidor-go-como-unico-web-layer.md) en lo relativo al web layer)
- Fecha: 2026-11-18

## Contexto

El enfoque clásico de los servidores web escribe cada sitio en un árbol del sistema y lo activa con un enlace simbólico; la configuración se valida con herramientas externas contra ese árbol. Ese modelo encaja con un administrador que tiene `root` y con una distro que posee la configuración del servidor.

Lair no encaja en ese modelo:

- El daemon corre como el usuario (`lair@<usuario>.service`, `User=%i`) y la fase privilegiada es **única y previa** (un script que el usuario revisa y ejecuta una vez). No hay una ruta de privilegios en caliente para crear o borrar enlaces en `/etc` cada vez que se activa un sitio.
- El [ADR 004](0004-servidor-go-embebido.md) y el [ADR 010](010-servidor-go-como-unico-web-layer.md) fijan el **servidor Go integrado como único web layer**: no hay binario externo ni configuración generada en disco.
- El inventario de acciones es compartido (35 acciones): desktop, TUI y CLI deben activar y desactivar sitios con la misma lógica, sin duplicar scripts ni depender de permisos de sistema.
- Con el **enable/disable de sitios** y la **página de estado** aparecía la tentación de replicar el patrón de enlaces del sistema, o de introducir un servidor externo. Además hacía falta poder ver el host virtual generado y evitar que un sitio con la raíz mal configurada devolviera un error 502 o un listado que revelara rutas.

## Decisión

Generar los **hosts virtuales en el servidor Go integrado**, derivados de `config.json`, y materializar el enable/disable **publicando o retirando el host del router**, sin crear enlaces simbólicos ni escribir en `/etc`:

- El router web Go (`internal/web`) incluye o descarta cada sitio según `Enabled`. Un sitio desactivado **no genera host virtual**: desaparece de la tabla de enrutado; reactivarlo lo vuelve a publicar desde `config.json`.
- La acción `site` incorpora dos operaciones sin cambiar el inventario de **35 acciones**: `set-enabled` (campo `enabled` en el payload) y `config` (devuelve la definición del host virtual generado). `SiteDetail` expone `enabled` y `root_state`; el router usa esos campos para construir el host virtual y decidir qué sirve.
- Cuando `root_state != ok` (raíz ausente, vacía o con `index.php` en la raíz), el servidor sirve una **página de estado estática `not-configured.html`** en lugar de pasar la petición a PHP-FPM. La página es genérica: indica que el sitio no está configurado y ofrece sugerencias, mostrando solo el host solicitado y la ruta esperada del front controller dentro de cada proyecto (`<raíz>/public/index.php`); no imprime rutas internas de `LAIR_HOME`, tokens ni variables del entorno.
- La desktop añade el botón **View generated vhost** en el Overview del detalle y la TUI la tecla **`V`** y un toggle de activado/desactivado, ambos sobre la acción `site`. Los dominios y alias se registran en el host virtual y en el SNI, siempre restringido a los nombres registrados.

## Alternativas

- **Configuración en `/etc` con enlaces simbólicos** (el enfoque clásico): exige escribir fuera de `$LAIR_HOME` con privilegios en cada cambio, mezcla estado gestionado por Lair con configuración de la distro, complica la reversión (`uninstall` tendría que limpiar `/etc`) y rompe el principio de privilegios únicos y previos. Se rechaza.
- **Editar directamente la configuración del sistema con un `include`**: evita los enlaces pero comparte todos los problemas de propiedad y permisos, y valida configuración ajena a Lair. Se rechaza.
- **Introducir un servidor web externo**: reduce algo de código de enrutado, pero añade un binario que Lair no instala, un árbol de configuración que validar y un segundo modelo de hosts virtuales. Se rechaza (ver [ADR 010](010-servidor-go-como-unico-web-layer.md)).
- **Helper privilegiado que cree/borre enlaces en caliente**: permitiría el layout clásico, pero añadiría una superficie sudo en cada activación; Lair solo ejecuta scripts privilegiados revisados una vez. Se rechaza.
- **No servir página de estado** (dejar que PHP devuelva 502/404): es más simple, pero las páginas de error pueden filtrar rutas del documento y ofrecen una experiencia confusa. Se rechaza en favor de `not-configured.html`.

## Consecuencias

- Lair **nunca escribe en `/etc`** y **no instala ningún servidor web**: los hosts virtuales viven en el servidor Go y se derivan de `config.json`. Desinstalar solo borra el estado de Lair.
- Desactivar un sitio es una operación reversible y auditable: se retira del router con `set-enabled: true` para volver a publicarlo; el inventario sigue en 35 acciones porque `set-enabled` y `config` son `op` de `site`.
- El host virtual generado no contiene secretos (solo rutas, sockets, certificados, dominios y puertos) y puede inspeccionarse desde la desktop y la TUI (`V`). La página de estado solo muestra el host y la ruta esperada del front controller dentro del proyecto, sin revelar rutas internas de Lair ni secretos; un sitio desactivado no deja ninguna ruta que pueda seguir sirviendo tráfico.
- Cambios documentales en `README.md`, `docs/CLI.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md` y `RELEASE_NOTES.md`; la TUI añade `V` y el toggle, junto al botón de la desktop.
- No hay cambios en CI ni nuevos jobs: la paridad desktop/TUI sigue cubierta por `app/scripts/check-actions.mjs` y los tests Go existentes. **No se mueve el tag `v1.0.0` ni se publica ningún release**; la novedad se registra como entrada nueva en `RELEASE_NOTES.md`.
