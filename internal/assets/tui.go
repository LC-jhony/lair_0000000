package assets

import _ "embed"

// TUIBundle is the self-contained React/Ink dashboard built from app/tui.
// It is embedded so `lair tui` needs no node_modules tree next to the binary.
//
//go:embed tui.mjs
var TUIBundle []byte
