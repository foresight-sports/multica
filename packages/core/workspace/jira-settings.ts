import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { JiraSettings } from "../api/jira-settings-schema";
export function useJiraSettings(wsId: string) {
  const qc = useQueryClient();
  const key = ["workspace", wsId, "jira-settings"];
  const query = useQuery({ queryKey: key, queryFn: () => api.getJiraSettings(wsId), enabled: !!wsId, refetchInterval: 15000 });
  const save = useMutation({ mutationFn: (value: JiraSettings) => api.saveJiraSettings(wsId, value), onSuccess: (data) => {
    qc.setQueryData(key, data);
    void qc.invalidateQueries({ queryKey: ["workspace", wsId] });
    void qc.invalidateQueries({ queryKey: ["properties", wsId] });
  } });
  return { query, save };
}
