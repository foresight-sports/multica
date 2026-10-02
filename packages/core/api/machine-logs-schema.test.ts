// @vitest-environment node
import { expect, it } from "vitest";
import { MachineLogsSchema } from "./machine-logs-schema";
it("rejects malformed snapshots instead of presenting them as empty logs", () => {
  expect(MachineLogsSchema.safeParse({}).success).toBe(false);
  expect(MachineLogsSchema.safeParse({ supported: true, log: [], crash: "", received_at: "invalid" }).success).toBe(false);
  expect(MachineLogsSchema.parse({ supported: false, log: "", crash: "", received_at: null }).truncated).toBe(false);
});
