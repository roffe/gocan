package gateway

import (
	"os"
	"path/filepath"

	"github.com/roffe/gocan/v2/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var socketFile = filepath.Join(os.TempDir(), "cangateway.sock")

// NewGRPCClient connects to the local gocangateway instance.
func NewGRPCClient() (*grpc.ClientConn, proto.GocanClient, error) {
	conn, err := grpc.NewClient(
		"unix:"+socketFile,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, proto.NewGocanClient(conn), nil
}
