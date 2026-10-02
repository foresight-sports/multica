import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "../api";
export function useInstanceUpdate(wsId: string, runtimeId: string) {
  const status = useQuery({
    queryKey: ["runtimes", wsId, runtimeId, "instance-update"],
    queryFn: () => api.getInstanceUpdateStatus(runtimeId),
    refetchInterval: 5000,
  });
  const mutation = useMutation({
    mutationFn: async (version: string) => ({
      request: await api.initiateUpdate(runtimeId, version),
      startedAt: Date.now(),
    }),
  });
  const request = mutation.data?.request.runtime_id === runtimeId ? mutation.data.request : undefined;
  const confirmed =
    !!request &&
    status.data?.currentVersion === request.target_version &&
    status.data.online;
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    setExpired(false);
    if (!request) return;
    const timer = setTimeout(() => setExpired(true), 240000);
    return () => clearTimeout(timer);
  }, [request]);
  const result = useQuery({
    queryKey: [
      "runtimes",
      wsId,
      runtimeId,
      "instance-update-result",
      request?.id,
    ],
    enabled: !!request && !confirmed && !expired,
    queryFn: () => api.getUpdateResult(runtimeId, request!.id),
    refetchInterval: (q) =>
      ["failed", "timeout"].includes(q.state.data?.status ?? "") ? false : 2000,
  });
  const failed =
    result.data?.status === "failed" || result.data?.status === "timeout";
  return {
    status,
    mutation,
    confirmed,
    expired,
    failed,
    result,
    busy:
      mutation.isPending || (!!request && !confirmed && !expired && !failed),
  };
}
