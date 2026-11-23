# Demo del dashboard TUI de Lair
# Uso: vhs scripts/tui-demo.tape
# Requiere el binario instalado en ~/.local/bin/lair

Output tui-demo.gif

Set Shell "bash"
Set FontSize 20
Set Width 1200
Set Height 800
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Framerate 24

Type "~/.local/bin/lair tui"
Enter
Sleep 2s

# Carpetas de proyectos: se abre con F desde Resumen (se cancela).
Type "F"
Sleep 1.2s
Esc
Sleep 400ms

# Recorrido por las secciones (el panel derecho se desplaza solo).
Tab
Sleep 800ms
Tab
Sleep 800ms
Tab
Sleep 800ms

# Ayuda completa y desplazamiento con el teclado.
Type "?"
Sleep 1s
Down
Sleep 500ms
Down
Sleep 500ms
Type "?"
Sleep 600ms

# Sitios: detalle del sitio seleccionado, registros y borrado (se cancela).
Tab
Sleep 600ms
Type "E"
Sleep 1.2s
Type "v"
Sleep 1s
Esc
Sleep 400ms
Type "E"
Sleep 1s
# Alternar enabled (se cancela) y ver el vhost generado del sitio.
Type "t"
Sleep 800ms
Esc
Sleep 400ms
Type "V"
Sleep 1s
Esc
Sleep 400ms
Esc
Sleep 400ms
Type "X"
Sleep 1s
Esc
Sleep 400ms

# Asistente de nuevo proyecto: abre, muestra opciones filtradas y se cancela.
Type "C"
Sleep 1.5s
Esc
Sleep 600ms

# Web: puertos por defecto 80/443 (el diálogo se muestra y se cancela).
Tab
Sleep 600ms
Type "w"
Sleep 1s
Esc
Sleep 400ms

# Correo, dumps y debugger: se cargan al entrar y muestran sus diálogos.
Tab
Sleep 1s
Type "M"
Sleep 1s
Esc
Sleep 400ms
Tab
Sleep 1s
Type "D"
Sleep 1s
Esc
Sleep 400ms
Tab
Sleep 1s
Type "B"
Sleep 1s
Esc
Sleep 400ms

Type "q"
Sleep 500ms
