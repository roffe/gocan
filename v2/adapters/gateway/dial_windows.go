package gateway

import (
	"github.com/Microsoft/go-winio"
	"github.com/roffe/gocan/v2/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NewGRPCClient connects to the local gocangateway instance.
func NewGRPCClient() (*grpc.ClientConn, proto.GocanClient, error) {
	conn, err := grpc.NewClient(
		`passthrough:\\.\pipe\gocangateway`,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(winio.DialPipeContext),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, proto.NewGocanClient(conn), nil
}
