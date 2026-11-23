# Lair v1.0.1

Parche de producción sobre v1.0.0. Corrige el lanzador de escritorio y deja el paquete `.deb` listo para usar desde el menú de aplicaciones.

## Correcciones

- **El `.deb` abre la app de escritorio**: el lanzador del menú ya no se queda en `lair ui` (que solo imprimía la conexión del daemon). El paquete incluye ahora la app Electron bajo `/usr/share/lair/desktop/` y el comando `lair-desktop`, que arranca el daemon si hace falta y abre la interfaz. Si el helper `chrome-sandbox` no tiene el bit setuid, se lanza con `--no-sandbox`.
- **Empaquetado reproducible**: `scripts/build-dist.sh` empaqueta la app Electron en los `.deb` y `scripts/create-release.sh [versión]` construye todos los artefactos antes de publicar. El release ya no depende de pasos manuales ocultos.
- **Documentación de instalación**: el README explica cómo instalar y verificar los binarios en Linux, macOS y Windows, y cómo abrir la app de escritorio desde el menú o con `lair-desktop`.

Sin cambios en el motor, los comandos ni el catálogo firmado. Los binarios CLI de todas las plataformas se recompilan desde el mismo commit.
