# Lair Desktop

Aplicación nativa de administración de Lair para Linux, macOS y Windows con una sola base de código: Electron + React + TypeScript + Vite. Sustituye al panel web embebido.

Estado actual: diez panes completas, i18n (es/en/cs), bridge de Electron, cliente HTTP y FakeDaemon con fixtures. Las capturas de cada pane están en [`../docs/screenshots/`](../docs/screenshots/).

## Principios

1. El daemon es la única fuente de verdad: la UI adopta los catálogos que devuelven las acciones.
2. Las operaciones largas son jobs con polling; nunca bloquean la ventana.
3. Nada destructivo sin confirmación; el texto explica qué pasa con datos y servicios.
4. Un mismo estilo de fila y acciones para PHP, Node, bases de datos y servicios.
5. Todo texto visible es una clave de lang/codigo/app.json; no hay literales en la UI.
6. Paridad en las tres plataformas: los mismos componentes React se empaquetan para Linux, macOS y Windows.

## Arranque

Necesitas Node 22+ y npm con el daemon de Lair instalado (CI usa Node 22).

Comandos: `npm install`, `npm run dev` (Vite), `npm run electron:dev` (Ventana Electron apuntando al puerto 5173), `npm test`, `npm run typecheck`, `npm run i18n:check`, `npm run build` y `npm run dist` (electron-builder).

La ventana Electron usa `contextIsolation` y un preload que solo expone `window.lairDesktop` con la plataforma y la versión de Electron. Los targets de `electron-builder` son AppImage/deb (Linux), dmg/zip (macOS) y nsis/portable (Windows).

## Estructura

- `electron/`: ventana nativa y bridge seguro.
- `src/i18n/`: catálogos, provider, plurales e interpolación.
- `src/components/`: componentes reutilizables.
- `src/api/`: cliente HTTP del bridge, modelos tolerantes, FakeDaemon y fixtures.
- `src/styles/`: tokens de tema claro/oscuro y layout.
- `src/test/`: configuración de Vitest + jsdom.

## Mapa de componentes

| Componente del prompt | Archivo |
| --- | --- |
| SettingsIcon | `src/components/SettingsIcon.tsx` |
| PaneHeader | `src/components/PaneHeader.tsx` |
| StatusDot | `src/components/StatusDot.tsx` |
| Badge | `src/components/Badge.tsx` |
| CodeBlock | `src/components/CodeBlock.tsx` |
| SettingsError | `src/components/SettingsError.tsx` |
| DetailHeader | `src/components/DetailHeader.tsx` |
| DetailSectionTitle | `src/components/DetailSectionTitle.tsx` |
| BusyLabel | `src/components/BusyLabel.tsx` |
| CheckForUpdatesLine | `src/components/CheckForUpdatesLine.tsx` |
| ComponentJobProgress | `src/components/ComponentJobProgress.tsx` |
| ComponentJobFailure | `src/components/ComponentJobFailure.tsx` |
| UpdateStrip | `src/components/UpdateStrip.tsx` |
| JobLogSheet | `src/components/JobLogSheet.tsx` |
| InstallInvitation | `src/components/InstallInvitation.tsx` |
| card | `src/components/Card.tsx` |
| ServiceInstallForm | `src/components/ServiceInstallForm.tsx` |
| Iconos SVG multiplataforma | `src/components/Icon.tsx` |

`Icon.tsx` sustituye a los símbolos nativos de cada plataforma por SVG, para que Linux, macOS y Windows compartan exactamente el mismo diseño. `types.ts` contiene `ComponentJob` y `normalizedProgress`, que convierte el `-1` del daemon en progreso desconocido.

## i18n

Los textos viven en `lang/en/app.json`, `lang/es/app.json` y `lang/cs/app.json`. `npm run i18n:check` verifica paridad de claves, valores vacíos y marcadores `{{...}}`. Los componentes solo reciben claves; nunca literales. Los plurales usan los sufijos `.one/.few/.other` de `Intl.PluralRules`.

## Tests

`npm test` cubre decodificación tolerante del progreso, interpolación y plurales, el render de los componentes base (jobs, validación de puerto, copia al portapapeles, confirmaciones visuales), el cliente HTTP (token, acciones, errores) y el FakeDaemon (progreso, cancelación y fallos).

Para desarrollar sin daemon real, abre `http://localhost:5173/?fake=1` con `npm run dev`, o compila con `VITE_FAKE_DAEMON=1`. `FakeDaemon.advance()` y `FakeDaemon.finish()` simulan el ciclo de vida de los jobs para capturas reproducibles.
