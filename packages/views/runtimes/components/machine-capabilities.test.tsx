// @vitest-environment jsdom
import {render,screen,cleanup,within} from "@testing-library/react";
import {afterEach,it,expect,vi} from "vitest";
import {I18nProvider} from "@multica/core/i18n/react";
import type {RuntimeDevice} from "@multica/core/types";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import {MachineCapabilities} from "./machine-capabilities";
vi.mock("@tanstack/react-query",()=>({queryOptions:(v:unknown)=>v,useQueryClient:()=>({}),useQueries:()=>[{data:{models:[]}},{data:{models:[]}}]}));
afterEach(cleanup);
it("keeps disabled plugins on their runtime and marks stale observations",()=>{
 const report={version:1,provider:"codex",scope:"user",status:"reported",observed_at:"2020-01-01T00:00:00Z",entries:[{kind:"plugin",name:"Design Plugin",enabled:false,callable:null,auth:"unknown"}]};
 const runtimes=[{id:"a",provider:"codex",name:"Machine A",status:"online",metadata:{runtime_capabilities:report}},{id:"b",provider:"claude",name:"Machine B",status:"online",metadata:{}}] as RuntimeDevice[];
 render(<I18nProvider locale="en" resources={{en:{common:enCommon,runtimes:enRuntimes}}}><MachineCapabilities runtimes={runtimes}/></I18nProvider>);
 const a=screen.getByText("Machine A").parentElement!.parentElement!;
 const b=screen.getByText("Machine B").parentElement!.parentElement!;
 expect(within(a).getByText("Design Plugin")).toBeTruthy();
 expect(within(a).getByText(/Disabled.*Authentication not verified/)).toBeTruthy();
 expect(within(a).getByText(/Inventory is stale/)).toBeTruthy();
 expect(within(b).queryByText("Design Plugin")).toBeNull();
 expect(within(b).getByText("Inventory unavailable")).toBeTruthy();
});
