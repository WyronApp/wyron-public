package grpc

import (
	"context"

	pb "github.com/wyronapp/wyron-public/golang-client/grpc/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ListUsersOptions struct {
	SocialID *int64
	Status   *string
	Search   *string
	Limit    int32
	Skip     int32
	Sort     string
	Order    string
}

func (c *Client) ListUsers(opt ListUsersOptions) ([]*User, int64, error) {
	if opt.Limit == 0 {
		opt.Limit = 50
	}
	if opt.Sort == "" {
		opt.Sort = "created_at"
	}
	if opt.Order == "" {
		opt.Order = "desc"
	}

	req := &pb.ListUsersRequest{
		Limit: opt.Limit,
		Skip:  opt.Skip,
		Sort:  opt.Sort,
		Order: opt.Order,
	}
	if opt.SocialID != nil {
		req.SocialId = opt.SocialID
	}
	if opt.Status != nil {
		req.Status = opt.Status
	}
	if opt.Search != nil {
		req.Search = opt.Search
	}

	var users []*User
	var count int64

	err := c.call(func(ctx context.Context) error {
		res, err := c.user.List(ctx, req)
		if err != nil {
			return err
		}
		count = res.GetCount()
		users = make([]*User, 0, len(res.GetUsers()))
		for _, u := range res.GetUsers() {
			users = append(users, c.parseUser(u))
		}
		return nil
	})

	return users, count, err
}

func (c *Client) GetUser(userKey string) (*User, error) {
	var out *User
	err := c.call(func(ctx context.Context) error {
		res, err := c.user.Get(ctx, &pb.UserKeyRequest{UserKey: userKey})
		if err != nil {
			return err
		}
		out = c.parseUser(res)
		return nil
	})
	return out, err
}

func (c *Client) CreateUser(req *pb.CreateUserRequest) (*User, error) {
	var out *User
	err := c.call(func(ctx context.Context) error {
		res, err := c.user.Create(ctx, req)
		if err != nil {
			return err
		}
		out = c.parseUser(res)
		return nil
	})
	return out, err
}

func (c *Client) EditUser(req *pb.EditUserRequest) (*User, error) {
	var out *User
	err := c.call(func(ctx context.Context) error {
		res, err := c.user.Edit(ctx, req)
		if err != nil {
			return err
		}
		out = c.parseUser(res)
		return nil
	})
	return out, err
}

func (c *Client) DeleteUser(userKey string) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.user.Delete(ctx, &pb.UserKeyRequest{UserKey: userKey})
		return err
	})
}

func (c *Client) EnableUser(userKey string) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.user.Enable(ctx, &pb.UserKeyRequest{UserKey: userKey})
		return err
	})
}

func (c *Client) DisableUser(userKey string) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.user.Disable(ctx, &pb.UserKeyRequest{UserKey: userKey})
		return err
	})
}

func (c *Client) ResetUsage(userKey string) error {
	return c.call(func(ctx context.Context) error {
		_, err := c.user.ResetUsage(ctx, &pb.UserKeyRequest{UserKey: userKey})
		return err
	})
}

func (c *Client) Metrics() (*pb.MetricsResponse, error) {
	var out *pb.MetricsResponse
	err := c.call(func(ctx context.Context) error {
		res, err := c.user.Metrics(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		out = res
		return nil
	})
	return out, err
}

func (c *Client) RevokeSubToken(userKey string) (*User, error) {
	var out *User
	err := c.call(func(ctx context.Context) error {
		res, err := c.user.RevokeSubToken(ctx, &pb.UserKeyRequest{UserKey: userKey})
		if err != nil {
			return err
		}
		out = c.parseUser(res)
		return nil
	})
	return out, err
}
