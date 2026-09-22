"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useConfigStore } from "@multica/core/config";
import { useAuthStore } from "@multica/core/auth";
import { runtimeInstallationOptions } from "@multica/core/permissions";
import { copyText } from "@multica/ui/lib/clipboard";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

const HOSTED_ORIGIN = "https://multica.edgeofglory.dev";
export const HOSTED_SETUP_COMMAND =
  `multica setup self-host --server-url ${HOSTED_ORIGIN} --app-url ${HOSTED_ORIGIN}`;

export function useHostedClient() {
  return useConfigStore(
    (s) => s.daemonAppUrl?.replace(/\/+$/, "") === HOSTED_ORIGIN,
  );
}

export function HostedClientInstall() {
  const { t } = useT("runtimes");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const allowed = useAuthStore((s) => s.user?.permissions?.register_runtimes === true);
  const query = useQuery(runtimeInstallationOptions(userId, allowed));
  const [copied, setCopied] = useState(false);
  if (!allowed) return <p role="status">{t(($) => $.hosted_client.denied)}</p>;
  return (
    <div className="space-y-2 text-caption">
      <p className="font-medium">1. {t(($) => $.connect.step1_label)}</p>
      <p className="text-muted-foreground">{t(($) => $.hosted_client.instructions)}</p>
      {(query.isPending || query.isFetching) && <p role="status">{t(($) => $.hosted_client.loading)}</p>}
      {query.isError && <div role="alert"><p>{t(($) => $.hosted_client.unavailable)}</p><Button variant="outline" size="sm" onClick={() => void query.refetch()}>{t(($) => $.hosted_client.retry)}</Button></div>}
      {query.data && !query.isError && !query.isFetching && (
        <div className="space-y-2 rounded-lg bg-muted p-3">
          <code className="block max-h-48 overflow-y-auto break-all whitespace-pre-wrap font-mono text-body">{query.data.command}</code>
          <Button variant="outline" size="sm" onClick={() => void copyText(query.data.command).then(setCopied)}>
            {copied ? t(($) => $.hosted_client.copied) : t(($) => $.connect.copy_aria)}
          </Button>
        </div>
      )}
    </div>
  );
}