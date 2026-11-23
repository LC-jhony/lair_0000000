import {
  Tab,
  TabGroup,
  TabList,
  TabPanel,
  TabPanels,
} from '@headlessui/react';
import type { ComponentProps } from 'react';
import { cn } from '@/lib/utils';

export function Tabs({ className, ...props }: ComponentProps<typeof TabGroup>) {
  return <TabGroup className={cn('flex flex-col gap-4', className)} {...props} />;
}

export function TabsList({ className, ...props }: ComponentProps<typeof TabList>) {
  return (
    <TabList
      className={cn('inline-flex gap-1 rounded-lg border border-border bg-muted p-1', className)}
      {...props}
    />
  );
}

export function TabsTrigger({ className, ...props }: ComponentProps<typeof Tab>) {
  return (
    <Tab
      className={cn(
        'rounded-md px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        'data-selected:bg-background data-selected:text-foreground data-selected:shadow-sm',
        className,
      )}
      {...props}
    />
  );
}

export function TabsPanels({ className, ...props }: ComponentProps<typeof TabPanels>) {
  return <TabPanels className={cn('focus:outline-none', className)} {...props} />;
}

export function TabsPanel({ className, ...props }: ComponentProps<typeof TabPanel>) {
  return <TabPanel className={cn('focus:outline-none', className)} {...props} />;
}
