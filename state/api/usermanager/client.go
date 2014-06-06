// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
)

// TODO(mattyw) 2014-03-07 bug #1288750
// Need a SetPassword method.
type Client struct {
	st *api.State
}

func (c *Client) call(method string, params, result interface{}) error {
	return c.st.Call("UserManager", "", method, params, result)
}

func NewClient(st *api.State) *Client {
	return &Client{st}
}

func (c *Client) Close() error {
	return c.st.Close()
}

func (c *Client) AddUser(username, displayName, password string) error {
	if !names.IsUser(username) {
		return fmt.Errorf("invalid user name %q", username)
	}
	userArgs := params.ModifyUsers{
		Changes: []params.ModifyUser{{Username: username, DisplayName: displayName, Password: password}},
	}
	results := new(params.ErrorResults)
	err := c.call("AddUser", userArgs, results)
	if err != nil {
		return err
	}
	return results.OneError()
}

func (c *Client) RemoveUser(tag string) error {
	u := params.Entity{Tag: tag}
	p := params.Entities{Entities: []params.Entity{u}}
	results := new(params.ErrorResults)
	err := c.call("RemoveUser", p, results)
	if err != nil {
		return err
	}
	return results.OneError()
}
