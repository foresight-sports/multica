"use client";
import { useEffect, useRef, useState } from "react";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCreateComment } from "@multica/core/issues/mutations";
import { deploymentApprovalPhrase, deploymentApprovalRequest } from "@multica/core/issues/deployment-approval";
import type { TimelineEntry } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

export function DeploymentApprovals({ issueId, timeline, currentUserId }: {
  issueId: string; timeline: TimelineEntry[]; currentUserId?: string;
}) {
  const workspaceId = useWorkspaceId();
  return <>{timeline.map(entry => {
    const request = deploymentApprovalRequest(entry, issueId, workspaceId);
    return request ? <Approval key={`${entry.id}:${request.plan_sha256}`} issueId={issueId} entry={entry} request={request} timeline={timeline} currentUserId={currentUserId} /> : null;
  })}</>;
}

function Approval({ issueId, entry, request, timeline, currentUserId }: {
  issueId: string; entry: TimelineEntry; request: NonNullable<ReturnType<typeof deploymentApprovalRequest>>;
  timeline: TimelineEntry[]; currentUserId?: string;
}) {
  const { t } = useT("issues");
  const create = useCreateComment(issueId);
  const lock = useRef(false);
  const [now, setNow] = useState(Date.now);
  useEffect(() => { const timer = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(timer); }, []);
  const prepared = Date.parse(request.prepared_at);
  const expired = now < prepared || now >= prepared + 24 * 60 * 60 * 1000;
  const phrase = deploymentApprovalPhrase(request);
  const recorded = create.isSuccess || timeline.some(e => e.type === "comment" && !e.deleted_at && e.actor_type === "member"
    && request.approver_ids.includes(e.actor_id) && e.content?.trim() === phrase && Date.parse(e.created_at) >= prepared);
  async function approve() {
    if (lock.current || recorded || Date.now() >= prepared + 24 * 60 * 60 * 1000 || !currentUserId || !request.approver_ids.includes(currentUserId)) return;
    lock.current = true;
    try {
      // Reply to the agent's actual request so normal server routing wakes its
      // author. No client-supplied author or direct production execution.
      await create.mutateAsync({ content: phrase, parentId: entry.id });
    } catch { lock.current = false; }
  }
  return <section className="space-y-2 rounded-lg border p-4 text-body-sm" aria-label={t($ => $.deployment.title)}>
    <h3 className="font-medium">{t($ => $.deployment.title)}</h3>
    <p>{request.host} · <code>{request.commit}</code></p>
    <p className="break-all text-caption">{t($ => $.deployment.plan)}: {request.plan_sha256}</p>
    <a className="underline" href={`#comment-${entry.id}`}>{t($ => $.deployment.review)}</a>
    {recorded ? <p role="status">{t($ => $.deployment.recorded)}</p> : expired ? <p>{t($ => $.deployment.expired)}</p> : currentUserId && request.approver_ids.includes(currentUserId) ?
      <div><Button disabled={create.isPending} onClick={() => void approve()}>{t($ => $.deployment.approve)}</Button></div> : <p>{t($ => $.deployment.restricted)}</p>}
    {create.isError && <p role="alert">{create.error.message}</p>}
  </section>;
}
