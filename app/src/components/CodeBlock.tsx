import { useEffect, useRef, useState } from 'react';
import { Copy } from 'lucide-react';
import { useT } from '../i18n';
import { Button } from './ui/ActionButton';

const COPIED_MS = 2000;

export interface CodeBlockProps {
  text: string;
  copyLabelKey?: string;
  copiedLabelKey?: string;
}

export function CodeBlock({
  text,
  copyLabelKey = 'common.copy',
  copiedLabelKey = 'common.copied',
}: CodeBlockProps) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const timerRef = useRef(0);

  useEffect(() => () => window.clearTimeout(timerRef.current), []);

  const copy = async () => {
    try {
      await navigator.clipboard?.writeText(text);
      setCopied(true);
      window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setCopied(false), COPIED_MS);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/40 px-3 py-2">
      <pre className="overflow-x-auto text-xs">{text}</pre>
      <Button variant="ghost" size="sm" className="h-7 shrink-0 px-2" onClick={() => void copy()}>
        <Copy className="size-3.5" aria-hidden="true" />
        {t(copied ? copiedLabelKey : copyLabelKey)}
      </Button>
    </div>
  );
}
