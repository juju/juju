// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

type Client struct {
	st *api.State
}

func NewClient(st *api.State) *Client {
	return &Client{st}
}

func (c *Client) Close() error {
	return c.st.Close()
}

func (c *Client) AddUser(tag, password string) (params.ErrorResult, error) {
	p := params.ModifyUser{Tag: tag, Password: password}
	var result params.ErrorResult
	err := c.st.Call("UserManager", "", "AddUser", p, &result)
	return result, err
}

func (c *Client) RemoveUser(tag string) (params.ErrorResult, error) {
	p := params.ModifyUser{Tag: tag}
	var result params.ErrorResult
	err := c.st.Call("UserManager", "", "RemoveUser", p, &result)
	return result, err
}
