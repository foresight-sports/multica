"use client";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { Loader2, CheckCircle2, AlertTriangle } from "lucide-react";
import { useT } from "../../i18n";
export function WorkspaceReadiness({runtimeId}:{runtimeId:string}) {
 const wsId=useWorkspaceId();
 const {t}=useT("runtimes");
 const query=useQuery({queryKey:["repository-readiness",wsId,runtimeId],queryFn:()=>api.getRepositoryReadiness(wsId,runtimeId),refetchInterval:5000,retry:false});
 const data=query.data;
 const preparing=!query.isError && (!data || ["preparing","cloning"].includes(data.state));
 const Icon=preparing?Loader2:data?.usable?CheckCircle2:AlertTriangle;
 return <span className="mt-1 flex items-start gap-1.5 text-caption text-muted-foreground" title={data?.message}>
  <Icon aria-hidden="true" className={`mt-0.5 size-3 shrink-0 ${preparing?"animate-spin":""}`}/>
  <span>{query.isError?t(($)=>$.repository_readiness.error):data?.message || t(($)=>$.repository_readiness.checking)}</span>
 </span>;
}
