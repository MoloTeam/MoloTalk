// Кросс-языковая проверка Gateway: TypeScript декодирует кадр, собранный Go,
// и кодирует кадр, который Go читает тем же контрактом.
import assert from "node:assert/strict";
import test from "node:test";

import { create, fromBinary, toBinary } from "@bufbuild/protobuf";

import {
  GatewayEnvelopeSchema,
  MessageDeletedSchema,
  ReceiptUpdatedSchema,
  ServerEventSchema,
} from "../gen/molotalk/v1/gateway_pb.ts";

// Байты, снятые с Go: GatewayEnvelope{identify{token:"token"}}.
const goIdentifyHex = "0a070a05746f6b656e";

test("TypeScript decodes the frame produced by Go", () => {
  const bytes = Uint8Array.from(Buffer.from(goIdentifyHex, "hex"));
  const envelope = fromBinary(GatewayEnvelopeSchema, bytes);

  assert.equal(envelope.payload.case, "identify");
  assert.equal(envelope.payload.value.token, "token");
});

test("TypeScript encodes a frame the same way Go does", () => {
  const envelope = create(GatewayEnvelopeSchema, {
    payload: { case: "heartbeat", value: {} },
  });

  const bytes = toBinary(GatewayEnvelopeSchema, envelope);
  // heartbeat — поле 3 оболочки, пустое сообщение: тег 0x1a и длина 0.
  assert.deepEqual(Array.from(bytes), [0x1a, 0x00]);

  const decoded = fromBinary(GatewayEnvelopeSchema, bytes);
  assert.equal(decoded.payload.case, "heartbeat");
});

test("ServerEvent keeps exactly one event", () => {
  const event = create(ServerEventSchema, {
    eventId: "01ARZ3NDEKTSV4RRFFQ69G5FA1",
    occurredAt: { seconds: 1795000000n, nanos: 0 },
    event: {
      case: "messageDeleted",
      value: create(MessageDeletedSchema, {
        chatId: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
        messageId: "01ARZ3NDEKTSV4RRFFQ69G5FA3",
        revision: 2,
      }),
    },
  });

  const decoded = fromBinary(ServerEventSchema, toBinary(ServerEventSchema, event));
  assert.equal(decoded.event.case, "messageDeleted");
  assert.equal(decoded.event.value.revision, 2);
  assert.equal(decoded.eventId, "01ARZ3NDEKTSV4RRFFQ69G5FA1");
});

test("ReceiptUpdated carries either summary or personal receipt", () => {
  const summary = create(ReceiptUpdatedSchema, {
    chatId: "01ARZ3NDEKTSV4RRFFQ69G5FA1",
    messageId: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
    receiptSummary: { recipientCount: 2, deliveredCount: 2, readCount: 1, state: 3 },
  });

  const decoded = fromBinary(ReceiptUpdatedSchema, toBinary(ReceiptUpdatedSchema, summary));
  assert.ok(decoded.receiptSummary);
  assert.equal(decoded.receiptSummary.readCount, 1);
  assert.equal(decoded.receipt, undefined);
});
