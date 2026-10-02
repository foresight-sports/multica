import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  issueIntakeOptions,
  intakeSquadsOptions,
  useSaveIssueIntake,
  type IssueIntake,
} from "@multica/core/workspace";
import { projectListOptions } from "@multica/core/projects";
import { Button } from "@multica/ui/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { SettingsSection, SettingsCard, SettingsRow } from "./settings-layout";
import { useT } from "../../i18n";

export function IssueIntakeSection({
  workspaceId,
  canManage,
}: {
  workspaceId: string;
  canManage: boolean;
}) {
  const { t } = useT("settings");
  const query = useQuery(issueIntakeOptions(workspaceId));
  const squads = useQuery(intakeSquadsOptions(workspaceId));
  const projects = useQuery(projectListOptions(workspaceId));
  const save = useSaveIssueIntake(workspaceId);
  const [draft, setDraft] = useState<IssueIntake | null>(null);
  const value = draft ?? query.data;
  const update = (next: IssueIntake) => {
    save.reset();
    setDraft(next);
  };
  const picker = (
    selected: string,
    onChange: (value: string) => void,
    inherit = false,
    label = "",
  ) => (
    <Select
      items={{
        [selected]: t(($) => $.intake.unavailable),
        off: t(($) => $.intake.off),
        inherit: t(($) => $.intake.inherit),
        ...Object.fromEntries((squads.data ?? []).map((s) => [s.id, s.name])),
      }}
      value={selected}
      disabled={!canManage || save.isPending || squads.isError || !squads.data}
      onValueChange={(v) => {
        if (v !== null) onChange(v);
      }}
    >
      <SelectTrigger aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {inherit && (
          <SelectItem value="inherit">{t(($) => $.intake.inherit)}</SelectItem>
        )}
        <SelectItem value="off">{t(($) => $.intake.off)}</SelectItem>
        {(squads.data ?? []).map((s) => (
          <SelectItem key={s.id} value={s.id}>
            {s.name}
          </SelectItem>
        ))}
        {selected !== "off" &&
          selected !== "inherit" &&
          !squads.data?.some((s) => s.id === selected) && (
            <SelectItem value={selected}>
              {t(($) => $.intake.unavailable)}
            </SelectItem>
          )}
      </SelectContent>
    </Select>
  );
  return (
    <SettingsSection
      title={t(($) => $.intake.title)}
      description={t(($) => $.intake.description)}
    >
      {!value ? (
        <p role={query.isError ? "alert" : "status"} className="text-caption">
          {query.isError
            ? t(($) => $.intake.load_error)
            : t(($) => $.intake.loading)}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void query.refetch()}
          >
            {t(($) => $.intake.reload)}
          </Button>
        </p>
      ) : (
        <>
          <SettingsCard>
            <SettingsRow
              label={t(($) => $.intake.default)}
              description={t(($) => $.intake.leader_access)}
              size="select-wide"
            >
              {picker(
                value.defaultSquadId || "off",
                (v) =>
                  update({ ...value, defaultSquadId: v === "off" ? "" : v }),
                false,
                t(($) => $.intake.default),
              )}
            </SettingsRow>
          </SettingsCard>
          {squads.isError && (
            <p role="alert" className="text-caption text-destructive">
              {t(($) => $.intake.squads_error)}
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void squads.refetch()}
              >
                {t(($) => $.intake.reload)}
              </Button>
            </p>
          )}
          {squads.data?.length === 0 && (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.intake.no_squads)}
            </p>
          )}
          <details className="space-y-3">
            <summary className="cursor-pointer text-caption font-medium">
              {t(($) => $.intake.projects)}
            </summary>
            {projects.isError ? (
              <p role="alert">{t(($) => $.intake.load_error)}</p>
            ) : (
              <SettingsCard>
                {[
                  ...(projects.data ?? []),
                  ...Object.keys(value.projects)
                    .filter(
                      (id) =>
                        projects.data &&
                        !projects.data.some((p) => p.id === id),
                    )
                    .map((id) => ({
                      id,
                      title: t(($) => $.intake.missing_project),
                    })),
                ].map((p) => (
                  <SettingsRow key={p.id} label={p.title} size="select-wide">
                    {picker(
                      Object.hasOwn(value.projects, p.id)
                        ? value.projects[p.id] || "off"
                        : "inherit",
                      (v) => {
                        const overrides = { ...value.projects };
                        if (v === "inherit") delete overrides[p.id];
                        else overrides[p.id] = v === "off" ? "" : v;
                        update({ ...value, projects: overrides });
                      },
                      true,
                      p.title,
                    )}
                  </SettingsRow>
                ))}
              </SettingsCard>
            )}
          </details>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.intake.scope)}
          </p>
          {save.isError && (
            <p role="alert" className="text-caption text-destructive">
              {save.error.message}
            </p>
          )}
          {save.isSuccess && (
            <p role="status" className="text-caption text-success">
              {t(($) => $.intake.saved)}
            </p>
          )}
          {canManage && (
            <div className="flex gap-2">
              <Button
                size="sm"
                disabled={!draft || save.isPending || squads.isError}
                onClick={() =>
                  save.mutate(value, { onSuccess: () => setDraft(null) })
                }
              >
                {save.isPending
                  ? t(($) => $.intake.saving)
                  : t(($) => $.intake.save)}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={save.isPending}
                onClick={() => {
                  setDraft(null);
                  save.reset();
                  void query.refetch();
                }}
              >
                {t(($) => $.intake.reload)}
              </Button>
            </div>
          )}
        </>
      )}
    </SettingsSection>
  );
}
