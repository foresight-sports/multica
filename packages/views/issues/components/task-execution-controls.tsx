"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { executionOptions, useTaskExecution } from "@multica/core/agents";
import type { AgentTask } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
export function TaskExecutionControls({
  task,
  issueId,
}: {
  task: AgentTask;
  issueId: string;
}) {
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState("");
  const [fresh, setFresh] = useState(false);
  const [instruction, setInstruction] = useState("");
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const policy = useQuery({
    ...executionOptions(wsId, task.agent_id),
    enabled: open && !!task.agent_id,
  });
  const update = useTaskExecution(wsId, task.id, issueId);
  const selection = task.execution;
  const queued = task.status === "queued";
  const terminal = ["failed", "cancelled", "completed"].includes(task.status);
  return (
    <details
      className="ml-7 mb-2 text-caption text-muted-foreground"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary className="cursor-pointer truncate">
        {selection?.profile?.name
          ? selection.profile.name + " · " + selection.profile.model
          : t(($) => $.execution.run_with)}
        {selection?.state === "blocked" ? " · " + selection.reason : ""}
      </summary>
      <div className="mt-2 space-y-3 rounded-md border p-3">
        {selection && (
          <>
            <p>{selection.reason}</p>
            {selection.profile && (
              <p className="break-all">
                {selection.profile.model} · {selection.profile.runtime_id}
              </p>
            )}
            {selection.history.length > 0 && (
              <details>
                <summary className="cursor-pointer">
                  {t(($) => $.execution.history)}
                </summary>
                <ol className="mt-2 space-y-2">
                  {selection.history.map((h, i) => (
                    <li key={i}>
                      <time>{new Date(h.at).toLocaleString()}</time>
                      <p>
                        {h.model} · {h.reason}
                      </p>
                    </li>
                  ))}
                </ol>
              </details>
            )}
          </>
        )}
        {policy.error && <p role="alert">{policy.error.message}</p>}
        {(queued || terminal) && !!policy.data?.profiles.length && (
          <>
            <label className="flex flex-col gap-1">
              {t(($) => $.execution.run_with)}
              <select
                className="h-9 max-w-full rounded-md border bg-background px-2 text-foreground"
                value={choice}
                onChange={(e) => setChoice(e.target.value)}
              >
                <option value="">{t(($) => $.execution.agent_policy)}</option>
                <option value="default">
                  {t(($) => $.execution.default_mode)}
                </option>
                <option value="automatic">
                  {t(($) => $.execution.auto_mode)}
                </option>
                {policy.data.profiles.map((p) => (
                  <option key={p.id} value={"profile:" + p.id}>
                    {p.name} · {p.model}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex gap-2">
              <input
                type="checkbox"
                checked={fresh}
                onChange={(e) => setFresh(e.target.checked)}
              />
              {t(($) => $.execution.fresh)}
            </label>
            {terminal && <label className="flex flex-col gap-1">{t(($) => $.execution.instruction)}<textarea className="rounded-md border bg-background p-2 text-foreground" value={instruction} maxLength={8000} onChange={(e) => setInstruction(e.target.value)} placeholder={t(($) => $.execution.instruction_hint)} /></label>}
            <Button
              size="sm"
              disabled={update.isPending}
              onClick={() =>
                update.mutate({
                  rerun: terminal,
                  execution: {
                    ...(choice.startsWith("profile:")
                      ? { profile_id: choice.slice(8) }
                      : choice === "default" || choice === "automatic"
                        ? { mode: choice }
                        : {}),
                    fresh_session: fresh,
                    ...(terminal && instruction.trim() ? { instruction: instruction.trim() } : {}),
                  },
                })
              }
            >
              {t(($) => (terminal ? $.execution.new_run : $.execution.apply))}
            </Button>
          </>
        )}
        {update.error && (
          <p role="alert" className="text-destructive">
            {update.error.message}
          </p>
        )}
        {update.isSuccess && <p role="status">{t(($) => $.execution.saved)}</p>}
      </div>
    </details>
  );
}
