# Servidor MCP (stdio)

`lair mcp` expone un servidor [Model Context Protocol](https://modelcontextprotocol.io/) sobre **stdio**: habla JSON-RPC 2.0, un mensaje por línea, en `stdin`/`stdout`. Usa la revisión de protocolo `2024-11-05` y anuncia la capacidad `tools`.

El servidor es un adaptador de solo lectura sobre el dispatcher del daemon: cada herramienta llama a una acción ya existente (`status`, `versions`, `jobs`, `logs`, `services-health`, `site-data`) y devuelve su resultado como contenido de texto JSON. **No hay herramientas que modifiquen** proyectos, versiones, servicios ni configuración. Funciona tanto con el daemon en marcha (usa su token local) como offline (crea un dispatcher en proceso).

> `stdout` está reservado para el protocolo: el servidor no escribe ahí logs ni mensajes de ayuda. Los diagnósticos van a `stderr` con el prefijo `lair-mcp:`.

## Configuración

El binario `lair` debe estar en el `PATH` del proceso que lanza el agente (instalado con `lair install`, o con `$LAIR_HOME/bin` en el `PATH`). No hace falta arrancar el daemon: si no responde, el propio comando usa el motor en proceso.

### Claude Code

Registro global (álmbito `user`):

```sh
claude mcp add lair -- lair mcp
```

Para un proyecto concreto, crea `.mcp.json` en la raíz del repositorio:

```json
{
  "mcpServers": {
    "lair": {
      "command": "lair",
      "args": ["mcp"]
    }
  }
}
```

También puede registrarse con `claude mcp add --scope project lair -- lair mcp`.

### Codex

```sh
codex mcp add lair -- lair mcp
```

O en `~/.codex/config.toml`:

```toml
[mcp_servers.lair]
command = "lair"
args = ["mcp"]
```

### Otros agentes

Cualquier cliente MCP con transporte stdio acepta la misma forma:

```json
{
  "mcpServers": {
    "lair": {
      "command": "lair",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

El agente lanza `lair mcp` como subproceso, escribe peticiones JSON-RPC 2.0 terminadas en salto de línea en `stdin` y lee las respuestas de `stdout`. Los métodos soportados son `initialize`, `notifications/initialized` (notificación, sin respuesta), `ping`, `tools/list` y `tools/call`. Un método desconocido devuelve el error estándar `-32601`.

## Herramientas

| Herramienta | Argumentos | Acción del daemon | Resultado |
|---|---|---|---|
| `lair_status` | ninguno | `status` | Estado de PHP, Node, servicios, sitios y jobs. |
| `lair_versions` | `package` (string): paquete cuyas versiones se consultan | `versions` | Versiones disponibles, de más reciente a más antigua. |
| `lair_jobs` | ninguno | `jobs` | Lista de jobs de instalación y su estado. |
| `lair_logs` | `name` (string): nombre del archivo de log | `logs` | Contenido del log. |
| `lair_services_health` | ninguno | `services-health` | Salud de los servicios gestionados por Lair. |
| `lair_site_data` | `site` (string): host del sitio aparcado | `site-data` | Metadatos, entorno y versiones del sitio. |

Ejemplo de llamada:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lair_versions","arguments":{"package":"php"}}}
```

La respuesta contiene el JSON devuelto por el daemon como un bloque de texto:

```json
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"versions\":[\"8.4.5\",\"8.3.19\"]}"}]}}
```

Si la acción falla, se devuelve un error JSON-RPC con el mensaje legible del daemon. Si el argumento requerido falta o la herramienta no existe, se devuelve `-32602` (parámetros inválidos).

## Seguridad

El servidor **hereda el token del daemon del usuario** (`run/api.token`) para poder hablar con la API local. Esto significa que cualquier agente conectado puede leer el estado local de los proyectos: sitios aparcados, variables de entorno de esos sitios, logs, versiones instaladas y estado de los servicios, con los privilegios del usuario que ejecuta `lair`.

- Conecta solo agentes de confianza. El token no debe compartirse ni pegarse en archivos de configuración: el comando lo lee de `$LAIR_HOME/run/api.token`.
- No existe una herramienta para instalar, borrar, reiniciar servicios ni crear bases de datos; las acciones expuestas son de lectura. Aun así, la información leída puede ser sensible (por ejemplo, `.env` de un sitio en `lair_site_data`).
- El transporte es local (un subproceso con `stdin`/`stdout`): no se abre ningún puerto de red. No expongas el subproceso a clientes remotos.
- Para revocar el acceso, elimina el servidor de la configuración del agente. Si además quieres invalidar el token, elimina `run/api.token` y reinicia el daemon.
