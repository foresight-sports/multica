"use client";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkRecords } from "@multica/core/issues";
import type { WorkRecord } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

function safeLink(value: string) {
  try { const u = new URL(value); return u.protocol === "https:" && !u.username && !u.password ? u.href : undefined; } catch { return undefined; }
}

export function IssueWorkPanel({ issueId }: { issueId: string }) {
  const wsId = useWorkspaceId();
  const { query, update } = useWorkRecords(wsId, issueId);
  const { t } = useT("issues");
  const records = query.data?.records ?? [];
  const closed = new Set(["resolved", "declined", "completed", "cancelled"]);
  const active = records.filter((r) => !closed.has(r.state));
  const history = records.filter((r) => closed.has(r.state));
  function record(r: WorkRecord) {
    const d = r.data;
    return <article key={r.id} className="space-y-2 rounded-lg border p-3 text-body-sm break-words">
      <div className="flex flex-wrap items-center justify-between gap-2"><strong>{r.kind === "installation" ? `${d.tool} ${d.version}` : r.kind === "checkpoint" ? `${d.repository} · ${d.branch}` : r.kind === "decision" ? t(($) => $.work.decision) : t(($) => $.work.blocker)}</strong><span className="text-caption text-muted-foreground">{r.state}</span></div>
      {r.kind === "installation" && <>
        <p>{t(($) => $.work.machine)}: {d.machine || r.runtime_id} · {d.scope}</p>
        <p>{d.reason}</p>{d.effects && <p>{d.effects}</p>}
        {safeLink(d.source) && <a className="underline" href={safeLink(d.source)} target="_blank" rel="noreferrer">{t(($) => $.work.source)}</a>}
        {r.state === "pending" && <p className="text-muted-foreground">{t(($) => $.work.approval_help)}</p>}
        {["approved", "installed"].includes(r.state) && <p>{t(($) => $.work.same_machine)}</p>}
        {d.verification && <p>{d.verification}</p>}
        {r.state === "pending" && r.can_approve && <div className="flex gap-2">
          <Button size="sm" disabled={update.isPending || query.isError} onClick={() => update.mutate({ id: r.id, revision: r.revision, action: "approve" })}>{t(($) => $.work.approve)}</Button>
          <Button variant="outline" size="sm" disabled={update.isPending || query.isError} onClick={() => update.mutate({ id: r.id, revision: r.revision, action: "decline" })}>{t(($) => $.work.decline)}</Button>
        </div>}
      </>}
      {r.kind === "checkpoint" && <>
        {d.commit ? <code className="block select-all break-all text-caption">{d.commit}</code> : <p>{t(($) => $.work.no_checkpoint)}</p>}
        {safeLink(d.pr_url) && <a className="underline" href={safeLink(d.pr_url)} target="_blank" rel="noreferrer">{t(($) => $.work.pull_request)}</a>}
        {d.validation && <p>{d.validation}</p>}
        {d.dependencies.map((dep) => <p key={`${dep.repository}:${dep.commit}`} className="text-caption">{dep.repository} · {dep.commit}</p>)}
      </>}
      {["decision", "blocker"].includes(r.kind) && <span className="text-caption text-muted-foreground">{d.author_type === "member" ? t(($) => $.work.human_record) : t(($) => $.work.agent_record)}</span>}
      {d.text && <p className="whitespace-pre-wrap">{d.text}</p>}
      {d.owner && <p>{t(($) => $.work.owner)}: {d.owner}</p>}
      {d.next_step && <p>{t(($) => $.work.next_step)}: {d.next_step}</p>}
    </article>;
  }
  if (!query.isError && !records.length) return null;
  return <section className="space-y-3" aria-label={t(($) => $.work.title)}>
    <h3 className="text-body-sm font-medium">{t(($) => $.work.title)}</h3>
    {(query.isError || update.isError) && <div role="alert"><p className="text-destructive text-caption">{update.error?.message || query.error?.message}</p><Button variant="ghost" size="sm" onClick={() => void query.refetch()}>{t(($) => $.work.reload)}</Button></div>}
    {active.map(record)}
    {!!history.length && <details><summary className="cursor-pointer text-caption text-muted-foreground">{t(($) => $.work.history)} ({history.length})</summary><div className="space-y-2 pt-2">{history.map(record)}</div></details>}
    {query.data?.truncated && <p className="text-caption text-muted-foreground">{t(($) => $.work.truncated)}</p>}
  </section>;
}
