// @vitest-environment node
import { expect,it } from "vitest";
import { RepositoryReadinessSchema } from "./repository-readiness-schema";
it("fails closed for missing or unknown readiness",()=>{
 expect(RepositoryReadinessSchema.parse({workspace_id:"ws"}).usable).toBe(false);
 expect(RepositoryReadinessSchema.parse({workspace_id:"ws",state:"unknown",usable:true}).usable).toBe(false);
 expect(RepositoryReadinessSchema.parse({workspace_id:"ws",state:"ready",usable:true}).usable).toBe(true);
 expect(RepositoryReadinessSchema.safeParse({workspace_id:"ws",usable:"yes"}).success).toBe(false);
});
