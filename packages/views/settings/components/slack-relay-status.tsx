"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

export function SlackRelayStatus({ wsId }: { wsId: string }) {
  const { t } = useT("settings");
  const queryClient = useQueryClient();
  const queryKey = ["slack-relay", wsId];
  const status = useQuery({
    queryKey,
    queryFn: () => api.getSlackRelayStatus(wsId),
    refetchInterval: 15000,
    retry: false,
  });
  const replay = useMutation({
    mutationFn: (id: string) => api.replaySlackRelay(wsId, id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  const labels: Record<string, string> = {
    pending: t(($) => $.slack_relay.pending),
    waiting_root: t(($) => $.slack_relay.waiting_root),
    delivered: t(($) => $.slack_relay.delivered),
    held_disabled: t(($) => $.slack_relay.held_disabled),
    failed: t(($) => $.slack_relay.failed),
    onyx: t(($) => $.slack_relay.onyx),
    ignored: t(($) => $.slack_relay.ignored),
  };
  return (
    <section aria-label={t(($) => $.slack_relay.title)} className="space-y-3">
      <h3 className="text-body font-medium">{t(($) => $.slack_relay.title)}</h3>
      {status.isPending && (
        <p role="status">{t(($) => $.slack_intake.loading)}</p>
      )}
      {status.isError && (
        <div role="alert" className="space-y-2">
          <p>{t(($) => $.slack_relay.load_error)}</p>
          <Button variant="outline" onClick={() => void status.refetch()}>
            {t(($) => $.slack_intake.retry)}
          </Button>
        </div>
      )}
      {status.data && (
        <>
          {status.data.installations.map((app) => (
            <div key={app.id} className="text-body break-words">
              <span>
                {app.appId} · {app.teamId} —{" "}
              </span>
              <span>
                {app.mode === "relay"
                  ? t(($) => $.slack_relay.relay_mode)
                  : app.mode === "socket_mode"
                    ? t(($) => $.slack_relay.socket_mode)
                    : t(($) => $.slack_relay.unknown)}
              </span>
              {app.mode === "relay" &&
                app.active &&
                app.credentialConfigured && (
                  <p
                    className={
                      app.contactRecent
                        ? "text-caption text-muted-foreground"
                        : "text-caption text-destructive"
                    }
                  >
                    {app.contactRecent
                      ? t(($) => $.slack_relay.connected)
                      : t(($) => $.slack_relay.stale)}
                  </p>
                )}
              {app.mode === "relay" &&
                (!app.active || !app.credentialConfigured) && (
                  <p role="alert" className="text-destructive">
                    {t(($) => $.slack_relay.config_error)}
                  </p>
                )}
            </div>
          ))}
          <p className="text-caption text-muted-foreground">
            {t(($) => $.slack_relay.counts, {
              queued:
                (status.data.counts.pending ?? 0) +
                (status.data.counts.waiting_root ?? 0),
              held: status.data.counts.held_disabled ?? 0,
              failed: status.data.counts.failed ?? 0,
            })}
          </p>
          {status.data.recent.length === 0 && (
            <p className="text-body">{t(($) => $.slack_relay.empty)}</p>
          )}
          <ul className="divide-y divide-border">
            {status.data.recent.map((receipt) => (
              <li
                key={receipt.id}
                className="flex flex-col items-start justify-between gap-3 py-3 sm:flex-row"
              >
                <div className="min-w-0 flex-1 space-y-1 break-words">
                  <p className="text-body">
                    {receipt.channelId} ·{" "}
                    {labels[receipt.state] ?? t(($) => $.slack_relay.unknown)}
                  </p>
                  <p className="text-caption text-muted-foreground">
                    <time dateTime={receipt.receivedAt}>
                      {new Date(receipt.receivedAt).toLocaleString()}
                    </time>{" "}
                    ·{" "}
                    {t(($) => $.slack_relay.attempts, {
                      count: receipt.attempts,
                    })}
                  </p>
                  {receipt.lastError && (
                    <p className="text-caption text-destructive">
                      {receipt.lastError}
                    </p>
                  )}
                  {receipt.state === "held_disabled" && (
                    <p className="text-caption">
                      {t(($) => $.slack_relay.held_hint)}
                    </p>
                  )}
                </div>
                {["failed", "held_disabled", "waiting_root"].includes(
                  receipt.state,
                ) && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={replay.isPending}
                    aria-busy={
                      replay.isPending && replay.variables === receipt.id
                    }
                    onClick={() => replay.mutate(receipt.id)}
                  >
                    {t(($) => $.slack_relay.replay)}
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
      {replay.isError && (
        <p role="alert" className="text-destructive">
          {replay.error.message}
        </p>
      )}
      {replay.isSuccess && (
        <p role="status">{t(($) => $.slack_relay.replay_queued)}</p>
      )}
    </section>
  );
}
