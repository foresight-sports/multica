import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";

export function useWorkRecords(wsId: string, issueId: string) {
  const qc = useQueryClient();
  const key = ["issues", wsId, issueId, "work-records"];
  const query = useQuery({ queryKey: key, queryFn: () => api.getWorkRecords(issueId), enabled: !!wsId && !!issueId, refetchInterval: 10000 });
  const update = useMutation({
    mutationFn: (input: { id: string; revision: number; action: "approve" | "decline" | "resolve" }) => api.updateWorkRecord(issueId, input.id, input),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: key }); },
  });
  return { query, update };
}
