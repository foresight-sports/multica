import { describe, it, expect } from "vitest";
import {
  InstanceUpdateSchema,
  RuntimeUpdateResponseSchema,
} from "./runtime-update-schema";
describe("instance update responses", () => {
  it("fails closed when capabilities are absent", () => {
    expect(InstanceUpdateSchema.parse({ channel: "instance" })).toMatchObject({
      canUpdate: false,
      remoteSupported: false,
      updateAvailable: false,
      online: false,
    });
  });
  it("rejects malformed capabilities and update outcomes", () => {
    expect(
      InstanceUpdateSchema.safeParse({
        channel: "instance",
        can_update: "true",
      }).success,
    ).toBe(false);
    expect(
      RuntimeUpdateResponseSchema.safeParse({ id: "x", status: "surprise" })
        .success,
    ).toBe(false);
  });
});
