// Guards desktop/TUI parity: every action the desktop client declares must
// exist in the daemon protocol, so a typo cannot silently diverge the UIs.
import { readFileSync } from 'node:fs';

const protocol = readFileSync(new URL('../../internal/daemon/protocol.go', import.meta.url), 'utf8');
const client = readFileSync(new URL('../src/api/client.ts', import.meta.url), 'utf8');

const daemonActions = new Set(
  [...protocol.matchAll(/Action\w+\s*=\s*"([^"]+)"/g)].map((match) => match[1]),
);

const block = client.match(/export const ACTIONS = \{([\s\S]*?)\} as const;/);
if (!block) {
  console.error('actions:check: no se encontró ACTIONS en src/api/client.ts');
  process.exit(1);
}
const clientActions = [...block[1].matchAll(/'([^']+)'/g)].map((match) => match[1]);

// The three tooling panes (mail, dumps, debugger) plus site inspection
// complete the 35-action desktop contract; a missing declaration must fail
// even if the protocol check below would otherwise pass.
const EXPECTED_DESKTOP_ACTIONS = 35;
if (clientActions.length !== EXPECTED_DESKTOP_ACTIONS) {
  console.error(
    `actions:check: el cliente declara ${clientActions.length} acciones; se esperan ${EXPECTED_DESKTOP_ACTIONS}`,
  );
  process.exit(1);
}

const missing = clientActions.filter((action) => !daemonActions.has(action));

if (missing.length > 0) {
  console.error(`actions:check: acciones desconocidas en el daemon: ${missing.join(', ')}`);
  process.exit(1);
}

// Reverse direction: a daemon action added for another front end must either
// land in the desktop client or be explicitly documented as CLI/TUI-only.
const DAEMON_ONLY = new Set(['uninstall', 'shutdown']);
const knownByClient = new Set(clientActions);
const unpaired = [...daemonActions].filter(
  (action) => !knownByClient.has(action) && !DAEMON_ONLY.has(action),
);

if (unpaired.length > 0) {
  console.error(`actions:check: acciones del daemon sin cliente desktop: ${unpaired.join(', ')}`);
  process.exit(1);
}

console.log(
  `actions OK: ${clientActions.length} acciones del desktop existen en el daemon ` +
    `y ${daemonActions.size - DAEMON_ONLY.size} acciones del daemon están cubiertas`,
);
