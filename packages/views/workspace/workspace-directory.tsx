"use client";

import { useState } from "react";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceDirectory } from "@multica/core/workspace/directory";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { useT } from "../i18n";

export function WorkspaceDirectory({ onOpen, onCreate, busy = false }: {
  onOpen: (id: string, slug: string) => void;
  onCreate?: () => void;
  busy?: boolean;
}) {
  const { t } = useT("onboarding");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const { directory, reviews, mutation } = useWorkspaceDirectory(userId, query);
  const disabled = busy || mutation.isPending;
  return <div className="space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><h2 className="text-title font-semibold">{t(($) => $.directory.title)}</h2><p className="mt-1 text-body text-muted-foreground">{t(($) => $.directory.description)}</p></div>
      {directory.data?.can_create === true && onCreate && <Button variant="outline" onClick={onCreate}>{t(($) => $.directory.create)}</Button>}
    </div>
    <form className="flex gap-2" onSubmit={(event) => { event.preventDefault(); setQuery(search); }}>
      <Input aria-label={t(($) => $.directory.search)} placeholder={t(($) => $.directory.search)} value={search} maxLength={200} onChange={(e) => setSearch(e.target.value)} />
      <Button type="submit" variant="outline">{t(($) => $.directory.search_button)}</Button>
    </form>
    {directory.isPending && <p role="status">{t(($) => $.directory.loading)}</p>}
    {directory.isError && <div role="alert"><p>{t(($) => $.directory.load_error)}</p><Button variant="outline" onClick={() => void directory.refetch()}>{t(($) => $.directory.retry)}</Button></div>}
    {mutation.isError && <p role="alert" className="text-body text-destructive">{mutation.error.message || t(($) => $.directory.save_error)}</p>}
    {directory.data && !directory.isError && <>
      {directory.data.workspaces.length === 0 && <p className="text-body text-muted-foreground">{t(($) => $.directory.empty)}</p>}
      <ul className="divide-y rounded-lg border">
        {directory.data.workspaces.map((ws) => <li key={ws.id} className="flex flex-wrap items-center justify-between gap-3 p-4">
          <div className="min-w-0"><p className="break-words text-body font-medium">{ws.name}</p>
            <p className="text-caption text-muted-foreground">{ws.role ? t(($) => $.directory.member) : ws.request_status === "pending" ? t(($) => $.directory.pending) : ws.request_status === "declined" ? t(($) => $.directory.declined) : t(($) => $.directory.approval_required)}</p></div>
          {ws.role ? <Button variant="outline" disabled={disabled} onClick={() => onOpen(ws.id, ws.slug)}>{t(($) => $.directory.open)}</Button>
            : <Button variant={ws.request_status === "pending" ? "ghost" : "outline"} disabled={disabled} onClick={() => mutation.mutate({ workspaceId: ws.id, action: ws.request_status === "pending" ? "cancel" : "request" })}>
              {ws.request_status === "pending" ? t(($) => $.directory.cancel) : t(($) => $.directory.request)}</Button>}
        </li>)}
      </ul>
      {directory.data.truncated && <p className="text-caption text-muted-foreground">{t(($) => $.directory.truncated)}</p>}
    </>}
    {reviews.isError && <div role="alert"><p>{t(($) => $.directory.review_error)}</p><Button variant="outline" onClick={() => void reviews.refetch()}>{t(($) => $.directory.retry)}</Button></div>}
    {!!reviews.data?.requests.length && <section className="space-y-3" aria-label={t(($) => $.directory.review_title)}>
      <h3 className="text-body font-semibold">{t(($) => $.directory.review_title)}</h3>
      <p className="text-caption text-muted-foreground">{t(($) => $.directory.review_description)}</p>
      <ul className="divide-y rounded-lg border">{reviews.data.requests.map((request) => <li key={`${request.workspace_id}:${request.user_id}`} className="space-y-3 p-4">
        <div><p className="text-body font-medium">{request.workspace_name}</p><p className="break-words text-caption text-muted-foreground">{request.name} · {request.email}</p></div>
        <div className="flex gap-2"><Button size="sm" disabled={disabled} onClick={() => mutation.mutate({ workspaceId: request.workspace_id, userId: request.user_id, action: "approved" })}>{t(($) => $.directory.approve)}</Button>
          <Button size="sm" variant="outline" disabled={disabled} onClick={() => mutation.mutate({ workspaceId: request.workspace_id, userId: request.user_id, action: "declined" })}>{t(($) => $.directory.decline)}</Button></div>
      </li>)}</ul>
      {reviews.data.truncated && <p className="text-caption text-muted-foreground">{t(($) => $.directory.review_truncated)}</p>}
    </section>}
  </div>;
}
