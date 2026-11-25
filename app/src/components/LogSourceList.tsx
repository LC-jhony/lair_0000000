import { FileText, FolderOpen, Server } from 'lucide-react';
import type { ReactNode } from 'react';
import type { LogSource } from '../api';
import { useT } from '../i18n';
import { Badge } from './Badge';
import { EmptyState } from './ui/EmptyState';

export interface LogSourceListProps {
  sources?: LogSource[];
  items?: string[];
  selected?: string;
  query?: string;
  onSelect: (name: string) => void;
}

interface Entry {
  key: string;
  name: string;
  label: string;
  path?: string;
  kind: 'site' | 'service';
}

/** Site log files live in the project, so the daemon needs a qualified key. */
export function logKey(source: LogSource, file: string): string {
  return source.kind === 'site' ? `site:${source.id}:${file}` : file;
}

function entries(sources: LogSource[], items: string[]): Entry[] {
  const fromSources: Entry[] = sources.flatMap((source) =>
    (source.files ?? []).map((file) => ({
      key: logKey(source, file),
      name: file,
      label: source.name || source.id || file,
      path: source.path,
      kind: source.kind === 'site' ? 'site' : 'service',
    })),
  );
  const known = new Set(fromSources.flatMap((entry) => [entry.key, entry.name]));
  // Older daemons omit sources; newer ones can still list files outside them.
  const leftovers = items
    .filter((name) => !known.has(name))
    .map((name) => ({ key: name, name, label: name, kind: 'service' as const }));
  return [...fromSources, ...leftovers];
}

/** Sites and services that expose log files, with file-level selection. */
export function LogSourceList({ sources = [], items = [], selected, query = '', onSelect }: LogSourceListProps) {
  const t = useT();
  const needle = query.trim().toLowerCase();
  const all = entries(sources, items).filter((entry) =>
    !needle || [entry.name, entry.label, entry.path]
      .some((value) => (value ?? '').toLowerCase().includes(needle)),
  );
  const sites = all.filter((entry) => entry.kind === 'site');
  const services = all.filter((entry) => entry.kind === 'service');

  if (all.length === 0) {
    return <EmptyState icon={<FileText />} title={t('logs.noSources')} description={t('logs.noSourcesHelp')} />;
  }

  const group = (title: string, icon: ReactNode, list: Entry[]) => list.length === 0 ? null : (
    <div className="flex flex-col gap-1.5">
      <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        {icon}
        {title}
      </p>
      {list.map((entry) => (
        <button
          key={entry.key}
          type="button"
          aria-current={selected === entry.key ? 'true' : undefined}
          onClick={() => onSelect(entry.key)}
          className={`flex w-full flex-col gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
            selected === entry.key
              ? 'border-primary/50 bg-primary/10'
              : 'border-border bg-muted/30 hover:bg-accent'
          }`}
        >
          <span className="flex flex-wrap items-center justify-between gap-1.5">
            <span className="truncate text-xs font-medium">{entry.label}</span>
            <Badge tone={entry.kind === 'site' ? 'blue' : 'neutral'}>
              {t(entry.kind === 'site' ? 'logs.kind.site' : 'logs.kind.service')}
            </Badge>
          </span>
          <span className="truncate font-mono text-[11px] text-muted-foreground">{entry.name}</span>
          {entry.path && <span className="truncate text-[11px] text-muted-foreground">{entry.path}</span>}
        </button>
      ))}
    </div>
  );

  return (
    <div className="flex flex-col gap-4">
      {group(t('logs.group.sites'), <FolderOpen className="size-3.5" aria-hidden="true" />, sites)}
      {group(t('logs.group.services'), <Server className="size-3.5" aria-hidden="true" />, services)}
    </div>
  );
}
