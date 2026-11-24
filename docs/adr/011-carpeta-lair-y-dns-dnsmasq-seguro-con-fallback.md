# ADR 011 — Carpeta `~/Lair` y DNS dnsmasq seguro con fallback

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

Con el plan Valet Linux, Lair debe resultar familiar para quien viene de Laravel Valet: una carpeta de proyectos única, sitios `*.test` que resuelven sin configuración manual y URLs sin puerto explícito. La documentación y el código heredados usaban `~/lair` (minúscula) como carpeta de proyectos y un resolver Go propio en `127.0.0.1:53535` con `systemd-resolved`/`Domains=~test`. Los puertos web por defecto eran 8080/8443 para no necesitar privilegios.

Aparecen tres requisitos nuevos:

1. La carpeta de proyectos por defecto debe ser **`~/Lair`** (con L mayúscula), creada automáticamente si falta y registrada como aparcada, con migración del `~/lair` existente y capacidad de importar la configuración de un Valet ya instalado.
2. Los sitios deben servirse en los puertos web estándar **80/443**, sin ejecutar Valet ni el daemon como root.
3. El DNS debe ser un servicio **dnsmasq seguro**, acotado a `*.test`, con fallback automático y posibilidad de revertir los cambios del sistema.

La página **Sites**, el detalle de sitio de siete pestañas, `SiteDetail` y los hosts virtuales generados en el servidor Go ([ADR 008](008-detalle-de-sitio-en-pestanas.md), [ADR 009](009-virtual-hosts-en-servidor-go.md), [ADR 010](010-servidor-go-como-unico-web-layer.md)) deben permanecer intactos: la novedad es de carpeta, puertos y DNS.

## Decisión

- **`ProjectDirs`**: `config.json` guarda `project_dirs` como **array** (la primera ruta es la por defecto) y `projects_dir` como vista derivada; `DefaultProjectsDir` devuelve **`~/Lair`**, siempre fuera de `$LAIR_HOME`. En el arranque, `SyncProjectDirs` crea `~/Lair` si no existe y aparca los proyectos que contenga (`artisan` o `composer.json`).
- **Migración**: `lair migrate-dirs` (acción `project`, `op: migrate-dirs`) mueve los proyectos de `~/lair` a `~/Lair`, conserva los archivos y actualiza las rutas. No se ejecuta automáticamente al arrancar.
- **Importación de Valet**: `lair import-valet` (`op: import-valet`) lee `~/.valet/config.json` y `~/.valet/Sites` **en solo lectura** y añade sus rutas a `project_dirs` y el dominio; no crea sitios aparcados (para eso, `lair project sync`). Lair no modifica esos archivos, no instala Valet y nunca lo ejecuta como root.
- **Puertos 80/443**: el servidor Go integrado escucha en `127.0.0.1:80`/`:443` por defecto. El plan privilegiado concede `CAP_NET_BIND_SERVICE` a la unidad (`AmbientCapabilities=CAP_NET_BIND_SERVICE`, `User=%i`, `NoNewPrivileges=yes`) y ofrece `setcap 'cap_net_bind_service=+ep'` sobre el binario como alternativa; sin capacidad se vuelve a 8080/8443 desde Configuración de Lair o los flags del daemon.
- **DNS con dnsmasq seguro**: dnsmasq escucha solo en `127.0.0.1:53535`, nunca en el puerto 53 global. El drop-in de `systemd-resolved` fija `DNS=127.0.0.1#53535` y `Domains=~test`, de modo que solo `*.test` se envía al resolver local. Si dnsmasq o resolved fallan, se aplica **fallback automático** al upstream del sistema; el plan guarda copia de seguridad de la configuración previa y `revert-privileged.sh` permite el rollback.
- Las operaciones de carpetas (`dirs-list`, `dirs-add`, `dirs-remove`, `dirs-default`, `migrate-dirs`, `import-valet`) son valores de `op` de la acción `project`, por lo que el inventario compartido sigue en 35 acciones. La TUI las agrupa bajo la tecla **`F`**. Sites y el router web Go no cambian.

## Alternativas

- **Mantener `~/lair` en minúsculas** (rechazada): evita la migración, pero conserva la carpeta ambigua que motivó el cambio y no ofrece el punto de entrada único que espera quien llega desde Valet.
- **Copiar la configuración de Valet y ejecutar Valet** (rechazada): exigiría instalar Valet, ejecutarlo (potencialmente con `sudo`) y sincronizar dos Estados; la importación en solo lectura obtiene lo mismo sin tocar los archivos del usuario ni ampliar la superficie privilegiada.
- **Ejecutar el servidor web como root para usar 80/443** (rechazada): contradice el modelo de seguridad de Lair. Se prefiere `CAP_NET_BIND_SERVICE`/`setcap` sobre el binario del usuario, con `NoNewPrivileges=yes`.
- **Exigir puertos altos 8080/8443 siempre** (rechazada como valor por defecto): es seguro, pero no coincide con la experiencia `http://proyecto.test` del plan Valet; se mantiene como opción sin capacidad.
- **Reemplazar el DNS propio por un dnsmasq global en el puerto 53** (rechazada): secuestra toda la resolución del equipo y choca con el resolved del sistema. Se acota a `127.0.0.1:53535` y `Domains=~test`.
- **Resolver `*.test` solo con `systemd-resolved` sin servicio local ni fallback** (rechazada): no ofrece un registro predecible de los sitios ni recuperación automática; el drop-in quedaría sin servidor detrás.
- **Sin backup/rollback** (rechazada): cualquier cambio en la resolución del sistema debe ser reversible; se guarda la configuración previa antes de escribir el drop-in.

## Consecuencias

- `~/Lair` se crea sola, se aparca y alimenta **Sites** sin cambios en la página ni en la acción `site`; `~/lair` se migra sin borrar datos sin confirmación.
- Un Valet existente se importa en solo lectura: no se instala ni se ejecuta Valet, y nunca como root.
- Los sitios se sirven en `http://<nombre>.test` / `https://<nombre>.test` con 80/443; hace falta `CAP_NET_BIND_SERVICE` o `setcap`, o bien configurar puertos altos.
- dnsmasq queda confinado a loopback:53535 y `*.test`; el resto de nombres siguen resolviéndose upstream, con fallback y rollback verificables con `resolvectl status | grep ~test`, `ss -lunp | grep 53535`, `dig @127.0.0.1 -p 53535 <sitio>.test` y `lair status`/`lair doctor`.
- Se actualizan `README.md`, `RELEASE_NOTES.md`, `docs/CLI.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md` y el índice de ADRs. **No se mueve el tag `v1.0.0` ni se publica ningún release**; la novedad se registra como entrada nueva en `RELEASE_NOTES.md`.
