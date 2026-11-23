import { useCallback, useEffect, useRef, useState } from 'react';
import { FileText, RefreshCw, Search, Trash2 } from 'lucide-react';
import type { LogSource } from '../api';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  EmptyState,
  Input,
  LogSourceList,
  LogViewer,
  Select,
  Switch,
} from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';

const LEVELS = ['all', 'error', 'warn', 'info', 'debug'] as const;
type Level = typeof LEVELS[number];

const LEVEL_PATTERNS: Record<Exclude<Level, 'all'>, RegExp> = {
  error: /\b(error|critical|fatal|emergency)\b/i,
  warn: /\b(warn|warning)\b/i,
  info: /\binfo\b/i,
  debug: /\bdebug\b/i,
};

const REFRESH_MS = 5000;

/** Log explorer for sites and services, backed by `logs-list` and `logs`. */
export function LogsPane({ state }: PaneProps) {
  const t = useT();
  const [sources, setSources] = useState<LogSource[]>([]);
  const [items, setItems] = useState<string[]>([]);
  const [selected, setSelected] = useState('');
  const [text, setText] = useState('');
  const [query, setQuery] = useState('');
  const [level, setLevel] = useState<Level>('all');
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [viewerOpen, setViewerOpen] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const [error, setError] = useState('');
  const aliveRef = useRef(true);

  const loadList = useCallback(async () => {
    if (!state.client) return;
    try {
      const [names, found] = await Promise.all([
        state.client.logsList(),
        state.client.logSources(),
      ]);
      if (!aliveRef.current) return;
      setItems(names);
      setSources(found);
      setSelected((current) => {
        if (current && (names.includes(current) || found.some((s) => s.files?.includes(current)))) return current;
        const firstFile = found.flatMap((s) => s.files ?? [])[0];
        return firstFile ?? names[0] ?? '';
      });
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client]);

  const loadText = useCallback(async (name: string) => {
    if (!state.client || !name) return;
    try {
      const next = await state.client.log(name);
      if (aliveRef.current) setText(next);
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client]);

  useEffect(() => {
    aliveRef.current = true;
    void loadList();
    return () => { aliveRef.current = false; };
  }, [loadList]);

  useEffect(() => {
    if (!selected) {
      setText('');
      return;
    }
    void loadText(selected);
  }, [selected, loadText]);

  useEffect(() => {
    if (!autoRefresh || !selected) return;
    const timer = window.setInterval(() => void loadText(selected), REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [autoRefresh, selected, loadText]);

  const lines = text.split('\n');
  const visibleLines = level === 'all'
    ? lines
    : lines.filter((line) => LEVEL_PATTERNS[level].test(line));
  const filteredText = visibleLines.join('\n').trim();
  const levelOptions = LEVELS.map((value) => ({
    value,
    label: t(`logs.level.${value}`),
  }));

  const clear = () => {
    setText('');
    setSelected('');
    setQuery('');
    setLevel('all');
    setViewerOpen(false);
    setConfirmClear(false);
  };

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <FileText className="size-4 text-primary" aria-hidden="true" />
              {t('pane.logs')}
            </CardTitle>
            <CardDescription>{t('pane.logs.subtitle')}</CardDescription>
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => void loadList()}>
              <RefreshCw className="size-3.5" aria-hidden="true" />
              {t('dash.refresh')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={!selected && !text}
              onClick={() => setConfirmClear(true)}
            >
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('common.clear')}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,14rem)]">
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t('logs.search')}</span>
              <Input
                type="search"
                aria-label={t('logs.search')}
                placeholder={t('logs.searchPlaceholder')}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            <Select
              label={t('logs.level')}
              value={level}
              options={levelOptions}
              onChange={(value) => setLevel(value as Level)}
            />
          </div>
          <Switch
            checked={autoRefresh}
            onChange={setAutoRefresh}
            label={t('logs.autoRefresh')}
            description={t('logs.autoRefreshHelp')}
          />
          {error && <p role="alert" className="text-xs text-red-600 dark:text-red-400">{error}</p>}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle>{t('logs.sources')}</CardTitle>
            <CardDescription>{t('logs.sourcesHelp')}</CardDescription>
          </CardHeader>
          <CardContent>
            <LogSourceList
              sources={sources}
              items={items}
              selected={selected}
              query={query}
              onSelect={setSelected}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle className="flex items-center gap-2">
                {selected || t('logs.noSelection')}
                {selected && <Badge tone="indigo">{visibleLines.length}</Badge>}
              </CardTitle>
              <CardDescription>{t('logs.contentHelp')}</CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              disabled={!selected}
              onClick={() => setViewerOpen(true)}
            >
              <Search className="size-3.5" aria-hidden="true" />
              {t('logs.viewer')}
            </Button>
          </CardHeader>
          <CardContent>
            {!selected ? (
              <EmptyState icon={<FileText />} title={t('logs.noSelection')} description={t('logs.noSelectionHelp')} />
            ) : (
              <pre className="max-h-[46vh] overflow-auto rounded-lg border border-border bg-muted/40 p-3 text-xs whitespace-pre-wrap">
                {filteredText || t('logs.noLines')}
              </pre>
            )}
          </CardContent>
        </Card>
      </div>

      {viewerOpen && (
        <LogViewer title={selected} text={filteredText} onClose={() => setViewerOpen(false)} />
      )}

      <Dialog
        open={confirmClear}
        onClose={() => setConfirmClear(false)}
        title={t('logs.clear.title')}
        description={t('logs.clear.message')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirmClear(false)}>{t('common.cancel')}</Button>
            <Button variant="danger" onClick={clear}>
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('common.clear')}
            </Button>
          </>
        }
      />
    </div>
  );
}
