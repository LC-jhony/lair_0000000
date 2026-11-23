// Builds the React/Ink dashboard (termcn components) into one Node bundle.
// The Go CLI embeds this file so `lair tui` works without a node_modules tree.
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';
import fs from 'node:fs';
import path from 'node:path';

const app = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

await build({
  entryPoints: [path.join(app, 'tui/main.tsx')],
  outfile: path.join(app, 'tui-dist/lair-tui.mjs'),
  bundle: true,
  platform: 'node',
  format: 'esm',
  banner: {
    js: "import { createRequire as __lairCreateRequire } from 'node:module'; const require = __lairCreateRequire(import.meta.url);",
  },
  target: 'node22',
  jsx: 'automatic',
  alias: {
    '@': path.join(app, 'tui'),
    // Ink only uses DevTools when DEV=true; the bundle uses a harmless stub.
    'react-devtools-core': path.join(app, 'tui/shims/react-devtools-core.ts'),
  },
  define: { 'process.env.NODE_ENV': '"production"' },
  logLevel: 'info',
});

// The Go CLI embeds this exact file so `lair tui` is self-contained.
const embedded = path.join(app, '..', 'internal', 'assets', 'tui.mjs');
fs.copyFileSync(path.join(app, 'tui-dist/lair-tui.mjs'), embedded);
console.log(`bundle copied to ${embedded}`);
