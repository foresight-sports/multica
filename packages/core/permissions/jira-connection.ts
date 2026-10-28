import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";

export function useJiraConnection(userId: string, watch = false) {
  const client = useQueryClient();
  const queryKey = ["jira-connection", userId];
  const status = useQuery({
    queryKey, queryFn: () => api.jiraConnection(), enabled: !!userId,
    refetchInterval: (q) => watch || q.state.data?.pending ? 3000 : false,
  });
  const refresh = () => client.invalidateQueries({ queryKey });
  const connect = useMutation({ mutationFn: () => api.connectJira(), onSuccess: refresh });
  const disconnect = useMutation({ mutationFn: () => api.disconnectJira(), onSuccess: refresh });
  return { status, connect, disconnect };
}

export function useIssueJiraBlockers(workspaceId: string, issueId: string, userId: string) {
  const client = useQueryClient();
  const queryKey = ["jira-blockers", workspaceId, issueId, userId];
  const query = useQuery({ queryKey, queryFn: () => api.issueJiraBlockers(issueId),
    enabled: !!workspaceId && !!issueId && !!userId, refetchInterval: 5000 });
  const resume = useMutation({ mutationFn: (taskId: string) => api.resumeJiraWork(issueId, taskId),
    onSuccess: () => { void client.invalidateQueries({ queryKey }); void client.invalidateQueries({ queryKey: ["inbox"] }); },
  });
  return { query, resume };
}
