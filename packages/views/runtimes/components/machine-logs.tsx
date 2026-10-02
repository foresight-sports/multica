"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { useT } from "../../i18n";

export function MachineLogs({ wsId, runtimeId, owner }: { wsId: string; runtimeId?: string; owner: boolean }) {
  const { t } = useT("runtimes");
  const [live, setLive] = useState(true);
  const [search, setSearch] = useState("");
  const query = useQuery({
    queryKey: ["machine-logs", wsId, runtimeId],
    queryFn: () => api.getMachineLogs(runtimeId!),
    enabled: owner && !!runtimeId,
    refetchInterval: live ? 5000 : false,
    retry: false,
  });
  if (!owner) return <p className="text-body text-muted-foreground">{t(($) => $.logs.owner_only)}</p>;
  if (!runtimeId) return <p className="text-body text-muted-foreground">{t(($) => $.logs.no_runtime)}</p>;
  const data = query.data;
  const stale = data?.received_at && Date.now() - Date.parse(data.received_at) > 30000;
  const filter = (value: string) => value.split("\n").filter((line) => line.toLowerCase().includes(search.toLowerCase())).join("\n");
  return <section className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div><h2 className="text-body font-semibold">{t(($) => $.logs.title)}</h2>
        <p className="text-caption text-muted-foreground">{t(($) => $.logs.description)}</p></div>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={() => setLive(!live)}>{live ? t(($) => $.logs.pause) : t(($) => $.logs.resume)}</Button>
        <Button variant="outline" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}>{t(($) => $.logs.refresh)}</Button>
      </div>
    </div>
    {query.isError && <p role="alert" className="text-caption text-destructive">{t(($) => $.logs.error)}</p>}
    {query.isPending && <p className="text-caption text-muted-foreground">{t(($) => $.logs.loading)}</p>}
    {data && <>
      {!data.supported && <p className="text-caption text-muted-foreground">{t(($) => $.logs.upgrade)}</p>}
      {data.received_at ? <p className="text-caption text-muted-foreground">{stale ? t(($) => $.logs.stale) : t(($) => $.logs.received)} {new Date(data.received_at).toLocaleString()}</p> : data.supported && <p className="text-caption text-muted-foreground">{t(($) => $.logs.waiting)}</p>}
      {data.message && <p className="text-caption text-muted-foreground">{data.message}</p>}
      {data.truncated && <p className="text-caption text-muted-foreground">{t(($) => $.logs.truncated)}</p>}
      <Input aria-label={t(($) => $.logs.search)} placeholder={t(($) => $.logs.search)} value={search} onChange={(event) => setSearch(event.target.value)} />
      <h3 className="text-caption font-medium">{t(($) => $.logs.daemon_file)}</h3>
      <pre tabIndex={0} aria-label={t(($) => $.logs.daemon_file)} className="max-h-[55vh] min-h-48 overflow-auto rounded-lg border bg-muted/30 p-4 font-mono text-caption whitespace-pre-wrap break-words">{filter(data.log) || t(($) => $.logs.empty)}</pre>
      <h3 className="text-caption font-medium">{t(($) => $.logs.crash_file)}</h3>
      <pre tabIndex={0} aria-label={t(($) => $.logs.crash_file)} className="max-h-64 overflow-auto rounded-lg border bg-muted/30 p-4 font-mono text-caption whitespace-pre-wrap break-words">{filter(data.crash) || t(($) => $.logs.empty)}</pre>
    </>}
  </section>;
}
