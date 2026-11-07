import { describe, it, expect } from "vitest";
import { InstanceConfigurationSchema } from "./instance-schema";
describe("instance audience guidance contract", () => {
  it("keeps older server payloads compatible", () => {
    expect(
      InstanceConfigurationSchema.parse({ instructions: "Policy", revision: 1 })
        .audience_instructions,
    ).toBeUndefined();
  });
  it("accepts wording but rejects malformed values", () => {
    expect(
      InstanceConfigurationSchema.parse({
        instructions: "Policy",
        revision: 1,
        audience_instructions: "CS guidance",
      }).audience_instructions,
    ).toBe("CS guidance");
    expect(
      InstanceConfigurationSchema.safeParse({
        instructions: "Policy",
        revision: 1,
        audience_instructions: 42,
      }).success,
    ).toBe(false);
  });
});
