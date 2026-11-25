// Контрактный HTTP-обмен TypeScript-клиента с тестовым Connect-сервером Go.
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { ViolationsSchema } from "../gen/buf/validate/validate_pb.ts";
import { AuthService } from "../gen/molotalk/v1/auth_pb.ts";
import { MediaService } from "../gen/molotalk/v1/media_pb.ts";

const backend = path.resolve(import.meta.dirname, "../../../backend");
const workDir = path.resolve(import.meta.dirname, "../../../.tmp");
const binary = path.join(workDir, `testserver${process.platform === "win32" ? ".exe" : ""}`);

function buildServer() {
  fs.mkdirSync(workDir, { recursive: true });
  execFileSync("go", ["build", "-o", binary, "./internal/platform/rpc/testserver"], {
    cwd: backend,
    stdio: "inherit",
  });
}

function startServer() {
  const child = spawn(binary, ["-addr", "127.0.0.1:0"], {
    cwd: backend,
    stdio: ["ignore", "pipe", "pipe"],
  });

  return new Promise((resolve, reject) => {
    let buffered = "";
    const timer = setTimeout(() => {
      child.kill();
      reject(new Error("test server did not start within 60s"));
    }, 60_000);

    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.stdout.on("data", (chunk) => {
      buffered += chunk.toString();
      const match = buffered.match(/listening (http:\/\/\S+)/);
      if (match) {
        clearTimeout(timer);
        resolve({ child, baseUrl: match[1] });
      }
    });
    child.stderr.on("data", (chunk) => process.stderr.write(chunk));
    child.on("exit", (code) => {
      clearTimeout(timer);
      reject(new Error(`test server exited with ${code}`));
    });
  });
}

const invalidUpload = {
  purpose: 2,
  filename: "photo.png",
  contentType: "image/png",
  sizeBytes: 20971521,
};

test("TypeScript client talks to the Go Connect server", async (t) => {
  buildServer();
  const { child, baseUrl } = await startServer();
  t.after(() => {
    child.kill();
    child.stdout.destroy();
    child.stderr.destroy();
  });

  // Connect-ES не вызывает onHeader для unary: заголовок читаем интерцептором.
  function headerCapture() {
    const state = { value: undefined };
    const interceptor = (next) => async (request) => {
      try {
        const response = await next(request);
        state.value = response.header.get("X-Request-Id");
        return response;
      } catch (error) {
        state.value = error?.metadata?.get?.("X-Request-Id");
        throw error;
      }
    };
    return { interceptor, state };
  }

  const client = createClient(AuthService, createConnectTransport({ baseUrl }));
  const media = createClient(MediaService, createConnectTransport({ baseUrl }));

  await t.test("success returns typed data, empty array and request id", async () => {
    const capture = headerCapture();
    const withHeader = createClient(
      AuthService,
      createConnectTransport({ baseUrl, interceptors: [capture.interceptor] }),
    );
    const response = await withHeader.searchUsers(
      { query: "alice" },
      { headers: { "X-Request-Id": "abc-123" } },
    );
    assert.deepEqual(response.users, []);
    assert.equal(capture.state.value, "abc-123");
  });

  await t.test("response decodes Timestamp and enum", async () => {
    const response = await media.prepareUpload({
      purpose: 2,
      filename: "photo.png",
      contentType: "image/png",
      sizeBytes: 1024,
    });
    assert.equal(response.attachmentId, "01ARZ3NDEKTSV4RRFFQ69G5FA1");
    assert.equal(timestampDate(response.uploadExpiresAt).toISOString(), "2026-11-24T09:00:00.000Z");
  });

  await t.test("invalid request is rejected before the handler with typed details", async () => {
    const capture = headerCapture();
    const withHeader = createClient(
      MediaService,
      createConnectTransport({ baseUrl, interceptors: [capture.interceptor] }),
    );
    await assert.rejects(
      withHeader.prepareUpload(invalidUpload, { headers: { "X-Request-Id": "abc-123" } }),
      (error) => {
        assert.ok(error instanceof ConnectError);
        assert.equal(error.code, Code.InvalidArgument);
        assert.match(error.rawMessage, /size_bytes/);

        const details = error.findDetails(ViolationsSchema);
        assert.ok(details.length > 0, "typed violations are missing");
        assert.ok(details[0].violations.length > 0);
        return true;
      },
    );
    assert.equal(capture.state.value, "abc-123");
  });

  await t.test("GetMe decodes an Empty request and a typed response", async () => {
    const response = await client.getMe({});
    assert.equal(response.user?.displayName, "Alice");
  });
});
