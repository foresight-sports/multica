"use client";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { SettingsSection } from "./settings-layout";
import { useT } from "../../i18n";

export function WorkspaceLinksSection({ wsId }: { wsId: string }) {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const key = ["workspace-links", wsId];
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.getWorkspaceLinks(wsId),
    retry: false,
  });
  const save = useMutation({
    mutationFn: ({ id, linked }: { id: string; linked: boolean }) =>
      api.saveWorkspaceLink(wsId, id, linked),
    onSuccess: (value) => {
      qc.setQueryData(key, value);
    },
  });
  return (
    <SettingsSection
      title={t(($) => $.linked_workspaces.title)}
      description={t(($) => $.linked_workspaces.description)}
    >
      <p className="text-caption text-muted-foreground">
        {t(($) => $.linked_workspaces.permissions)}
      </p>
      {query.isPending && (
        <p role="status">{t(($) => $.linked_workspaces.loading)}</p>
      )}
      {query.isError && (
        <div role="alert">
          <p>{t(($) => $.linked_workspaces.load_error)}</p>
          <Button variant="outline" onClick={() => void query.refetch()}>
            {t(($) => $.linked_workspaces.retry)}
          </Button>
        </div>
      )}
      {save.isError && (
        <p role="alert">{t(($) => $.linked_workspaces.save_error)}</p>
      )}
      {query.data?.length === 0 && <p>{t(($) => $.linked_workspaces.empty)}</p>}
      <div className="divide-y">
        {query.data?.map((workspace) => (
          <div
            key={workspace.id}
            className="flex flex-wrap items-center justify-between gap-4 py-4"
          >
            <div className="min-w-0 flex-1">
              <p className="font-medium break-words">{workspace.name}</p>
              <p className="text-caption text-muted-foreground">
                {workspace.linked
                  ? t(($) => $.linked_workspaces.linked)
                  : t(($) => $.linked_workspaces.not_linked)}
              </p>
            </div>
            {workspace.can_edit ? (
              <Button
                variant="outline"
                disabled={save.isPending}
                aria-busy={
                  save.isPending && save.variables?.id === workspace.id
                }
                aria-label={`${workspace.linked ? t(($) => $.linked_workspaces.unlink) : t(($) => $.linked_workspaces.link)} ${workspace.name}`}
                onClick={() =>
                  save.mutate({ id: workspace.id, linked: !workspace.linked })
                }
              >
                {workspace.linked
                  ? t(($) => $.linked_workspaces.unlink)
                  : t(($) => $.linked_workspaces.link)}
              </Button>
            ) : (
              <span className="text-caption text-muted-foreground">
                {t(($) => $.linked_workspaces.admin_required)}
              </span>
            )}
          </div>
        ))}
      </div>
    </SettingsSection>
  );
}
