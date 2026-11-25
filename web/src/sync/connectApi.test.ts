import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { AuthService } from "@api/molotalk/v1/auth_pb";
import { GetMeResponseSchema } from "@api/molotalk/v1/auth_pb";
import { UserSchema } from "@api/molotalk/v1/types_pb";
import { describe, expect, it } from "vitest";

import { createConnectApi } from "./connectApi";

const userId = "01ARZ3NDEKTSV4RRFFQ69G5FA1";

describe("connectApi", () => {
  it("returns typed profile data through the test transport", async () => {
    const transport = createRouterTransport(({ service }) => {
      service(AuthService, {
        getMe: () =>
          create(GetMeResponseSchema, {
            user: create(UserSchema, { id: userId, displayName: "Alice" }),
          }),
      });
    });

    const api = createConnectApi({ transport });

    await expect(api.getMe()).resolves.toEqual({ id: userId, displayName: "Alice" });
  });

  it("propagates the RPC error code", async () => {
    const transport = createRouterTransport(({ service }) => {
      service(AuthService, {
        getMe: () => {
          throw new ConnectError("session expired", Code.Unauthenticated);
        },
      });
    });

    const api = createConnectApi({ transport });

    await expect(api.getMe()).rejects.toMatchObject({ code: Code.Unauthenticated });
  });
});
