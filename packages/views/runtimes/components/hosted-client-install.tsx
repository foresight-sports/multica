"use client";

import { useConfigStore } from "@multica/core/config";
import { useT } from "../../i18n";

const HOSTED_ORIGIN = "https://multica.edgeofglory.dev";
export const HOSTED_INSTALL_COMMAND =
  "powershell -ExecutionPolicy Bypass -File .\\install-cloudflare-client.ps1";
export const HOSTED_SETUP_COMMAND =
  `multica setup self-host --server-url ${HOSTED_ORIGIN} --app-url ${HOSTED_ORIGIN}`;

export function useHostedClient() {
  return useConfigStore(
    (s) => s.daemonAppUrl?.replace(/\/+$/, "") === HOSTED_ORIGIN,
  );
}

export function HostedClientDownload() {
  const { t } = useT("runtimes");
  return (
    <div className="space-y-2 text-caption">
      <a
        href={`${HOSTED_ORIGIN}/download/multica-cloudflare-windows-amd64.zip`}
        download
        className="font-medium text-primary underline underline-offset-4"
      >
        {t(($) => $.hosted_client.download)}
      </a>
      <p className="text-muted-foreground">
        {t(($) => $.hosted_client.instructions)}
      </p>
    </div>
  );
}
