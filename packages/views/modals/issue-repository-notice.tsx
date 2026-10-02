"use client";
import { useRepositorySettings } from "@multica/core/workspace";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { AppLink } from "../navigation";
import { useT } from "../i18n";
export function useIssueRepositoryGate() {
 const wsId=useWorkspaceId();
 const {query}=useRepositorySettings(wsId);
 return {query,blocked:query.isPending || query.isError || !(query.data?.repository || query.data?.folder)};
}
export function IssueRepositoryNotice({onConfigure}:{onConfigure?:()=>void}) {
 const {query,blocked}=useIssueRepositoryGate();
 const paths=useWorkspacePaths();
 const {t}=useT("settings");
 if(!blocked)return null;
 return <div role="status" className="mx-5 my-2 space-y-2 rounded-md border bg-muted/40 p-3 text-caption">
  <p>{query.isError?t(($)=>$.repository.load_error):query.isPending?t(($)=>$.repository.loading):t(($)=>$.repository.required)}</p>
  {query.isError?<Button variant="outline" size="sm" onClick={()=>void query.refetch()}>{t(($)=>$.repository.reload)}</Button>:!query.isPending && (query.data?.can_edit?<AppLink className="text-info underline" href={paths.settings()} onClick={onConfigure}>{t(($)=>$.repository.configure)}</AppLink>:<p>{t(($)=>$.repository.admin_only)}</p>)}
 </div>;
}
