import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { CodeDelivery } from "../api/code-delivery-schema";
export type { CodeDelivery, Delivery } from "../api/code-delivery-schema";
export function useCodeDelivery(wsId: string) {
  const qc = useQueryClient();
  const queryKey = ["code-delivery", wsId];
  const query = useQuery({
    queryKey,
    queryFn: () => api.getCodeDelivery(wsId),
    retry: false,
  });
  const save = useMutation({
    mutationFn: (value: CodeDelivery) => api.saveCodeDelivery(wsId, value),
    onSuccess: (value) => qc.setQueryData(queryKey, value),
  });
  return { query, save };
}
