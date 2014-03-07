// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

// TODO(mattyw) 2014-03-07 bug #1288750
// Need a SetPassword method.
type Client struct {
	st *api.State
}

func NewClient(st *api.State) *Client {
	return &Client{st}
}

func (c *Client) Close() error {
	return c.st.Close()
}

func (c *Client) AddUser(tag, password string) (params.ErrorResults, error) {
	u := params.ModifyUser{Tag: tag, Password: password}
	p := params.ModifyUsers{Params: []params.ModifyUser{u}}
	results := new(params.ErrorResults)
	err := c.st.Call("UserManager", "", "AddUser", p, results)
	return *results, err
}

func (c *Client) RemoveUser(tag string) (params.ErrorResults, error) {
	u := params.ModifyUser{Tag: tag}
	p := params.ModifyUsers{Params: []params.ModifyUser{u}}
	results := new(params.ErrorResults)
	err := c.st.Call("UserManager", "", "RemoveUser", p, results)
	return *results, err
}
