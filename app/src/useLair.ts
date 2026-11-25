import { useCallback, useEffect, useRef, useState } from 'react';
import { createClient, type DaemonClient, type JobStatus, type LairStatus } from './api';

/**
 * Single connection to the daemon for the whole desktop app. The status is
 * polled so the UI always reflects changes made from the CLI; every
 * action goes through `act`, which refreshes afterwards and reports errors in
 * the same banner used by the sidebar.
 */
export interface LairController {
  client: DaemonClient | null;
  status: LairStatus | null;
  error: string;
  busy: string;
  /** Progress of the job currently being awaited, if any. */
  jobProgress?: JobStatus | null;
  refresh: () => Promise<void>;
  act: (
    label: string,
    fn: (client: DaemonClient, signal?: AbortSignal) => Promise<unknown>,
  ) => Promise<void>;
  /** Aborts the in-flight action (if any) without reporting an error. */
  cancel: () => void;
  clearError: () => void;
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function isConnectionError(err: unknown): boolean {
  const message = errorMessage(err).toLowerCase();
  return message.includes('daemon is not available') ||
    message.includes('network error') ||
    message.includes('timed out') ||
    message.includes('failed to fetch') ||
    message.includes('daemon responded 0');
}

/** Resolves early when aborted so callers can check `signal.aborted` instead
 * of dealing with a rejected promise. */
function wait(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) return resolve();
    let timer: ReturnType<typeof setTimeout>;
    const done = () => {
      clearTimeout(timer);
      signal.removeEventListener('abort', done);
      resolve();
    };
    timer = setTimeout(done, ms);
    signal.addEventListener('abort', done, { once: true });
  });
}

const REFRESH_MS = 3000;
const MAX_REFRESH_MS = 30000;
const JOB_POLL_MS = 300;
const MAX_JOB_POLL_MS = 3000;

/**
 * `clientFactory` lets a different daemon client supply its
 * own daemon connector while reusing the exact same state machine.
 */
export function useLair(
  clientFactory: () => Promise<DaemonClient> = createClient,
): LairController {
  const [client, setClient] = useState<DaemonClient | null>(null);
  const [status, setStatus] = useState<LairStatus | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [jobProgress, setJobProgress] = useState<JobStatus | null>(null);

  const aliveRef = useRef(true);
  const busyRef = useRef(false);
  const controllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef<DaemonClient | null>(null);

  // The daemon may start after the window (or restart after a crash). Try again
  // periodically instead of leaving a permanent "daemon is not available".
  useEffect(() => {
    let alive = true;
    let attempts = 0;
    aliveRef.current = true;
    const connect = async () => {
      if (clientRef.current || !alive) return;
      try {
        const next = await clientFactory();
        if (!alive) return;
        clientRef.current = next;
        setClient(next);
        setError('');
      } catch (e) {
        if (!alive) return;
        if (attempts === 0) setError(errorMessage(e));
        attempts++;
      }
    };
    void connect();
    const timer = setInterval(() => void connect(), 2000);
    return () => {
      alive = false;
      clearInterval(timer);
      aliveRef.current = false;
      controllerRef.current?.abort();
      controllerRef.current = null;
      busyRef.current = false;
    };
  }, [clientFactory]);

  const invalidateClient = useCallback(() => {
    clientRef.current = null;
    setClient(null);
    setStatus(null);
  }, []);

  const refresh = useCallback(async () => {
    if (!client || !aliveRef.current) return;
    try {
      const next = await client.status();
      if (aliveRef.current) setStatus(next);
    } catch (e) {
      if (!aliveRef.current) return;
      setError(errorMessage(e));
      if (isConnectionError(e)) invalidateClient();
    }
  }, [client, invalidateClient]);

  // Status polling uses exponential backoff on failure and resets on success,
  // so a temporarily unreachable daemon does not flood the bridge.
  useEffect(() => {
    if (!client) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    let delayMs = REFRESH_MS;
    const tick = async () => {
      if (cancelled) return;
      // A hidden window does not need a fresh status snapshot: skip the bridge
      // round trip and re-render until it becomes visible again.
      if (typeof document !== 'undefined' && document.hidden) {
        timer = setTimeout(() => void tick(), MAX_REFRESH_MS);
        return;
      }
      try {
        const next = await client.status();
        if (cancelled) return;
        setStatus(next);
        delayMs = REFRESH_MS;
      } catch (e) {
        if (isConnectionError(e)) {
          invalidateClient();
          return;
        }
        delayMs = Math.min(delayMs * 2, MAX_REFRESH_MS);
      } finally {
        if (!cancelled) timer = setTimeout(() => void tick(), delayMs);
      }
    };
    void tick();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [client, invalidateClient]);

  const cancel = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    busyRef.current = false;
    if (aliveRef.current) setBusy('');
    setJobProgress(null);
  }, []);

  const act = useCallback(
    async (
      label: string,
      fn: (c: DaemonClient, signal?: AbortSignal) => Promise<unknown>,
    ) => {
      if (!client || busyRef.current || !aliveRef.current) return;
      busyRef.current = true;
      const controller = new AbortController();
      controllerRef.current = controller;
      setBusy(label);
      setError('');
      try {
        const result = await fn(client, controller.signal);
        if (controller.signal.aborted || !aliveRef.current) return;

        // Install actions return a job: wait for it, then make sure the shims
        // exist so `node -v`/`npm -v` work no matter which UI started it.
        const job = result as JobStatus | undefined;
        if (job && typeof job === 'object' && job.id) {
          setJobProgress(job);
          let failed = Boolean(job.error);
          let canceled = Boolean(job.canceled);
          if (!job.done && !failed && !canceled) {
            let attempt = 0;
            for (;;) {
              await wait(Math.min(JOB_POLL_MS * 2 ** attempt, MAX_JOB_POLL_MS), controller.signal);
              if (controller.signal.aborted || !aliveRef.current) {
                canceled = true;
                break;
              }
              try {
                const current = await client.job(job.id as string, controller.signal);
                setJobProgress(current);
                if (current.error || current.canceled) {
                  failed = Boolean(current.error);
                  canceled = Boolean(current.canceled);
                  break;
                }
                if (current.done) break;
                attempt = 0;
              } catch {
                if (controller.signal.aborted || !aliveRef.current) {
                  canceled = true;
                  break;
                }
                // Transient poll failure: back off and try again.
                attempt += 1;
              }
            }
          }
          // A failed or canceled job must not trigger the shim repair.
          if (!failed && !canceled) {
            try {
              await client.ensureShims(controller.signal);
            } catch {
              if (typeof window !== 'undefined') {
                await window.lairDesktop?.repairShims?.().catch(() => undefined);
              }
            }
          }
        }
        if (!controller.signal.aborted && aliveRef.current) await refresh();
      } catch (e) {
        if (!controller.signal.aborted && aliveRef.current) setError(errorMessage(e));
      } finally {
        if (controllerRef.current === controller) {
          controllerRef.current = null;
          busyRef.current = false;
          if (aliveRef.current) setBusy('');
        }
        setJobProgress(null);
      }
    },
    [client, refresh],
  );

  const clearError = useCallback(() => setError(''), []);

  return { client, status, error, busy, jobProgress, refresh, act, cancel, clearError };
}
