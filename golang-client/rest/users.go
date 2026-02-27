package rest

import (
	"net/url"
	"strconv"
)

type ListUsersOptions struct {
	SocialID *int64
	Status   string
	Search   string
	Limit    int
	Skip     int
	Sort     string
	Order    string
}

func (c *Client) ListUsers(opt ListUsersOptions) ([]User, error) {
	if opt.Limit == 0 {
		opt.Limit = 50
	}
	if opt.Sort == "" {
		opt.Sort = "created_at"
	}
	if opt.Order == "" {
		opt.Order = "desc"
	}

	q := url.Values{}
	q.Set("limit", strconv.Itoa(opt.Limit))
	q.Set("skip", strconv.Itoa(opt.Skip))
	q.Set("sort", opt.Sort)
	q.Set("order", opt.Order)
	if opt.SocialID != nil {
		q.Set("social_id", strconv.FormatInt(*opt.SocialID, 10))
	}
	if opt.Status != "" {
		q.Set("status", opt.Status)
	}
	if opt.Search != "" {
		q.Set("search", opt.Search)
	}

	var out struct {
		Result []User `json:"result"`
	}
	err := c.requestJSON("GET", "/users", q, nil, &out)
	return out.Result, err
}

func (c *Client) GetUser(userID string) (User, error) {
	var out struct {
		Result User `json:"result"`
	}
	err := c.requestJSON("GET", "/users/"+userID, nil, nil, &out)
	return out.Result, err
}

func (c *Client) CreateUser(payload map[string]any) (User, error) {
	var out struct {
		Result User `json:"result"`
	}
	err := c.requestJSON("POST", "/users", nil, payload, &out)
	return out.Result, err
}

func (c *Client) EditUser(userID string, payload map[string]any) (User, error) {
	var out struct {
		Result User `json:"result"`
	}
	err := c.requestJSON("PATCH", "/users/"+userID, nil, payload, &out)
	return out.Result, err
}

func (c *Client) DeleteUser(userID string) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("DELETE", "/users/"+userID, nil, nil, &out)
	return out, err
}

func (c *Client) EnableUser(userID string) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("POST", "/users/"+userID+"/enable", nil, nil, &out)
	return out, err
}

func (c *Client) DisableUser(userID string) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("POST", "/users/"+userID+"/disable", nil, nil, &out)
	return out, err
}

func (c *Client) ResetUsage(userID string) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("POST", "/users/"+userID+"/reset-usage", nil, nil, &out)
	return out, err
}

func (c *Client) Metrics() (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("GET", "/users/metrics", nil, nil, &out)
	return out, err
}
