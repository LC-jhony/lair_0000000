import { act as rtlAct, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createClient, type DaemonClient } from '../api';
import { useLair } from '../useLair';

vi.mock('../api', () => ({ createClient: vi.fn() }));

function fakeClient(overrides: Partial<DaemonClient> = {}): DaemonClient {
  return {
    status: vi.fn(async () => ({})),
    job: vi.fn(async () => ({ done: true })),
    ensureShims: vi.fn(async () => {}),
    ...overrides,
  } as unknown as DaemonClient;
}

describe('useLair', () => {
  beforeEach(() => {
    vi.mocked(createClient).mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('reintenta la conexión cuando el daemon vuelve', async () => {
    const client = fakeClient();
    vi.mocked(createClient)
      .mockRejectedValueOnce(new Error('daemon is not available'))
      .mockResolvedValue(client);

    const { result } = renderHook(() => useLair());
    await waitFor(() => expect(result.current.error).toContain('daemon is not available'));
    await waitFor(() => expect(result.current.client).toBe(client), { timeout: 5000 });
    expect(createClient).toHaveBeenCalledTimes(2);
    expect(result.current.error).toBe('');
  });

  it('impide acciones solapadas y cancela la petición en curso', async () => {
    let finish: (value: unknown) => void = () => {};
    const inFlight = new Promise((resolve) => {
      finish = resolve;
    });
    const client = fakeClient();
    vi.mocked(createClient).mockResolvedValue(client);
    const { result } = renderHook(() => useLair());
    await waitFor(() => expect(result.current.client).toBe(client));

    let seen: AbortSignal | undefined;
    const first = vi.fn((_c: DaemonClient, signal?: AbortSignal) => {
      seen = signal;
      return inFlight;
    });
    const second = vi.fn(async () => {});

    void result.current.act('trabajando', first);
    await waitFor(() => expect(result.current.busy).toBe('trabajando'));
    await result.current.act('otra', second);
    expect(second).not.toHaveBeenCalled();

    rtlAct(() => result.current.cancel());
    expect(seen?.aborted).toBe(true);
    expect(result.current.busy).toBe('');
    finish({});
    await waitFor(() => expect(first).toHaveBeenCalledTimes(1));
  });

  it('no regenera shims cuando el job termina en error', async () => {
    let jobSignal: AbortSignal | undefined;
    const job = vi.fn(async (_id: string, signal?: AbortSignal) => {
      jobSignal = signal;
      return { id: 'job-1', done: true, error: 'boom' };
    });
    const ensureShims = vi.fn(async () => {});
    const client = fakeClient({ job, ensureShims });
    vi.mocked(createClient).mockResolvedValue(client);
    const { result } = renderHook(() => useLair());
    await waitFor(() => expect(result.current.client).toBe(client));

    await result.current.act('instalando', async () => ({ id: 'job-1', done: false }));
    expect(jobSignal).toBeInstanceOf(AbortSignal);
    expect(ensureShims).not.toHaveBeenCalled();
  });
});
