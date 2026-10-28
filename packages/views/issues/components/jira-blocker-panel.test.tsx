// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import en from "../../locales/en/issues.json";

const mock = vi.hoisted(() => ({ connect: vi.fn(), resume: vi.fn(), open: vi.fn(), connected: false, terminal: true, blockers: true, error: null as Error | null }));
vi.mock("@multica/core/auth", () => ({ useAuthStore: (select: (state: unknown) => unknown) => select({ user: { id: "requester" } }) }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("@multica/core/permissions", () => ({
  useIssueJiraBlockers: () => ({ query: { data: mock.blockers ? [{ task_id: "task", agent_name: "Agent", can_resume: mock.terminal }] : [], error: mock.error, isError: !!mock.error }, resume: { mutate: mock.resume } }),
  useJiraConnection: () => ({ status: { data: { configured: true, status: mock.connected ? "connected" : "disconnected" } }, connect: { mutate: mock.connect } }),
}));
vi.mock("../../platform", () => ({ openExternal: mock.open }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (select: (v: typeof en) => string) => select(en) }) }));
import { JiraBlockerPanel } from "./jira-blocker-panel";

beforeEach(() => { cleanup(); vi.clearAllMocks(); mock.connected = false; mock.terminal = true; mock.blockers = true; mock.error = null; });
it("offers personal sign-in and instructions without allowing an unauthenticated resume", () => {
  mock.connect.mockImplementation((_: unknown, options: { onSuccess: (value: { redirect_url: string }) => void }) => options.onSuccess({ redirect_url: "https://multica.example/authorize" }));
  render(<JiraBlockerPanel issueId="issue" />);
  expect(screen.getByText(en.jira_blocker.instructions)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: en.jira_blocker.connect }));
  expect(mock.open).toHaveBeenCalledWith("https://multica.example/authorize", { webTarget: "same-tab" });
  expect(screen.queryByRole("button", { name: en.jira_blocker.resume })).toBeNull();
});
it("requires an explicit resume after connecting and waits for the current run", () => {
  mock.connected = true; mock.terminal = false;
  const view = render(<JiraBlockerPanel issueId="issue" />);
  expect((screen.getByRole("button", { name: en.jira_blocker.resume }) as HTMLButtonElement).disabled).toBe(true);
  expect(mock.resume).not.toHaveBeenCalled();
  mock.terminal = true; view.rerender(<JiraBlockerPanel issueId="issue" />);
  fireEvent.click(screen.getByRole("button", { name: en.jira_blocker.resume }));
  expect(mock.resume).toHaveBeenCalledWith("task");
});
it("does not show a banner for unrelated work", () => {
  mock.blockers = false;
  const view = render(<JiraBlockerPanel issueId="issue" />);
  expect(view.container.textContent).toBe("");
});
