import {
  useMutation,
  useQueryClient,
  queryOptions,
} from "@tanstack/react-query";
import { api } from "../api";
import type { IssueIntake } from "../api/issue-intake-schema";
export type { IssueIntake } from "../api/issue-intake-schema";
export const issueIntakeKey = (wsId: string) =>
  ["workspace", wsId, "issue-intake"] as const;
export const issueIntakeOptions = (wsId: string) =>
  queryOptions({
    queryKey: issueIntakeKey(wsId),
    queryFn: () => api.getIssueIntake(wsId),
  });
export const intakeSquadsOptions = (wsId: string) =>
  queryOptions({
    queryKey: ["squads", wsId, "list"],
    queryFn: () => api.listSquads(),
  });
export function useSaveIssueIntake(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (config: IssueIntake) => api.updateIssueIntake(wsId, config),
    onSuccess: (data) => {
      qc.setQueryData(issueIntakeKey(wsId), data);
      void qc.invalidateQueries({ queryKey: ["issues", "issue-trigger-preview"] });
    },
  });
}
