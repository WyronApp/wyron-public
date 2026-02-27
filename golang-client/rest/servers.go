package rest

func (c *Client) ListServers() ([]Server, error) {
	var out struct {
		Data []Server `json:"data"`
	}
	err := c.requestJSON("GET", "/servers", nil, nil, &out)
	return out.Data, err
}

func (c *Client) GetServer(serverID string) (Server, error) {
	var out struct {
		Data Server `json:"data"`
	}
	err := c.requestJSON("GET", "/servers/"+serverID, nil, nil, &out)
	return out.Data, err
}

func (c *Client) CreateOrUpdateServerRaw(payload map[string]any) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("POST", "/servers", nil, payload, &out)
	return out, err
}

func (c *Client) DeleteServer(serverID string) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("DELETE", "/servers/"+serverID, nil, nil, &out)
	return out, err
}

func (c *Client) UpdateInterface(serverID string, payload map[string]any) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("POST", "/servers/"+serverID+"/interfaces", nil, payload, &out)
	return out, err
}

func (c *Client) DeleteInterface(serverID, ifaceName string) (map[string]any, error) {
	var out map[string]any
	err := c.requestJSON("DELETE", "/servers/"+serverID+"/interfaces/"+ifaceName, nil, nil, &out)
	return out, err
}
