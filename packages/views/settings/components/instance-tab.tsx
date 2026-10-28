"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { instanceConfigurationOptions, instanceAgentsOptions, useUpdateInstanceConfiguration, useSaveInstanceAgent, useSetInstanceAgentEnabled, useCurrentMember, type InstanceConfiguration, type InstanceAgent } from "@multica/core/permissions";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsTab, SettingsSection, SettingsCard } from "./settings-layout";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
import { instanceMembersOptions, useSetInstanceMembership } from "@multica/core/permissions";

export function InstanceTab({ section = "general" }: { section?: "general" | "members" | "agents" }) {
  const { t } = useT("settings");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const canManage = useAuthStore((s) => s.user?.permissions?.manage_permission_access === true);
  const canCreate = useAuthStore((s) => s.user?.permissions?.create_instance_agents === true);
  const canEdit = useAuthStore((s) => s.user?.permissions?.edit_agents === true);
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const { role } = useCurrentMember(wsId);
  const config = useQuery({ ...instanceConfigurationOptions(userId), enabled: !!userId && section === "general" });
  const agents = useQuery({ ...instanceAgentsOptions(wsId, userId), enabled: !!userId && !!wsId && section === "agents" });
  const members = useQuery(instanceMembersOptions(userId, canManage && section === "members"));
  const activeQuery = section === "general" ? config : section === "agents" ? agents : members;
  const reload = () => { if (section !== "members" || canManage) void activeQuery.refetch(); };
  if (section === "members" && !canManage) return <p role="status">{t(($) => $.permission_access.denied)}</p>;
  return (
    <SettingsTab title={t(($) => section === "general" ? $.page.tabs.general : section === "members" ? $.instance.members : $.instance.agents)} description={t(($) => section === "general" ? $.instance.description : section === "members" ? $.instance.members_hint : $.instance.agents_hint)}>
      <Button variant="outline" onClick={reload}>{t(($) => $.instance.reload)}</Button>
      {activeQuery.isPending && <p role="status">{t(($) => $.instance.loading)}</p>}
      {activeQuery.error && <p role="alert" className="text-destructive">{activeQuery.error?.message}</p>}
      {!canManage && <p className="text-body text-muted-foreground">{t(($) => $.instance.read_only)}</p>}
      {section === "general" && config.data && <InstructionsEditor key={`${userId}:${config.data.revision}`} config={config.data} userId={userId} canManage={canManage} />}
      {section === "members" && canManage && <div className="space-y-4">
        {members.data?.members.map((member) => <SettingsCard key={member.id}>
          <div className="space-y-1 break-words p-4">
            <p className="font-medium">{member.name}</p><p>{member.email}</p>
            <p className="text-muted-foreground">{member.workspaces.join(", ") || t(($) => $.instance.no_workspaces)}</p>
            <InstanceMembershipEditor userId={userId} member={member} workspaces={members.data?.workspaces ?? []} />
          </div>
        </SettingsCard>)}
        {members.data?.truncated && <p>{t(($) => $.instance.members_truncated)}</p>}
      </div>}
      {section === "agents" && <div className="space-y-4">
        <p className="text-caption text-muted-foreground">{t(($) => $.instance.disable_hint)}</p>
        {agents.data?.length === 0 && <p>{t(($) => $.instance.empty)}</p>}
        {agents.data?.map((agent) => <InstanceAgentCard key={`${wsId}:${agent.id}`} agent={agent} wsId={wsId} userId={userId} canManage={canEdit && (canManage || (agent.source_workspace_id === wsId && (agent.source_owner_id === userId || role === "owner" || role === "admin")))} canEnable={canEdit && (role === "owner" || role === "admin")} />)}
        {canCreate && <AppLink className="text-body underline underline-offset-4" href={paths.newAgent()}>{t(($) => $.instance.new_agent)}</AppLink>}
        {canCreate && <p className="text-caption text-muted-foreground">{t(($) => $.instance.create_flow_hint)}</p>}
      </div>}
    </SettingsTab>
  );
}

type MembershipRole = "none" | "member" | "admin" | "owner";
function InstanceMembershipEditor({ userId, member, workspaces }: {
  userId: string;
  member: { id: string; name: string; memberships?: { workspace_id: string; role: "member" | "admin" | "owner" }[] };
  workspaces: { id: string; name: string; slug: string }[];
}) {
  const { t } = useT("settings");
  const [editing, setEditing] = useState(false);
  return <div className="space-y-3 pt-3">
    <Button variant="outline" onClick={() => setEditing(!editing)}>{t(($) => editing ? $.instance.cancel : $.instance.edit_memberships)}</Button>
    {editing && <>
      <p className="text-caption text-muted-foreground">{t(($) => $.instance.membership_warning)}</p>
      {workspaces.map((ws) => {
        const role = member.memberships?.find((m) => m.workspace_id === ws.id)?.role ?? "none";
        return <MembershipRow key={`${ws.id}:${role}`} userId={userId} memberId={member.id} workspace={ws} currentRole={role} />;
      })}
    </>}
  </div>;
}

function MembershipRow({ userId, memberId, workspace, currentRole }: {
  userId: string; memberId: string; workspace: { id: string; name: string; slug: string }; currentRole: MembershipRole;
}) {
  const { t } = useT("settings");
  const [role, setRole] = useState<MembershipRole>(currentRole);
  const save = useSetInstanceMembership(userId);
  const id = `membership-${memberId}-${workspace.id}`;
  return <form className="flex flex-wrap items-center gap-3 rounded-md border p-3" onSubmit={(event) => {
    event.preventDefault();
    if (role === currentRole || save.isPending) return;
    save.mutate({ memberId, workspaceId: workspace.id, role }, { onSuccess: () => toast.success(t(($) => $.instance.saved)) });
  }}>
    <label htmlFor={id}>{workspace.name} <span className="text-muted-foreground">({workspace.slug})</span></label>
    <select id={id} value={role} disabled={save.isPending} className="rounded-md border bg-background p-2 text-body" onChange={(event) => {
      const value = event.target.value;
      if (value === "none" || value === "member" || value === "admin" || value === "owner") setRole(value);
    }}>
      <option value="none">{t(($) => $.instance.role_none)}</option>
      <option value="member">{t(($) => $.instance.role_member)}</option>
      <option value="admin">{t(($) => $.instance.role_admin)}</option>
      <option value="owner">{t(($) => $.instance.role_owner)}</option>
    </select>
    <Button type="submit" disabled={save.isPending || role === currentRole}>{t(($) => save.isPending ? $.instance.saving : $.instance.save)}</Button>
    {save.error && <p role="alert" className="w-full text-destructive">{save.error.message}</p>}
  </form>;
}

function InstructionsEditor({ config, userId, canManage }: { config: InstanceConfiguration; userId: string; canManage: boolean }) {
  const { t } = useT("settings");
  const [instructions, setInstructions] = useState(config.instructions);
  const save = useUpdateInstanceConfiguration(userId);
  return <SettingsSection title={t(($) => $.instance.instructions)} description={t(($) => $.instance.instructions_hint)}>
    <SettingsCard>
      <form className="space-y-4 p-5" onSubmit={(e) => { e.preventDefault(); save.mutate({ instructions, revision: config.revision }, { onSuccess: () => toast.success(t(($) => $.instance.saved)) }); }}>
        <label htmlFor="instance-instructions" className="text-body font-medium">{t(($) => $.instance.instructions)}</label>
        <Textarea id="instance-instructions" value={instructions} onChange={(e) => setInstructions(e.target.value)} rows={9} maxLength={100000} readOnly={!canManage} disabled={save.isPending} />
        {save.error && <p role="alert" className="text-destructive">{save.error.message}</p>}
        {canManage && <Button type="submit" disabled={save.isPending || instructions === config.instructions}>{t(($) => save.isPending ? $.instance.saving : $.instance.save)}</Button>}
      </form>
    </SettingsCard>
  </SettingsSection>;
}

function InstanceAgentCard({ agent, wsId, userId, canManage, canEnable }: { agent: InstanceAgent; wsId: string; userId: string; canManage: boolean; canEnable: boolean }) {
  const { t } = useT("settings");
  const paths = useWorkspacePaths();
  const [editing, setEditing] = useState(false);
  const toggle = useSetInstanceAgentEnabled(wsId, userId);
  return <SettingsCard>
    <div className="space-y-3 p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0"><h3 className="break-words text-body font-medium">{agent.name}</h3><p className="break-words text-body text-muted-foreground">{agent.description}</p></div>
        <span className="text-caption">{t(($) => agent.enabled ? $.instance.enabled : $.instance.disabled)}</span>
      </div>
      {agent.enabled && !agent.runtime_bound && <p className="text-caption text-muted-foreground">{t(($) => $.instance.runtime_required)}</p>}
      <div className="flex flex-wrap items-center gap-3">
        {canEnable && <Button variant="outline" disabled={toggle.isPending} onClick={() => toggle.mutate({ id: agent.agent_id, enabled: !agent.enabled })}>{t(($) => agent.enabled ? $.instance.disable : $.instance.enable)}</Button>}
        {canManage && <Button variant="outline" onClick={() => setEditing(!editing)}>{t(($) => $.instance.edit)}</Button>}
        {canManage && agent.source_workspace_id === wsId && agent.source_agent_id && <AppLink className="text-body underline underline-offset-4" href={paths.agentDetail(agent.source_agent_id)}>{t(($) => $.instance.configure)}</AppLink>}
      </div>
      {toggle.error && <p role="alert" className="text-destructive">{toggle.error.message}</p>}
      {editing && <AgentEditor key={`${agent.id}:${agent.revision}`} agent={agent} onClose={() => setEditing(false)} />}
    </div>
  </SettingsCard>;
}

function AgentEditor({ agent, onClose }: { agent: InstanceAgent; onClose: () => void }) {
  const { t } = useT("settings");
  const [name, setName] = useState(agent?.name ?? "");
  const [description, setDescription] = useState(agent?.description ?? "");
  const [instructions, setInstructions] = useState(agent?.instructions ?? "");
  const save = useSaveInstanceAgent();
  const prefix = agent?.id ?? "new-instance-agent";
  return <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate({ id: agent?.id, data: { name, description, instructions, revision: agent?.revision ?? 0 } }, { onSuccess: () => { toast.success(t(($) => $.instance.saved)); onClose(); } }); }}>
    <div className="space-y-2"><label htmlFor={`${prefix}-name`}>{t(($) => $.instance.name)}</label><Input id={`${prefix}-name`} value={name} onChange={(e) => setName(e.target.value)} maxLength={100} required disabled={save.isPending} /></div>
    <div className="space-y-2"><label htmlFor={`${prefix}-description`}>{t(($) => $.instance.description_label)}</label><Textarea id={`${prefix}-description`} value={description} onChange={(e) => setDescription(e.target.value)} maxLength={255} rows={2} disabled={save.isPending} /></div>
    <div className="space-y-2"><label htmlFor={`${prefix}-instructions`}>{t(($) => $.instance.agent_instructions)}</label><Textarea id={`${prefix}-instructions`} value={instructions} onChange={(e) => setInstructions(e.target.value)} maxLength={100000} rows={6} disabled={save.isPending} /></div>
    {save.error && <p role="alert" className="text-destructive">{save.error.message}</p>}
    <div className="flex gap-3"><Button type="submit" disabled={save.isPending || !name.trim()}>{t(($) => save.isPending ? $.instance.saving : $.instance.save)}</Button><Button type="button" variant="outline" disabled={save.isPending} onClick={onClose}>{t(($) => $.instance.cancel)}</Button></div>
  </form>;
}
