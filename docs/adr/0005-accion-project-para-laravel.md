# ADR 005 — Acción `project` para Laravel

- Estado: Aceptada
- Fecha: 2026-11-18

## Contexto

Crear un proyecto Laravel implica instalar `laravel/installer`, ejecutar `laravel new`, preparar la base de datos, escribir `.env` y registrar el sitio `*.test` con HTTPS. Si cada interfaz (CLI, TUI y desktop) implementara esos pasos, aparecerían divergencias y credenciales en argumentos o logs.

## Decisión

Implementar una **única acción `project` del `Dispatcher`** en `internal/project`, compartida por `lair project info` / `lair project new`, el asistente **Sites → New project** de la desktop y la tecla `C` de la TUI. `laravel/installer` se instala en un `COMPOSER_HOME` aislado de Lair, sin tocar el Composer global. Las credenciales viajan solo por variables `DB_*`: se piden sin eco o se leen de `LAIR_DB_PASSWORD`, y nunca aparecen en argv ni en registros del job. El progreso, el log en vivo y la cancelación reutilizan `job`, `logs` y `job-cancel`.

Cuando el motor de BD es gestionado por Lair (MariaDB, MySQL o PostgreSQL), el asistente arranca el servicio si está parado y crea la BD antes de las migraciones. **SQL Server no es gestionado por Lair**: se admite como opción, pero Lair avisa y solo escribe las credenciales en `.env` —no arranca servicio ni crea la base de datos—.

## Alternativas

- Un script por interfaz: duplicación lógica, paridad imposible de garantizar y mayor riesgo de filtrar secretos.
- Confiar en `composer global require laravel/installer`: modifica el entorno global del usuario.
- Pasar la contraseña como flag: queda en el historial, en `ps` y en los logs del job.

## Consecuencias

- Las tres interfaces ofrecen exactamente las mismas opciones: nombre, carpeta padre, HTTPS, interfaz, autenticación, framework de pruebas, gestor Node, Git y base de datos.
- `app/scripts/check-actions.mjs` preserva la paridad en CI.
- Los proyectos con SQLite y con BD gestionada funcionan sin pasos manuales; SQL Server queda documentado como limitación.
