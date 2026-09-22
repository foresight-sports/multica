"use client";

import { useState } from "react";
import { toast } from "sonner";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { runtimePermissionPolicyOptions, useUpdateRuntimePermissionPolicy } from "@multica/core/permissions";
import type { RuntimePermissionPolicy } from "@multica/core/api/schemas";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsTab, SettingsSection, SettingsCard } from "./settings-layout";
import { useT } from "../../i18n";

export function PermissionAccessTab() {
  const { t } = useT("settings");
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const canManage = useAuthStore((state) => state.user?.permissions?.manage_permission_access === true);
  const query = useQuery(runtimePermissionPolicyOptions(userId, canManage));
  if (!canManage) return <p role="status">{t(($) => $.permission_access.denied)}</p>;
  return (
    <SettingsTab title={t(($) => $.permission_access.title)} description={t(($) => $.permission_access.description)}>
      {(query.isPending || query.isFetching) && <p role="status">{t(($) => $.permission_access.loading)}</p>}
      {query.isError && <div role="alert"><p>{t(($) => $.permission_access.load_error)}</p><Button variant="outline" onClick={() => void query.refetch()}>{t(($) => $.permission_access.reload)}</Button></div>}
      {query.data && !query.isError && !query.isFetching && <PermissionEditor key={`${userId}:${query.data.revision}`} policy={query.data} userId={userId} reload={() => void query.refetch()} />}
    </SettingsTab>
  );
}

function PermissionEditor({ policy, userId, reload }: { policy: RuntimePermissionPolicy; userId: string; reload: () => void }) {
  const { t } = useT("settings");
  const [draft, setDraft] = useState(policy.allowed_emails.join("\n"));
  const mutation = useUpdateRuntimePermissionPolicy(userId);
  const initial = policy.allowed_emails.join("\n");
  const emails = draft.split(/[\n,;]/).map((email) => email.trim()).filter(Boolean);
  return (
    <SettingsSection title={t(($) => $.permission_access.runtime_title)} description={t(($) => $.permission_access.runtime_description)}>
      <SettingsCard>
        <form className="space-y-4 p-5" onSubmit={(event) => { event.preventDefault(); mutation.mutate({ allowed_emails: emails, revision: policy.revision }, { onSuccess: () => toast.success(t(($) => $.permission_access.saved)) }); }}>
          {!policy.restricted && <p className="text-body text-muted-foreground">{t(($) => $.permission_access.unrestricted)}</p>}
          <label className="block text-body font-medium" htmlFor="runtime-permission-emails">{t(($) => $.permission_access.users)}</label>
          <Textarea id="runtime-permission-emails" rows={7} value={draft} onChange={(event) => { setDraft(event.target.value); mutation.reset(); }} disabled={mutation.isPending} aria-describedby="runtime-permission-hint" spellCheck={false} />
          <p id="runtime-permission-hint" className="text-caption text-muted-foreground">{t(($) => $.permission_access.hint)}</p>
          {emails.length === 0 && <p role="status" className="text-body">{t(($) => $.permission_access.empty)}</p>}
          {mutation.error && <p role="alert" className="text-body text-destructive">{mutation.error.message}</p>}
          <div className="flex items-center gap-3">
            <Button type="submit" disabled={mutation.isPending || (draft === initial && policy.restricted)}>{mutation.isPending ? t(($) => $.permission_access.saving) : t(($) => $.permission_access.save)}</Button>
            <Button type="button" variant="outline" disabled={mutation.isPending} onClick={reload}>{t(($) => $.permission_access.reload)}</Button>
          </div>
          <p className="text-caption text-muted-foreground">{t(($) => $.permission_access.managers)}</p>
        </form>
      </SettingsCard>
    </SettingsSection>
  );
}
