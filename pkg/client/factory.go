package client

import (
	"context"
	"fmt"
	"time"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ydb-platform/ydb-ops/pkg/options"
)

const (
	BufferSize = 32 << 20
)

type OperationResponse interface {
	GetOperation() *Ydb_Operations.Operation
}

type Factory struct {
	auth    options.AuthOptions
	grpc    options.GRPC
	restart options.RestartOptions
	token   string
}

func NewConnectionFactory(auth options.AuthOptions, grpc options.GRPC, restart options.RestartOptions) *Factory {
	return &Factory{
		auth:    auth,
		grpc:    grpc,
		restart: restart,
	}
}

func (f *Factory) SetAuthToken(t string) {
	f.token = t
}

func (f *Factory) Connection() (*grpc.ClientConn, error) {
	cr, err := f.makeCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %v", err)
	}

	return grpc.Dial(f.endpoint(),
		grpc.WithTransportCredentials(cr),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(BufferSize),
			grpc.MaxCallRecvMsgSize(BufferSize)))
}

func (f *Factory) OperationParams() *Ydb_Operations.OperationParams {
	return &Ydb_Operations.OperationParams{
		OperationMode:    Ydb_Operations.OperationParams_SYNC,
		OperationTimeout: durationpb.New(time.Duration(f.grpc.TimeoutSeconds) * time.Second),
		CancelAfter:      durationpb.New(time.Duration(f.grpc.TimeoutSeconds) * time.Second),
	}
}

func (f *Factory) makeCredentials() (credentials.TransportCredentials, error) {
	if !f.grpc.GRPCSecure {
		return insecure.NewCredentials(), nil
	}

	if f.grpc.CaFile == "" {
		// TODO verify that this will use system pool
		return credentials.NewClientTLSFromCert(nil, ""), nil
	}

	return credentials.NewClientTLSFromFile(f.grpc.CaFile, "")
}

func (f *Factory) endpoint() string {
	// TODO decide if we want to support multiple endpoints or just one
	// Endpoint in rootOpts will turn from string -> []string in this case
	return fmt.Sprintf("%s:%d", f.grpc.Endpoint, f.grpc.GRPCPort)
}

func (f Factory) ContextWithAuth() (context.Context, context.CancelFunc, error) {
	ctx, cf := context.WithTimeout(context.Background(), time.Second*time.Duration(f.grpc.TimeoutSeconds))

	return metadata.AppendToOutgoingContext(ctx,
		"x-ydb-auth-ticket", f.token), cf, nil
}

func (f Factory) ContextWithoutAuth() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Second*time.Duration(f.grpc.TimeoutSeconds))
}

func (f Factory) GetRetryNumber() int {
	return f.restart.RestartRetryNumber
}