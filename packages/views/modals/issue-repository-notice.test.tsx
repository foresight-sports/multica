// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import en from "../locales/en/settings.json";
import { IssueRepositoryNotice } from "./issue-repository-notice";
const state=vi.hoisted(()=>({query:{data:{repository:"",folder:"",can_edit:true},isPending:false,isError:false,refetch:vi.fn()}}));
vi.mock("@multica/core/workspace",()=>({useRepositorySettings:()=>state}));
vi.mock("@multica/core/hooks",()=>({useWorkspaceId:()=>"ws"}));
vi.mock("@multica/core/paths",()=>({useWorkspacePaths:()=>({settings:()=>"/ws/settings"})}));
vi.mock("../navigation",()=>({AppLink:({href,children}:{href:string;children:React.ReactNode})=><a href={href}>{children}</a>}));
afterEach(()=>{cleanup();state.query.data={repository:"",folder:"",can_edit:true};state.query.isError=false;state.query.isPending=false;});
function show(){render(<I18nProvider locale="en" resources={{en:{settings:en}}}><IssueRepositoryNotice/></I18nProvider>);}
it("links missing configuration to settings without discarding the draft",()=>{show();expect(screen.getByRole("link",{name:"Configure workspace repository"})).toHaveAttribute("href","/ws/settings");expect(screen.getByText(/Your draft is kept/)).toBeTruthy();});
it("accepts a local folder without GitHub",()=>{state.query.data.folder="local";show();expect(screen.queryByRole("status")).toBeNull();});
it("offers retry when configuration cannot be loaded",()=>{state.query.isError=true;show();expect(screen.getByRole("button",{name:"Reload"})).toBeTruthy();expect(screen.queryByRole("link")).toBeNull();});
