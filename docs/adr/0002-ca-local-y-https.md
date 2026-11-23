# ADR 002 — CA local y HTTPS para sitios `*.test`

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

Los sitios aparcados se sirven por `*.test` y necesitan HTTPS para que Laravel y el navegador se comporten como en producción. No hay un certificado público para dominios `.test`, y pedir al usuario que configure certificados a mano rompería la promesa de «un solo binario».

## Decisión

Generar una **CA ECDSA P-256 local** con clave privada `0600` en `certs/` (`0700`) y emitir un certificado por sitio con SAN base + wildcard. La confianza se instala mediante `trust-ca.sh --add` (y `trust-ca-user.sh --add` para navegadores), siempre revisado y ejecutado por el usuario; se revierte con `--remove`. La capa TLS usa `GetCertificate` y **restringe el SNI a los sitios aparcados registrados** bajo el dominio configurado: no emite certificados para nombres arbitrarios que pida un cliente.

## Alternativas

- Comprar certificados wildcard: imposible para `.test` y añade dependencia de red y coste.
- Desactivar HTTPS y usar solo HTTP: rompe la paridad con producción y algunos flujos de Laravel.
- Confiar en certificados autofirmados por sitio sin CA: obliga a aceptar excepciones en cada navegador y cada sitio.

## Consecuencias

- Si la CA se instala en el store del sistema, es plenamente confiable: quien consiga `certs/ca-key.pem` puede firmar cualquier dominio. Tratar ese archivo como secreto.
- Los certificados solo son válidos en las máquinas donde se confió la CA; no son identidad pública.
- Con `bind 0.0.0.0` los sitios y sus certificados locales quedan expuestos a la LAN; usar firewall y red de confianza.
- `lair secure [sitio]` emite los certificados; `lair uninstall` elimina CA y certificados, y revierte la confianza.
