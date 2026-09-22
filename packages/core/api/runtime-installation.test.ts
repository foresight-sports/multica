// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import { ApiClient } from "./client";
import { setSchemaLogger } from "./schema";
import { noopLogger } from "../logger";

afterEach(() => { vi.unstubAllGlobals(); setSchemaLogger(noopLogger); });

it("retrieves the installer without HTTP caching", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ command: "test-install-command" })));
  vi.stubGlobal("fetch", fetchMock);
  await expect(new ApiClient("https://api.example.test").getRuntimeInstallation()).resolves.toEqual({ command: "test-install-command" });
  expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/runtime-installation", expect.objectContaining({ cache: "no-store" }));
});

it("rejects malformed installer responses without logging secret values", async () => {
  const warn = vi.fn();
  setSchemaLogger({ ...noopLogger, warn });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ command: { secret: "do-not-log-me" } }))));
  await expect(new ApiClient("https://api.example.test").getRuntimeInstallation()).rejects.toThrow("Could not load runtime installer");
  expect(warn).toHaveBeenCalled();
  expect(JSON.stringify(warn.mock.calls)).not.toContain("do-not-log-me");
});
