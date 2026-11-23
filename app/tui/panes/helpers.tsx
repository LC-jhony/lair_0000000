import { Box, Text, useInput } from 'ink';
import React, { useEffect, useState, type ReactNode } from 'react';
import { Card } from '../components/ui/card';
import { KeyValue } from '../components/ui/key-value';

export function Section({
  title,
  subtitle,
  children,
  width = 70,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  width?: number;
}) {
  return (
    <Card title={title} subtitle={subtitle} width={width} marginBottom={1}>
      {children}
    </Card>
  );
}

export function Facts({ items, keyWidth = 20 }: { items: { key: string; value: ReactNode }[]; keyWidth?: number }) {
  return <KeyValue items={items} keyWidth={keyWidth} />;
}

/** Maps single keys to actions; active panes only. */
export function useKeys(actions: Record<string, () => void>, enabled = true) {
  useInput((input, key) => {
    if (!enabled || key.ctrl || key.meta || key.tab || key.return) return;
    const run = actions[input.toLowerCase()];
    if (run) run();
  });
}

/** Minimal line prompt for panes that need an argument (database name, host…). */
export function usePrompt(submit: (value: string) => void) {
  const [active, setActive] = useState(false);
  const [value, setValue] = useState('');
  useInput((input, key) => {
    if (!active) return;
    if (key.return) {
      submit(value);
      setValue('');
      setActive(false);
      return;
    }
    if (key.escape) {
      setValue('');
      setActive(false);
      return;
    }
    if (key.backspace) {
      setValue((v) => v.slice(0, -1));
      return;
    }
    if (input && !key.ctrl && !key.meta) setValue((v) => v + input);
  });
  return { active, value, start: () => setActive(true) };
}

export function PromptLine({ label, value }: { label: string; value: string }) {
  return (
    <Box>
      <Text color="yellow">{label}: </Text>
      <Text>{value}</Text>
      <Text inverse> </Text>
      <Text dimColor> (Enter: confirm, Esc: cancel)</Text>
    </Box>
  );
}

/** Loads a tooling payload and keeps it in sync with the daemon state. */
export function useAsync<T>(loader: () => Promise<T>, deps: unknown[]): T | null {
  const [data, setData] = useState<T | null>(null);
  useEffect(() => {
    let alive = true;
    void loader().then((value) => { if (alive) setData(value); }).catch(() => undefined);
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return data;
}

export function Hint({ text }: { text: string }) {
  return (
    <Box marginTop={1}>
      <Text dimColor>{text}</Text>
    </Box>
  );
}
