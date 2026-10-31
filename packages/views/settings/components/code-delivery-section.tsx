"use client";
import { useT } from "../../i18n";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { projectListOptions } from "@multica/core/projects";
import {
  useCodeDelivery,
  type CodeDelivery,
  type Delivery,
} from "@multica/core/workspace";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { SettingsSection, SettingsCard, SettingsRow } from "./settings-layout";

export function CodeDeliverySection({ wsId }: { wsId: string }) {
  const { t } = useT("settings");
  const { query, save } = useCodeDelivery(wsId);
  const projects = useQuery(projectListOptions(wsId));
  const [draft, setDraft] = useState<CodeDelivery | null>(null);
  const [project, setProject] = useState("");
  const value = draft ?? query.data;
  const policy = value && (value.projects[project] ?? value.default);
  const disabled = !query.data?.can_edit || save.isPending || query.isError;
  const edit = (patch: Partial<Delivery>) => {
    if (!value || !policy) return;
    const next = { ...policy, ...patch };
    setDraft(
      project
        ? { ...value, projects: { ...value.projects, [project]: next } }
        : { ...value, default: next },
    );
  };
  return (
    <SettingsSection
      title={t(($) => $.code_delivery.title)}
      description={t(($) => $.code_delivery.description)}
    >
      {!value || !policy ? (
        <p role="status">
          {query.isError
            ? t(($) => $.code_delivery.load_error)
            : t(($) => $.code_delivery.loading)}
        </p>
      ) : (
        <>
          {!value.automation_available && (
            <p role="status">{t(($) => $.code_delivery.unavailable)}</p>
          )}
          {value.automation_errors.map((message) => (
            <p key={message} role="status">
              {message}
            </p>
          ))}
          <SettingsCard>
            <SettingsRow label={t(($) => $.code_delivery.scope)} size="text">
              <select
                aria-label={t(($) => $.code_delivery.scope_label)}
                className="bg-background border rounded px-2 py-1"
                value={project}
                onChange={(e) => setProject(e.target.value)}
              >
                <option value="">
                  {t(($) => $.code_delivery.workspace_default)}
                </option>
                {projects.data?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.title}
                  </option>
                ))}
              </select>
            </SettingsRow>
            {project && (
              <SettingsRow
                label={
                  value.projects[project]
                    ? t(($) => $.code_delivery.override)
                    : t(($) => $.code_delivery.inherits)
                }
              >
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={disabled || !value.projects[project]}
                  onClick={() => {
                    const overrides = { ...value.projects };
                    delete overrides[project];
                    setDraft({ ...value, projects: overrides });
                  }}
                >
                  {t(($) => $.code_delivery.reset)}
                </Button>
              </SettingsRow>
            )}
            <SettingsRow label={t(($) => $.code_delivery.method)} size="text">
              <select
                aria-label={t(($) => $.code_delivery.method)}
                className="bg-background border rounded px-2 py-1"
                disabled={disabled}
                value={policy.mode}
                onChange={(e) =>
                  edit({
                    mode:
                      e.target.value === "direct" ? "direct" : "pull_request",
                  })
                }
              >
                <option value="pull_request">
                  {t(($) => $.code_delivery.pr)}
                </option>
                <option value="direct">
                  {t(($) => $.code_delivery.direct)}
                </option>
              </select>
            </SettingsRow>
            <SettingsRow
              label={t(($) => $.code_delivery.branch)}
              description={t(($) => $.code_delivery.branch_hint)}
              size="text"
            >
              <Input
                aria-label={t(($) => $.code_delivery.branch_label)}
                disabled={disabled}
                value={policy.target_branch}
                onChange={(e) => edit({ target_branch: e.target.value })}
              />
            </SettingsRow>
            {policy.mode === "pull_request" && (
              <>
                <SettingsRow
                  label={t(($) => $.code_delivery.ai)}
                  description={t(($) => $.code_delivery.ai_hint)}
                  size="text"
                >
                  <Input
                    aria-label={t(($) => $.code_delivery.ai)}
                    disabled={disabled}
                    value={policy.ai_reviewers.join(",")}
                    onChange={(e) =>
                      edit({ ai_reviewers: e.target.value.split(",") })
                    }
                  />
                </SettingsRow>
                <SettingsRow
                  label={t(($) => $.code_delivery.humans)}
                  description={t(($) => $.code_delivery.humans_hint)}
                  size="text"
                >
                  <Input
                    aria-label={t(($) => $.code_delivery.humans)}
                    disabled={disabled}
                    value={policy.human_reviewers.join(",")}
                    onChange={(e) =>
                      edit({ human_reviewers: e.target.value.split(",") })
                    }
                  />
                </SettingsRow>
                <SettingsRow
                  label={t(($) => $.code_delivery.teams)}
                  description={t(($) => $.code_delivery.teams_hint)}
                  size="text"
                >
                  <Input
                    aria-label={t(($) => $.code_delivery.teams)}
                    disabled={disabled}
                    value={policy.human_teams.join(",")}
                    onChange={(e) =>
                      edit({ human_teams: e.target.value.split(",") })
                    }
                  />
                </SettingsRow>
                {policy.ai_reviewers.length === 0 && (
                  <p className="text-caption text-muted-foreground p-3">
                    {t(($) => $.code_delivery.off)}
                  </p>
                )}
              </>
            )}
          </SettingsCard>
          {save.isError && <p role="alert">{save.error.message}</p>}
          <Button
            size="sm"
            disabled={disabled || !draft}
            onClick={() =>
              save.mutate(value, { onSuccess: () => setDraft(null) })
            }
          >
            {save.isPending
              ? t(($) => $.code_delivery.saving)
              : t(($) => $.code_delivery.save)}
          </Button>
          {save.isSuccess && !draft && (
            <span role="status">{t(($) => $.code_delivery.saved)}</span>
          )}
        </>
      )}
      <Button
        size="sm"
        variant="ghost"
        onClick={() => {
          setDraft(null);
          save.reset();
          void query.refetch();
        }}
      >
        {t(($) => $.code_delivery.reload)}
      </Button>
    </SettingsSection>
  );
}
