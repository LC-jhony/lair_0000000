import { useState } from 'react';
import { useDialogFocus } from '../hooks/use-dialog-focus';
import { useT } from '../i18n';

export interface PromptField {
  label: string;
  value?: string;
  placeholder?: string;
}

export interface PromptDialogProps {
  title: string;
  description?: string;
  fields: PromptField[];
  confirmLabel?: string;
  onSubmit: (values: string[]) => void;
  onCancel: () => void;
}

/** Small modal for the actions that need one or two values (park, link, ...). */
export function PromptDialog({
  title,
  description,
  fields,
  confirmLabel,
  onSubmit,
  onCancel,
}: PromptDialogProps) {
  const t = useT();
  const dialogRef = useDialogFocus<HTMLDivElement>(true, onCancel);
  const [values, setValues] = useState(() => fields.map((f) => f.value ?? ''));
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onCancel}>
      <div
        ref={dialogRef}
        className="dialog"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2 className="card-title">{title}</h2>
        {description && <p className="item-meta">{description}</p>}
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit(values);
          }}
        >
          {fields.map((field, index) => (
            <label className="dialog-field" key={`${field.label}-${index}`}>
              <span>{field.label}</span>
              <input
                autoFocus={index === 0}
                value={values[index]}
                placeholder={field.placeholder}
                onChange={(event) => {
                  const next = [...values];
                  next[index] = event.target.value;
                  setValues(next);
                }}
              />
            </label>
          ))}
          <div className="dialog-actions">
            <button className="button button-small" type="button" onClick={onCancel}>
              {t('common.cancel')}
            </button>
            <button className="button button-small" type="submit">
              {confirmLabel ?? t('common.save')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
