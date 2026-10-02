import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { RepositorySettings } from "../api/repository-settings-schema";
export function useRepositorySettings(wsId: string, runtimeId?: string) {
  const qc = useQueryClient();
  const key = ["repository-settings", wsId, runtimeId ?? "workspace"];
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.getRepositorySettings(wsId, runtimeId),
    retry: false,
    refetchInterval: 15000,
  });
  const save = useMutation({
    mutationFn: (value: RepositorySettings) =>
      api.saveRepositorySettings(wsId, value, runtimeId),
    onSuccess: (data) => {
      qc.setQueryData(key, data);
      void qc.invalidateQueries({ queryKey: ["repository-settings"] });
    },
  });
  return { query, save };
}
export type { RepositorySettings } from "../api/repository-settings-schema";
