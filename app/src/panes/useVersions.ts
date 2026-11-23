import { useEffect, useState } from 'react';
import type { DaemonClient } from '../api';

/** Loads the signed-catalog version list for a fixed set of packages. */
export function useVersions(client: DaemonClient | null | undefined, packages: string[]) {
  const [versions, setVersions] = useState<Record<string, string[]>>({});
  // JSON avoids collisions when a package name contains the old separator.
  const key = JSON.stringify(packages);

  useEffect(() => {
    if (!client) return;
    let alive = true;
    const names = (JSON.parse(key) as string[]);
    void Promise.all(
      names.map(async (name) => {
        try {
          return [name, await client.versions(name)] as const;
        } catch {
          return [name, []] as const;
        }
      }),
    ).then((entries) => {
      if (!alive) return;
      setVersions(Object.fromEntries(entries));
    });
    return () => {
      alive = false;
    };
  }, [client, key]);

  return versions;
}
