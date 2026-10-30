import { z } from "zod";
import type { TimelineEntry } from "../types";

const requestSchema = z.object({
  deployment: z.string().uuid(),
  issue: z.string().uuid(),
  workspace_id: z.string().uuid(),
  commit: z.string().regex(/^[a-f0-9]{40}$/),
  plan_sha256: z.string().regex(/^[a-f0-9]{64}$/),
  host: z.string().min(1).max(200),
  prepared_at: z.string().datetime({ offset: true }),
  approver_ids: z.array(z.string().uuid()).min(1).max(100),
});

// Presentation only. The host independently validates the immutable plan,
// configured approvers, comment author, timestamp, images and expiry.
export function deploymentApprovalRequest(entry: TimelineEntry, issue: string, workspace: string) {
  if (entry.type !== "comment" || entry.actor_type !== "agent" || entry.deleted_at || entry.resolved_at) return null;
  const blocks = [...(entry.content ?? "").matchAll(/^```multica-deployment-approval\s*\r?\n([\s\S]*?)^```\s*$/gm)];
  if (blocks.length !== 1) return null;
  try {
    const parsed = requestSchema.safeParse(JSON.parse(blocks[0]![1]!));
    if (!parsed.success || parsed.data.issue !== issue || parsed.data.workspace_id !== workspace) return null;
    return parsed.data;
  } catch { return null; }
}

export function deploymentApprovalPhrase(request: NonNullable<ReturnType<typeof deploymentApprovalRequest>>) {
  return `Approve deployment ${request.deployment} ${request.plan_sha256}`;
}
