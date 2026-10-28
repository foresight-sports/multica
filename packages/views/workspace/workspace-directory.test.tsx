import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { WorkspaceDirectory } from "./workspace-directory";

const mocks = vi.hoisted(() => ({ directory: vi.fn(), reviews: vi.fn(), request: vi.fn(), review: vi.fn() }));
vi.mock("@multica/core/auth", () => ({ useAuthStore: (selector: (state: { user: { id: string } }) => unknown) => selector({ user: { id: "viewer" } }) }));
vi.mock("@multica/core/api", () => ({ api: { workspaceDirectory: mocks.directory, workspaceAccessRequests: mocks.reviews, requestWorkspaceAccess: mocks.request, reviewWorkspaceAccess: mocks.review } }));
const workspace = { id: "ws", name: "Premier", slug: "premier", role: "", request_status: "" };
function renderDirectory(onOpen = vi.fn(), onCreate = vi.fn()) {
  return renderWithI18n(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><WorkspaceDirectory onOpen={onOpen} onCreate={onCreate} /></QueryClientProvider>);
}
beforeEach(() => {
  vi.resetAllMocks();
  mocks.directory.mockResolvedValue({ workspaces: [workspace], can_create: false, truncated: false });
  mocks.reviews.mockResolvedValue({ requests: [], truncated: false });
  mocks.request.mockResolvedValue(undefined);
  mocks.review.mockResolvedValue(undefined);
});
it("lets an account without membership request access without offering creation", async () => {
  renderDirectory();
  fireEvent.click(await screen.findByRole("button", { name: "Request access" }));
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith("ws", false));
  expect(screen.queryByRole("button", { name: "Create workspace" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Open workspace" })).not.toBeInTheDocument();
});
it("shows pending status and lets the requester cancel", async () => {
  mocks.directory.mockResolvedValue({ workspaces: [{ ...workspace, request_status: "pending" }], can_create: false });
  renderDirectory();
  fireEvent.click(await screen.findByRole("button", { name: "Cancel request" }));
  await waitFor(() => expect(mocks.request).toHaveBeenCalledWith("ws", true));
});
it("opens a workspace after membership is granted", async () => {
  mocks.directory.mockResolvedValue({ workspaces: [{ ...workspace, role: "member", request_status: "approved" }], can_create: false });
  const open = vi.fn(); renderDirectory(open);
  fireEvent.click(await screen.findByRole("button", { name: "Open workspace" }));
  expect(open).toHaveBeenCalledWith("ws", "premier");
});
it("exposes creation only when the server grants it", async () => {
  mocks.directory.mockResolvedValue({ workspaces: [], can_create: true });
  const create = vi.fn(); renderDirectory(vi.fn(), create);
  fireEvent.click(await screen.findByRole("button", { name: "Create workspace" }));
  expect(create).toHaveBeenCalled();
});
it("reviews only the server-provided requests and preserves errors", async () => {
  mocks.reviews.mockResolvedValue({ requests: [{ workspace_id: "ws", workspace_name: "Premier", user_id: "new", name: "New member", email: "new@example.test", requested_at: "2026-10-22" }] });
  mocks.review.mockRejectedValue(new Error("Request already resolved"));
  renderDirectory();
  fireEvent.click(await screen.findByRole("button", { name: "Approve" }));
  await waitFor(() => expect(mocks.review).toHaveBeenCalledWith("ws", "new", "approved"));
  expect(await screen.findByRole("alert")).toHaveTextContent("Request already resolved");
});
