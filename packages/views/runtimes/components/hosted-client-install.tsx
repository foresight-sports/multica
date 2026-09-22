"use client";

import { useConfigStore } from "@multica/core/config";
import { useT } from "../../i18n";

const HOSTED_ORIGIN = "https://multica.edgeofglory.dev";
export const HOSTED_INSTALL_COMMAND = String.raw`& { $ErrorActionPreference = 'Stop'; Set-ExecutionPolicy Bypass -Scope Process -Force; $id = Read-Host 'Cloudflare Client ID'; $secret = Read-Host 'Cloudflare Client Secret' -AsSecureString; $headers = @{ 'CF-Access-Client-Id' = $id.Trim(); 'CF-Access-Client-Secret' = [Net.NetworkCredential]::new('', $secret).Password.Trim() }; try { $script = (Invoke-WebRequest -UseBasicParsing -MaximumRedirection 0 -Headers $headers '${HOSTED_ORIGIN}/download/install.ps1').Content; if ($script -is [byte[]]) { $script = [Text.Encoding]::UTF8.GetString($script) }; if (!$script.StartsWith('# Multica Cloudflare installer')) { throw 'Unexpected installer response. Check Cloudflare Service Auth.' }; & ([scriptblock]::Create($script)) -ClientId $id -ClientSecret $secret } finally { $headers.Clear() } }`;
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
