import { useState } from "react";
import { useWorkspaceId } from "@multica/core/hooks";
import { useInstanceUpdate } from "@multica/core/runtimes";
import { Button } from "@multica/ui/components/ui/button";
import { useHostedClient } from "./hosted-client-install";
import { useT } from "../../i18n";
export function InstanceUpdateSection({ runtimeId, currentVersion }: { runtimeId: string; currentVersion?: string | null }) {
  const { t } = useT("runtimes");
  const wsId = useWorkspaceId();
  const u = useInstanceUpdate(wsId, runtimeId);
  const info = u.status.data;
  const hosted = useHostedClient();
  const [install, setInstall] = useState(false);
  const messages: Record<string, string> = {
    bootstrap_required: t(($) => $.instance_update.bootstrap),
    managed_by_desktop: t(($) => $.update.managed_by_desktop_title),
    read_only: t(($) => $.update.read_only_title),
    offline: t(($) => $.instance_update.offline),
    platform_unavailable: t(($) => $.instance_update.platform),
    release_unavailable: t(($) => $.instance_update.unavailable),
    current: t(($) => $.update.latest),
    unrecognized_or_newer: t(($) => $.instance_update.unrecognized),
  };
  return (
    <section
      className="space-y-2 text-caption"
      aria-label={t(($) => $.instance_update.title)}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-muted-foreground">
          {t(($) => $.update.cli_version_label)}
        </span>
        <span className="font-mono">
          {info?.currentVersion || currentVersion || t(($) => $.update.version_unknown)}
        </span>
        {info?.updateAvailable && (
          <span className="text-info">
            {info.latestVersion} · {t(($) => $.update.available)}
          </span>
        )}
        {info?.canUpdate && (
          <Button
            variant="outline"
            size="xs"
            disabled={u.busy || u.status.isError}
            onClick={() => u.mutation.mutate(info.latestVersion)}
          >
            {t(($) => $.update.action)}
          </Button>
        )}
      </div>
      {u.status.isError ? (
        <p role="alert">
          {t(($) => $.instance_update.unavailable)}{" "}
          <Button
            variant="ghost"
            size="xs"
            onClick={() => void u.status.refetch()}
          >
            {t(($) => $.update.retry)}
          </Button>
        </p>
      ) : !info ? (
        <p role="status">{t(($) => $.instance_update.checking)}</p>
      ) : (
        <p className="text-muted-foreground">
          {messages[info.reason] ?? t(($) => $.instance_update.idle)}
        </p>
      )}
      {u.busy && (
        <p role="status">
          {u.result.isError
            ? t(($) => $.instance_update.connection)
            : u.result.data?.status === "completed"
              ? t(($) => $.instance_update.restarting)
              : t(($) => $.instance_update.updating)}
        </p>
      )}
      {u.confirmed && (
        <p role="status" className="text-success">
          {t(($) => $.instance_update.confirmed)}
        </p>
      )}
      {!u.confirmed && (u.failed || u.expired || u.mutation.isError) && (
        <p role="alert" className="text-destructive">
          {u.expired
            ? t(($) => $.instance_update.timeout)
            : u.result.data?.error ||
              u.mutation.error?.message ||
              t(($) => $.update.unknown_error)}
        </p>
      )}
      {u.result.data?.output && (
        <div className="space-y-2">
          <p className="font-medium">{t(($) => $.instance_update.logs)}</p>
          <pre role="log" aria-label={t(($) => $.instance_update.logs)} aria-live="polite" className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-caption">
            {u.result.data.output}
          </pre>
        </div>
      )}
        <>
          <Button
            variant="outline"
            size="xs"
            onClick={() => setInstall(!install)}
            aria-expanded={install}
          >
            {t(($) => $.instance_update.manual)}
          </Button>
          {install && (
            <div className="space-y-3 rounded-md border p-3">
              <p>{t(($) => $.instance_update.manual_intro)}</p>
              {hosted && <a className="text-info underline" href="/download/multica-cloudflare-windows-amd64.zip">{t(($) => $.instance_update.download_windows)}</a>}
              <p>{t(($) => $.instance_update.manual_steps)}</p>
              <pre className="overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-caption">{'multica daemon stop\nCopy-Item -LiteralPath .\\multica.exe -Destination "$env:USERPROFILE\\.multica\\bin\\multica.exe" -Force\n& "$env:USERPROFILE\\.multica\\bin\\multica.exe" version\n& "$env:USERPROFILE\\.multica\\bin\\multica.exe" daemon start\n& "$env:USERPROFILE\\.multica\\bin\\multica.exe" daemon status'}</pre>
              <p className="text-muted-foreground">{t(($) => $.instance_update.manual_logs)}</p>
            </div>
          )}
        </>
    </section>
  );
}
