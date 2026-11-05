import { expect, it } from "vitest";
import { WorkspaceLinksSchema } from "./workspace-links-schema";
it("defaults editing rights to false and rejects missing link state", () => {
  expect(
    WorkspaceLinksSchema.parse([
      { id: "sdk", name: "SDK", slug: "sdk", linked: true },
    ])[0]?.can_edit,
  ).toBe(false);
  expect(
    WorkspaceLinksSchema.safeParse([{ id: "sdk", name: "SDK", slug: "sdk" }])
      .success,
  ).toBe(false);
});
