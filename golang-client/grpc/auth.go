package grpc

import (
	"context"

	pb "github.com/wyronapp/wyron-public/golang-client/grpc/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (c *Client) Login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()
	}

	res, err := c.auth.Login(ctx, &pb.LoginRequest{
		Username: c.cfg.Username,
		Password: c.cfg.Password,
	})
	if err != nil {
		return err
	}

	c.setToken(res.GetToken())
	return nil
}

func (c *Client) Me() (string, error) {
	var username string
	err := c.call(func(ctx context.Context) error {
		res, err := c.auth.Me(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		username = res.GetUsername()
		return nil
	})
	return username, err
}

func (c *Client) CreateAdmin(username, password string) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.auth.CreateAdmin(ctx, &pb.CreateAdminRequest{
			Username: username,
			Password: password,
		})
		return err
	})
}
