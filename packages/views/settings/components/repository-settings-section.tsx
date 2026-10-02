"use client";
import { useState } from "react";
import { useRepositorySettings } from "@multica/core/workspace";
import type { RepositorySettings } from "@multica/core/workspace";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Switch } from "@multica/ui/components/ui/switch";
import { useT } from "../../i18n";
import { SettingsSection, SettingsCard, SettingsRow } from "./settings-layout";

export function RepositorySettingsSection({
  wsId,
  runtimeId,
}: {
  wsId: string;
  runtimeId?: string;
}) {
  const { t } = useT("settings");
  const { query, save } = useRepositorySettings(wsId, runtimeId);
  const [draft, setDraft] = useState<RepositorySettings | null>(null);
  const value = draft ?? query.data;
  const machine = !!runtimeId;
  const edit = (patch: Partial<RepositorySettings>) => {
    if (value) setDraft({ ...value, ...patch });
  };
  return (
    <SettingsSection
      title={t(($) =>
        machine ? $.repository.machine_title : $.repository.title,
      )}
      description={t(($) =>
        machine ? $.repository.machine_description : $.repository.description,
      )}
    >
      {!value ? (
        <p className="text-caption text-muted-foreground" role="status">
          {query.isError
            ? t(($) => $.repository.load_error)
            : t(($) => $.repository.loading)}{" "}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void query.refetch()}
          >
            {t(($) => $.repository.reload)}
          </Button>
        </p>
      ) : (
        <>
          <SettingsCard>
            {machine ? (
              <SettingsRow label={t(($) => $.repository.root)} size="text">
                <Input
                  aria-label={t(($) => $.repository.root)}
                  placeholder={t(($) => $.repository.root_placeholder)}
                  value={value.root}
                  disabled={!query.data?.can_edit || save.isPending}
                  onChange={(e) => edit({ root: e.target.value })}
                />
              </SettingsRow>
            ) : (
              <>
                <SettingsRow label={t(($) => $.repository.repo)} size="text">
                  <Input
                    aria-label={t(($) => $.repository.repo)}
                    placeholder={t(($) => $.repository.repo_placeholder)}
                    value={value.repository}
                    disabled={!query.data?.can_edit || save.isPending}
                    onChange={(e) => edit({ repository: e.target.value })}
                  />
                </SettingsRow>
                <SettingsRow label={t(($) => $.repository.folder)} size="text">
                  <Input
                    aria-label={t(($) => $.repository.folder)}
                    placeholder={value.repository.split("/")[1] || "repository"}
                    value={value.folder}
                    disabled={!query.data?.can_edit || save.isPending}
                    onChange={(e) => edit({ folder: e.target.value })}
                  />
                </SettingsRow>
                <SettingsRow
                  label={t(($) => $.repository.worktree)}
                  description={t(($) => $.repository.worktree_description)}
                >
                  <Switch
                    aria-label={t(($) => $.repository.worktree)}
                    checked={value.mode === "worktree" || (!value.mode && !!value.repository)}
                    disabled={!query.data?.can_edit || save.isPending}
                    onCheckedChange={(checked) =>
                      edit({ mode: checked ? "worktree" : "in_place" })
                    }
                  />
                </SettingsRow>
              </>
            )}
          </SettingsCard>
          {machine && !value.supported && (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.repository.upgrade)}
            </p>
          )}
          {!query.data?.can_edit && (
            <p className="text-caption text-muted-foreground">
              {t(($) =>
                machine ? $.repository.owner_only : $.repository.admin_only,
              )}
            </p>
          )}
          {query.isError && (
            <p role="alert" className="text-caption text-destructive">
              {t(($) => $.repository.load_error)}
            </p>
          )}
          {save.isError && (
            <p role="alert" className="text-caption text-destructive">
              {save.error.message}
            </p>
          )}
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              disabled={
                !draft ||
                !query.data?.can_edit ||
                save.isPending ||
                query.isError
              }
              onClick={() =>
                save.mutate(value, { onSuccess: () => setDraft(null) })
              }
            >
              {t(($) =>
                save.isPending ? $.repository.saving : $.repository.save,
              )}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setDraft(null);
                save.reset();
                void query.refetch();
              }}
            >
              {t(($) => $.repository.reload)}
            </Button>
            {save.isSuccess && !draft && (
              <span
                role="status"
                className="text-caption text-muted-foreground"
              >
                {t(($) => $.repository.saved)}
              </span>
            )}
          </div>
        </>
      )}
    </SettingsSection>
  );
}
