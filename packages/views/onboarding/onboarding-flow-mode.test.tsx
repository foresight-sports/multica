vi.mock("../workspace/workspace-directory", () => ({ WorkspaceDirectory: ({onOpen}: {onOpen: (id: string) => void}) => state.ready ? <button onClick={() => onOpen("ws-1")}>Open Existing</button> : <p role="status">Loading your workspaces</p> }));
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../locales/en/common.json";
import enOnboarding from "../locales/en/onboarding.json";
import enWorkspace from "../locales/en/workspace.json";

const state = vi.hoisted(() => ({ canCreate: true, ready: true, unavailable: false, complete: vi.fn(), workspaces: [{ id: "ws-1", name: "Existing", slug: "existing" }] }));
const TEST_RESOURCES = {
  en: { common: enCommon, onboarding: enOnboarding, workspace: enWorkspace },
};

vi.mock("../auth", () => ({ useLogout: () => vi.fn() }));

vi.mock("@multica/core/config", () => ({
  useConfigStore: (
    selector: (s: { workspaceCreationDisabled: boolean; daemonAppUrl: string }) => unknown,
  ) => selector({ workspaceCreationDisabled: false, daemonAppUrl: "" }),
}));

vi.mock("@multica/core/api", () => ({
  api: { getBaseUrl: () => "https://multica.ai", getWorkspace: () => Promise.resolve(state.workspaces[0]) },
}));

vi.mock("@multica/core/workspace/mutations", () => ({
  useCreateWorkspace: () => ({ mutate: vi.fn(), isPending: false }),
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: Object.assign(
    (selector: (s: { user: unknown }) => unknown) =>
      selector({ user: { id: "u-1", onboarding_questionnaire: {}, permissions: { create_workspaces: state.canCreate } } }),
    { getState: () => ({ user: { id: "u-1" } }) },
  ),
}));

// Returning one workspace proves new-workspace mode does not offer to
// continue with it.
vi.mock("@multica/core/workspace", () => {
  return {
    useWorkspaceList: () => ({
      workspaces: state.workspaces,
      ready: state.ready,
    }),
  };
});

vi.mock("@multica/core/onboarding", async () => {
  const actual = await vi.importActual<Record<string, unknown>>(
    "@multica/core/onboarding",
  );
  return { ...actual, completeOnboarding: state.complete, useBootstrapMika: () => ({ mutateAsync: vi.fn() }) };
});

import { OnboardingFlow } from "./onboarding-flow";

function renderFlow(props: Record<string, unknown>) {
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <OnboardingFlow onComplete={vi.fn()} {...props} />
    </I18nProvider>,
  );
}

describe("OnboardingFlow — new-workspace mode", () => {
  it("starts at the workspace step instead of the product intro", () => {
    renderFlow({ mode: "new_workspace", onCancel: vi.fn() });

    // The welcome screen teaches what Multica is; someone creating a second
    // workspace already knows, so the flow opens on naming it.
    expect(
      screen.getByRole("heading", { name: /Name your workspace/i }),
    ).toBeInTheDocument();
  });

  it("always creates a workspace rather than offering the existing one", () => {
    renderFlow({ mode: "new_workspace", onCancel: vi.fn() });

    // First-run onboarding offers "Continue with {name}" when an abandoned
    // workspace exists. Doing that here would defeat the whole intent.
    expect(screen.queryByText(/Continue with/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Workspace name")).toBeInTheDocument();
  });

  it("still opens on the product intro in first-run mode", () => {
    renderFlow({});

    expect(
      screen.queryByRole("heading", { name: /Name your workspace/i }),
    ).not.toBeInTheDocument();
  });
});

beforeEach(() => { state.canCreate = true; state.ready = true; state.complete.mockReset(); state.complete.mockResolvedValue(undefined); });
describe("web onboarding membership entry", () => {
  it("opens an existing workspace without creation or runtime setup", async () => {
    const done = vi.fn();
    renderFlow({ runtimeInstructions: <div>CLI setup</div>, onComplete: done });
    expect(screen.queryByText("Download Desktop")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Workspace name")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open Existing" }));
    await waitFor(() => expect(done).toHaveBeenCalledWith(state.workspaces[0]));
    expect(state.complete).toHaveBeenCalledWith("skip_existing", "ws-1");
  });
  it("does not offer creation while memberships are loading", () => {
    state.ready = false;
    renderFlow({ runtimeInstructions: <div>CLI setup</div> });
    expect(screen.getByRole("status")).toHaveTextContent("Loading your workspaces");
    expect(screen.queryByLabelText("Workspace name")).not.toBeInTheDocument();
  });
});
it("offers the directory when an unauthorized user opens the creation route", () => {
 state.canCreate = false;
 renderFlow({ mode: "new_workspace" });
 expect(screen.getByRole("button", { name: "Open Existing" })).toBeInTheDocument();
 expect(screen.queryByLabelText("Workspace name")).not.toBeInTheDocument();
});