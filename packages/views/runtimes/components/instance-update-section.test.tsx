// @vitest-environment jsdom
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, it, expect, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import { InstanceUpdateSection } from "./instance-update-section";
const state = vi.hoisted(() => ({
  reason: "bootstrap_required",
  canUpdate: false,
  busy: false,
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("@multica/core/runtimes", () => ({
  useInstanceUpdate: () => ({
    status: {
      data: {
        currentVersion: "0.4.44-foresight.4",
        latestVersion: "0.4.44-foresight.11",
        updateAvailable: true,
        ...state,
      },
    },
    mutation: { mutate: vi.fn() },
    result: { data: { output: "Downloading release..." } },
    busy: state.busy,
  }),
}));
vi.mock("./hosted-client-install", () => ({
  useHostedClient: () => true,
  HostedClientInstall: () => null,
}));
afterEach(cleanup);
function show() {
  render(
    <I18nProvider
      locale="en"
      resources={{ en: { common: enCommon, runtimes: enRuntimes } }}
    >
      <InstanceUpdateSection runtimeId="runtime" />
    </I18nProvider>,
  );
}
it("shows current and available versions with a one-time bootstrap instead of remote action", () => {
  state.reason = "bootstrap_required";
  state.canUpdate = false;
  show();
  expect(screen.getByText("0.4.44-foresight.4")).toBeTruthy();
  expect(screen.getByText(/0.4.44-foresight.11/)).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Manual update instructions" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Update" })).toBeNull();
});
it("prevents duplicate requests while updating", () => {
  state.reason = "";
  state.canUpdate = true;
  state.busy = true;
  show();
  expect(screen.getByRole("button", { name: "Update" })).toBeDisabled();
});

it("shows progress and manual recovery while offline", () => {
 state.reason="offline"; state.canUpdate=false; state.busy=false; show();
 expect(screen.getByRole("log")).toHaveTextContent("Downloading release...");
 fireEvent.click(screen.getByRole("button", {name:"Manual update instructions"}));
 expect(screen.getByRole("link", {name:"Download Windows installer ZIP"})).toBeTruthy();
 expect(screen.getByText(/Copy-Item/)).toBeTruthy();
});
