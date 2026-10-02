import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import { it, expect, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { SubscriptionQuotaSection } from "./subscription-quota-section";
const get = vi.hoisted(() => vi.fn());
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { getSubscriptionQuota: get },
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws" }));
it("loads runtime capacity and shows shared-account warning and remaining percentage", async () => {
  get.mockResolvedValue({
    status: "reported",
    observedAt: new Date().toISOString(),
    windows: [
      {
        id: "codex:0",
        usedPercent: 72,
        resetsAt: 0,
        durationMinutes: 300,
        exhausted: false,
        appliesAll: true,
      },
    ],
  });
  renderWithI18n(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <SubscriptionQuotaSection runtimeId="rt" />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("28% remaining")).toBeInTheDocument();
  expect(get).toHaveBeenCalledWith("rt");
  expect(screen.getByText(/Balances are not additive/)).toBeInTheDocument();
});
