import { useState } from "react";
import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useAuthStore } from "@multica/core/auth";
import { EMPTY_USER } from "@multica/core/api/schemas";
import { renderWithI18n } from "../../test/i18n";
import { InstanceAgentScopeField } from "./instance-agent-scope-field";

vi.mock("@multica/core/auth", async () => {
  const { create } = await import("zustand");
  return { useAuthStore: create(() => ({ user: null })) };
});
function Form({ initial = false }: { initial?: boolean }) {
  const [checked, setChecked] = useState(initial);
  return <InstanceAgentScopeField checked={checked} onChange={setChecked} />;
}
beforeEach(() => {
  useAuthStore.setState({ user: { ...EMPTY_USER, permissions: { register_runtimes: false, manage_permission_access: true, create_instance_agents: true, create_agents: true, edit_agents: true } } });
});
it("lets an administrator mark the normal creation draft as instance scoped", () => {
  renderWithI18n(<Form />);
  const checkbox = screen.getByRole("checkbox", { name: "Instance agent" });
  expect(checkbox).not.toBeChecked();
  fireEvent.click(checkbox);
  expect(checkbox).toBeChecked();
  fireEvent.click(checkbox);
  expect(checkbox).not.toBeChecked();
});
it("hides instance creation from ordinary members", () => {
  useAuthStore.setState({ user: EMPTY_USER });
  renderWithI18n(<Form />);
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
});
it("allows clearing a restored instance draft after permission is revoked", () => {
  useAuthStore.setState({ user: EMPTY_USER });
  renderWithI18n(<Form initial />);
  fireEvent.click(screen.getByRole("checkbox", { name: "Instance agent" }));
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
});
