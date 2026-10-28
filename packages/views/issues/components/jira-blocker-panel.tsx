"use client";

import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useIssueJiraBlockers, useJiraConnection } from "@multica/core/permissions";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
import { openExternal } from "../../platform";

export function JiraBlockerPanel({ issueId }: { issueId: string }) {
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const workspaceId = useWorkspaceId();
  const { t } = useT("issues");
  const { query, resume } = useIssueJiraBlockers(workspaceId, issueId, userId);
  const blockers = query.data ?? [];
  const { status, connect } = useJiraConnection(blockers.length ? userId : "", true);
  if (!blockers.length && !query.isError) return null;
  const connected = status.data?.status === "connected" && !status.isError;
  const error = query.error ?? status.error ?? connect.error ?? resume.error;
  return <section className="my-4 space-y-3 rounded-lg border border-border bg-muted/30 p-4" aria-label={t(($) => $.jira_blocker.title)}>
    <h3 className="text-body font-medium">{t(($) => $.jira_blocker.title)}</h3>
    {!!blockers.length && <p className="text-caption text-muted-foreground">{connected ? t(($) => $.jira_blocker.ready) : t(($) => $.jira_blocker.instructions)}</p>}
    {error && <div role="alert" className="text-caption text-destructive">{error.message}<Button variant="ghost" size="sm" onClick={() => { void query.refetch(); if (blockers.length) void status.refetch(); }}>{t(($) => $.jira_blocker.reload)}</Button></div>}
    {!!blockers.length && !connected && <Button disabled={!status.data?.configured || status.isError || connect.isPending} aria-busy={connect.isPending} onClick={() => connect.mutate(undefined, {
      onSuccess: (value) => openExternal(value.redirect_url, { webTarget: "same-tab" }),
    })}>{t(($) => $.jira_blocker.connect)}</Button>}
    {!!blockers.length && status.data && !status.data.configured && <p role="status" className="text-caption">{t(($) => $.jira_blocker.unavailable)}</p>}
    {blockers.map((blocker) => <div key={blocker.task_id} className="flex flex-wrap items-center justify-between gap-2 text-caption">
      <span>{blocker.agent_name}</span>
      {connected && <Button size="sm" disabled={!blocker.can_resume || resume.isPending} aria-busy={resume.isPending} onClick={() => resume.mutate(blocker.task_id)}>{t(($) => $.jira_blocker.resume)}</Button>}
      {!blocker.can_resume && <span className="text-muted-foreground">{t(($) => $.jira_blocker.wait)}</span>}
    </div>)}
  </section>;
}
