import { useState } from 'react';
import {
  Button,
  Dialog,
  StatusDot,
  Switch,
} from '../components';
import type { ServiceView } from '../api';
import { useT } from '../i18n';
import type { LairController } from '../useLair';

interface ServiceToggleProps {
  id: string;
  service: ServiceView;
  state: LairController;
}

/** Same semantics as the TUI: stop a running service, restart a stopped one. */
export function ServiceToggle({ id, service, state }: ServiceToggleProps) {
  const t = useT();
  const [confirm, setConfirm] = useState(false);
  const running = Boolean(service.running);

  const apply = () => {
    void state.act(t(running ? 'dash.stopping' : 'dash.starting', { name: id }), (c) =>
      running ? c.stopService(id) : c.restartService(id),
    );
    setConfirm(false);
  };

  return (
    <>
      <Switch
        checked={running}
        onChange={() => setConfirm(true)}
        disabled={!!state.busy}
        label={id}
        description={running ? t('common.active') : t('common.stopped')}
      />
      <Dialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={t(running ? 'dash.confirmStop' : 'dash.confirmStart', { name: id })}
        description={running ? t('services.stop.message', { name: id }) : undefined}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(false)}>{t('common.cancel')}</Button>
            <Button variant={running ? 'danger' : 'primary'} onClick={apply}>
              <StatusDot on={running} />
              {running ? t('common.stop') : t('common.start')}
            </Button>
          </>
        }
      />
    </>
  );
}
