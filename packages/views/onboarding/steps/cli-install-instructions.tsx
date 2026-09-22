"use client";

import { useState } from "react";
import { Check, Copy, Terminal } from "lucide-react";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { CODE_LIGATURE_CLASS } from "@multica/ui/lib/code-style";
import { cn } from "@multica/ui/lib/utils";
import { copyText } from "@multica/ui/lib/clipboard";
import { useT } from "../../i18n";

import { HostedClientDownload, HOSTED_INSTALL_COMMAND, HOSTED_SETUP_COMMAND, useHostedClient } from "../../runtimes/components/hosted-client-install";

const INSTALL_CMD =
  "curl -fsSL https://raw.githubusercontent.com/multica-ai/multica/main/scripts/install.sh | bash";
const SETUP_CMD = "multica setup";

function CopyButton({ text }: { text: string }) {
  const { t } = useT("onboarding");
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    void copyText(text).then((ok) => {
      if (!ok) return;
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="shrink-0 rounded-xs p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      aria-label={t(($) => $.cli_install.copy_aria)}
    >
      {copied ? (
        <Check className="h-3.5 w-3.5 text-success" />
      ) : (
        <Copy className="h-3.5 w-3.5" />
      )}
    </button>
  );
}

function Step({ n, label, cmd }: { n: number; label: string; cmd: string }) {
  return (
    <div>
      <p className="mb-1.5 text-caption font-medium text-foreground">
        {n}. {label}
      </p>
      <div className="flex items-start gap-2 rounded-lg bg-muted px-3 py-2.5 font-mono text-body">
        <Terminal className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <code
          className={cn(
            "min-w-0 flex-1 whitespace-pre-wrap break-all",
            CODE_LIGATURE_CLASS,
          )}
        >
          {cmd}
        </code>
        <CopyButton text={cmd} />
      </div>
    </div>
  );
}

/** Install the client appropriate for this deployment. */
export function CliInstallInstructions() {
  const hostedClient = useHostedClient();
  const { t } = useT("onboarding");
  return (
    <Card className="w-full">
      <CardContent className="space-y-4 pt-4">
        <p className="text-caption leading-[1.55] text-muted-foreground">
          {t(($) => $.cli_install.intro)}
        </p>
        {hostedClient && <HostedClientDownload />}
        <Step n={1} label={t(($) => $.cli_install.step1_label)} cmd={hostedClient ? HOSTED_INSTALL_COMMAND : INSTALL_CMD} />
        <Step n={2} label={t(($) => $.cli_install.step2_label)} cmd={hostedClient ? HOSTED_SETUP_COMMAND : SETUP_CMD} />
      </CardContent>
    </Card>
  );
}
