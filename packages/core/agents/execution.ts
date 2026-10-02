import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api, type ExecutionPolicy, type ExecutionRequest } from "../api";
export const executionKey = (wsId: string, id: string) =>
  ["agents", wsId, "execution", id] as const;
export const executionOptions = (wsId: string, id: string) =>
  queryOptions({
    queryKey: executionKey(wsId, id),
    queryFn: () => api.getAgentExecution(id),
    enabled: !!wsId && !!id,
  });
export function useSaveExecution(wsId: string, id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: ExecutionPolicy) => api.saveAgentExecution(id, p),
    onSuccess: (p) => {
      qc.setQueryData(executionKey(wsId, id), p);
      void qc.invalidateQueries({ predicate: q => q.queryKey.includes(wsId) && !q.queryKey.includes("execution") });
    },
  });
}
export function usePreviewExecution(id: string) {
  return useMutation({
    mutationFn: (prompt: string) => api.previewAgentExecution(id, prompt),
  });
}
export function useTaskExecution(
  wsId: string,
  taskId: string,
  issueId: string,
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      execution,
      rerun,
    }: {
      execution: ExecutionRequest;
      rerun: boolean;
    }) =>
      rerun
        ? api.rerunIssue(issueId, taskId, execution)
        : api.updateTaskExecution(taskId, execution),
    onSuccess: () =>
      qc.invalidateQueries({ predicate: (q) => q.queryKey.includes(wsId) }),
  });
}
