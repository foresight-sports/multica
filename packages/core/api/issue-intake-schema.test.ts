// @vitest-environment node
import { it, expect } from "vitest";
import { IssueIntakeSchema } from "./issue-intake-schema";
it("preserves disabled project overrides and maps the default squad", () => {
  expect(
    IssueIntakeSchema.parse({
      revision: 2,
      default_squad_id: "squad",
      projects: { project: "" },
    }),
  ).toEqual({
    revision: 2,
    defaultSquadId: "squad",
    projects: { project: "" },
  });
});
it("rejects malformed versions and overrides", () => {
  expect(IssueIntakeSchema.safeParse({ revision: -1 }).success).toBe(false);
  expect(
    IssueIntakeSchema.safeParse({ projects: { project: true } }).success,
  ).toBe(false);
});
