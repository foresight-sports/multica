import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { InstanceAgentInput, InstanceConfiguration } from "../api/instance-schema";

const configKey = (userId: string) => ["instance-configuration", userId] as const;
export function instanceMembersOptions(userId: string, canManage: boolean) {
  return queryOptions({ queryKey: ["instance-members", userId], queryFn: () => api.listInstanceMembers(),
    enabled: !!userId && canManage, retry: false });
}
export function useSetInstanceMembership(userId: string) {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (input: { memberId: string; workspaceId: string; role: "none" | "member" | "admin" | "owner" }) => api.setInstanceMembership(input.memberId, input.workspaceId, input.role),
    onSuccess: async () => { await Promise.all([qc.invalidateQueries({ queryKey: ["instance-members", userId] }), qc.invalidateQueries({ queryKey: ["workspaces"] })]); } });
}
const agentsKey = (wsId: string, userId: string) => ["instance-agents", wsId, userId] as const;

export function instanceConfigurationOptions(userId: string) {
  return queryOptions({ queryKey: configKey(userId), queryFn: () => api.getInstanceConfiguration(),
    enabled: !!userId, retry: false, refetchOnWindowFocus: false, refetchOnReconnect: false });
}
export function instanceAgentsOptions(wsId: string, userId: string) {
  return queryOptions({ queryKey: agentsKey(wsId, userId), queryFn: () => api.listInstanceAgents(),
    enabled: !!wsId && !!userId, retry: false, refetchOnWindowFocus: false, refetchOnReconnect: false });
}
export function useUpdateInstanceConfiguration(userId: string) {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (data: InstanceConfiguration) => api.updateInstanceConfiguration(data),
    onSuccess: (data) => { qc.setQueryData(configKey(userId), data); } });
}
export function useSaveInstanceAgent() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: ({ data, id }: { data: InstanceAgentInput; id?: string }) => api.saveInstanceAgent(data, id),
    onSuccess: async () => { await Promise.all([
      qc.invalidateQueries({ queryKey: ["instance-agents"] }),
      qc.invalidateQueries({ queryKey: ["workspaces"] }),
    ]); } });
}
export function useSetInstanceAgentEnabled(wsId: string, userId: string) {
  const qc = useQueryClient();
  return useMutation({ mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => enabled ? api.restoreAgent(id) : api.archiveAgent(id),
    onSuccess: async () => { await Promise.all([
      qc.invalidateQueries({ queryKey: agentsKey(wsId, userId) }),
      qc.invalidateQueries({ queryKey: ["workspaces", wsId] }),
    ]); } });
}
