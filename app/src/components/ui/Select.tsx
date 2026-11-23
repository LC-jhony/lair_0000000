import {
  Listbox,
  ListboxButton,
  ListboxOption,
  ListboxOptions,
} from '@headlessui/react';
import { Check, ChevronsUpDown } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface SelectOption {
  value: string;
  label: string;
  description?: string;
}

export interface SelectProps {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  label?: string;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

/** Searchable-looking version picker built on Headless UI Listbox. */
export function Select({
  value,
  onChange,
  options,
  label,
  placeholder = 'Select…',
  disabled,
  className,
}: SelectProps) {
  const selected = options.find((option) => option.value === value);
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
      {label && <span className="text-xs font-medium text-muted-foreground">{label}</span>}
      <Listbox value={value} onChange={onChange} disabled={disabled}>
        <div className="relative">
          <ListboxButton
            aria-label={label ?? placeholder}
            className={cn(
              'flex h-9 w-full items-center justify-between gap-2 rounded-lg border border-input bg-background px-3 text-left text-sm shadow-sm',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            <span className="truncate">{selected?.label ?? placeholder}</span>
            <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          </ListboxButton>
          <ListboxOptions
            anchor="bottom start"
            className={cn(
              'z-50 max-h-72 w-[var(--button-width)] overflow-auto rounded-lg border border-border bg-popover p-1 shadow-lg',
              'focus:outline-none [--anchor-gap:4px]',
            )}
          >
            {options.map((option) => (
              <ListboxOption
                key={option.value}
                value={option.value}
                className="flex cursor-default select-none items-start justify-between gap-2 rounded-md px-2 py-1.5 text-sm data-focus:bg-accent data-focus:text-accent-foreground"
              >
                <span className="min-w-0">
                  <span className="block truncate font-medium">{option.label}</span>
                  {option.description && (
                    <span className="block truncate text-xs text-muted-foreground">{option.description}</span>
                  )}
                </span>
                {option.value === value && <Check className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />}
              </ListboxOption>
            ))}
          </ListboxOptions>
        </div>
      </Listbox>
    </div>
  );
}
