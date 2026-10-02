import { describe, expect, it } from "vitest";
import type { AgentDraft } from "./draft";
import {
  buildCreateAgentRequest,
  isDraftExecutionReady,
  getDraftExecutionPolicy,
  applyDraftRuntimeChange,
} from "./draft";
import { StoredAgentDraftSchema } from "../api/schemas";
import {
  fromStoredAgentDraft,
  storedAgentDraftsEqual,
  toStoredAgentDraft,
} from "./stored-draft";

const draft = (): AgentDraft => ({
  name: "Release manager",
  description: "Ships carefully",
  instructions: "# Role\nShip.",
  conversationStarters: [
    { label: "Plan a release", prompt: "Plan the next release." },
  ],
  avatarUrl: "🚀",
  runtimeId: "runtime-1",
  model: "gpt-5.6-sol",
  thinkingLevel: "high",
  serviceTier: "priority",
  skillIds: new Set(["skill-1", "skill-2"]),
  permissionScope: "members",
  memberIds: new Set(["member-1"]),
  teamIds: new Set(["team-1"]),
});

describe("stored agent draft", () => {
  it("preserves model requirements when the creation assistant changes machine", () => {
    const initial = draft();
    initial.executionPolicy = getDraftExecutionPolicy(initial, undefined, true, "codex");
    const switched = applyDraftRuntimeChange(initial, "runtime-2");
    const restored = fromStoredAgentDraft(toStoredAgentDraft(switched, null), "runtime-2");
    expect(restored.executionPolicy).toEqual(initial.executionPolicy);
    expect(restored.model).toBe(initial.model);
    expect(restored.executionPolicy?.profiles[0]?.runtime_id).toBe("");
    expect(isDraftExecutionReady(restored.executionPolicy)).toBe(true);
  });
  it("restores execution profiles and submits them atomically with a new revision", () => {
    const policy = {
      revision: 7,
      mode: "automatic" as const,
      preference: "balanced" as const,
      default_profile: "fast",
      router_profile: "fast",
      allow_fallback: false,
      profiles: [
        {
          id: "fast",
          name: "Fast",
          provider: "codex", runtime_id: "",
          model: "m",
          thinking_level: "",
          service_tier: "",
          purpose: "",
          keywords: [],
          required_os: "",
          required_tools: [],
          quality: 3,
          speed: 3,
          cost: 3,
        },
      ],
    };
    const stored = StoredAgentDraftSchema.parse(
      toStoredAgentDraft({ ...draft(), executionPolicy: policy }, null),
    );
    const restored = fromStoredAgentDraft(stored, "runtime-1");
    expect(restored.executionPolicy).toEqual(policy);
    expect(
      buildCreateAgentRequest({
        draft: restored,
        runtimeId: restored.runtimeId,
      }).execution_policy,
    ).toEqual({ ...policy, revision: 0 });
    expect(isDraftExecutionReady(policy)).toBe(true);
    expect(
      isDraftExecutionReady({
        ...policy,
        profiles: [{ ...policy.profiles[0]!, model: "" }],
      }),
    ).toBe(false);
    expect(
      isDraftExecutionReady({ ...policy, default_profile: "missing" }),
    ).toBe(false);
    expect(isDraftExecutionReady()).toBe(true);
  });
  it("preserves an explicit instance opt-out across draft restoration", () => {
    const restored = fromStoredAgentDraft(
      toStoredAgentDraft({ ...draft(), instanceAgent: false }, null),
      "runtime-1",
    );
    expect(restored.instanceAgent).toBe(false);
  });
  it("preserves instance scope through storage, API parsing and normal submission", () => {
    const original = { ...draft(), instanceAgent: true };
    const stored = StoredAgentDraftSchema.parse(
      toStoredAgentDraft(original, null),
    );
    const restored = fromStoredAgentDraft(stored, original.runtimeId);
    expect(restored.instanceAgent).toBe(true);
    expect(
      buildCreateAgentRequest({
        draft: restored,
        runtimeId: restored.runtimeId,
      }).instance_agent,
    ).toBe(true);
    expect(
      buildCreateAgentRequest({ draft: draft(), runtimeId: "runtime-1" }),
    ).not.toHaveProperty("instance_agent");
  });

  it("never enables instance scope from malformed API values", () => {
    for (const value of ["true", 1, null, {}, undefined]) {
      const stored = StoredAgentDraftSchema.parse({
        ...toStoredAgentDraft(draft(), null),
        instance_agent: value,
      });
      expect(fromStoredAgentDraft(stored, "runtime-1").instanceAgent).not.toBe(
        true,
      );
    }
  });
  it("round-trips every editable field", () => {
    const original = draft();
    const restored = fromStoredAgentDraft(
      toStoredAgentDraft(original, "msg-1"),
      original.runtimeId,
    );
    expect(restored).toEqual(original);
  });

  // The runtime a conversation executes on is owned by its carrier agent. A
  // copy inside the draft could only go stale and put the picker on a runtime
  // that runs nothing (MUL-5163), so the restore takes it from the caller.
  it("takes the runtime from the caller, not the stored copy", () => {
    const stored = toStoredAgentDraft(draft(), null);
    expect(stored).not.toHaveProperty("runtimeId");
    expect(fromStoredAgentDraft(stored, "runtime-2").runtimeId).toBe(
      "runtime-2",
    );
  });

  // The marker is what stops a restore from re-applying the last reply over
  // edits made after it, so it has to survive the round trip.
  it("carries the applied message marker", () => {
    expect(toStoredAgentDraft(draft(), "msg-9").applied_message_id).toBe(
      "msg-9",
    );
    expect(toStoredAgentDraft(draft(), null).applied_message_id).toBeNull();
  });

  // Autosave compares on the stored form: a Set rebuilt with the same members
  // is a new reference every render and would otherwise write on every keystroke
  // elsewhere in the form.
  it("treats rebuilt sets with the same members as unchanged", () => {
    expect(
      storedAgentDraftsEqual(
        toStoredAgentDraft(draft(), "msg-1"),
        toStoredAgentDraft(draft(), "msg-1"),
      ),
    ).toBe(true);
    expect(
      storedAgentDraftsEqual(
        toStoredAgentDraft(draft(), "msg-1"),
        toStoredAgentDraft({ ...draft(), name: "Other" }, "msg-1"),
      ),
    ).toBe(false);
  });
});
