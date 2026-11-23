package rpc_test

import (
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
)

func envelope(payload *molotalkv1.GatewayEnvelope) *molotalkv1.GatewayEnvelope {
	return payload
}

func TestGatewayEnvelopeRequiresOneof(t *testing.T) {
	t.Parallel()

	empty := &molotalkv1.GatewayEnvelope{}
	if err := protovalidate.Validate(empty); err == nil {
		t.Fatal("empty envelope is accepted")
	}

	identify := &molotalkv1.GatewayEnvelope{
		Payload: &molotalkv1.GatewayEnvelope_Identify{Identify: &molotalkv1.Identify{Token: "token"}},
	}
	if err := protovalidate.Validate(identify); err != nil {
		t.Fatalf("identify envelope is rejected: %v", err)
	}
}

func TestGatewayEnvelopeRoundTripKeepsCase(t *testing.T) {
	t.Parallel()

	envelope := &molotalkv1.GatewayEnvelope{
		Payload: &molotalkv1.GatewayEnvelope_HeartbeatAck{HeartbeatAck: &molotalkv1.HeartbeatAck{}},
	}

	encoded, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &molotalkv1.GatewayEnvelope{}
	if err := proto.Unmarshal(encoded, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded.Payload.(*molotalkv1.GatewayEnvelope_HeartbeatAck); !ok {
		t.Fatalf("payload case = %T, want heartbeat ack", decoded.Payload)
	}
}

func TestServerEventRequiresExactlyOneEvent(t *testing.T) {
	t.Parallel()

	base := func() *molotalkv1.ServerEvent {
		return &molotalkv1.ServerEvent{
			EventId:    "01ARZ3NDEKTSV4RRFFQ69G5FA1",
			OccurredAt: timestamppb.New(time.Date(2026, time.November, 23, 9, 0, 0, 0, time.UTC)),
		}
	}

	if err := protovalidate.Validate(base()); err == nil {
		t.Fatal("event without payload is accepted")
	}

	deleted := base()
	deleted.Event = &molotalkv1.ServerEvent_MessageDeleted{MessageDeleted: &molotalkv1.MessageDeleted{
		ChatId:    "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		MessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA3",
		Revision:  1,
	}}
	if err := protovalidate.Validate(deleted); err != nil {
		t.Fatalf("message deleted event is rejected: %v", err)
	}

	created := base()
	created.Event = &molotalkv1.ServerEvent_MessageCreated{MessageCreated: &molotalkv1.Message{
		Id:              "01ARZ3NDEKTSV4RRFFQ69G5FA4",
		ChatId:          "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		AuthorId:        "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		ClientMessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA5",
		Revision:        1,
		CreatedAt:       timestamppb.New(time.Date(2026, time.November, 23, 9, 0, 0, 0, time.UTC)),
	}}
	if err := protovalidate.Validate(created); err != nil {
		t.Fatalf("message created event is rejected: %v", err)
	}

	// Неизвестный номер enum отвергается defined_only.
	unknown := base()
	unknown.Event = &molotalkv1.ServerEvent_MessageDeleted{MessageDeleted: &molotalkv1.MessageDeleted{
		ChatId:    "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		MessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA3",
		Revision:  1,
	}}
	if err := protovalidate.Validate(unknown); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
}

func TestReceiptUpdatedAcceptsExactlyOnePayload(t *testing.T) {
	t.Parallel()

	summary := &molotalkv1.ReceiptUpdated{
		ChatId:         "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		MessageId:      "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		ReceiptSummary: &molotalkv1.ReceiptSummary{RecipientCount: 2, DeliveredCount: 2, ReadCount: 1, State: molotalkv1.ReceiptState_RECEIPT_STATE_READ},
	}
	if err := protovalidate.Validate(summary); err != nil {
		t.Fatalf("summary payload is rejected: %v", err)
	}

	receipt := &molotalkv1.ReceiptUpdated{
		ChatId:    "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		MessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		Receipt:   &molotalkv1.Receipt{MessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA2", UserId: "01ARZ3NDEKTSV4RRFFQ69G5FA3"},
	}
	if err := protovalidate.Validate(receipt); err != nil {
		t.Fatalf("receipt payload is rejected: %v", err)
	}

	both := &molotalkv1.ReceiptUpdated{
		ChatId:         "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		MessageId:      "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		ReceiptSummary: &molotalkv1.ReceiptSummary{RecipientCount: 2, DeliveredCount: 2, ReadCount: 1, State: molotalkv1.ReceiptState_RECEIPT_STATE_READ},
		Receipt:        &molotalkv1.Receipt{MessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA2", UserId: "01ARZ3NDEKTSV4RRFFQ69G5FA3"},
	}
	if err := protovalidate.Validate(both); err == nil {
		t.Fatal("receipt with both payloads is accepted")
	}

	none := &molotalkv1.ReceiptUpdated{
		ChatId:    "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		MessageId: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
	}
	if err := protovalidate.Validate(none); err == nil {
		t.Fatal("receipt without payload is accepted")
	}
}

func TestGatewayEnumAndProfileRules(t *testing.T) {
	t.Parallel()

	invalidKind := &molotalkv1.Chat{Id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", Kind: molotalkv1.ChatKind_CHAT_KIND_UNSPECIFIED}
	if err := protovalidate.Validate(invalidKind); err == nil {
		t.Fatal("enum zero value is accepted")
	}

	validKind := &molotalkv1.Chat{Id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", Kind: molotalkv1.ChatKind_CHAT_KIND_GROUP}
	if err := protovalidate.Validate(validKind); err != nil {
		t.Fatalf("valid kind is rejected: %v", err)
	}

	if field := (&molotalkv1.PublicProfile{}).ProtoReflect().Descriptor().Fields().ByName("identifier"); field != nil {
		t.Fatal("PublicProfile exposes identifier")
	}
}
