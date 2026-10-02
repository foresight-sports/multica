"use client";

import { useAuthStore } from "@multica/core/auth";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { useT } from "../../i18n";

export function InstanceAgentScopeField({ checked, onChange }: {
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  const { t } = useT("settings");
  const canManage = useAuthStore((s) => s.user?.permissions?.create_instance_agents === true);
  // A restored draft remains visible if permission was revoked, so its owner
  // can turn instance scope off and continue creating a workspace agent.
  if (!canManage && !checked) return null;
  return <div className="space-y-2 px-4 py-4">
    <label className="flex items-center gap-2 text-body font-medium">
      <Checkbox checked={checked} onCheckedChange={(value) => onChange(value === true)} />
      {t(($) => $.instance.create_scope)}
    </label>
    <p className="text-caption text-muted-foreground">{t(($) => $.instance.create_scope_hint)}</p>
  </div>;
}
