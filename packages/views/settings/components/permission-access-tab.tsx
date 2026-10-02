"use client";

import { useState } from "react";
import { Bot, ChevronDown, Globe2, Monitor, Pencil, ShieldCheck, type LucideIcon } from "lucide-react";
import { toast } from "sonner";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { runtimePermissionPolicyOptions, useUpdateRuntimePermissionPolicy } from "@multica/core/permissions";
import type { RuntimePermissionPolicy } from "@multica/core/api/schemas";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { SettingsTab, SettingsCard } from "./settings-layout";
import { useT } from "../../i18n";

export function PermissionAccessTab() {
  const { t } = useT("settings");
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const canManage = useAuthStore((state) => state.user?.permissions?.manage_permission_access === true);
  if (!canManage) return <p role="status">{t(($) => $.permission_access.denied)}</p>;
  return (
    <SettingsTab title={t(($) => $.permission_access.title)} description={t(($) => $.permission_access.description)}>
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-caption text-muted-foreground"><Globe2 className="size-4" aria-hidden="true" />{t(($) => $.permission_access.scope)}</div>
        <SettingsCard>
          <PolicySection userId={userId} action="runtime-register" icon={Monitor} title={t(($) => $.permission_access.runtime_title)} description={t(($) => $.permission_access.runtime_description)} />
          <PolicySection userId={userId} action="instance-agent-create" icon={ShieldCheck} title={t(($) => $.permission_access.instance_create_title)} description={t(($) => $.permission_access.instance_create_description)} />
          <PolicySection userId={userId} action="agent-create" icon={Bot} title={t(($) => $.permission_access.agent_create_title)} description={t(($) => $.permission_access.agent_create_description)} />
          <PolicySection userId={userId} action="agent-edit" icon={Pencil} title={t(($) => $.permission_access.agent_edit_title)} description={t(($) => $.permission_access.agent_edit_description)} />
        </SettingsCard>
        <p className="px-1 text-caption text-muted-foreground">{t(($) => $.permission_access.managers)}</p>
      </div>
    </SettingsTab>
  );
}

type PolicySectionProps = { userId: string; action: string; title: string; description: string; icon: LucideIcon };

function PolicySection(props: PolicySectionProps) {
  const { t } = useT("settings");
  const query = useQuery(runtimePermissionPolicyOptions(props.userId, true, props.action));
  if (query.data && !query.isError && !query.isFetching) {
    return <PermissionEditor key={`${props.userId}:${query.data.revision}`} {...props} policy={query.data} reload={() => void query.refetch()} />;
  }
  return <section className="space-y-2 px-5 py-5" aria-label={props.title}>
    <h3 className="text-body font-medium">{props.title}</h3>
    {query.isError
      ? <div role="alert" className="flex flex-wrap items-center gap-3 text-body"><p>{t(($) => $.permission_access.load_error)}</p><Button variant="outline" onClick={() => void query.refetch()}>{t(($) => $.permission_access.reload)}</Button></div>
      : <p role="status" className="text-caption text-muted-foreground">{t(($) => $.permission_access.loading)}</p>}
  </section>;
}

function PermissionEditor({ policy, userId, action, title, description, icon: Icon, reload }: PolicySectionProps & { policy: RuntimePermissionPolicy; reload: () => void }) {
  const { t } = useT("settings");
  const initial = policy.allowed_emails.join("\n");
  const [draft, setDraft] = useState(initial);
  const [expanded, setExpanded] = useState(false);
  const mutation = useUpdateRuntimePermissionPolicy(userId, action);
  const emails = [...new Set(draft.split(/[\n,;]/).map((email) => email.trim().toLowerCase()).filter(Boolean))];
  const dirty = draft !== initial;
  const status = !policy.restricted ? t(($) => $.permission_access.everyone)
    : policy.allowed_emails.length === 0 ? t(($) => $.permission_access.nobody)
      : t(($) => $.permission_access.selected_users, { count: policy.allowed_emails.length });
  return <section aria-label={title}>
    <button type="button" className="flex w-full items-start gap-3 px-4 py-5 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset sm:gap-4 sm:px-5" aria-expanded={expanded} aria-controls={`${action}-editor`} onClick={() => setExpanded(!expanded)}>
      <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/40"><Icon className="size-4 text-muted-foreground" aria-hidden="true" /></span>
      <span className="flex min-w-0 flex-1 flex-col gap-1.5">
        <span className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
          <span className="text-body font-medium">{title}</span>
          <span className="rounded-md bg-muted px-2 py-0.5 text-caption text-muted-foreground">{status}</span>
          {dirty && <span className="text-caption text-muted-foreground">{t(($) => $.permission_access.unsaved)}</span>}
        </span>
        <span className="max-w-2xl text-caption leading-relaxed text-muted-foreground">{description}</span>
      </span>
      <ChevronDown className={cn("mt-2 size-4 shrink-0 text-muted-foreground transition-transform", expanded && "rotate-180")} aria-hidden="true" />
    </button>
    <div id={`${action}-editor`} hidden={!expanded}>
      <form className="space-y-4 border-t border-dashed bg-muted/20 px-4 py-5 sm:px-5" onSubmit={(event) => {
        event.preventDefault();
        mutation.mutate({ allowed_emails: emails, revision: policy.revision }, { onSuccess: () => toast.success(t(($) => $.permission_access.saved)) });
      }}>
        <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] sm:gap-8">
          <div className="space-y-2">
            <label className="block text-body font-medium" htmlFor={`${action}-emails`}>{t(($) => $.permission_access.users)}</label>
            <p id={`${action}-hint`} className="text-caption leading-relaxed text-muted-foreground">{t(($) => $.permission_access.hint)}</p>
          </div>
          <div className="min-w-0 space-y-3">
            <Textarea id={`${action}-emails`} rows={5} value={draft} onChange={(event) => { setDraft(event.target.value); mutation.reset(); }} disabled={mutation.isPending} aria-describedby={`${action}-hint`} spellCheck={false} autoCapitalize="none" autoCorrect="off" />
            {!policy.restricted && <p className="text-caption text-muted-foreground">{t(($) => $.permission_access.unrestricted)}</p>}
            {emails.length === 0 && <p role="status" className="text-caption text-muted-foreground">{t(($) => $.permission_access.empty)}</p>}
            {mutation.error && <div role="alert" className="space-y-2 text-body text-destructive"><p>{mutation.error.message}</p><Button type="button" variant="outline" onClick={reload}>{t(($) => $.permission_access.reload)}</Button></div>}
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button type="button" variant="ghost" disabled={mutation.isPending} onClick={() => { setDraft(initial); mutation.reset(); setExpanded(false); }}>{t(($) => $.permission_access.cancel)}</Button>
              <Button type="submit" disabled={mutation.isPending || (!dirty && policy.restricted)}>{mutation.isPending ? t(($) => $.permission_access.saving) : t(($) => $.permission_access.save)}</Button>
            </div>
          </div>
        </div>
      </form>
    </div>
  </section>;
}
