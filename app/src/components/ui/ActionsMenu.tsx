import { Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/react';
import { Ellipsis } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface MenuAction {
  label: string;
  onSelect: () => void;
  disabled?: boolean;
  destructive?: boolean;
}

export function ActionsMenu({ label, actions }: { label: string; actions: MenuAction[] }) {
  return (
    <Menu>
      <MenuButton
        aria-label={label}
        className="inline-flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Ellipsis className="size-4" aria-hidden="true" />
      </MenuButton>
      <MenuItems
        anchor="bottom end"
        className="z-50 w-48 rounded-lg border border-border bg-popover p-1 shadow-lg focus:outline-none [--anchor-gap:4px]"
      >
        {actions.map((action) => (
          <MenuItem key={action.label} as="button" disabled={action.disabled} onClick={action.onSelect}
            className={cn(
              'flex w-full items-center rounded-md px-2 py-1.5 text-left text-sm transition-colors',
              'data-focus:bg-accent data-focus:text-accent-foreground data-disabled:opacity-40',
              action.destructive && 'text-destructive data-focus:bg-destructive/10 data-focus:text-destructive',
            )}
          >
            {action.label}
          </MenuItem>
        ))}
      </MenuItems>
    </Menu>
  );
}
