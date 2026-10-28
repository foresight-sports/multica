import { useWorkspaceId } from "@multica/core/hooks";
import { useProviderAuth } from "@multica/core/runtimes";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

export function ProviderAuthSection({ runtimeId }: { runtimeId: string }) {
  const { t } = useT("runtimes");
  const auth = useProviderAuth(useWorkspaceId(), runtimeId);
  const state = auth.status.data?.status;
  return <section className="space-y-2 text-caption" aria-label={t(($) => $.provider_auth.title)}>
    <p className="font-medium">{t(($) => $.provider_auth.title)}</p>
    <p>{t(($) => $.provider_auth.description)}</p>
    <div className="flex gap-2">
      <Button variant="outline" size="xs" disabled={auth.busy || !auth.status.data || auth.status.isError} onClick={() => auth.start.mutate("claude")}>{t(($) => $.provider_auth.claude)}</Button>
      <Button variant="outline" size="xs" disabled={auth.busy || !auth.status.data || auth.status.isError} onClick={() => auth.start.mutate("codex")}>{t(($) => $.provider_auth.codex)}</Button>
    </div>
    {state === "pending" && <p role="status">{t(($) => $.provider_auth.pending)}</p>}
    {state === "running" && <p role="status">{t(($) => $.provider_auth.running)}</p>}
    {state === "completed" && <p role="status">{t(($) => $.provider_auth.completed)}</p>}
    {(state === "failed" || auth.start.isError) && <p role="alert">{auth.start.error?.message ?? t(($) => $.provider_auth.failed)}</p>}
    {auth.status.isError && <Button variant="ghost" size="xs" onClick={() => void auth.status.refetch()}>{t(($) => $.provider_auth.retry)}</Button>}
  </section>;
}
