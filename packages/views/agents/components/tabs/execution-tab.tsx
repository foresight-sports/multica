"use client";
import {
  useEffect,
  useState,
  Children,
  isValidElement,
  type ReactNode,
} from "react";
import { useQuery } from "@tanstack/react-query";
import type { Agent, AgentRuntime, MemberWithUser } from "@multica/core/types";
import type { ExecutionPolicy, ExecutionProfile } from "@multica/core/api";
import {
  executionOptions,
  useSaveExecution,
  usePreviewExecution,
} from "@multica/core/agents";

import { useWorkspaceId } from "@multica/core/hooks";
import { createSafeId } from "@multica/core/utils";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { PortableModelFields } from "../portable-model-fields";

import { SettingsCard } from "../../../settings/components/settings-layout";
import { useAuthStore } from "@multica/core/auth";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Plus, Trash2, Loader2 } from "lucide-react";
import { useT } from "../../../i18n";

function Choice({
  label,
  value,
  onChange,
  children,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  children: ReactNode;
}) {
  const items = Children.toArray(children)
    .filter(isValidElement<{ value: string | number; children: ReactNode }>)
    .map((child) => ({
      value: String(child.props.value),
      label: child.props.children,
    }));
  return (
    <div className="flex min-w-0 flex-col gap-1.5 text-caption text-muted-foreground">
      <span>{label}</span>
      <Select
        value={value}
        items={items}
        onValueChange={(v) => v !== null && onChange(v)}
      >
        <SelectTrigger aria-label={label} className="w-full text-foreground">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
export function ExecutionTab({
  agent,
  runtimes,
  onDirtyChange,
}: {
  agent: Agent;
  runtimes: AgentRuntime[];
  onDirtyChange?: (v: boolean) => void;
}) {
  const wsId = useWorkspaceId();
  const currentUserId = useAuthStore((state) => state.user?.id ?? null);
  const { t } = useT("agents");
  const q = useQuery(executionOptions(wsId, agent.id));
  const save = useSaveExecution(wsId, agent.id);
  const preview = usePreviewExecution(agent.id);
  const [draft, setDraft] = useState<ExecutionPolicy | null>(null);
  const [prompt, setPrompt] = useState("");
  const policy = draft ?? q.data;
  const dirty =
    draft !== null && JSON.stringify(draft) !== JSON.stringify(q.data);
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  if (q.isPending) return <Loader2 className="size-4 animate-spin" />;
  if (q.error || !policy) return <p role="alert">{q.error?.message}</p>;
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <ExecutionPolicyEditor
        policy={policy}
        onChange={setDraft}
        runtimes={runtimes}
        runtimeId={agent.runtime_id ?? ""}
        currentUserId={currentUserId}
      />
      {save.error && (
        <p role="alert" className="text-body text-destructive">
          {save.error.message}
        </p>
      )}
      <div className="flex items-center gap-3">
        <Button
          disabled={!dirty || save.isPending}
          onClick={() =>
            save.mutate(policy, { onSuccess: () => setDraft(null) })
          }
        >
          {save.isPending && <Loader2 className="size-4 animate-spin" />}
          {t(($) => $.execution.save)}
        </Button>
        {save.isSuccess && !dirty && (
          <span role="status" className="text-caption text-muted-foreground">
            {t(($) => $.execution.saved)}
          </span>
        )}
      </div>
      <section className="space-y-3 border-t pt-5">
        <h3 className="font-medium">{t(($) => $.execution.preview)}</h3>
        <p className="text-caption text-muted-foreground">
          {t(($) => $.execution.preview_help)}
        </p>
        <Textarea
          aria-label={t(($) => $.execution.task)}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder={t(($) => $.execution.task)}
        />
        <Button
          variant="outline"
          disabled={dirty || !prompt.trim() || preview.isPending}
          onClick={() => preview.mutate(prompt)}
        >
          {t(($) => $.execution.preview)}
        </Button>
        {preview.error && <p role="alert">{preview.error.message}</p>}
        {preview.data && (
          <div aria-live="polite" className="space-y-2 text-body">
            <p>{preview.data.blocked || preview.data.reason}</p>
            {preview.data.uses_router && (
              <p className="text-muted-foreground">
                {t(($) => $.execution.preview_router)}
              </p>
            )}
            {preview.data.candidates.map((c) => (
              <div
                key={`${c.profile.id}:${c.profile.runtime_id}`}
                className="flex flex-wrap justify-between gap-2 rounded-md bg-muted/40 p-3"
              >
                <span>
                  {c.profile.name} · {c.profile.model} ·{" "}
                  {runtimes.find((r) => r.id === c.profile.runtime_id)?.name ??
                    c.profile.runtime_id}
                </span>
                <span
                  className={
                    c.eligible ? "text-success" : "text-muted-foreground"
                  }
                >
                  {c.eligible ? t(($) => $.execution.eligible) : c.reason}
                </span>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
function ProfileEditor({
  profile: p,
  runtimes,
  onChange,
  onRemove,
  currentUserId = null,
}: {
  profile: ExecutionProfile;
  runtimes: AgentRuntime[];
  onChange: (p: ExecutionProfile) => void;
  onRemove?: () => void;
  members?: MemberWithUser[];
  currentUserId?: string | null;
  executionControls?: ReactNode;
}) {
  const { t } = useT("agents");

  const patch = (v: Partial<ExecutionProfile>) => onChange({ ...p, ...v });
  return (
    <SettingsCard>
      <div className="flex gap-2 px-4 py-3">
        <Input
          aria-label={t(($) => $.execution.name)}
          placeholder={t(($) => $.execution.name)}
          value={p.name}
          maxLength={100}
          onChange={(e) => patch({ name: e.target.value })}
        />
        {onRemove && (
          <Button
            variant="ghost"
            size="icon"
            aria-label={t(($) => $.execution.remove)}
            onClick={onRemove}
          >
            <Trash2 className="size-4" />
          </Button>
        )}
      </div>
      <PortableModelFields
        profile={p}
        runtimes={runtimes.filter(
          (r) => r.visibility === "public" || r.owner_id === currentUserId,
        )}
        onChange={patch}
      />
      <label className="block space-y-1.5 px-4 py-4 text-caption text-muted-foreground">
        {t(($) => $.execution.purpose)}
        <Textarea
          value={p.purpose}
          maxLength={2000}
          onChange={(e) => patch({ purpose: e.target.value })}
        />
      </label>
      <div className="space-y-4 border-t px-4 py-4 text-caption">
        <Choice
          label={t(($) => $.execution.os)}
          value={p.required_os}
          onChange={(v) => patch({ required_os: v })}
        >
          <option value="">{t(($) => $.execution.any)}</option>
          {["windows", "linux", "darwin"].map((v) => (
            <option key={v} value={v}>
              {v}
            </option>
          ))}
        </Choice>
        <label className="block space-y-1.5">
          {t(($) => $.execution.tools)}
          <Input
            value={p.required_tools.join(",")}
            onChange={(e) =>
              patch({
                required_tools: e.target.value
                  ? e.target.value.split(",").map((v) => v.trim())
                  : [],
              })
            }
          />
        </label>
      </div>
      <details className="px-4 py-4 text-caption">
        <summary className="cursor-pointer text-muted-foreground">
          {t(($) => $.execution.advanced)}
        </summary>
        <div className="mt-4 space-y-4">
          <label className="block space-y-1.5">
            {t(($) => $.execution.keywords)}
            <Input
              value={p.keywords.join(",")}
              onChange={(e) => patch({ keywords: e.target.value.split(",") })}
            />
          </label>
          <div className="grid grid-cols-3 gap-3">
            {(["quality", "speed", "cost"] as const).map((k) => (
              <Choice
                key={k}
                label={t(($) => $.execution[k])}
                value={String(p[k])}
                onChange={(v) => patch({ [k]: Number(v) })}
              >
                {[1, 2, 3, 4, 5].map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </Choice>
            ))}
          </div>
          <p className="text-muted-foreground">
            {t(($) => $.execution.ratings_help)}
          </p>
        </div>
      </details>
    </SettingsCard>
  );
}

export function ExecutionPolicyEditor({
  policy,
  onChange,
  runtimes,
  runtimeId,
  seed,
  members = [],
  currentUserId = null,
  creationPrimaryExecution,
}: {
  policy: ExecutionPolicy;
  onChange: (p: ExecutionPolicy) => void;
  runtimes: AgentRuntime[];
  runtimeId: string;
  seed?: Partial<ExecutionProfile>;
  members?: MemberWithUser[];
  currentUserId?: string | null;
  creationPrimaryExecution?: ReactNode;
}) {
  const { t } = useT("agents");
  const patch = (p: Partial<ExecutionPolicy>) => onChange({ ...policy, ...p });
  const choices = policy.profiles.map((p) => (
    <option key={p.id} value={p.id}>
      {p.name || p.model}
    </option>
  ));
  const add = () => {
    const id = createSafeId();
    const profile: ExecutionProfile = {
      id,
      name: "",
      runtime_id: "",
      provider:
        runtimes.find((r) => r.id === runtimeId)?.provider ??
        runtimes[0]?.provider ??
        "",
      model: seed?.model ?? "",
      thinking_level: seed?.thinking_level ?? "",
      service_tier: seed?.service_tier ?? "",
      purpose: "",
      keywords: [],
      required_os: "",
      required_tools: [],
      quality: 3,
      speed: 3,
      cost: 3,
    };
    patch({
      profiles: [...policy.profiles, profile],
      default_profile: policy.default_profile || id,
    });
  };
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      {!creationPrimaryExecution && (
        <p className="text-body text-muted-foreground">
          {t(($) => $.execution.intro)}
        </p>
      )}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <span />
          <Button
            variant="outline"
            size="sm"
            onClick={add}
            disabled={policy.profiles.length >= 24}
          >
            <Plus className="size-4" />
            {t(($) => $.execution.add)}
          </Button>
        </div>
        {policy.profiles.length === 0 && (
          <p className="rounded-lg border border-dashed p-6 text-body text-muted-foreground">
            {t(($) => $.execution.empty)}
          </p>
        )}
        {policy.profiles.map((p, index) => (
          <ProfileEditor
            key={p.id}
            profile={p}
            runtimes={runtimes}
            members={members}
            currentUserId={currentUserId}
            executionControls={
              index === 0 ? creationPrimaryExecution : undefined
            }
            onChange={(v) =>
              patch({
                profiles: policy.profiles.map((x) => (x.id === p.id ? v : x)),
              })
            }
            onRemove={
              policy.profiles.length === 1 ||
              (creationPrimaryExecution && index === 0)
                ? undefined
                : () => {
                    const profiles = policy.profiles.filter(
                      (x) => x.id !== p.id,
                    );
                    patch({
                      profiles,
                      default_profile:
                        policy.default_profile === p.id
                          ? (profiles[0]?.id ?? "")
                          : policy.default_profile,
                      router_profile:
                        policy.router_profile === p.id
                          ? ""
                          : policy.router_profile,
                    });
                  }
            }
          />
        ))}
      </div>
      <div className="grid gap-4 rounded-lg border p-4 sm:grid-cols-2">
        <Choice
          label={t(($) => $.execution.mode)}
          value={policy.mode}
          onChange={(v) =>
            patch({ mode: v === "automatic" ? "automatic" : "default" })
          }
        >
          <option value="default">{t(($) => $.execution.default_mode)}</option>
          <option value="automatic">{t(($) => $.execution.auto_mode)}</option>
        </Choice>
        <Choice
          label={t(($) => $.execution.default_profile)}
          value={policy.default_profile}
          onChange={(v) => patch({ default_profile: v })}
        >
          <option value="">—</option>
          {choices}
        </Choice>
        {policy.mode === "automatic" && (
          <>
            <Choice
              label={t(($) => $.execution.router)}
              value={policy.router_profile}
              onChange={(v) => patch({ router_profile: v })}
            >
              <option value="">{t(($) => $.execution.rules)}</option>
              {choices}
            </Choice>
            <Choice
              label={t(($) => $.execution.preference)}
              value={policy.preference}
              onChange={(v) =>
                patch({ preference: v as ExecutionPolicy["preference"] })
              }
            >
              {(["balanced", "quality", "speed", "cost"] as const).map((k) => (
                <option key={k} value={k}>
                  {t(($) => $.execution[k])}
                </option>
              ))}
            </Choice>
            <p className="text-caption text-muted-foreground sm:col-span-2">
              {t(($) => $.execution.router_help)}
            </p>
          </>
        )}
        <label className="flex items-start gap-2 text-body sm:col-span-2">
          <Checkbox
            checked={policy.allow_fallback}
            onCheckedChange={(checked) =>
              patch({ allow_fallback: checked === true })
            }
          />
          {t(($) => $.execution.fallback)}
        </label>
      </div>
    </div>
  );
}
