import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useAuthStore } from "../auth";

// Instance-wide policy, isolated by viewer so account switches never reuse it.
export const permissionPolicyKey = (userId: string, action = "runtime-register") => ["permission-policy", userId, action] as const;
export function runtimePermissionPolicyOptions(userId: string, enabled: boolean, action = "runtime-register") {
  return queryOptions({
    queryKey: permissionPolicyKey(userId, action),
    queryFn: () => api.getRuntimePermissionPolicy(action),
    enabled: enabled && !!userId,
    staleTime: 0,
    // Keep unsaved drafts stable; managers explicitly reload after a conflict.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
}
export function useUpdateRuntimePermissionPolicy(userId: string, action = "runtime-register") {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: { allowed_emails: string[]; revision: number }) => api.updateRuntimePermissionPolicy(data, action),
    onSuccess: (policy) => {
      queryClient.setQueryData(permissionPolicyKey(userId, action), policy);
      void useAuthStore.getState().refreshMe();
    },
  });
}

export function runtimeInstallationOptions(userId: string, enabled: boolean) {
  return queryOptions({
    queryKey: ["runtime-installation", userId],
    queryFn: () => api.getRuntimeInstallation(),
    enabled: enabled && !!userId,
    gcTime: 0,
    staleTime: 0,
    retry: false,
  });
}
