"use client";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
export function WorkspaceRequestsSection({
  issueId,
  workspaceId,
}: {
  issueId: string;
  workspaceId: string;
}) {
  const { t } = useT("issues");
  const query = useQuery({
    queryKey: ["workspace-requests", workspaceId, issueId],
    queryFn: () => api.listWorkspaceRequests(issueId),
    refetchInterval: 15000,
    retry: false,
  });
  if (query.isError)
    return (
      <p role="status" className="mt-6 text-caption text-muted-foreground">
        {t(($) => $.linked_requests.load_error)}
      </p>
    );
  if (!query.data?.length) return null;
  return (
    <section className="mt-8" aria-label={t(($) => $.linked_requests.title)}>
      <h3 className="mb-2 text-body font-medium">
        {t(($) => $.linked_requests.title)}
      </h3>
      <ul className="divide-y">
        {query.data.map((request) => (
          <li key={request.id} className="py-3">
            <AppLink
              className="text-body hover:underline"
              href={`/${request.workspace_slug}/issues/${request.identifier}`}
            >
              {request.identifier} · {request.title}
            </AppLink>
            <p className="text-caption text-muted-foreground">
              {request.status.replaceAll("_", " ")}
            </p>
            {request.notification_error && (
              <p role="status" className="text-caption">
                {request.notification_error}
              </p>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
