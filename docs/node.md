# Node.js en Lair

Lair instala Node a través de **nvm**: las versiones viven en
`~/.nvm/versions/node`, no bajo `$LAIR_HOME/node`. El catálogo firmado publica
`v26.13.1` (LTS, npm 11.20.0); otras mayores se resuelven con nvm y red.

```sh
lair node install              # última LTS del catálogo (v26.13.1)
lair node install 26           # última release de la mayor 26
lair node install lts          # LTS actual vía nvm
lair node list
lair node default 26
lair node update 26
lair node uninstall 26         # la default no se puede quitar
lair node install --from ./node-v26.13.1-linux-x64.tar.gz
```

`26`, `26.13` y `26.13.1` se resuelven a la release disponible de esa mayor.
**Node se instala siempre a través de nvm**: `lair node install` llama a
`lair nvm install <versión>`, que carga `~/.nvm/nvm.sh` con argv validado.
También puedes usar `lair nvm install|use|ls|current|exec|uninstall`
directamente; `lair nvm exec` es lo que usan los shims `node`/`npm`/`npx`.
Si nvm no está instalado, Lair avisa con la URL de instalación y no descarga
scripts remotos por su cuenta.

## npm

Con nvm, el prefijo global de npm vive junto a la versión activa en
`~/.nvm/versions/node/<versión>/`, por lo que `npm install -g` sobrevive a
actualizaciones dentro de la misma mayor. En instalaciones sin nvm (algunos
casos de Windows) se usa un prefijo bajo `$LAIR_HOME/node/global/<mayor>/` y un
enlace `current` que apunta a la mayor por defecto. En ambos casos los shims
inyectan `NPM_CONFIG_PREFIX` en el PATH.

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
