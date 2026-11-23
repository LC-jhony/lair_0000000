import { useState } from 'react';
import { Braces, Check, Hexagon, Package, Plus } from 'lucide-react';
import {
  Badge,
  Button,
  Card,
  CheckForUpdatesLine,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  Input,
  Select,
} from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';
import { useVersions } from './useVersions';

// nvm accepts exact versions, major lines, "lts" and "node". Lair passes the
// value straight to `lair nvm install <spec>`; only shell metacharacters and
// whitespace are rejected here.
const NVM_SPEC = /^(?:v?\d+(?:\.\d+){0,2}(?:-[0-9A-Za-z.-]+)?|lts(?:\/\*|\/[a-z]+)?|node|latest|--lts)$/i;
const NODE_EXAMPLES = ['26', 'lts', '22.16.0'];

function normalizeNodeVersion(input: string): string {
  return input.trim().replace(/^node[\s-]*/i, '');
}

export function RuntimesPane({ state }: PaneProps) {
  const t = useT();
  const catalog = useVersions(state.client, ['php', 'node']);
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [customNode, setCustomNode] = useState(false);
  const [nodeInput, setNodeInput] = useState('');
  const [nodeTouched, setNodeTouched] = useState(false);
  const s = state.status;
  const defaults = s?.defaults ?? {};
  const phpList = s?.php ?? [];
  const nodeList = s?.node ?? [];

  const typedNode = normalizeNodeVersion(nodeInput);
  const validNode = NVM_SPEC.test(typedNode);
  // Do not shout at the user while they are still typing the version.
  const showNodeError = nodeTouched && nodeInput.trim() !== '' && !validNode;

  const install = (name: 'php' | 'node', version: string) => {
    if (!version) return;
    const label = t('dash.installing', { name: `${name} ${version}` });
    if (name === 'php') void state.act(label, (c) => c.installPHP(version));
    else void state.act(label, (c) => c.installNode(version));
  };

  const installCustomNode = () => {
    if (nodeInput.trim() === '') return;
    if (!validNode) {
      setNodeTouched(true);
      return;
    }
    install('node', typedNode);
  };

  const versionSelect = (name: 'php' | 'node') => (
    <Select
      label={t('dash.version')}
      value={selected[name] ?? catalog[name]?.[0] ?? ''}
      onChange={(value) => setSelected((prev) => ({ ...prev, [name]: value }))}
      disabled={!!state.busy || !catalog[name]?.length}
      options={(catalog[name] ?? []).map((version) => ({ value: version, label: version }))}
    />
  );

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <div className="col-span-full">
        <CheckForUpdatesLine
          disabled={!!state.busy}
          onCheck={() => void state.act(t('dash.updatingCatalog'), (c) => c.updateCatalog())}
        />
      </div>
      <Card>
        <CardHeader className="flex-row items-center gap-3 space-y-0">
          <span className="grid size-9 place-items-center rounded-lg bg-indigo-500/10 text-indigo-500">
            <Braces className="size-4" aria-hidden="true" />
          </span>
          <div>
            <CardTitle>PHP</CardTitle>
            <CardDescription>{t('pane.runtimes.subtitle')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-end gap-2">
            {versionSelect('php')}
            <Button
              onClick={() => install('php', selected.php ?? catalog.php?.[0] ?? '')}
              disabled={!catalog.php?.length || !!state.busy}
            >
              <Plus className="size-4" aria-hidden="true" />
              {t('dash.installLatest', { name: 'PHP', version: selected.php ?? catalog.php?.[0] ?? '—' })}
            </Button>
          </div>

          {phpList.length === 0 ? (
            <EmptyState title={t('dash.noneInstalled')} />
          ) : (
            <ul className="flex flex-col gap-2">
              {phpList.map((info) => {
                const minor = info.minor ?? info.version ?? '';
                const isDefault = defaults.php === minor || Boolean(info.default);
                return (
                  <li key={minor} className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">
                          PHP {minor}
                          {/-/.test(info.version ?? '') ? (
                            <span className="ml-2 text-xs text-amber-600">Pre-release</span>
                          ) : null}
                          {s?.debug_running ? (
                            <span className="ml-2 text-xs text-violet-600">Xdebug</span>
                          ) : null}
                        </p>
                        <p className="truncate text-xs text-muted-foreground">{info.version ?? ''}</p>
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        {isDefault ? (
                          <Badge tone="indigo" dot>{t('common.default')}</Badge>
                        ) : (
                          <Button variant="ghost" size="sm" disabled={!!state.busy} onClick={() => void state.act(t('dash.switching', { name: `PHP ${minor}` }), (c) => c.usePHP(minor))}>
                            <Check className="size-3.5" aria-hidden="true" />
                            {t('common.makeDefault')}
                          </Button>
                        )}
                      </div>
                    </div>
                    <div className="flex flex-wrap items-center gap-1.5">
                      {catalog.php?.[0] && catalog.php[0] !== info.version ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!!state.busy}
                          onClick={() => void state.act(t('dash.installing', { name: `PHP ${catalog.php[0]}` }), (c) => c.installPHP(catalog.php[0]))}
                        >
                          {t('services.line.updateTo', { version: catalog.php[0] })}
                        </Button>
                      ) : null}
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!!state.busy}
                        onClick={() => void state.act(`php.ini ${minor}`, async (c) => { await c.log(`php${minor}-install`); })}
                      >
                        {t('common.showLog')}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!!state.busy || isDefault}
                        onClick={() => void state.act(t('dash.uninstalling', { name: `PHP ${minor}` }), (c) => c.uninstallPHP(minor))}
                      >
                        {t('common.uninstall')}
                      </Button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center gap-3 space-y-0">
          <span className="grid size-9 place-items-center rounded-lg bg-emerald-500/10 text-emerald-500">
            <Hexagon className="size-4" aria-hidden="true" />
          </span>
          <div>
            <CardTitle className="flex items-center gap-2">
              Node
              <Badge tone="green">catalog</Badge>
            </CardTitle>
            <CardDescription>Official Node.js builds, one version per major</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-3">
            <div className="inline-flex w-fit gap-1 rounded-lg bg-muted p-1">
              <Button
                size="sm"
                variant={customNode ? 'ghost' : 'secondary'}
                onClick={() => setCustomNode(false)}
              >
                {t('runtime.install.catalog')}
              </Button>
              <Button
                size="sm"
                variant={customNode ? 'secondary' : 'ghost'}
                onClick={() => setCustomNode(true)}
              >
                {t('runtime.install.other')}
              </Button>
            </div>

            {customNode ? (
              <div className="flex flex-col gap-2">
                <div className="flex items-end gap-2">
                  <label className="flex min-w-0 flex-1 flex-col gap-1.5">
                    <span className="text-xs font-medium text-muted-foreground">{t('dash.version')}</span>
                    <Input
                      value={nodeInput}
                      placeholder={t('runtime.install.placeholder')}
                      spellCheck={false}
                      invalid={showNodeError}
                      disabled={!!state.busy}
                      onBlur={() => setNodeTouched(true)}
                      onChange={(event) => setNodeInput(event.target.value)}
                    />
                  </label>
                  <Button
                    disabled={nodeInput.trim() === '' || !!state.busy}
                    onClick={installCustomNode}
                  >
                    <Plus className="size-4" aria-hidden="true" />
                    {t('dash.installLatest', { name: 'Node', version: typedNode || '—' })}
                  </Button>
                </div>
                {showNodeError ? (
                  <p className="text-xs text-destructive" role="alert">{t('runtime.install.invalidVersion')}</p>
                ) : validNode ? (
                  <p className="flex items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400">
                    <Check className="size-3" aria-hidden="true" />
                    {t('runtime.install.ready')}
                  </p>
                ) : (
                  <p className="text-xs text-muted-foreground">{t('runtime.install.otherHelp')}</p>
                )}
                <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                  <span>{t('runtime.install.examples')}</span>
                  {NODE_EXAMPLES.map((example) => (
                    <button
                      key={example}
                      type="button"
                      className="rounded-md border border-border px-1.5 py-0.5 font-mono text-xs transition-colors hover:bg-muted hover:text-foreground"
                      onClick={() => {
                        setNodeInput(example);
                        setNodeTouched(false);
                      }}
                    >
                      {example}
                    </button>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">{t('runtime.install.verified')}</p>
              </div>
            ) : (
              <div className="flex items-end gap-2">
                {versionSelect('node')}
                <Button
                  onClick={() => install('node', selected.node ?? catalog.node?.[0] ?? '')}
                  disabled={!catalog.node?.length || !!state.busy}
                >
                  <Plus className="size-4" aria-hidden="true" />
                  {t('dash.installLatest', { name: 'Node', version: selected.node ?? catalog.node?.[0] ?? '—' })}
                </Button>
              </div>
            )}
          </div>

          {nodeList.length === 0 ? (
            <EmptyState title={t('dash.noneInstalled')} />
          ) : (
            <ul className="flex flex-col gap-2">
              {nodeList.map((version) => {
                const major = version.replace(/^v/, '').split('.')[0];
                const isDefault = defaults.node === version || defaults.node === major;
                return (
                  <li key={version} className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate text-sm font-medium">Node {version}</span>
                      {isDefault ? (
                        <Badge tone="green" dot>{t('common.default')}</Badge>
                      ) : (
                        <Button variant="ghost" size="sm" disabled={!!state.busy} onClick={() => void state.act(t('dash.switching', { name: `Node ${version}` }), (c) => c.useNode(major))}>
                          <Check className="size-3.5" aria-hidden="true" />
                          {t('common.makeDefault')}
                        </Button>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-1.5">
                      {catalog.node?.[0] && catalog.node[0] !== version ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!!state.busy}
                          onClick={() => void state.act(t('dash.installing', { name: `Node ${catalog.node[0]}` }), (c) => c.updateNode(major))}
                        >
                          {t('services.line.updateTo', { version: catalog.node[0] })}
                        </Button>
                      ) : null}
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!!state.busy}
                        onClick={() => void state.act('npm', (c) => c.updateNpm())}
                      >
                        Update npm
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!!state.busy || isDefault}
                        onClick={() => void state.act(t('dash.uninstalling', { name: `Node ${major}` }), (c) => c.uninstallNode(major))}
                      >
                        {t('common.uninstall')}
                      </Button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center gap-3 space-y-0">
          <span className="grid size-9 place-items-center rounded-lg bg-amber-500/10 text-amber-600">
            <Package className="size-4" aria-hidden="true" />
          </span>
          <div>
            <CardTitle>Composer</CardTitle>
            <CardDescription>{t('runtime.composer.help')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-sm font-medium">
              {s?.composer ? `Composer ${s.composer}` : t('runtime.composer.missing')}
            </p>
            <p className="text-xs text-muted-foreground">{t('runtime.composer.pinned')}</p>
          </div>
          <Button
            variant="outline"
            disabled={!!state.busy}
            onClick={() => void state.act(t('runtime.composer.installing'), (c) => c.installComposer())}
          >
            <Plus className="size-4" aria-hidden="true" />
            {t('runtime.composer.install')}
          </Button>
          <Button
            variant="outline"
            disabled={!!state.busy || !s?.composer}
            onClick={() => void state.act(t('runtime.composer.installing'), (c) => c.updateComposer())}
          >
            Update Composer
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
