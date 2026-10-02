"use client";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import {
  runtimeModelsOptions,
  refreshRuntimeModels,
} from "@multica/core/runtimes";
import type { ExecutionProfile } from "@multica/core/api";
import type { RuntimeDevice } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";

function Field({
  label,
  value,
  items,
  onChange,
}: {
  label: string;
  value: string;
  items: { value: string; label: string }[];
  onChange: (v: string) => void;
}) {
  const choices = items.some((i) => i.value === value)
    ? items
    : [{ value, label: value }, ...items];
  return (
    <div className="space-y-1.5">
      <span className="text-caption text-muted-foreground">{label}</span>
      <Select
        value={value}
        items={choices}
        onValueChange={(v) => v !== null && onChange(v)}
      >
        <SelectTrigger className="w-full" aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {choices.map((i) => (
            <SelectItem key={i.value} value={i.value}>
              {i.label || "—"}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
export function PortableModelFields({
  profile,
  runtimes,
  onChange,
}: {
  profile: ExecutionProfile;
  runtimes: RuntimeDevice[];
  onChange: (value: Partial<ExecutionProfile>) => void;
}) {
  const { t } = useT("agents");
  const qc = useQueryClient();
  const hosts = runtimes.filter((r) => r.provider === profile.provider);
  const queries = useQueries({
    queries: hosts.map((r) =>
      runtimeModelsOptions(r.status === "online" ? r.id : null),
    ),
  });
  const models = queries.flatMap((q) => q.data?.models ?? []);
  const variants = models.filter((m) => m.id === profile.model);
  const unique = (items: { value: string; label: string }[]) =>
    Array.from(new Map(items.map((i) => [i.value, i])).values());
  const defaultItem = {
    value: "",
    label: t(($) => $.execution.provider_default),
  };
  const matched = queries.filter((q) =>
    q.data?.models.some(
      (m) =>
        m.id === profile.model &&
        (!profile.thinking_level ||
          m.thinking?.supported_levels.some(
            (l) => l.value === profile.thinking_level,
          )) &&
        (!profile.service_tier ||
          m.service_tiers?.some((t) => t.id === profile.service_tier) ||
          (profile.service_tier === "default" &&
            m.supports_explicit_standard_service_tier)),
    ),
  ).length;
  return (
    <div className="space-y-3 px-4 py-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label={t(($) => $.execution.provider)}
          value={profile.provider}
          items={unique(
            runtimes.map((r) => ({ value: r.provider, label: r.provider })),
          )}
          onChange={(provider) =>
            onChange({
              provider,
              runtime_id: "",
              model: "",
              thinking_level: "",
              service_tier: "",
            })
          }
        />
        <Field
          label={t(($) => $.execution.model)}
          value={profile.model}
          items={unique(models.map((m) => ({ value: m.id, label: m.label })))}
          onChange={(model) =>
            onChange({ model, thinking_level: "", service_tier: "" })
          }
        />
        <Field
          label={t(($) => $.creation_studio.thinking_label)}
          value={profile.thinking_level}
          items={[
            defaultItem,
            ...unique(
              variants.flatMap(
                (m) =>
                  m.thinking?.supported_levels.map((l) => ({
                    value: l.value,
                    label: l.label,
                  })) ?? [],
              ),
            ),
          ]}
          onChange={(thinking_level) => onChange({ thinking_level })}
        />
        <Field
          label={t(($) => $.execution.tier)}
          value={profile.service_tier}
          items={[
            defaultItem,
            ...unique(
              variants.flatMap((m) => [
                ...(m.service_tiers?.map((t) => ({
                  value: t.id,
                  label: t.name,
                })) ?? []),
                ...(m.supports_explicit_standard_service_tier
                  ? [
                      {
                        value: "default",
                        label: t(($) => $.execution.standard),
                      },
                    ]
                  : []),
              ]),
            ),
          ]}
          onChange={(service_tier) => onChange({ service_tier })}
        />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 text-caption text-muted-foreground">
        <span>{t(($) => $.execution.matching_models, { count: matched })}</span>
        <Button
          variant="ghost"
          size="sm"
          disabled={queries.some((q) => q.isFetching)}
          onClick={() =>
            void Promise.allSettled(
              hosts
                .filter((r) => r.status === "online")
                .map((r) => refreshRuntimeModels(qc, r.id)),
            )
          }
        >
          {t(($) => $.execution.refresh_catalogs)}
        </Button>
      </div>
      {queries.some((q) => q.isError) && (
        <p role="status" className="text-caption text-destructive">
          {t(($) => $.execution.catalog_error)}
        </p>
      )}
    </div>
  );
}
