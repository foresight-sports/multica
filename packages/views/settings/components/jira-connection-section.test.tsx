// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import en from "../../locales/en/settings.json";

const mock = vi.hoisted(() => ({ connect: vi.fn(), disconnect: vi.fn(), open: vi.fn(), state: "disconnected", configured: true }));
vi.mock("@multica/core/auth", () => ({ useAuthStore: (select: (state: unknown) => unknown) => select({ user: { id: "requester" } }) }));
vi.mock("@multica/core/permissions", () => ({ useJiraConnection: () => ({
  status: { data: { configured: mock.configured, status: mock.state }, refetch: vi.fn() },
  connect: { mutate: mock.connect }, disconnect: { mutate: mock.disconnect },
}) }));
vi.mock("../../platform", () => ({ openExternal: mock.open }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (select: (v: typeof en) => string) => select(en) }) }));
vi.mock("./settings-layout", () => ({
  SettingsSection: ({ children }: { children: React.ReactNode }) => <section>{children}</section>,
  SettingsCard: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SettingsRow: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
import { JiraConnectionSection } from "./jira-connection-section";

beforeEach(() => { cleanup(); vi.clearAllMocks(); mock.state = "disconnected"; mock.configured = true; });
it("starts personal OAuth without asking for Jira credentials", () => {
  mock.connect.mockImplementation((_: unknown, options: { onSuccess: (value: { redirect_url: string }) => void }) => options.onSuccess({ redirect_url: "https://multica.example/api/integrations/jira/authorize" }));
  render(<JiraConnectionSection />);
  fireEvent.click(screen.getByRole("button", { name: en.jira_connection.connect }));
  expect(mock.open).toHaveBeenCalledWith("https://multica.example/api/integrations/jira/authorize", { webTarget: "same-tab" });
  expect(screen.queryByRole("textbox")).toBeNull();
});
it("requires confirmation before disconnecting ongoing task access", () => {
  mock.state = "connected";
  render(<JiraConnectionSection />);
  fireEvent.click(screen.getByRole("button", { name: en.jira_connection.disconnect }));
  expect(mock.disconnect).not.toHaveBeenCalled();
  expect(screen.getByText(en.jira_connection.confirm)).toBeTruthy();
  fireEvent.click(screen.getAllByRole("button", { name: en.jira_connection.disconnect })[1]!);
  expect(mock.disconnect).toHaveBeenCalled();
});
it("shows unavailable configuration without an unusable connect button", () => {
  mock.configured = false;
  render(<JiraConnectionSection />);
  expect(screen.getByText(en.jira_connection.unavailable)).toBeTruthy();
  expect(screen.queryByRole("button", { name: en.jira_connection.connect })).toBeNull();
});
