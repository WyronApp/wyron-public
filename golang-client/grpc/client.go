package grpc

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sync"
	"time"

	pb "github.com/wyronapp/wyron-public/golang-client/grpc/proto"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpcmd "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Config struct {
	Host     string
	Username string
	Password string
	ProxyURL string

	Timeout time.Duration
	Secure  bool
	TLS     *credentials.TransportCredentials
}

type Client struct {
	cfg Config

	conn *grpc.ClientConn

	auth   pb.AuthServiceClient
	server pb.ServerServiceClient
	user   pb.UserServiceClient

	mu    sync.RWMutex
	token string

	loginMu sync.Mutex
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Host == "" || cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("host/username/password required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}

	var opts []grpc.DialOption
	if cfg.Secure {
		if cfg.TLS != nil {
			opts = append(opts, grpc.WithTransportCredentials(*cfg.TLS))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
		}
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, err
		}

		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}

		opts = append(opts, grpc.WithContextDialer(
			func(ctx context.Context, addr string) (net.Conn, error) {
				return dialer.Dial("tcp", addr)
			},
		))
	}

	conn, err := grpc.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, err
	}

	c := &Client{
		cfg:  cfg,
		conn: conn,

		auth:   pb.NewAuthServiceClient(conn),
		server: pb.NewServerServiceClient(conn),
		user:   pb.NewUserServiceClient(conn),
	}

	// initial login
	if err := c.Login(context.Background()); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return c, nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) getToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

func (c *Client) setToken(tok string) {
	c.mu.Lock()
	c.token = tok
	c.mu.Unlock()
}

func (c *Client) withAuth(ctx context.Context) context.Context {
	tok := c.getToken()
	if tok == "" {
		return ctx
	}
	return grpcmd.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tok)
}

func (c *Client) call(fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.Timeout)
	defer cancel()

	// 1st attempt
	err := fn(c.withAuth(ctx))
	if err == nil {
		return nil
	}

	// retry once if unauthenticated
	if status.Code(err) == codes.Unauthenticated {
		if lerr := c.Login(ctx); lerr != nil {
			return lerr
		}
		return fn(c.withAuth(ctx))
	}

	return err
}
