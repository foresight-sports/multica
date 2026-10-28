import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";

export function useWorkspaceDirectory(userId: string, search: string) {
  const qc = useQueryClient();
  const directory = useQuery({ queryKey: ["workspace-directory", userId, search],
    queryFn: () => api.workspaceDirectory(search), enabled: !!userId, refetchInterval: 15000 });
  const reviews = useQuery({ queryKey: ["workspace-access-requests", userId],
    queryFn: () => api.workspaceAccessRequests(), enabled: !!userId, refetchInterval: 15000 });
  const mutation = useMutation({
    mutationFn: (input: { workspaceId: string; action: "request" | "cancel" | "approved" | "declined"; userId?: string }) =>
      input.action === "request" || input.action === "cancel"
        ? api.requestWorkspaceAccess(input.workspaceId, input.action === "cancel")
        : api.reviewWorkspaceAccess(input.workspaceId, input.userId!, input.action),
    onSuccess: async () => { await Promise.all([
      qc.invalidateQueries({ queryKey: ["workspace-directory"] }),
      qc.invalidateQueries({ queryKey: ["workspace-access-requests"] }),
      qc.invalidateQueries({ queryKey: ["workspaces"] }),
      qc.invalidateQueries({ queryKey: ["instance-members"] }),
    ]); },
  });
  return { directory, reviews, mutation };
}
