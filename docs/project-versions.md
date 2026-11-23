# Versiones por proyecto

Cada proyecto elige su versión de PHP y Node. Los shims de `<root>/bin`
resuelven la versión según el directorio de trabajo usando `lair exec <tool>`.

## Precedencia

| Herramienta | Orden |
|---|---|
| PHP | `isolate` del sitio > `.lairrc` (`php=`) > `.flockrc` > default > última instalada |
| Node | `isolate` del sitio > `.lairrc` (`node=`) > `.flockrc` > `.nvmrc`/`.node-version` > default > última instalada |

Los alias de nvm (`lts/*`, `node`) se ignoran en los archivos de versión: solo
se aceptan números. Si la versión no está instalada y la terminal es
interactiva, Lair ofrece instalarla y reintentar; sin TTY devuelve un error.

## Comandos

```sh
lair pin php 8.2               # escribe php=8.2 en .lairrc
lair pin node 20               # escribe node=20 en .lairrc
lair pin root /otro/lair-home  # root=...: data root alternativo para el proyecto
lair unpin php                 # quita la línea
lair isolate php 8.4           # fija la versión en el sitio aparcado (config.json)
lair isolate node --off        # alias de unisolate
lair unisolate node
lair isolated                  # muestra los aislamientos activos
lair use 8.4                   # atajo de `lair php use`
```

`.flockrc` y `flock.json` se siguen leyendo por compatibilidad, pero los
archivos canónicos son `.lairrc` y `lair.json`.

## Ejemplo

Dos proyectos pueden convivir con versiones distintas:

```sh
cd ~/Sites/blog && echo 'php=8.2
node=20' > .lairrc
cd ~/Sites/api && echo 'php=8.4
node=22' > .lairrc

cd ~/Sites/blog && php -v && node -v   # PHP 8.2 / Node 20
cd ~/Sites/api  && php -v && node -v   # PHP 8.4 / Node 22
```

La versión de PHP elegida también se usa para ejecutar Composer: el shim
llama a `<php del proyecto> <root>/tools/composer.phar` con `argv[0]` absoluto,
de modo que `PHP_BINARY` y los scripts que relanzan PHP apuntan al mismo
binario. `lair ext` comprueba los requisitos `ext-*` y `lair ext add <x>`
instala la extensión con el Composer del proyecto.
