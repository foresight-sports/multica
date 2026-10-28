// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, cleanup } from "@testing-library/react";
import en from "../../locales/en/runtimes.json";

const mock = vi.hoisted(() => ({ mutate: vi.fn(), state: "idle", busy: false }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("@multica/core/runtimes", () => ({ useProviderAuth: () => ({
  status: { data: { status: mock.state }, isError: false },
  start: { mutate: mock.mutate, isError: false }, busy: mock.busy,
}) }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (v: typeof en) => string) => selector(en) }) }));
import { ProviderAuthSection } from "./provider-auth-section";

beforeEach(() => { cleanup(); mock.mutate.mockReset(); mock.state = "idle"; mock.busy = false; });
it("starts the selected provider without requesting a credential in the UI", () => {
  render(<ProviderAuthSection runtimeId="runtime" />);
  fireEvent.click(screen.getByRole("button", { name: en.provider_auth.claude }));
  expect(mock.mutate).toHaveBeenCalledWith("claude");
  expect(screen.queryByRole("textbox")).toBeNull();
});
it("blocks duplicate setup while the owner is signing in", () => {
  mock.state = "running"; mock.busy = true;
  render(<ProviderAuthSection runtimeId="runtime" />);
  expect(screen.getByRole("button", { name: en.provider_auth.codex }).hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("status").textContent).toBe(en.provider_auth.running);
});
it("does not claim service access was verified after saving credentials", () => {
  mock.state = "completed";
  render(<ProviderAuthSection runtimeId="runtime" />);
  expect(screen.getByRole("status").textContent).toBe(en.provider_auth.completed);
});
