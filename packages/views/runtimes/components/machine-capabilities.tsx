"use client";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import {
  runtimeModelsOptions,
  refreshRuntimeModels,
  parseMachineCapabilities,
} from "@multica/core/runtimes";
import type { RuntimeDevice } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
export function MachineCapabilities({
  runtimes,
}: {
  runtimes: RuntimeDevice[];
}) {
  const { t } = useT("runtimes");
  const qc = useQueryClient();
  const queries = useQueries({
    queries: runtimes.map((r) =>
      runtimeModelsOptions(r.status === "online" ? r.id : null),
    ),
  });
  const capabilities = runtimes.map((r) =>
    parseMachineCapabilities(r.metadata),
  );
  const tools = Array.from(
    new Set(capabilities.flatMap((c) => c.tools)),
  ).sort();
  const observedAt = capabilities
    .map((c) => c.execution_observed_at ?? "")
    .sort()
    .at(-1);
  return (
    <section className="space-y-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-body font-semibold">
            {t(($) => $.capabilities.title)}
          </h2>
          <p className="mt-1 text-caption text-muted-foreground">
            {t(($) => $.capabilities.help)}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={queries.some((q) => q.isFetching)}
          onClick={() =>
            void Promise.allSettled(
              runtimes
                .filter((r) => r.status === "online")
                .map((r) => refreshRuntimeModels(qc, r.id)),
            )
          }
        >
          {t(($) => $.capabilities.refresh)}
        </Button>
      </div>
      <div className="rounded-lg border p-4">
        <h3 className="text-body font-medium">
          {t(($) => $.capabilities.tools)}
        </h3>
        <p className="mt-1 text-caption text-muted-foreground">
          {observedAt
            ? t(($) => $.capabilities.observed, {
                at: new Date(observedAt).toLocaleString(),
              })
            : t(($) => $.capabilities.unknown)}
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          {tools.map((tool) => (
            <span
              key={tool}
              className="rounded-md bg-muted px-2 py-1 font-mono text-caption"
            >
              {tool}
            </span>
          ))}
          {!tools.length && (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.capabilities.no_tools)}
            </p>
          )}
        </div>
      </div>
      {runtimes.map((runtime, index) => (
        <div key={runtime.id} className="rounded-lg border p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-body font-medium">{runtime.provider}</h3>
            <span className="text-caption text-muted-foreground">
              {runtime.name}
            </span>
          </div>
          <div className="mt-3 space-y-2">
            <h4 className="text-caption font-medium">{t(($) => $.plugin_capabilities.title)}</h4>
            <p className="text-caption text-muted-foreground">{t(($) => $.plugin_capabilities.help)}</p>
            {(() => {
              const report=capabilities[index]?.runtime_capabilities;
              if(!report || report.provider!==runtime.provider || report.status!=="reported") return <p className="text-caption">{t(($)=>$.plugin_capabilities.unknown)}</p>;
              const age=Date.now()-Date.parse(report.observed_at);
              const stale=runtime.status!=="online" || !Number.isFinite(age) || age<0 || age>300000;
              return <>
                <p className="text-caption text-muted-foreground">{t(($)=>$.capabilities.observed,{at:new Date(report.observed_at).toLocaleString()})}</p>
                {stale && <p className="text-caption">{t(($)=>$.plugin_capabilities.stale)}</p>}
                <ul className="space-y-2">{report.entries.map((entry,i)=><li key={entry.kind+entry.name+i} className="text-caption">
                  <span className="font-medium">{entry.name}</span> <span className="text-muted-foreground">{"("}{entry.kind}{") · "}{entry.enabled===true?t(($)=>$.plugin_capabilities.enabled):entry.enabled===false?t(($)=>$.plugin_capabilities.disabled):t(($)=>$.plugin_capabilities.configured)}{" · "}{entry.callable===true?t(($)=>$.plugin_capabilities.callable):entry.callable===false?t(($)=>$.plugin_capabilities.unavailable):t(($)=>$.plugin_capabilities.auth)}</span>
                  {entry.description && <p className="text-muted-foreground">{entry.description}</p>}
                </li>)}</ul>
                {report.truncated && <p className="text-caption">{t(($)=>$.plugin_capabilities.truncated)}</p>}
              </>;
            })()}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {queries[index]?.data?.models.map((model) => (
              <span
                key={model.id}
                className="rounded-md bg-muted px-2 py-1 text-caption"
                title={model.id}
              >
                {model.label}
              </span>
            ))}
            {!queries[index]?.data?.models.length && (
              <p className="text-caption text-muted-foreground">
                {runtime.status === "offline"
                  ? t(($) => $.capabilities.offline)
                  : t(($) => $.capabilities.no_models)}
              </p>
            )}
          </div>
        </div>
      ))}
    </section>
  );
}
