// @vitest-environment node
import { expect, it } from "vitest";
import { RESOURCES } from "../../locales";
import settings from "../../locales/en/settings.json";
import issues from "../../locales/en/issues.json";
function keys(value: unknown) { return value && typeof value === "object" ? Object.keys(value).sort() : []; }
it("translates every linked-workspace and request key in every locale", () => {
  for (const locale of Object.values(RESOURCES)) {
    expect(keys(locale.settings?.linked_workspaces)).toEqual(keys(settings.linked_workspaces));
    expect(keys(locale.issues?.linked_requests)).toEqual(keys(issues.linked_requests));
  }
});
