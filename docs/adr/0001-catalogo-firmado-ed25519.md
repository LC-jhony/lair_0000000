# ADR 001 — Catálogo firmado con Ed25519

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

Lair descarga PHP, Node, Composer y servicios desde mirrors cuyo contenido puede cambiar o ser comprometido. Un índice editable bastaría para sustituir un binario por otro. Además, la instalación debe funcionar la primera vez aunque ningún mirror esté accesible.

## Decisión

Publicar un manifiesto JSON (`schema`, `channel`, `serial`, `generated`, `expires` y paquetes por plataforma) firmado con **Ed25519**. La clave pública va embebida en el binario (`go:embed` en `internal/assets`); el binario incluye también una copia del catálogo y su firma como ejemplo de desarrollo. Se rechazan firma inválida, canal distinto, esquema desconocido, expiración y serial menor. Los SHA-256 de cada artifact se verifican en streaming antes de extraer. La confianza depende de la firma, nunca del host: `LAIR_MANIFEST_BASE_URLS` solo indica mirrors a probar en orden.

## Alternativas

- Confiar en HTTPS y en el host de descarga: frágil ante un mirror comprometido y ante MITM con CA instalada.
- Usar TLS pinning por proveedor: no cubre los cambios de URL ni las firmas de versiones futuras.
- Manifiesto sin firmar pero con SHA-256: cualquiera que pueda editar el manifiesto puede sustituir el hash.

## Consecuencias

- Un mirror malicioso solo puede denegar servicio, no falsificar un artifact.
- La clave privada de desarrollo (`keys/private.ed25519`, `0600`) queda fuera de Git; producción requiere un par nuevo custodiado aparte.
- Editar el catálogo exige volver a firmarlo (`cmd/catalog-sign`) y copiar la pareja a `internal/assets/`; la firma cubre los bytes exactos del JSON.
