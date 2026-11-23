# ADR 010 — Servidor Go como único web layer

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

Lair necesita servir sitios `*.test` con archivos estáticos, FastCGI a PHP-FPM, HTTPS con CA local y redirección HTTP→HTTPS. El contrato del producto fija el servidor Go embebido escuchando en `127.0.0.1` con puertos 8080/8443 configurables y la acción `web` con las operaciones `status`/`configure`/`https-on`/`https-off`.

Se evaluó mantener un servidor web externo gestionado por Lair: exigiría instalar un paquete del sistema, generar un árbol de configuración con un archivo por sitio, validarlo antes de cada recarga y conservar copias de seguridad. Ese modelo depende de un binario que Lair no distribuye, obliga al usuario a instalarlo con `sudo apt install nginx` y añade una superficie de configuración que debe revisarse, respaldarse y revertirse. También se evaluó Caddy, con el mismo problema de binario externo y un modelo de configuración distinto.

## Decisión

Usar **únicamente el servidor Go integrado** (`internal/web`) como capa web:

- No hay paquete de servidor web, no se instala ninguno y no se escribe configuración en `/etc`. Se eliminan el árbol de configuración generado, los backups, la validación externa y la detección de puertos a partir de directivas `listen`.
- El router web Go enruta por `Host`, sirve estáticos desde `public/`, proxya a PHP-FPM por FastCGI, termina TLS con la CA local y redirige HTTP→HTTPS. Los hosts virtuales se derivan de `config.json` y se aplican en caliente (ver [ADR 009](009-virtual-hosts-en-servidor-go.md)).
- La acción `web` sigue existiendo con `status`, `configure`, `https-on` y `https-off`, pero se configura desde el modal **Configuración de Lair** de la pestaña Sitios: se retiran la pestaña Web, la tecla `w` de la TUI y el comando `lair web`. Desaparecen también `test`, `regenerate`, `start`, `stop`, `restart` y `engine`.
- La dirección de enlace es `127.0.0.1` por defecto; elegir `0.0.0.0` en Configuración de Lair es una decisión explícita del usuario que expone los sitios a la LAN. API, bridge de la app y DNS permanecen en loopback.
- Sites sigue generando hosts virtuales y el detalle de sitio con siete pestañas sigue disponible a través de la acción `site` (ver [ADR 008](008-detalle-de-sitio-en-pestanas.md)).

Esta decisión **sustituye al [ADR 004](0004-servidor-go-embebido.md)** en todo lo relativo al motor web.

## Alternativas

- **nginx gestionado por Lair** (rechazada): exige un binario que Lair no instala, configuración bajo `$LAIR_HOME`, validación con `nginx -t`, backups antes de regenerar y dos motores que mantener sincronizados. Rompe el compromiso de «sin dependencias de sistema» y añade instrucciones de instalación que el usuario no necesita.
- **Caddy** (rechazada): añade otro binario externo y un modelo de configuración distinto, sin beneficio sobre el router Go ya implementado.
- **Motor híbrido con selección de motor** (rechazada): duplica rutas de código, estados y documentación; el valor de `engine` ya no forma parte del contrato de la acción `web`.

## Consecuencias

- Un único web layer, sin configuración externa que validar ni copias que restaurar; los cambios de puertos, enlace, dominio y HTTPS se aplican en caliente.
- No hay instrucciones de instalación de paquetes web, ni variables de entorno para localizar un binario externo, ni rutas `/etc/nginx`, `web/nginx/` o `backups/web/` en la documentación.
- Los puertos altos por defecto evitan `CAP_NET_BIND_SERVICE`; para 80/443 hace falta `setcap` sobre el binario de Lair, bajo responsabilidad del usuario.
- Se actualizan `README.md`, `RELEASE_NOTES.md`, `docs/CLI.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/PHASES.md` y el índice de ADRs. **No se mueve el tag `v1.0.0` ni se publica ningún release**; la novedad se registra como entrada nueva en `RELEASE_NOTES.md`.
