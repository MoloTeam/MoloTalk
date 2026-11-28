package rpc

import (
	"context"
	"errors"
	"io"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/connect/v2/connectproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Mount регистрирует процедуры на mux с JSON-кодеком, который выдаёт
// обязательные false, 0 и пустые массивы, не превращая отсутствующие
// presence-поля в null.
func Mount(mux connecthttp.ServeMux, server *connect.Server) {
	connecthttp.Mount(mux, server, connecthttp.WithCodecs(connectproto.NewBinaryCodec(), defaultValuesCodec{}))
}

type defaultValuesCodec struct{}

func (defaultValuesCodec) Name() string { return "json" }

func (defaultValuesCodec) MarshalWrite(_ context.Context, dst io.Writer, message any) error {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return errors.New("rpc: message is not a proto.Message")
	}
	encoded, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(protoMessage)
	if err != nil {
		return err
	}
	_, err = dst.Write(encoded)
	return err
}

func (defaultValuesCodec) UnmarshalRead(_ context.Context, src io.Reader, message any) error {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return errors.New("rpc: message is not a proto.Message")
	}
	payload, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	return (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, protoMessage)
}
