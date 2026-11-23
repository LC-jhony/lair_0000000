# ADR 006 — Uninstall destructivo y validación de rutas

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

`lair uninstall` debe devolver la máquina a su estado anterior sin dejar restos, pero no puede borrar datos del usuario por accidente. Las rutas se construyen a partir de `LAIR_HOME`, nombres de sitios, versiones y rutas de proyecto: una validación floja permitiría `../` o rutas absolutas inesperadas. Además, los scripts privilegiados deben poder deshacerse incluso cuando la raíz de datos desaparece.

## Decisión

Hacer el uninstall **explícitamente destructivo pero con conservación de datos por defecto**: elimina shims, PATH, PHP, Composer, Node, binarios de todos los servicios, CA, certificados, logs, caché y `config.json`; conserva las bases de datos en `services/<id>/data/` salvo `--delete-data`. Genera `revert-privileged.sh` y mantiene los helpers con `--remove`; con `--delete-data` esos scripts se guardan en `$HOME` para que no desaparezcan al borrar la raíz. `revert-privileged.sh` ya no reinstala la CA: invoca `trust-ca.sh --remove` y `trust-ca-user.sh --remove`.

Toda ruta derivada de entrada del usuario se valida antes de usarla: se rechazan componentes `..`, rutas absolutas fuera de la raíz prevista y nombres de sitio/host inválidos. La extracción de artifacts usa `safeJoin` + `Prepare` + `os.Rename`, y los hosts virtuales del servidor Go se derivan de `config.json`.

## Alternativas

- Borrar también las bases de datos siempre: sería destructivo sin aviso y causaría pérdida de datos.
- Dejar los archivos «por si acaso»: deja shims y CA apuntando a binarios inexistentes y confunde al usuario.
- Confiar en `filepath.Clean` sin comprobar el prefijo: `../` puede escapar de la raíz.

## Consecuencias

- El usuario entiende qué se borra: los datos sobreviven salvo `--delete-data`, y la reversión privilegiada sigue disponible.
- No se tocan paquetes del sistema (servidores web, Valet, Mailcatcher); solo se limpia la configuración generada bajo `$LAIR_HOME`.
- Las validaciones de rutas cubren uninstall, sitios, projects y extracción; cualquier intento de escapar de la raíz falla antes de escribir.
- La CA se elimina y se retira del store del sistema/navegadores, evitando dejar confianza huérfana.
