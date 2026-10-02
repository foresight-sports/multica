import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({ mutate: vi.fn(), query: { data: { records: [] as unknown[], truncated: false }, isError: false, error: null, refetch: vi.fn() } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("@multica/core/issues", () => ({ useWorkRecords: () => ({ query: mocks.query, update: { mutate: mocks.mutate, isPending: false, isError: false } }) }));
vi.mock("../../i18n", async () => { const messages = (await import("../../locales/en/issues.json")).default; return { useT: () => ({ t: (selector: (v: typeof messages) => string) => selector(messages) }) }; });
import { IssueWorkPanel } from "./issue-work-panel";
beforeEach(() => { mocks.mutate.mockReset(); mocks.query.data.records = [{ id: "approval", revision: 3, kind: "installation", state: "pending", can_approve: true, data: { tool: "FVM", version: "3.2.1", machine: "Build machine", scope: "user", reason: "Run validation", source: "https://pub.dev/packages/fvm", dependencies: [] } }]; });
it("approves the displayed revision without hiding the pending record", () => {
  render(<IssueWorkPanel issueId="issue" />);
  expect(screen.getByText(/Build machine/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Approve and continue" }));
  expect(mocks.mutate).toHaveBeenCalledWith({ id: "approval", revision: 3, action: "approve" });
  expect(screen.getByText("pending")).toBeInTheDocument();
});
it("does not offer human approval to another viewer", () => {
  (mocks.query.data.records[0] as { can_approve: boolean }).can_approve = false;
  render(<IssueWorkPanel issueId="issue" />);
  expect(screen.queryByRole("button", { name: "Approve and continue" })).not.toBeInTheDocument();
});
