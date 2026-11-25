import { connectFromDesktop, DaemonError } from './client';
import { FakeDaemon, demoFixture } from './fake';
import type { DaemonClient } from './models';

export { ACTIONS, connectFromDesktop, DaemonError, HttpDaemonClient } from './client';
export { demoFixture, FakeDaemon } from './fake';
export { activeVersion, installedVersions, jobProgress, projectProgress, toComponentJob } from './models';
export type {
  DaemonClient,
  DBResult,
  SnapshotInfo,
  JobStatus,
  QueueInfo,
  ServiceHealth,
  SiteData,
  SiteStack,
  LairStatus,
  PhpInfo,
  ServiceView,
  SiteView,
  ProjectCreatePayload,
  ProjectInfo,
  ProjectOp,
  ProjectResult,
  WebView,
  WebResult,
  MailResult,
  MailMessage,
  MailAttachment,
  MailPayload,
  MailOp,
  DumpsResult,
  DumpEvent,
  QueryStat,
  SlowQuery,
  NPlusOne,
  DumpInsights,
  RequestTimeline,
  DumpsOptions,
  DumpsPayload,
  DumpsOp,
  DebuggerResult,
  DebuggerVersion,
  DebuggerPayload,
  DebuggerOp,
  LogSource,
  NameList,
  SiteDetail,
  SiteEnvEntry,
  SiteResult,
  SiteOp,
  SitePayload,
} from './models';

/** Uses the FakeDaemon when VITE_FAKE_DAEMON=1 or the URL has ?fake=1. */
export async function createClient(): Promise<DaemonClient> {
  const forced =
    import.meta.env.VITE_FAKE_DAEMON === '1' ||
    new URLSearchParams(window.location.search).get('fake') === '1';
  if (forced) return new FakeDaemon({ fixture: demoFixture });

  const client = await connectFromDesktop();
  if (!client) throw new DaemonError('daemon is not available', 0);
  return client;
}
