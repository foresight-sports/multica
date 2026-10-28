// @vitest-environment node
import { expect, it } from "vitest";
import { JiraConnectSchema, JiraConnectionSchema, JiraBlockersSchema, JiraResumeSchema } from "./jira-connection-schema";

it("rejects unknown connection states and malformed authorization links", () => {
  expect(JiraConnectionSchema.safeParse({ configured: true, status: "ready" }).success).toBe(false);
  expect(JiraConnectionSchema.safeParse({ status: "connected" }).success).toBe(false);
  expect(JiraConnectSchema.safeParse({ redirect_url: "not-a-url" }).success).toBe(false);
});
it("rejects malformed blockers and resume acknowledgements", () => {
  expect(JiraBlockersSchema.safeParse([{ task_id: "bad", agent_name: "Agent", can_resume: true }]).success).toBe(false);
  expect(JiraBlockersSchema.safeParse([{ task_id: "00000000-0000-4000-8000-000000000001", agent_name: "Agent", can_resume: "true" }]).success).toBe(false);
  expect(JiraResumeSchema.safeParse({}).success).toBe(false);
});
it("accepts the connection contract without exposing credentials", () => {
  expect(JiraConnectionSchema.parse({ configured: true, status: "connected", access_token: "discard" }))
    .toEqual({ configured: true, status: "connected", pending: false });
});
