package grpc

import (
	"context"

	pb "github.com/wyronapp/wyron-public/golang-client/grpc/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (c *Client) ListServers() ([]*Server, error) {
	var out []*Server
	err := c.call(func(ctx context.Context) error {
		res, err := c.server.List(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		out = make([]*Server, 0, len(res.GetServers()))
		for _, s := range res.GetServers() {
			out = append(out, c.parseServer(s))
		}
		return nil
	})
	return out, err
}

func (c *Client) GetServer(id string) (*Server, error) {
	var out *Server
	err := c.call(func(ctx context.Context) error {
		res, err := c.server.Get(ctx, &pb.ServerIDRequest{Id: id})
		if err != nil {
			return err
		}
		out = c.parseServer(res)
		return nil
	})
	return out, err
}

func (c *Client) CreateOrUpdateServer(req *pb.UpdateServerRequest) (*Server, error) {
	var out *Server
	err := c.call(func(ctx context.Context) error {
		res, err := c.server.Update(ctx, req)
		if err != nil {
			return err
		}
		out = c.parseServer(res)
		return nil
	})
	return out, err
}

func (c *Client) DeleteServer(id string) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.server.Delete(ctx, &pb.ServerIDRequest{Id: id})
		return err
	})
}

func (c *Client) UpdateInterface(req *pb.InterfaceRequest) (*WireGuardInterface, error) {

	var out *WireGuardInterface
	err := c.call(func(ctx context.Context) error {
		res, err := c.server.UpdateInterface(ctx, req)
		if err != nil {
			return err
		}
		i := res.GetInterface()
		out = &WireGuardInterface{
			Name:        i.GetName(),
			DisplayName: i.GetDisplayName(),
			Subnet:      i.GetSubnet(),
			Endpoint:    i.GetEndpoint(),
			DNS:         i.GetDns(),
			Port:        i.GetPort(),
			PublicKey:   i.GetPublicKey(),
			CreatedAt:   i.GetCreatedAt(),
		}
		return nil
	})
	return out, err
}

func (c *Client) DeleteInterface(req *pb.InterfaceRequest) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.server.DeleteInterface(ctx, req)
		return err
	})
}
