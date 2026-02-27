package wyron_client

import (
	"time"

	"github.com/wyronapp/wyron-public/golang-client/grpc"
	"github.com/wyronapp/wyron-public/golang-client/rest"
)

func NewRestClient(baseURL, username, password string, timeout time.Duration) (*rest.Client, error) {
	return rest.NewClient(baseURL, username, password, timeout)
}

func NewGRPCClient(cfg grpc.Config) (*grpc.Client, error) {
	return grpc.NewClient(cfg)
}
