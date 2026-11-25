package rpc_test

import (
	"context"
	"errors"
	"testing"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
)

func TestValidationErrorCarriesTypedDetails(t *testing.T) {
	t.Parallel()

	_, server := newServer(t)
	mediaClient := molotalkv1connect.NewMediaServiceClient(client(t, server))

	_, err := mediaClient.PrepareUpload(context.Background(), &molotalkv1.PrepareUploadRequest{
		Purpose:     molotalkv1.AttachmentPurpose_ATTACHMENT_PURPOSE_MESSAGE_ATTACHMENT,
		Filename:    "photo.png",
		ContentType: "image/png",
		SizeBytes:   20971521,
	})

	var connectError *connect.Error
	if !errors.As(err, &connectError) {
		t.Fatalf("error is not a connect.Error: %v", err)
	}

	details := connectError.Details()
	if len(details) != 1 {
		t.Fatalf("details = %d, want 1", len(details))
	}
	if details[0].Type != "buf.validate.Violations" {
		t.Fatalf("detail type = %q, want buf.validate.Violations", details[0].Type)
	}

	var violations validatepb.Violations
	if err := proto.Unmarshal(details[0].Value, &violations); err != nil {
		t.Fatalf("unmarshal violations: %v", err)
	}
	if len(violations.Violations) == 0 {
		t.Fatal("violations detail is empty")
	}
}
