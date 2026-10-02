-- Preserve the configured model (or the machine's advertised default) when
-- upgrading user agents that predate profiles. Never guess an unknown model.
WITH legacy AS (
 SELECT a.id,r.provider,COALESCE(NULLIF(a.model,''),(
   SELECT m->>'id' FROM jsonb_array_elements(COALESCE(r.metadata->'execution_models','[]')) m
   WHERE m->>'default'='true' LIMIT 1)) AS model,
   a.thinking_level,a.service_tier
 FROM agent a JOIN agent_runtime r ON r.id=a.runtime_id
 WHERE a.kind='user' AND COALESCE(jsonb_array_length(a.execution_policy->'profiles'),0)=0
 AND (a.instance_source_agent_id IS NULL OR a.instance_source_agent_id=a.id)
)
UPDATE agent a SET execution_policy=jsonb_build_object(
 'revision',1,'mode','default','preference','balanced','default_profile','default',
 'router_profile','','allow_fallback',false,'profiles',jsonb_build_array(jsonb_build_object(
 'id','default','name','Default profile','provider',legacy.provider,'model',legacy.model,
 'thinking_level',COALESCE(legacy.thinking_level,''),'service_tier',COALESCE(legacy.service_tier,''),
 'purpose','','keywords','[]'::jsonb,'required_os','','required_tools','[]'::jsonb,
 'quality',3,'speed',3,'cost',3)))
FROM legacy WHERE a.id=legacy.id AND NULLIF(legacy.model,'') IS NOT NULL;
