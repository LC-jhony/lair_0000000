import { SearchX } from 'lucide-react';
import type { DumpEvent } from '../api';
import { useT } from '../i18n';
import { Badge, type BadgeTone } from './Badge';
import { EmptyState } from './ui/EmptyState';

export interface DumpsTimelineProps {
  events?: DumpEvent[];
  query?: string;
  /** Keep only the newest event for each request. */
  latestOnly?: boolean;
}

const TYPE_TONES: Record<string, BadgeTone> = {
  queries: 'indigo',
  logs: 'green',
  views: 'blue',
  jobs: 'orange',
  requests: 'purple',
};

/** Compact, searchable timeline of captured dump events. */
export function DumpsTimeline({ events = [], query = '', latestOnly = false }: DumpsTimelineProps) {
  const t = useT();
  const needle = query.trim().toLowerCase();

  let visible = [...events].reverse();
  if (latestOnly) {
    const seen = new Set<string>();
    visible = visible.filter((event) => {
      const key = event.request || event.id || '';
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }
  if (needle) {
    visible = visible.filter((event) =>
      [event.site, event.type, event.time, event.request, event.message]
        .some((value) => (value ?? '').toLowerCase().includes(needle)),
    );
  }

  if (visible.length === 0) {
    return (
      <EmptyState
        icon={<SearchX />}
        title={t('dumps.noEvents')}
        description={needle ? t('dumps.noMatches') : t('dumps.noEventsHelp')}
      />
    );
  }

  return (
    <ol className="flex flex-col gap-3 border-l border-border pl-5">
      {visible.map((event, index) => (
        <li key={event.id ?? `${event.time}-${index}`} className="relative">
          <span
            className="absolute -left-[1.6rem] top-1.5 size-2.5 rounded-full border-2 border-card bg-primary"
            aria-hidden="true"
          />
          <div className="flex flex-col gap-1.5 rounded-lg border border-border bg-muted/30 p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex flex-wrap items-center gap-1.5">
                <Badge tone={TYPE_TONES[event.type ?? ''] ?? 'neutral'}>
                  {t(`dumps.type.${event.type ?? 'other'}`)}
                </Badge>
                <span className="text-xs font-medium">{event.site || '—'}</span>
              </div>
              <span className="font-mono text-[11px] text-muted-foreground">{event.time || '—'}</span>
            </div>
            <p className="truncate text-xs text-muted-foreground">{event.request || '—'}</p>
            <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded-md bg-background/70 p-2 text-xs">
              {event.message || '—'}
            </pre>
          </div>
        </li>
      ))}
    </ol>
  );
}
