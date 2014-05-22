// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
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

func (c *Client) AddUser(tag, password string) error {
	if !names.IsUser(tag) {
		return fmt.Errorf("invalid user name %q", tag)
	}
	u := params.EntityPassword{Tag: tag, Password: password}
	p := params.EntityPasswords{Changes: []params.EntityPassword{u}}
	results := new(params.ErrorResults)
	err := c.call("AddUser", p, results)
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
