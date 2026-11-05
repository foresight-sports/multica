import { StrictMode } from "react";
import { expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { useWelcomeStore } from "@multica/core/onboarding";
import { WelcomeAfterOnboarding } from "./welcome-after-onboarding";
const createIssue = vi.hoisted(() => vi.fn());
vi.mock("@multica/core/api", () => ({ api: { createIssue } }));
it("discards legacy skip signals without creating an issue or showing a failure", async () => {
  useWelcomeStore.getState().set({ workspaceId: "ws-1", choice: "skip" });
  const view = render(
    <StrictMode>
      <WelcomeAfterOnboarding />
    </StrictMode>,
  );
  await waitFor(() => expect(useWelcomeStore.getState().signal).toBeNull());
  expect(createIssue).not.toHaveBeenCalled();
  expect(view.container).toBeEmptyDOMElement();
});
