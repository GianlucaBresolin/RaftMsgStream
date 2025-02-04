package client

func (c *Client) GetMembership(group string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.groups[group] != nil
}
