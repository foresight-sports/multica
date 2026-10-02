import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, it, expect, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { MachineLogs } from "./machine-logs";
const get = vi.hoisted(() => vi.fn());
vi.mock("@multica/core/api", () => ({ api: { getMachineLogs: get } }));
beforeEach(() => get.mockReset());
function render(owner = true) {
  return renderWithI18n(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><MachineLogs wsId="ws" runtimeId="rt" owner={owner}/></QueryClientProvider>);
}
it("shows cached offline logs and filters lines", async () => {
  get.mockResolvedValue({supported:true,log:"connected\nclone failed",crash:"crash output",message:"",received_at:"2020-01-01T00:00:00Z",truncated:true});
  render();
  expect(await screen.findByText(/Last received snapshot/)).toBeInTheDocument();
  fireEvent.change(screen.getByRole("textbox"), {target:{value:"clone"}});
  expect(screen.getByLabelText("daemon.log")).toHaveTextContent("clone failed");
  expect(screen.getByLabelText("daemon.log")).not.toHaveTextContent("connected");
  fireEvent.click(screen.getByRole("button",{name:"Pause updates"}));
  expect(screen.getByRole("button",{name:"Resume updates"})).toBeInTheDocument();
});
it("does not request machine-wide logs for another owner", () => {
  render(false);
  expect(screen.getByText(/Only the machine owner/)).toBeInTheDocument();
  expect(get).not.toHaveBeenCalled();
});
it("explains how to enable logs on older daemons", async () => {
  get.mockResolvedValue({supported:false,log:"",crash:"",received_at:null});
  render();
  expect(await screen.findByText(/Update this machine/)).toBeInTheDocument();
});
