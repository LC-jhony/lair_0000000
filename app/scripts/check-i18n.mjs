#!/usr/bin/env node
// Verifica que todos los idiomas tengan exactamente las mismas claves y los
// mismos marcadores de interpolación. Se ejecuta con: npm run i18n:check
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'lang');
const locales = readdirSync(root, { withFileTypes: true })
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name)
  .sort();

function flatten(value, prefix = '', out = {}) {
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (child && typeof child === 'object' && !Array.isArray(child)) {
      flatten(child, path, out);
    } else {
      out[path] = String(child);
    }
  }
  return out;
}

function placeholders(text) {
  return [...text.matchAll(/\{\{(\w+)\}\}/g)].map((match) => match[1]).sort().join(',');
}

// createTranslator only replaces {{name}}; a lone {name} would reach the UI
// literally, so flag it in every catalog (including English).
function simplePlaceholders(text) {
  return [...text.matchAll(/(?<!\{)\{(\w+)\}(?!\})/g)].map((match) => match[1]);
}

const catalogs = new Map();
for (const locale of locales) {
  const file = join(root, locale, 'app.json');
  catalogs.set(locale, flatten(JSON.parse(readFileSync(file, 'utf8'))));
}

const base = catalogs.get('en');
if (!base) {
  console.error('i18n: falta lang/en/app.json');
  process.exit(1);
}

let failed = false;
for (const [locale, messages] of catalogs) {
  const missing = Object.keys(base).filter((key) => !(key in messages));
  const extra = Object.keys(messages).filter((key) => !(key in base));
  const empty = Object.entries(messages).filter(([, value]) => value.trim() === '').map(([key]) => key);
  const mismatched = Object.keys(base)
    .filter((key) => key in messages)
    .filter((key) => placeholders(base[key]) !== placeholders(messages[key]));
  const singleBrace = Object.entries(messages)
    .filter(([, value]) => simplePlaceholders(value).length > 0)
    .map(([key]) => key);

  if (missing.length || extra.length || empty.length || mismatched.length || singleBrace.length) {
    failed = true;
    console.error(`i18n: ${locale}`);
    if (missing.length) console.error(`  claves faltantes: ${missing.join(', ')}`);
    if (extra.length) console.error(`  claves sobrantes: ${extra.join(', ')}`);
    if (empty.length) console.error(`  valores vacíos: ${empty.join(', ')}`);
    if (mismatched.length) console.error(`  marcadores distintos: ${mismatched.join(', ')}`);
    if (singleBrace.length) console.error(`  llave simple sin doble llave (usa {{...}}): ${singleBrace.join(', ')}`);
  }
}

if (failed) process.exit(1);
console.log(`i18n OK: ${locales.length} idiomas, ${Object.keys(base).length} claves`);
