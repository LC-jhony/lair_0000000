# Nota de riesgos de seguridad

Lair ejecuta software real del usuario y modifica la configuración del sistema. Esta nota describe los riesgos y las mitigaciones implementadas. **Léela antes de instalar la CA o el drop-in de DNS.**

## Modelo de confianza

- La confianza de los binarios **no depende del host**: todo se verifica con una firma Ed25519 cuya clave pública va embebida en el binario (`internal/assets/public.ed25519`). Un mirror comprometido no puede servir un artifact modificado.
- La clave privada de desarrollo (`keys/private.ed25519`) **no se embebe ni se distribuye**. Sirve solo para firmar manifiestos de prueba. Una clave de producción debe custodiarse fuera del repositorio.
- El catálogo embebido (`internal/assets/catalog.json`) y su copia en `catalog/` son un **ejemplo de desarrollo**: apuntan a URLs oficiales de proveedores con sus SHA-256 publicados, pero no son un feed de producción. Un mirror con un manifiesto firmado de serial mayor los sustituye automáticamente.
- El daemon **nunca corre como root** y Lair **no ejecuta Valet como root**: la unidad `lair@<usuario>.service` usa `User=%i` y `NoNewPrivileges=yes` impide escalada posterior. Los puertos web por defecto son **80/443**, por lo que hace falta `CAP_NET_BIND_SERVICE` en la unidad (`AmbientCapabilities=CAP_NET_BIND_SERVICE`) o `setcap 'cap_net_bind_service=+ep'` sobre el binario de Lair. Con Configuración de Lair en Sitios (o los flags del daemon) se vuelve a puertos altos sin capacidad.
- No hay listeners de administración accesibles desde la red: la API es un socket unix en `run/` (`0700`), el bridge de la app nativa y el DNS enlazan `127.0.0.1`, y el servidor web Go integrado enlaza `127.0.0.1` **por defecto**. El DNS es un servicio **dnsmasq solo en `127.0.0.1:53535`**, nunca en el puerto 53 global. La excepción es explícita: Configuración de Lair con `bind 0.0.0.0` expone los sitios a la LAN y el servidor Go respeta ese valor; usa firewall y red de confianza.
- Los trabajos de instalación y creación de proyectos **no dependen del ciclo de una petición HTTP**: el bridge desktop usa el contexto de vida del daemon, y las acciones que inician jobs lo desacoplan explícitamente con `context.WithoutCancel`. Cerrar una pestaña o recibir la respuesta no cancela un job; se cancela con `job-cancel` o al apagar el daemon.

## Riesgos y mitigaciones

| Riesgo | Mitigación | Riesgo residual |
|---|---|---|
| Manifiesto manipulado o *rollback* | Firma Ed25519, esquema/canal/expiración y serial monótono por canal; caché reverificada al cargar | Si la clave pública embebida se sustituye en el binario, toda la cadena cae; protege el binario con hashes/checksums propios. |
| Mirror malicioso | Múltiples URLs base en orden de fallback; la firma, no el host, decide | Un mirror puede hacer DoS (negar el manifiesto), no falsificarlo. |
| Artifact corrupto o *yanked* | SHA-256 en streaming a `.part`, staging con `safeJoin`, build markers, rechazo de yanked y reemplazo automático | Un build firmado pero defectuoso sigue siendo confiable criptográficamente; depende del proceso de release. |
| Extracción con *path traversal* | `safeJoin` + `Prepare` + verificación de ejecutables esperados antes del rename | Ninguno conocido en el código propio. |
| DNS `*.test` secuestrado | dnsmasq seguro escucha **solo en `127.0.0.1:53535`** (no en el puerto 53 global) y el drop-in de `systemd-resolved` usa `Domains=~test` con `DNS=127.0.0.1#53535`: **solo** `*.test` se envía al resolver local; el resto se reenvía upstream. Hay **fallback automático** y copia de seguridad del estado previo, reversible con `revert-privileged.sh` | Cualquier proceso local que escuche en `127.0.0.1:53535` podría responder si el puerto quedara libre; el servicio lo mantiene enlazado. Durante el fallback, la resolución de `*.test` depende del upstream del sistema. |
| Carpetas de proyecto e importación de Valet | `~/Lair` (L mayúscula) se crea si falta y se registra como aparcada; `migrate-dirs` mueve `~/lair` sin borrar la carpeta antigua sin confirmación. `import-valet` lee `~/.valet/config.json` y `~/.valet/Sites` **en solo lectura**: Lair no escribe en ellos, no instala Valet y nunca lo ejecuta como root | Los dominios/rutas importados se toman tal cual de la configuración de Valet: revisa `lair status` y el detalle de Sites antes de exponerlos; un archivo manipulado puede registrar rutas ajenas. |
| Enlazar 80/443 sin root | Se usa `CAP_NET_BIND_SERVICE` en la unidad del daemon o `setcap` sobre el binario de Lair; el proceso sigue corriendo como el usuario con `NoNewPrivileges=yes`. Alternativamente, 8080/8443 no requieren capacidad | `setcap` concede la capacidad a **cualquier** invocación de ese binario; protégelo con permisos de usuario y verifica la ruta a la que aplicas `setcap`. |
| CA local instalada en el sistema | La CA es ECDSA P-256, clave `0600` dentro de `certs/` (`0700`), y se instala mediante un script que el usuario revisa; se revierte con `trust-ca.sh --remove`. | **La CA es plenamente confiable para todo el sistema.** Cualquier proceso que obtenga `certs/ca-key.pem` puede emitir certificados válidos para cualquier dominio. Protege `certs/` y no lo compartas. |
| HTTPS engañoso en `*.test` | La CA emite certificados solo para hosts bajo el dominio configurado (`test`) y la capa TLS usa `GetCertificate` con validación del nombre: el **SNI queda restringido a los sitios aparcados** registrados, no a cualquier nombre que solicite un cliente | Un sitio `*.test` sigue siendo un sitio local; no implica identidad real. |
| Sitios expuestos a la LAN | `bind` es `127.0.0.1` por defecto; `0.0.0.0` es una decisión explícita del usuario y el servidor Go lo respeta en HTTP y HTTPS | Con `0.0.0.0` cualquier equipo de la red alcanza los sitios y los certificados locales; no sustituyen a un certificado público. Usa firewall. |
| API local sin autenticar | Socket unix en `run/` (`0700`), permiso `0600`, token aleatorio de 256 bits comparado en tiempo constante | Cualquier proceso del mismo usuario puede leer el token; eso equivale a controlar Lair, pero no a escalar privilegios. |
| Bridge de la app nativa | `POST /api` en `127.0.0.1:9797`; token en la cabecera **`X-Lair-Token`** (`subtle.ConstantTimeCompare`), **nunca en la URL**: se rechazan query/fragment y solo se aceptan orígenes `null`/`localhost`/`127.0.0.1`/`::1`. El renderer de Electron recibe URL y token por IPC con `contextIsolation`, sin acceso a filesystem | Cualquier proceso local del mismo usuario puede conectarse; el bridge no sirve HTML/JS y no amplía la superficie más allá del `Dispatcher`. |
| Correo saliente capturado por el SMTP catcher | El catcher integrado de `internal/mail` escucha **solo en `127.0.0.1:2525`**; no se instala Mailcatcher y no hay listener en la LAN | Cualquier proceso local puede enviar correo a la bandeja; no es un servidor de correo real y no entrega mensajes fuera de la máquina. |
| Agente de dumps inyectado en PHP-FPM | Se usa un **token dedicado** en `run/dumps.token` (`0600`), distinto del token de la API; el agente entra por `auto_prepend_file` en los pools existentes y solo envía volcados al daemon local | El token permite volcar datos de las peticiones: protégelo como un secreto y no lo compartas. La captura se puede desactivar con `capture-off`. |
| Volcados y correo acumulados | Las acciones `clear` de `mail` y `dumps` piden **confirmación explícita** (`confirm: true`) antes de borrar la bandeja | Un usuario con acceso al daemon puede vaciar la bandeja; no hay retención ni cifrado adicional más allá de los permisos de `run/`. |
| Xdebug siempre activo | El engine lo habilita **bajo demanda** con la acción `debugger`; `stop`/`disable` desactiva el módulo y **reutiliza los pools FPM del sitio**, sin dejar un pool de depuración permanente | Si se deja una sesión iniciada, el puerto `9003` sigue aceptando un cliente local; ciérrala cuando termines. |
| Servicios con configuración externa | Bind/puerto forzados por `Spec`; MySQL/MariaDB validan `mysqld-auto.cnf` y rechazan claves que muevan puerto/bind/socket/datadir | Un servicio puede tener sus propias vulnerabilidades; Lair no los aísla (son procesos nativos del usuario). |
| Fuga de secretos en logs | Los logs de servicios van a `log/<id>.log` con `0600` y la API no expone variables de entorno | Riesgo residual bajo; aun así protege `log/` y mantén `bind` en loopback salvo en redes de confianza. |
| Secretos y entorno expuestos en el detalle de sitio | La acción `site` **enmascara el entorno en el servidor** (`SiteEnvEntry`: `value` vacío cuando `secret: true`, `set` indica si la clave existe) y los secretos **nunca aparecen en `status`**: `env-set` solo informa del nombre de la clave (`output`), nunca de su valor ni en los registros del job | Un proceso con acceso al daemon puede leer la configuración no sensible del sitio; las credenciales reales permanecen en `.env` del proyecto (`0600` por convención) y en `config.json` (`0600`). |
| Inyección de claves en `.env` | `env-set` valida el nombre con `^[A-Za-z_][A-Za-z0-9_]*$` y rechaza valores con saltos de línea o NUL, de modo que no se pueden añadir líneas `KEY=...` arbitrarias al archivo | Un usuario con acceso al daemon puede modificar cualquier clave no secreta del `.env`; el valor de las claves marcadas como secretas no se devuelve. |
| Borrado accidental de la carpeta de un sitio | El borrado es **opcional**: `site` con `delete_files: true` (o la tecla `X` de la TUI) y confirmación en la interfaz. `lair unlink` sin esa opción no toca los archivos. El motor exige una ruta absoluta, fuera de `LAIR_HOME`, sin enlaces simbólicos y que `EvalSymlinks` resuelva a la misma ruta | Un usuario con acceso al daemon y que confirme puede eliminar un proyecto; no hay papelera ni retención adicional, así que depende de las copias del usuario. Las rutas rechazadas por el validador no se borran. |
| Secretos en el host virtual generado | El host virtual del servidor Go contiene solo rutas, sockets FPM, certificados, dominios/alias y puertos: **ningún valor del `.env`, token ni credencial**. Puede inspeccionarse desde la desktop (**View generated vhost**) y la TUI (`V`, `op: config`) | Revela la topología local (hosts, rutas y puertos): trátalo como información sensible y no lo publiques. |
| Sitio desactivado que sigue sirviendo tráfico | `site` con `op: set-enabled` y `enabled: false` retira el host virtual del router web Go: sin host virtual no hay ruta. No se crean ni borran archivos ni enlaces en `/etc` (ver [ADR 009](adr/009-virtual-hosts-en-servidor-go.md)), así que no se amplía la superficie del sistema; reactivarlo recrea el enrutado desde `config.json` | El registro y los archivos del proyecto se conservan. |
| Página de estado con información sensible | Cuando `root_state != ok`, el servidor Go sirve **`not-configured.html`** en lugar de pasar la petición a PHP-FPM. La página es genérica: muestra el host solicitado y la **ruta esperada del front controller dentro de cada proyecto** (`<raíz>/public/index.php`) con sugerencias, pero no imprime rutas internas de `LAIR_HOME`, tokens ni variables de entorno: **no expone rutas sensibles fuera del proyecto** | Un sitio mal configurado muestra un aviso útil en vez de un 502 o un listado; la ruta que revela es la del propio proyecto, no la del entorno de Lair, y no hay fuga de secretos del `.env`. |
| Daemon único | `Server.Serve` rechaza iniciarse si ya hay algo escuchando en el socket | Un socket huérfano se elimina solo si no responde. |

## Superficie privilegiada

El único código que toma privilegios es el contenido de:

- `<LAIR_HOME>/setup-privileged.sh` — instala la unidad systemd (con `CAP_NET_BIND_SERVICE` para 80/443), la regla polkit, el servicio dnsmasq en `127.0.0.1:53535` y el drop-in de `systemd-resolved` (`DNS=127.0.0.1#53535`, `Domains=~test`), guardando copia de seguridad de la configuración previa;
- `<LAIR_HOME>/trust-ca.sh` — instala la CA en el store del sistema;
- `<LAIR_HOME>/revert-privileged.sh` — deshace todo lo anterior y restaura la copia de seguridad del DNS si el fallback no es suficiente.

**No se ejecutan automáticamente.** El usuario debe revisarlos y ejecutarlos una vez con `sudo`/`pkexec`. La regla polkit concede control sobre `lair@<usuario>.service` **solo** al usuario dueño de esa unidad; no es una regla global para systemd. Tras aplicar el plan conviene verificar `resolvectl status | grep ~test`, `ss -lunp | grep 53535` y `lair status`; si la resolución falla, `revert-privileged.sh` restaura la configuración anterior.

## Permisos

| Ruta | Permiso | Contenido |
|---|---|---|
| `run/` | `0700` | Sockets unix, `api.token`, `desktop.token`, `desktop.url` |
| `run/api.token`, `run/desktop.token`, `run/desktop.url` | `0600` | Secretos del daemon y del bridge (token en cabecera, nunca en URL) |
| `run/dumps.token` | `0600` | Token dedicado del agente PHP que inyecta dumps por `auto_prepend_file` |
| `config.json` | `0600` | Estado persistente, escrito solo por el daemon |
| `certs/` | `0700` | CA y certificados; `ca-key.pem` es `0600` |
| `log/*.log` | `0600` | Logs de servicios e instalaciones |
| `web/pages/` | `0644` | `not-configured.html`, página de estado servida por el servidor Go cuando `root_state != ok` |
| `bin/` | `0755` | Shims ejecutables |
| `keys/private.ed25519` | `0600` | Solo desarrollo, excluida de Git y del binario |
| `~/.valet/config.json`, `~/.valet/Sites` | **solo lectura** | Los consulta `lair import-valet`; Lair nunca los modifica ni ejecuta Valet |
| copia de seguridad del drop-in de resolved / dnsmasq | gestionada por `setup-privileged.sh` | Permite el rollback con `revert-privileged.sh` cuando hay fallback de DNS |

## Recomendaciones

1. Ejecuta `lair install` desde un binario Lair obtenido por un canal en el que confíes.
2. Revisa los scripts generados antes de `sudo`.
3. Trata `certs/ca-key.pem` como un secreto; no lo sincronices ni lo subas a un repositorio.
4. Usa `lair uninstall` para revertir PATH, shims y CA antes de desinstalar el binario.
5. En máquinas con datos sensibles, evita instalar la CA en el store del sistema y usa `NODE_EXTRA_CA_CERTS`/`SSL_CERT_FILE` apuntando a `certs/ca.pem` en su lugar.
6. Desactiva la captura de dumps (`dumps capture-off`) y cierra la sesión de Xdebug (`debugger stop`) cuando termines de depurar; el SMTP catcher solo escucha en loopback y las acciones `clear` requieren confirmación.
7. Revisa los hosts y alias del detalle de Sites: el servidor Go integrado sincroniza los cambios desde `config.json`, pero `bind 0.0.0.0` sigue exponiendo los sitios a la LAN. Usa firewall y red de confianza.
8. Confirma con cuidado el borrado de la carpeta de un sitio (`site` con `delete_files: true` o la tecla `X` de la TUI): no hay papelera. Prefiere `lair unlink` (sin borrar archivos) si solo quieres retirar el host virtual; para retirarlo sin tocar el proyecto usa `set-enabled` con `enabled: false`.
9. Revisa el vhost con **View generated vhost** (TUI `V`, `op: config`) antes de exponer un sitio: no contiene secretos, pero sí rutas y puertos de tu entorno. La página `not-configured.html` avisa cuando `root_state != ok` sin revelar rutas del proyecto; no la sustituyas por páginas de error que impriman la ruta del documento.
10. Tras aplicar el plan privilegiado, comprueba `resolvectl status | grep ~test` y `ss -lunp | grep 53535`: dnsmasq debe escuchar solo en loopback:53535 y `*.test` debe resolver localmente. Usa `revert-privileged.sh` si necesitas volver al DNS anterior. Si no quieres conceder `CAP_NET_BIND_SERVICE` con `setcap`, cambia los puertos web a 8080/8443. Revisa además lo que `lair import-valet` haya registrado: la importación es solo lectura, pero los sitios se añaden a `config.json`.

## Limitaciones conocidas

- No hay sandbox ni aislamiento de procesos: los servicios son procesos normales del usuario.
- La integración privilegiada de Linux usa systemd/dnsmasq. macOS genera `/etc/resolver/<dominio>` apuntando al servidor DNS embebido de Lair en `127.0.0.1:53535` e instala la CA con `security`. Windows genera un bloque gestionado en el archivo `hosts`, instala la CA con `certutil` y usa workers `php-cgi` en puertos loopback en lugar de FPM/sockets UNIX. Cada plan se entrega como script revisable y reversible.
- El DNS propio es un resolver mínimo UDP: no implementa TCP, EDNS completo ni DNSSEC; confía en el upstream para el resto de nombres.
