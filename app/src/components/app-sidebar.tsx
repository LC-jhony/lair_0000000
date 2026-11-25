import {
  BugPlay,
  Code2,
  Database,
  FileText,
  Globe2,
  Languages,
  History,
  LayoutDashboard,
  ListTodo,
  Mail,
  ReceiptText,
  RefreshCw,
  Server,
  Settings2,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@/components/ui/sidebar';
import { PANES, type PaneDefinition, type PaneId } from '../panes';
import { LOCALES, useI18n, useT, type Locale } from '../i18n';
import type { LairController } from '../useLair';
import { Button } from './ui/ActionButton';
import { Select } from './ui/Select';
import { StatusDot } from './StatusDot';

const PANE_ICONS: Record<PaneId, LucideIcon> = {
  overview: LayoutDashboard,
  databases: Database,
  runtimes: Code2,
  services: Server,
  sites: Globe2,
  mail: Mail,
  dumps: ReceiptText,
  debugger: BugPlay,
  logs: FileText,
  jobs: ListTodo,
  activity: History,
  system: Settings2,
};

function PaneLink({
  pane,
  active,
  onSelect,
}: {
  pane: PaneDefinition;
  active: boolean;
  onSelect: (id: PaneId) => void;
}) {
  const t = useT();
  const Icon = PANE_ICONS[pane.id];
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        type="button"
        isActive={active}
        onClick={() => onSelect(pane.id)}
        aria-current={active ? 'page' : undefined}
      >
        <Icon aria-hidden="true" />
        <span>{t(pane.titleKey)}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function LanguageSelect() {
  const { locale, setLocale, t } = useI18n();
  return (
    <div className="flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
      <Languages className="size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
        <Select
          value={locale}
          onChange={(value) => setLocale(value as Locale)}
          placeholder={t('sidebar.language')}
          options={LOCALES.map((code) => ({ value: code, label: code.toUpperCase() }))}
        />
      </div>
    </div>
  );
}

export function AppSidebar({
  activePane,
  onSelect,
  state,
}: {
  activePane: PaneId;
  onSelect: (id: PaneId) => void;
  state: LairController;
}) {
  const t = useT();
  const connected = Boolean(state.status && !state.error);
  const workspace = PANES.filter((pane) => pane.id !== 'system');
  const system = PANES.filter((pane) => pane.id === 'system');

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader className="border-b border-sidebar-border px-4 py-3 group-data-[collapsible=icon]:px-0 group-data-[collapsible=icon]:pb-4">
        <div className="flex items-center gap-2.5 group-data-[collapsible=icon]:justify-center">
          <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-red-600 text-white shadow-sm group-data-[collapsible=icon]:size-9">
            <span className="text-xs font-bold tracking-tight">L</span>
          </span>
          <div className="min-w-0 group-data-[collapsible=icon]:hidden">
            <p className="truncate text-sm font-semibold tracking-tight">{t('common.appName')}</p>
            <p className="truncate text-[11px] text-muted-foreground">Local development</p>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>{t('sidebar.workspace')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {workspace.map((pane) => (
                <PaneLink key={pane.id} pane={pane} active={activePane === pane.id} onSelect={onSelect} />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>{t('pane.system')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {system.map((pane) => (
                <PaneLink key={pane.id} pane={pane} active={activePane === pane.id} onSelect={onSelect} />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="border-t border-sidebar-border p-3">
        <LanguageSelect />
        <div className="flex items-center gap-2 rounded-lg bg-sidebar-accent px-2.5 py-2 text-sidebar-accent-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
          <StatusDot on={connected} />
          <span className="min-w-0 flex-1 truncate text-xs group-data-[collapsible=icon]:hidden">
            {connected ? t('sidebar.connected') : t('dash.waiting')}
          </span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="w-full justify-start text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          disabled={!!state.busy}
          onClick={() => void state.refresh()}
        >
          <RefreshCw className="size-4" aria-hidden="true" />
          <span className="group-data-[collapsible=icon]:hidden">{t('dash.refresh')}</span>
        </Button>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
