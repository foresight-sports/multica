// @vitest-environment jsdom
import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, it, expect, vi } from "vitest";
import type { ReactNode } from "react";
import { useInstanceUpdate } from "./instance-update";
import { api } from "../api";
vi.mock("../api", () => ({
  api: {
    getInstanceUpdateStatus: vi.fn(),
    initiateUpdate: vi.fn(),
    getUpdateResult: vi.fn(),
  },
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("waits for the new version heartbeat after the download completes", async () => {
  let version = "0.4.44-foresight.10";
  vi.mocked(api.getInstanceUpdateStatus).mockImplementation(async () => ({
    channel: "instance",
    currentVersion: version,
    latestVersion: "0.4.44-foresight.11",
    canUpdate: true,
    remoteSupported: true,
    online: true,
    updateAvailable: true,
    reason: "",
  }));
  const request = {
    id: "request",
    runtime_id: "runtime",
    target_version: "0.4.44-foresight.11",
    status: "pending" as const,
    created_at: "now",
    updated_at: "now",
  };
  vi.mocked(api.initiateUpdate).mockResolvedValue(request);
  vi.mocked(api.getUpdateResult).mockResolvedValue({
    ...request,
    status: "completed",
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const { result } = renderHook(
    () => useInstanceUpdate("workspace", "runtime"),
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    },
  );
  await waitFor(() => expect(result.current.status.isSuccess).toBe(true));
  await act(async () => {
    await result.current.mutation.mutateAsync(request.target_version);
  });
  await waitFor(() =>
    expect(result.current.result.data?.status).toBe("completed"),
  );
  expect(result.current.confirmed).toBe(false);
  expect(result.current.busy).toBe(true);
  version = request.target_version;
  await act(async () => {
    await result.current.status.refetch();
  });
  await waitFor(() => expect(result.current.confirmed).toBe(true));
  expect(result.current.busy).toBe(false);
  client.clear();
});
