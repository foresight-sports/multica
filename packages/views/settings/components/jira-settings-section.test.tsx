// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { renderWithI18n } from "../../test/i18n";
import { JiraSettingsSection } from "./jira-settings-section";
const state=vi.hoisted(()=>({data:{property_id:"jira",required:false,revision:3,can_edit:true},mutate:vi.fn()}));
vi.mock("@multica/core/workspace",()=>({useJiraSettings:()=>({query:{data:state.data,isError:false},save:{mutate:state.mutate,isPending:false}})}));
afterEach(()=>{cleanup();state.mutate.mockClear();state.data.can_edit=true;});
it("saves the requirement with the current revision",()=>{
 renderWithI18n(<JiraSettingsSection wsId="workspace"/>);
 fireEvent.click(screen.getByRole("switch",{name:"Require JIRA ticket ID before agent work"}));
 expect(state.mutate).toHaveBeenCalledWith({...state.data,required:true});
});
it("does not let workspace members change the requirement",()=>{
 state.data.can_edit=false;
 renderWithI18n(<JiraSettingsSection wsId="workspace"/>);
 expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
 fireEvent.click(screen.getByRole("switch"));
 expect(state.mutate).not.toHaveBeenCalled();
});
