import { it, expect } from "vitest";
import { RepositorySettingsSchema } from "./repository-settings-schema";
it("defaults optional repository fields but requires a revision", () => {
  expect(RepositorySettingsSchema.parse({ revision: 0 })).toMatchObject({
    repository: "",
    root: "",
    can_edit: false,
    supported: false,
  });
  expect(RepositorySettingsSchema.safeParse({ revision: -1 }).success).toBe(
    false,
  );
  expect(
    RepositorySettingsSchema.safeParse({ revision: 0, mode: "unsafe" }).success,
  ).toBe(false);
});
