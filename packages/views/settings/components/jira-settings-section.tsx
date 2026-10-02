"use client";
import { useJiraSettings } from "@multica/core/workspace";
import { Switch } from "@multica/ui/components/ui/switch";
import { Button } from "@multica/ui/components/ui/button";
import { SettingsSection, SettingsCard, SettingsRow } from "./settings-layout";
import { useT } from "../../i18n";
export function JiraSettingsSection({ wsId }: { wsId: string }) {
  const { query, save } = useJiraSettings(wsId);
  const { t } = useT("settings");
  return <SettingsSection title={t(($) => $.jira.title)} description={t(($) => $.jira.description)}>
    {query.data ? <SettingsCard><SettingsRow label={t(($) => $.jira.require)} description={t(($) => $.jira.require_description)}>
      <Switch aria-label={t(($) => $.jira.require)} checked={query.data.required} disabled={!query.data.can_edit || save.isPending || query.isError}
        onCheckedChange={(required) => save.mutate({ ...query.data!, required })} />
    </SettingsRow></SettingsCard> : <p role="status">{t(($) => $.jira.loading)}</p>}
    {(query.isError || save.isError) && <p role="alert" className="text-caption text-destructive">{save.error?.message || query.error?.message}
      <Button variant="ghost" size="sm" onClick={() => void query.refetch()}>{t(($) => $.repository.reload)}</Button>
    </p>}
  </SettingsSection>;
}
