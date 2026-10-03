// @vitest-environment node
import {describe,it,expect} from "vitest";
import {parseMachineCapabilities} from "./capabilities";
describe("runtime capability inventory",()=>{
 it("preserves tools while rejecting malformed plugin data",()=>{
  const data=parseMachineCapabilities({tools:["git"],runtime_capabilities:{version:99,entries:[{password:"secret"}]}});
  expect(data.tools).toEqual(["git"]);expect(data.runtime_capabilities).toBeNull();
 });
 it("preserves unknown authentication and strips unexpected fields",()=>{
  const data=parseMachineCapabilities({runtime_capabilities:{version:1,provider:"codex",scope:"user",status:"reported",observed_at:"2026-10-02T00:00:00Z",entries:[{kind:"plugin",name:"test",enabled:false,callable:null,auth:"unknown",token:"secret"}]}});
  expect(data.runtime_capabilities?.entries[0]).toEqual({kind:"plugin",name:"test",enabled:false,callable:null,auth:"unknown"});
 });
});
