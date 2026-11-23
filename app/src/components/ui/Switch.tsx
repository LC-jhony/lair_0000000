import { Description, Field, Label, Switch as HeadlessSwitch } from '@headlessui/react';
import { cn } from '@/lib/utils';

export interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  description?: string;
  disabled?: boolean;
  className?: string;
}

/** Accessible on/off control used for service start/stop. */
export function Switch({ checked, onChange, label, description, disabled, className }: SwitchProps) {
  return (
    <Field className={cn('flex items-center justify-between gap-3', className)} disabled={disabled}>
      <span className="min-w-0">
        <Label className="block truncate text-sm font-medium text-foreground">{label}</Label>
        {description && (
          <Description className="mt-0.5 block text-xs text-muted-foreground">{description}</Description>
        )}
      </span>
      <HeadlessSwitch
        checked={checked}
        onChange={onChange}
        className={cn(
          'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
          'disabled:cursor-not-allowed disabled:opacity-50',
          checked ? 'bg-primary' : 'bg-input',
        )}
      >
        <span
          className="size-5 rounded-full bg-white shadow-sm transition-transform"
          style={{ transform: checked ? 'translateX(1.4rem)' : 'translateX(0.15rem)' }}
          aria-hidden="true"
        />
      </HeadlessSwitch>
    </Field>
  );
}
