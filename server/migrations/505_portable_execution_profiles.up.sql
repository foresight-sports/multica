-- Convert saved profiles to provider/model requirements. Runtime IDs belong to
-- task selections only; historical and running selections remain untouched.
UPDATE agent a SET execution_policy=jsonb_set(a.execution_policy,'{profiles}',(
 SELECT jsonb_agg((p - 'runtime_id') || jsonb_build_object('provider',COALESCE(NULLIF(p->>'provider',''),r.provider,'')) ORDER BY ordinal)
 FROM jsonb_array_elements(a.execution_policy->'profiles') WITH ORDINALITY AS item(p,ordinal)
 LEFT JOIN agent_runtime r ON r.id::text=p->>'runtime_id'
)) WHERE jsonb_array_length(COALESCE(a.execution_policy->'profiles','[]'))>0;

CREATE OR REPLACE FUNCTION agent_allows_runtime(p_agent_id UUID,p_runtime_id UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
 SELECT EXISTS (
 SELECT 1 FROM agent a JOIN agent source ON source.id=COALESCE(a.instance_source_agent_id,a.id)
 JOIN agent_runtime r ON r.id=p_runtime_id
 WHERE a.id=p_agent_id AND a.archived_at IS NULL AND source.archived_at IS NULL
 AND (a.instance_agent_id IS NULL OR EXISTS (SELECT 1 FROM instance_agent i
 WHERE i.id=a.instance_agent_id AND i.source_agent_id=source.id AND source.instance_agent_id=i.id))
 AND r.workspace_id IN (a.workspace_id,source.workspace_id)
 AND (r.visibility='public' OR r.owner_id=source.owner_id OR
  (COALESCE(jsonb_array_length(source.execution_policy->'profiles'),0)=0 AND (source.owner_id IS NULL OR r.owner_id IS NULL)))
 AND ((COALESCE(jsonb_array_length(source.execution_policy->'profiles'),0)=0 AND a.runtime_id=r.id)
 OR EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(source.execution_policy->'profiles','[]')) p WHERE p->>'provider'=r.provider))
 )
$$;
