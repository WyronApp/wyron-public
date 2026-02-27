package grpc

import (
	"errors"
	"fmt"

	pb "github.com/wyronapp/wyron-public/golang-client/grpc/proto"
)

var (
	ErrPeerNoClient        = errors.New("peer has no client bound")
	ErrInterfaceNotFound   = errors.New("interface not found")
	ErrInterfaceMissingKey = errors.New("interface missing key")
)

type ServerResolver interface {
	GetServer(id string) (*Server, error)
}

type PeerState struct {
	ServerID       string
	Interface      string
	AllowedAddress string
	PrivateKey     string

	client *Client
}

type WireGuardInterface struct {
	Name        string
	DisplayName string
	Subnet      string
	Endpoint    string
	DNS         string
	Port        int32
	PublicKey   string
	CreatedAt   int64
}

type Server struct {
	Name        string
	Address     string
	Username    string
	DisplayName string
	CreatedAt   int64
	Interfaces  []WireGuardInterface
}

type User struct {
	UserKey          string
	SubToken         string
	SocialID         int64
	Active           bool
	TrafficLimit     uint64
	Usage            uint64
	DurationSeconds  int32
	CreatedAt        int64
	FirstConnectedAt int64
	LastConnectedAt  int64
	CreatedBy        string
	Peers            []*PeerState
}

func (c *Client) parseServer(s *pb.Server) *Server {
	if s == nil {
		return nil
	}
	ifaces := make([]WireGuardInterface, 0, len(s.GetInterfaces()))
	for _, i := range s.GetInterfaces() {
		ifaces = append(ifaces, WireGuardInterface{
			Name:        i.GetName(),
			DisplayName: i.GetDisplayName(),
			Subnet:      i.GetSubnet(),
			Endpoint:    i.GetEndpoint(),
			DNS:         i.GetDns(),
			Port:        i.GetPort(),
			PublicKey:   i.GetPublicKey(),
			CreatedAt:   i.GetCreatedAt(),
		})
	}

	return &Server{
		Name:        s.GetId(),
		Address:     s.GetAddress(),
		Username:    s.GetUsername(),
		DisplayName: s.GetDisplayName(),
		CreatedAt:   s.GetCreatedAt(),
		Interfaces:  ifaces,
	}
}

func (c *Client) parseUser(u *pb.User) *User {
	if u == nil {
		return nil
	}
	peers := make([]*PeerState, 0, len(u.GetPeers()))
	for _, p := range u.GetPeers() {
		peer := PeerState{
			ServerID:       p.GetServerId(),
			Interface:      p.GetInterface(),
			AllowedAddress: p.GetAllowedAddress(),
			PrivateKey:     p.GetPrivateKey(),
		}

		peer.client = c
		peers = append(peers, &peer)
	}

	return &User{
		UserKey:          u.GetUserKey(),
		SubToken:         u.GetSubToken(),
		SocialID:         u.GetSocialId(),
		Active:           u.GetActive(),
		TrafficLimit:     u.GetTrafficLimit(),
		Usage:            u.GetUsage(),
		DurationSeconds:  u.GetDurationSeconds(),
		CreatedAt:        u.GetCreatedAt(),
		FirstConnectedAt: u.GetFirstConnectedAt(),
		LastConnectedAt:  u.GetLastConnectedAt(),
		CreatedBy:        u.GetCreatedBy(),
		Peers:            peers,
	}
}

func (p *PeerState) resolveServer() (*Server, error) {
	if p.client == nil {
		return nil, ErrPeerNoClient
	}
	return p.client.GetServer(p.ServerID)
}

func (p *PeerState) resolveInterface() (*WireGuardInterface, error) {
	server, err := p.resolveServer()
	if err != nil {
		return nil, err
	}

	for i := range server.Interfaces {
		if server.Interfaces[i].Name == p.Interface {
			return &server.Interfaces[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %s on server %s",
		ErrInterfaceNotFound,
		p.Interface,
		p.ServerID,
	)
}

func (p *PeerState) GenerateConfig() (string, error) {

	if p.PrivateKey == "" {
		return "", ErrInterfaceMissingKey
	}

	iface, err := p.resolveInterface()
	if err != nil {
		return "", err
	}

	if iface.Endpoint == "" {
		return "", fmt.Errorf("%w: endpoint missing", ErrInterfaceMissingKey)
	}
	if iface.PublicKey == "" {
		return "", fmt.Errorf("%w: public_key missing", ErrInterfaceMissingKey)
	}

	cfg := fmt.Sprintf(`[Interface]
Address = %s
DNS = %s
PrivateKey = %s

[Peer]
AllowedIPs = 0.0.0.0/0
Endpoint = %s:%d
PublicKey = %s
`,
		p.AllowedAddress,
		iface.DNS,
		p.PrivateKey,
		iface.Endpoint,
		iface.Port,
		iface.PublicKey,
	)

	return cfg, nil
}
