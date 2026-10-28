"use client";

import { useState } from "react";
import { useAuthStore } from "@multica/core/auth";
import { useJiraConnection } from "@multica/core/permissions";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
import { openExternal } from "../../platform";
import { SettingsCard, SettingsRow, SettingsSection } from "./settings-layout";

export function JiraConnectionSection() {
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const { status, connect, disconnect } = useJiraConnection(userId);
  const { t } = useT("settings");
  const [confirmDisconnect, setConfirmDisconnect] = useState(false);
  const busy = connect.isPending || disconnect.isPending;
  const error = status.error ?? connect.error ?? disconnect.error;
  return (
    <SettingsSection title={t(($) => $.jira_connection.title)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.jira_connection.title)} description={t(($) => $.jira_connection.description)}>
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-muted-foreground">
              {status.isPending ? t(($) => $.jira_connection.loading)
                : !status.data?.configured ? t(($) => $.jira_connection.unavailable)
                  : status.data.pending ? t(($) => $.jira_connection.pending)
                    : status.data.status === "connected" ? t(($) => $.jira_connection.connected)
                      : status.data.status === "reconnect_required" ? t(($) => $.jira_connection.expired)
                        : t(($) => $.jira_connection.disconnected)}
            </p>
            {status.data?.configured && <div className="flex flex-wrap gap-2">
              <Button disabled={busy} onClick={() => connect.mutate(undefined, {
                onSuccess: (value) => openExternal(value.redirect_url, { webTarget: "same-tab" }),
              })}>{status.data.status === "disconnected" ? t(($) => $.jira_connection.connect) : t(($) => $.jira_connection.reconnect)}</Button>
              {(status.data.status !== "disconnected" || status.data.pending) && <Button variant="outline" disabled={busy} onClick={() => setConfirmDisconnect(true)}>{t(($) => $.jira_connection.disconnect)}</Button>}
            </div>}
            {confirmDisconnect && <div className="flex flex-col gap-2">
              <p>{t(($) => $.jira_connection.confirm)}</p>
              <div className="flex gap-2">
                <Button variant="destructive" disabled={busy} onClick={() => disconnect.mutate(undefined, { onSuccess: () => setConfirmDisconnect(false) })}>{t(($) => $.jira_connection.disconnect)}</Button>
                <Button variant="ghost" onClick={() => setConfirmDisconnect(false)}>{t(($) => $.jira_connection.cancel)}</Button>
              </div>
            </div>}
            {error && <div role="alert" className="text-sm text-destructive">{error.message}<Button variant="link" onClick={() => void status.refetch()}>{t(($) => $.jira_connection.retry)}</Button></div>}
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
