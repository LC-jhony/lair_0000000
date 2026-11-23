import { useState, type CSSProperties } from 'react';
import { LoaderCircle, RefreshCw } from 'lucide-react';
import { AppSidebar, Button, PaneErrorBoundary, SettingsError } from './components';
import { SidebarInset, SidebarProvider, SidebarTrigger } from './components/ui/sidebar';
import { useT } from './i18n';
import { PANES, PaneContent, type PaneId } from './panes';
import { isMac, isWindows } from './platform';
import { useLair } from './useLair';

// The window keeps the OS title bar. On macOS and Windows the header doubles
// as the drag handle because the OS draws the window controls over it.
const drag = isMac || isWindows ? ({ WebkitAppRegion: 'drag' } as CSSProperties) : undefined;
const noDrag = { WebkitAppRegion: 'no-drag' } as CSSProperties;

export default function App() {
  const t = useT();
  const state = useLair();
  const [activePane, setActivePane] = useState<PaneId>('overview');
  const pane = PANES.find((item) => item.id === activePane) ?? PANES[0]!;

  return (
    <SidebarProvider className="h-svh overflow-hidden bg-background">
      <AppSidebar activePane={activePane} onSelect={setActivePane} state={state} />
      <SidebarInset className="flex min-w-0 flex-col">
        <header
          style={{ ...drag, ...(isWindows ? { paddingRight: 148 } : null) }}
          className="sticky top-0 z-30 flex h-16 shrink-0 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur md:px-6"
        >
          <SidebarTrigger style={noDrag} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span>{t('common.appName')}</span>
              <span aria-hidden="true">/</span>
              <span className="font-medium text-foreground">{t(pane.titleKey)}</span>
            </div>
            <h1 className="truncate text-base font-semibold tracking-tight">{t(pane.titleKey)}</h1>
            <p className="truncate text-xs text-muted-foreground">{t(pane.subtitleKey)}</p>
          </div>
          <div className="hidden items-center gap-2 sm:flex">
            {state.busy && (
              <span role="status" className="flex items-center gap-2 rounded-full bg-primary/10 px-3 py-1.5 text-xs text-primary">
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                <span className="max-w-56 truncate">{state.busy}</span>
              </span>
            )}
            <Button style={noDrag} variant="outline" size="icon" aria-label={t('dash.refresh')} disabled={!!state.busy} onClick={() => void state.refresh()}>
              <RefreshCw className="size-4" aria-hidden="true" />
            </Button>
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
          <div className="mx-auto w-full max-w-6xl">
            <PaneErrorBoundary key={activePane} onRetry={() => void state.refresh()}>
              <PaneContent id={activePane} state={state} onNavigate={setActivePane} />
            </PaneErrorBoundary>
          </div>
        </main>
      </SidebarInset>
      <SettingsError message={state.error} onDismiss={state.clearError} />
    </SidebarProvider>
  );
}
