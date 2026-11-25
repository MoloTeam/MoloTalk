import { createClient, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "@api/molotalk/v1/auth_pb";

import type { Profile } from "../state/store";
import type { ApiPort } from "./ports";

export interface ConnectApiOptions {
  baseUrl?: string;
  transport?: Transport;
}

// Производственный API-адаптер: типизированный клиент поверх публичного
// пакета #6. В тестах транспорт подменяется.
export function createConnectApi(options: ConnectApiOptions = {}): ApiPort {
  const transport =
    options.transport ?? createConnectTransport({ baseUrl: options.baseUrl ?? window.location.origin });
  const client = createClient(AuthService, transport);

  return {
    async getMe(): Promise<Profile> {
      const response = await client.getMe({});
      const user = response.user;
      if (!user) {
        throw new Error("profile is missing in the response");
      }
      return { id: user.id, displayName: user.displayName };
    },
  };
}
