"use client";
import { subscriptionQuotaOptions } from "@multica/core/runtimes/queries";
import { useQuery } from "@tanstack/react-query";
import { quotaWindowState } from "@multica/core/api";
import { useT } from "../../i18n";
import { useWorkspaceId } from "@multica/core/hooks";
export function SubscriptionQuotaSection({ runtimeId }: { runtimeId: string }) {
  const { t } = useT("runtimes");
  const workspaceId = useWorkspaceId();
  const query = useQuery(subscriptionQuotaOptions(workspaceId, runtimeId));
  const report = query.data;
  return (
    <section
      className="rounded-lg border bg-card p-4 space-y-3"
      aria-label={t(($) => $.quota.title)}
    >
      <div>
        <h2 className="text-sm font-medium">{t(($) => $.quota.title)}</h2>
        <p className="text-xs text-muted-foreground">
          {t(($) => $.quota.shared)}
        </p>
      </div>
      {query.isPending ? (
        <p className="text-sm text-muted-foreground">
          {t(($) => $.quota.loading)}
        </p>
      ) : query.isError ? (
        <p className="text-sm text-muted-foreground">
          {t(($) => $.quota.error)}
        </p>
      ) : report?.status === "not_applicable" ? (
        <p className="text-sm text-muted-foreground">{t(($) => $.quota.api)}</p>
      ) : !report || report.windows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t(($) => $.quota.unknown)}
        </p>
      ) : (
        <div className="space-y-2">
          {report.windows.map((w) => {
            const state = quotaWindowState(report, w);
            return (
              <div
                key={w.id}
                className="flex flex-wrap items-center justify-between gap-2 text-sm"
              >
                <div>
                  <span>
                    {w.durationMinutes === 300
                      ? t(($) => $.quota.five_hour)
                      : w.durationMinutes === 10080
                        ? t(($) => $.quota.weekly)
                        : w.durationMinutes > 0
                          ? t(($) => $.quota.minutes, {
                              minutes: w.durationMinutes,
                            })
                          : w.id}
                  </span>
                  <span className="ml-2 text-xs text-muted-foreground">
                    {w.model || (w.appliesAll ? t(($) => $.quota.all) : w.id)}
                  </span>
                  {w.resetsAt > 0 && (
                    <p className="text-xs text-muted-foreground">
                      {t(($) => $.quota.reset, {
                        time: new Date(w.resetsAt * 1000).toLocaleString(),
                      })}
                    </p>
                  )}
                </div>
                <span
                  className={
                    state.status === "exhausted"
                      ? "text-destructive font-medium"
                      : "text-muted-foreground"
                  }
                >
                  {state.status === "unknown"
                    ? t(($) => $.quota.refresh)
                    : state.status === "exhausted"
                      ? t(($) => $.quota.exhausted)
                      : state.remaining === null
                        ? t(($) => $.quota.missing)
                        : t(($) => $.quota.remaining, {
                            percent: Math.round(state.remaining),
                          })}
                </span>
              </div>
            );
          })}
        </div>
      )}
      {report?.observedAt && report.status !== "unknown" && (
        <p className="text-xs text-muted-foreground">
          {t(($) => $.quota.observed, {
            time: new Date(report.observedAt).toLocaleString(),
          })}
          . {t(($) => $.quota.note)}
        </p>
      )}
    </section>
  );
}
