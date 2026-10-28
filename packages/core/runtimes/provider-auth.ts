import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";

export function useProviderAuth(wsId: string, runtimeId: string) {
  const client = useQueryClient();
  const queryKey = ["runtimes", wsId, runtimeId, "provider-auth"];
  const status = useQuery({
    queryKey,
    queryFn: () => api.providerAuth(runtimeId),
    refetchInterval: (q) => ["pending", "running"].includes(q.state.data?.status ?? "") ? 2000 : false,
  });
  const start = useMutation({
    mutationFn: (provider: "claude" | "codex") => api.providerAuth(runtimeId, provider),
    onSuccess: (value) => client.setQueryData(queryKey, value),
  });
  return { status, start, busy: start.isPending || ["pending", "running"].includes(status.data?.status ?? "") };
}
