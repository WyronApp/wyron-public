package rest

func (c *Client) Me() (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("GET", "/auth/me", nil, nil, &out)
	return out, err
}

func (c *Client) Logout() (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("POST", "/auth/logout", nil, nil, &out)
	return out, err
}
