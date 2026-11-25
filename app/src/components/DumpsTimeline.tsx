import { useState } from 'react';
import { Eye, EyeOff, SearchX } from 'lucide-react';
import type { DumpEvent } from '../api';
import { useT } from '../i18n';
import { Badge, type BadgeTone } from './Badge';
import { Button } from './ui/ActionButton';
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

/** Symfony VarDumper output arrives through the log capture; classify it as a view. */
function eventType(event: DumpEvent): string {
  const message = event.message ?? '';
  if (message.includes('sf-dump') || message.includes('Sfdump')) return 'views';
  if (/LAIR-DUMPS-DEMO|\[.*\].*(RUNNING|DONE)/.test(message)) return 'jobs';
  return event.type ?? 'other';
}

/** Strip Symfony's inline CSS/JS so the dump payload is readable in the pane. */
function readable(message: string): string {
  let text = message
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
    .replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '')
    .replace(/<[^>]+>/g, '')
    .replace(/&quot;/g, '"')
    .replace(/&#039;|&apos;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
    .trim();
  if (text.length > 4000) text = `${text.slice(0, 4000)}…`;
  return text || message.slice(0, 4000);
}

/** Compact, searchable timeline of captured dump events. */
export function DumpsTimeline({ events = [], query = '', latestOnly = false }: DumpsTimelineProps) {
  const t = useT();
  const needle = query.trim().toLowerCase();
  const [rawIds, setRawIds] = useState<Set<string>>(new Set());

  let visible = [...events].reverse();
  if (latestOnly) {
    const seen = new Set<string>();
    visible = visible.filter((event) => {
      const key = event.request_id || event.request || event.id || '';
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }
  if (needle) {
    visible = visible.filter((event) =>
      [event.site, eventType(event), event.time, event.request, event.message]
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
                <Badge tone={TYPE_TONES[eventType(event)] ?? 'neutral'}>
                  {t(`dumps.type.${eventType(event)}`)}
                </Badge>
                <span className="text-xs font-medium">{event.site || '—'}</span>
              </div>
              <span className="font-mono text-[11px] text-muted-foreground">{event.time || '—'}</span>
            </div>
            <div className="flex items-center justify-between gap-2">
              <p className="truncate text-xs text-muted-foreground">{event.request || '—'}</p>
              <Button
                variant="ghost"
                size="sm"
                aria-label={rawIds.has(event.id ?? '') ? t('dumps.hideRaw') : t('dumps.viewRaw')}
                onClick={() => setRawIds((previous) => {
                  const next = new Set(previous);
                  const key = event.id ?? '';
                  if (next.has(key)) next.delete(key); else next.add(key);
                  return next;
                })}
              >
                {rawIds.has(event.id ?? '') ? <EyeOff className="size-3.5" aria-hidden="true" /> : <Eye className="size-3.5" aria-hidden="true" />}
                {rawIds.has(event.id ?? '') ? t('dumps.hideRaw') : t('dumps.viewRaw')}
              </Button>
            </div>
            <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded-md bg-background/70 p-2 text-xs">
              {rawIds.has(event.id ?? '') ? (event.message || '—') : readable(event.message ?? '') || '—'}
            </pre>
          </div>
        </li>
      ))}
    </ol>
  );
}
