package rest

import (
	"errors"
	"fmt"
)

var (
	ErrInterfaceNotFound   = errors.New("interface not found")
	ErrInterfaceMissingKey = errors.New("interface missing key")
)

type WireGuardInterface struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Subnet      string `json:"subnet,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	DNS         string `json:"dns,omitempty"`
	Port        int    `json:"port,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
}

type Server struct {
	Name        string               `json:"name"`
	Address     string               `json:"address"`
	Username    string               `json:"username"`
	DisplayName string               `json:"display_name,omitempty"`
	CreatedAt   int64                `json:"created_at,omitempty"`
	Interfaces  []WireGuardInterface `json:"interfaces,omitempty"`
}

type User struct {
	UserKey          string      `json:"user_key"`
	SubToken         string      `json:"sub_token"`
	SocialID         int64       `json:"social_id"`
	Active           bool        `json:"active"`
	TrafficLimit     int64       `json:"traffic_limit"`
	Usage            int64       `json:"usage"`
	DurationSeconds  int64       `json:"duration_seconds"`
	CreatedAt        int64       `json:"created_at"`
	FirstConnectedAt int64       `json:"first_connected_at"`
	LastConnectedAt  int64       `json:"last_connected_at"`
	CreatedBy        string      `json:"created_by"`
	Peers            []PeerState `json:"peers,omitempty"`
}

type PeerState struct {
	ServerID       string `json:"server_id"`
	Interface      string `json:"interface"`
	AllowedAddress string `json:"allowed_address"`
	PrivateKey     string `json:"private_key,omitempty"`
}

func (p PeerState) GenerateConfig(srv *Server) (string, error) {
	if p.PrivateKey == "" {
		return "", ErrInterfaceMissingKey
	}

	var iface *WireGuardInterface
	for i := range srv.Interfaces {
		if srv.Interfaces[i].Name == p.Interface {
			iface = &srv.Interfaces[i]
			break
		}
	}
	if iface == nil {
		return "", fmt.Errorf("%w: %s", ErrInterfaceNotFound, p.Interface)
	}
	if iface.Endpoint == "" || iface.PublicKey == "" || iface.Port == 0 {
		return "", ErrInterfaceMissingKey
	}

	return fmt.Sprintf(`[Interface]
Address = %s
DNS = %s
PrivateKey = %s

[Peer]
AllowedIPs = 0.0.0.0/0
Endpoint = %s:%d
PublicKey = %s
`, p.AllowedAddress, iface.DNS, p.PrivateKey, iface.Endpoint, iface.Port, iface.PublicKey), nil
}
