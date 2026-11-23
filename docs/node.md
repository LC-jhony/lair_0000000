# Node.js en Lair

Lair instala builds oficiales de Node verificados contra el catálogo firmado.
Se conserva **una versión por mayor** bajo `<root>/node/<versión>/`; al
instalar una release más nueva de la misma mayor se eliminan las antiguas.

```sh
lair node install 22           # última release publicada de la mayor 22
lair node install 22.11.0      # también resuelve a la última de la mayor
lair node list
lair node default 22
lair node update 22
lair node uninstall 22         # la default no se puede quitar
lair node install --from ./node-v22.11.0-linux-x64.tar.gz
```

`22`, `22.11` y `22.11.0` son equivalentes: el catálogo guarda una release por
mayor. En la primera ejecución se instala la LTS más nueva y se fija como
default. Las versiones ya instaladas por nvm siguen siendo visibles y se
resuelven de solo lectura; Lair no usa nvm para instalar ni actualizar.

## npm

Cada mayor tiene su propio prefijo global en `<root>/node/global/<mayor>/`, de
modo que `npm install -g` sobrevive a una actualización dentro de la mayor. El
enlace estable `<root>/node/global/current` apunta al prefijo de la mayor por
defecto (symlink en Unix, *junction* en Windows) y se añade al PATH del shell.

```sh
lair npm update                # última npm compatible con los engines del Node
npm install -g <herramienta>
```

Los shims ejecutan npm con `NPM_CONFIG_PREFIX`, `PATH=<prefijo>/bin:<bin de
Node>:$PATH` y `NODE_EXTRA_CA_CERTS=<root>/certs/ca.pem`. En sitios seguros se
pueden usar `VITE_DEV_SERVER_KEY` y `VITE_DEV_SERVER_CERT` para
`laravel-vite-plugin`.

## Resolución por proyecto

Precedencia: aislamiento del sitio > `.lairrc` (`node=`) > legacy `.flockrc` >
`.nvmrc`/`.node-version` (solo números; se ignoran `lts/*` y `node`) > default
> última instalada. Si falta la versión y hay TTY, el shim ofrece instalarla y
reintenta; sin TTY devuelve un error accionable.
