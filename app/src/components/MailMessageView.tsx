import { Mail, Paperclip } from 'lucide-react';
import type { MailMessage } from '../api';
import { useT } from '../i18n';
import { Badge } from './Badge';
import { EmptyState } from './ui/EmptyState';
import { Tabs, TabsList, TabsPanel, TabsPanels, TabsTrigger } from './ui/Tabs';

export interface MailMessageViewProps {
  message: MailMessage | null;
}

const HTML_PATTERN = /<[a-z][\s\S]*>/i;

function looksLikeHtml(value: string | undefined): boolean {
  return typeof value === 'string' && HTML_PATTERN.test(value);
}

function headersText(message: MailMessage): string {
  const headers = message.headers;
  if (typeof headers === 'string') return headers;
  if (headers && typeof headers === 'object') {
    return Object.entries(headers)
      .map(([key, value]) => `${key}: ${typeof value === 'string' ? value : JSON.stringify(value)}`)
      .join('\n');
  }
  return '';
}

function attachmentLabel(attachment: NonNullable<MailMessage['attachments']>[number]): string {
  if (typeof attachment === 'string') return attachment;
  const name = attachment.name || '—';
  return typeof attachment.size === 'number' ? `${name} (${attachment.size} B)` : name;
}

/** Detail view for one caught message, with the four classic mail tabs. */
export function MailMessageView({ message }: MailMessageViewProps) {
  const t = useT();
  if (!message) {
    return <EmptyState icon={<Mail />} title={t('mail.noSelection')} description={t('mail.noSelectionHelp')} />;
  }

  const body = message.body ?? '';
  const html = message.html || (looksLikeHtml(body) ? body : '');
  const text = message.text || (looksLikeHtml(body) ? '' : body);
  const raw = message.raw || body;
  const headers = headersText(message);
  const tabs = [
    { id: 'html', label: t('mail.tab.html'), content: html },
    { id: 'text', label: t('mail.tab.text'), content: text },
    { id: 'raw', label: t('mail.tab.raw'), content: raw },
    { id: 'headers', label: t('mail.tab.headers'), content: headers },
  ] as const;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <p className="text-sm font-semibold">{message.subject || t('mail.noSubject')}</p>
        <p className="text-xs text-muted-foreground">
          {t('mail.from')}: {message.from || '—'} · {t('mail.to')}: {message.to || '—'}
        </p>
        <p className="text-xs text-muted-foreground">{message.date || '—'}</p>
      </div>

      {(message.attachments?.length ?? 0) > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <Paperclip className="size-3.5 text-muted-foreground" aria-hidden="true" />
          {message.attachments?.map((attachment, index) => (
            <Badge key={attachmentLabel(attachment) + index} tone="neutral">
              {attachmentLabel(attachment)}
            </Badge>
          ))}
        </div>
      )}

      <Tabs>
        <TabsList>
          {tabs.map((tab) => <TabsTrigger key={tab.id}>{tab.label}</TabsTrigger>)}
        </TabsList>
        <TabsPanels>
          {tabs.map((tab) => (
            <TabsPanel key={tab.id}>
              <pre className="max-h-64 overflow-auto rounded-lg border border-border bg-muted/40 p-3 text-xs whitespace-pre-wrap">
                {tab.content || t('mail.emptyPart')}
              </pre>
            </TabsPanel>
          ))}
        </TabsPanels>
      </Tabs>
    </div>
  );
}
