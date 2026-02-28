package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

type Client struct {
	baseURL  string
	username string
	password string
	token    string

	httpc   *http.Client
	timeout time.Duration
}

func NewClient(baseURL, username, password, proxyURL string, timeout time.Duration) (*Client, error) {
	if baseURL == "" || username == "" || password == "" {
		return nil, errors.New("baseURL/username/password required")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	// http2: Go by default tries HTTP/2 over TLS; for h2c you’d need extra setup.
	tr := &http.Transport{
		ForceAttemptHTTP2: true,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}

		if u.Scheme == "http" || u.Scheme == "https" {
			tr.Proxy = http.ProxyURL(u)
		} else {
			dialer, err := proxy.FromURL(u, proxy.Direct)
			if err != nil {
				return nil, err
			}
			tr.Proxy = nil
			tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	}

	c := &Client{
		baseURL:  strings.TrimRight(baseURL, "/") + "/api",
		username: username,
		password: password,
		timeout:  timeout,
		httpc: &http.Client{
			Transport: tr,
			Timeout:   timeout,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := c.Login(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) authHeader(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *Client) Login(ctx context.Context) error {
	body := map[string]any{
		"username": c.username,
		"password": c.password,
	}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/login", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Token == "" {
		return errors.New("login failed: token missing")
	}
	c.token = out.Token
	return nil
}

func (c *Client) requestJSON(method, path string, query url.Values, payload any, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	full := c.baseURL + path
	if query != nil && len(query) > 0 {
		full += "?" + query.Encode()
	}

	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	doOnce := func() (*http.Response, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, method, full, body)
		if err != nil {
			return nil, nil, err
		}
		c.authHeader(req)

		resp, err := c.httpc.Do(req)
		if err != nil {
			return nil, nil, err
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp, raw, err
	}

	resp, raw, err := doOnce()
	if err != nil {
		return err
	}

	// auto re-login on 401
	if resp.StatusCode == http.StatusUnauthorized {
		if err := c.Login(ctx); err != nil {
			return err
		}
		// reset body reader for retry (اگر payload داشت)
		if payload != nil {
			b, _ := json.Marshal(payload)
			body = bytes.NewReader(b)
		}
		resp, raw, err = doOnce()
		if err != nil {
			return err
		}
	}

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("api error: %s %s status=%d body=%s", method, path, resp.StatusCode, string(raw))
	}

	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
