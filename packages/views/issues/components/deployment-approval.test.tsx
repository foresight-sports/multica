import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { TimelineEntry } from "@multica/core/types";
import { deploymentApprovalRequest } from "@multica/core/issues/deployment-approval";
const mocks = vi.hoisted(() => ({ mutateAsync: vi.fn() }));
const ws = "62fb0492-7ec5-43be-a038-7ceae9ae62dc";
const issue = "01a18ee4-da5f-7055-b848-f17dfe8c1f12";
const user = "5fb652e5-c8e8-473e-aa1d-070f741e957b";
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "62fb0492-7ec5-43be-a038-7ceae9ae62dc" }));
vi.mock("@multica/core/issues/mutations", () => ({ useCreateComment: () => ({ mutateAsync: mocks.mutateAsync, isPending: false, isSuccess: false, isError: false }) }));
vi.mock("../../i18n", async () => { const messages = (await import("../../locales/en/issues.json")).default; return { useT: () => ({ t: (selector: (v: typeof messages) => string) => selector(messages) }) }; });
import { DeploymentApprovals } from "./deployment-approval";
const plan = () => ({ deployment: "f9d3f1fc-b34a-411f-8941-bf42994387f7", issue, workspace_id: ws, commit: "a".repeat(40), plan_sha256: "b".repeat(64), host: "Build host", prepared_at: new Date(Date.now() - 60000).toISOString(), approver_ids: [user] });
const entry = (override = {}): TimelineEntry => ({ id: "request", type: "comment", actor_type: "agent", actor_id: "deployer", created_at: new Date().toISOString(), content: "```multica-deployment-approval\n" + JSON.stringify({ ...plan(), ...override }) + "\n```" });
beforeEach(() => { mocks.mutateAsync.mockReset().mockResolvedValue({ id: "approval" }); });
it("records the exact approval as a reply to the requesting agent and prevents double submission", async () => {
  render(<DeploymentApprovals issueId={issue} timeline={[entry()]} currentUserId={user} />);
  const button = screen.getByRole("button", { name: "Approve deployment" });
  fireEvent.click(button); fireEvent.click(button);
  await waitFor(() => expect(mocks.mutateAsync).toHaveBeenCalledTimes(1));
  expect(mocks.mutateAsync).toHaveBeenCalledWith({ content: `Approve deployment ${plan().deployment} ${"b".repeat(64)}`, parentId: "request" });
});
it("does not offer approval to other users or for an expired plan", () => {
  const view = render(<DeploymentApprovals issueId={issue} timeline={[entry()]} currentUserId="someone-else" />);
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  view.rerender(<DeploymentApprovals issueId={issue} timeline={[entry({ prepared_at: "2020-01-01T00:00:00Z" })]} currentUserId={user} />);
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  expect(screen.getByText(/plan has expired/)).toBeVisible();
});
it("recognizes existing human approval across reloads without claiming deployment started", () => {
  const approval: TimelineEntry = { ...entry(), id: "approved", actor_type: "member", actor_id: user, content: `Approve deployment ${plan().deployment} ${"b".repeat(64)}` };
  render(<DeploymentApprovals issueId={issue} timeline={[entry(), approval]} currentUserId={user} />);
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("Waiting for the deployer");
});
it("rejects untrusted contexts and malformed or ambiguous requests", () => {
  for (const candidate of [
    { ...entry(), actor_type: "member" }, { ...entry(), deleted_at: new Date().toISOString() },
    { ...entry(), resolved_at: new Date().toISOString() }, entry({ issue: ws }), entry({ workspace_id: issue }),
    entry({ plan_sha256: "bad" }), entry({ approver_ids: [] }),
    { ...entry(), content: entry().content + "\n" + entry().content },
  ]) expect(deploymentApprovalRequest(candidate, issue, ws)).toBeNull();
});
