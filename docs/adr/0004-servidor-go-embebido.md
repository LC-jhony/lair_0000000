# ADR 004 — Servidor Go integrado

- Estado: Superada por el [ADR 010](010-servidor-go-como-unico-web-layer.md)
- Fecha: 2026-11-18

## Contexto

Necesitamos servir sitios `*.test` con archivos estáticos, FastCGI a PHP-FPM, HTTPS y redirección HTTP→HTTPS sin exigir al usuario la instalación de paquetes del sistema. El servidor Go integrado (`internal/web`) cubre estos requisitos con la biblioteca estándar y comparte el `Router` con el daemon.

## Decisión

Lair sirve los sitios con **su propio servidor Go embebido** en `127.0.0.1`, con puertos 8080/8443 configurables, HTTPS con la CA local y redirección HTTP→HTTPS. Una única acción `web` configura puertos, HTTPS, dominio y `bind`: `0.0.0.0` expone los sitios a la LAN y `127.0.0.1` es el valor aislado por defecto.

> Nota histórica: esta decisión se registró originalmente evaluando un motor externo como opción preferente. El [ADR 010](010-servidor-go-como-unico-web-layer.md) la sustituye y fija el servidor Go como **único** web layer.

## Alternativas

- **Servidor web externo gestionado por Lair**: exigía un binario que Lair no instala, un árbol de configuración propio y una validación antes de cada recarga. Se descarta.
- **Distribuir un binario propio**: aumenta mucho el tamaño del release y la superficie de mantenimiento. Se descarta.
- **Caddy**: añade otro modelo de configuración y un binario externo. Se descarta.

## Consecuencias

- Un único motor que usa el mismo `Router` y la misma configuración de sitios para CLI, daemon, desktop y TUI.
- El comportamiento de `bind` se documenta como decisión de seguridad: la exposición a la LAN es explícita.
- Los puertos altos evitan `CAP_NET_BIND_SERVICE`; no hay configuración externa que validar ni copias que restaurar.
- El modal **Configuración de Lair** de la pestaña Sitios controla el servidor Go con la misma acción `web` del daemon; se retiran la pestaña Web y el comando `lair web`.
