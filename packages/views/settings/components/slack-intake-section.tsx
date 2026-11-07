"use client";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import type { SlackIntakeRule } from "@multica/core/api/slack-intake-schema";
import { agentListOptions } from "@multica/core/workspace/queries";
import { slackInstallationsOptions } from "@multica/core/slack";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { SettingsSection } from "./settings-layout";
import { useT } from "../../i18n";

export function SlackIntakeSection({ wsId }: { wsId: string }) {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const rules = useQuery({
    queryKey: ["slack-intake", wsId],
    queryFn: () => api.getSlackIntake(wsId),
  });
  const activity = useQuery({
    queryKey: ["slack-intake-activity", wsId],
    queryFn: () => api.getSlackSupportActivity(wsId),
    refetchInterval: 15000,
  });
  const agents = useQuery(agentListOptions(wsId));
  const apps = useQuery(slackInstallationsOptions(wsId));
  const [draft, setDraft] = useState<Partial<SlackIntakeRule>>({
    workflow: "support_diagnosis",
    revision: 0,
    timeout_minutes: 60,
    enabled: false,
  });
  const directory = useQuery({
    queryKey: ["slack-intake-directory", wsId, draft.installation_id],
    queryFn: () => api.getSlackSupportDirectory(wsId, draft.installation_id!),
    enabled: !!draft.installation_id,
    retry: false,
  });
  const save = useMutation({
    mutationFn: () =>
      api.saveSlackIntake(wsId, {
        ...draft,
        coordinator_id: undefined,
        analyst_id: undefined,
        composer_id: undefined,
        audience: undefined,
        style_instructions: undefined,
        ...(draft.id && !draft.enabled && directory.isError
          ? { disable_only: true }
          : {}),
      }),
    onSuccess: (value) => {
      qc.setQueryData(["slack-intake", wsId], value);
      const current = value.find(
        (x) =>
          x.channel_id === draft.channel_id &&
          x.installation_id === draft.installation_id,
      );
      if (current) setDraft(current);
    },
  });
  const selectClass =
    "h-9 w-full rounded-md border border-input bg-background px-3 text-body";
  const update = (
    key: keyof SlackIntakeRule,
    value: string | number | boolean,
  ) => setDraft((d) => ({ ...d, [key]: value }));
  const valid =
    !!draft.installation_id &&
    !!draft.channel_id &&
    !!draft.escalation_user_id &&
    (!directory.isError || (!!draft.id && !draft.enabled)) &&
    !rules.isError;
  return (
    <SettingsSection
      title={t(($) => $.slack_intake.title)}
      description={t(($) => $.slack_intake.description)}
    >
      <p className="text-caption text-muted-foreground">
        {t(($) => $.slack_intake.identity)}{" "}
        {t(($) => $.slack_intake.coordinator_hint)}
      </p>
      {rules.isPending && (
        <p role="status">{t(($) => $.slack_intake.loading)}</p>
      )}
      {rules.isError && (
        <div role="alert">
          <p>{t(($) => $.slack_intake.load_error)}</p>
          <Button onClick={() => void rules.refetch()} variant="outline">
            {t(($) => $.slack_intake.retry)}
          </Button>
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        {rules.data?.map((rule) => (
          <Button
            key={rule.id}
            variant={draft.id === rule.id ? "default" : "outline"}
            onClick={() => {
              save.reset();
              setDraft(rule);
            }}
          >
            #{rule.channel_name} ·{" "}
            {rule.enabled
              ? t(($) => $.slack_intake.on)
              : t(($) => $.slack_intake.off)}
          </Button>
        ))}
        <Button
          variant="outline"
          onClick={() => {
            save.reset();
            setDraft({
              workflow: "support_diagnosis",
              revision: 0,
              timeout_minutes: 60,
              enabled: false,
            });
          }}
        >
          {t(($) => $.slack_intake.new)}
        </Button>
      </div>
      <form
        className="grid gap-4 max-w-xl"
        onSubmit={(event) => {
          event.preventDefault();
          if (valid) save.mutate();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="support-app">{t(($) => $.slack_intake.app)}</Label>
          <select
            id="support-app"
            className={selectClass}
            value={draft.installation_id ?? ""}
            disabled={!!draft.id || save.isPending}
            onChange={(e) =>
              setDraft((d) => ({
                ...d,
                installation_id: e.target.value,
                channel_id: "",
                escalation_user_id: "",
              }))
            }
          >
            <option value="">{t(($) => $.slack_intake.choose)}</option>
            {apps.data?.installations
              .filter((a) => a.status === "active")
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {agents.data?.find((x) => x.id === a.agent_id)?.name ??
                    a.team_id}
                </option>
              ))}
          </select>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.slack_intake.connect_hint)}
          </p>
        </div>
        {directory.isPending && draft.installation_id && (
          <p role="status">{t(($) => $.slack_intake.loading)}</p>
        )}
        {directory.isError && (
          <div role="alert">
            <p>
              {directory.error instanceof Error
                ? directory.error.message
                : t(($) => $.slack_intake.directory_error)}
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={() => void directory.refetch()}
            >
              {t(($) => $.slack_intake.retry)}
            </Button>
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="support-channel">
            {t(($) => $.slack_intake.channel)}
          </Label>
          <select
            id="support-channel"
            className={selectClass}
            value={draft.channel_id ?? ""}
            disabled={!!draft.id || save.isPending}
            onChange={(e) => update("channel_id", e.target.value)}
          >
            <option value="">{t(($) => $.slack_intake.choose)}</option>
            {directory.data?.channels.map((x) => (
              <option key={x.id} value={x.id}>
                {x.name}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="support-escalation">
            {t(($) => $.slack_intake.escalation)}
          </Label>
          <select
            id="support-escalation"
            className={selectClass}
            value={draft.escalation_user_id ?? ""}
            disabled={save.isPending}
            onChange={(e) => update("escalation_user_id", e.target.value)}
          >
            <option value="">{t(($) => $.slack_intake.choose)}</option>
            {directory.data?.people.map((x) => (
              <option key={x.id} value={x.id}>
                {x.name}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="support-timeout">
            {t(($) => $.slack_intake.timeout)}
          </Label>
          <Input
            id="support-timeout"
            type="number"
            min={5}
            max={1440}
            value={draft.timeout_minutes}
            disabled={save.isPending}
            onChange={(e) => update("timeout_minutes", Number(e.target.value))}
          />
        </div>
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="support-enabled">
            {t(($) => $.slack_intake.enabled)}
          </Label>
          <Switch
            id="support-enabled"
            checked={draft.enabled ?? false}
            disabled={save.isPending}
            onCheckedChange={(v) => update("enabled", v)}
          />
        </div>
        <p className="text-caption text-muted-foreground">
          {t(($) => $.slack_intake.policy)}
        </p>
        {save.isError && (
          <p role="alert" className="text-destructive">
            {save.error.message}
          </p>
        )}
        {save.isSuccess && (
          <p role="status">{t(($) => $.slack_intake.saved)}</p>
        )}
        <Button type="submit" disabled={!valid || save.isPending}>
          {save.isPending
            ? t(($) => $.slack_intake.saving)
            : t(($) => $.slack_intake.save)}
        </Button>
      </form>
      <div className="space-y-2">
        <h3 className="font-medium">{t(($) => $.slack_intake.activity)}</h3>
        {activity.isError ? (
          <p role="alert">{t(($) => $.slack_intake.activity_error)}</p>
        ) : activity.data?.length === 0 ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.slack_intake.empty)}
          </p>
        ) : (
          activity.data?.map((a) => (
            <div key={a.id} className="border-t py-2 text-caption">
              <span>
                #{a.channel_name} · {a.state.replaceAll("_", " ")}
              </span>
              {a.error && <p className="text-destructive">{a.error}</p>}
            </div>
          ))
        )}
      </div>
    </SettingsSection>
  );
}
