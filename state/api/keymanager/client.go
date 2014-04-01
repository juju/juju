// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/ssh"
)

// Client provides access to the keymanager, used to add/delete/list authorised ssh keys.
type Client struct {
	st *api.State
}

func (c *Client) call(method string, params, result interface{}) error {
	return c.st.Call("KeyManager", "", method, params, result)
}

// NewClient returns a new keymanager client.
func NewClient(st *api.State) *Client {
	return &Client{st}
}

// Close closes the underlying State connection.
func (c *Client) Close() error {
	return c.st.Close()
}

// ListKeys returns the authorised ssh keys for the specified users.
func (c *Client) ListKeys(mode ssh.ListMode, users ...string) ([]params.StringsResult, error) {
	p := params.ListSSHKeys{Mode: mode}
	p.Entities.Entities = make([]params.Entity, len(users))
	for i, userName := range users {
		p.Entities.Entities[i] = params.Entity{Tag: userName}
	}
	results := new(params.StringsResults)
	err := c.call("ListKeys", p, results)
	return results.Results, err
}

// AddKeys adds the authorised ssh keys for the specified user.
func (c *Client) AddKeys(user string, keys ...string) ([]params.ErrorResult, error) {
	p := params.ModifyUserSSHKeys{User: user, Keys: keys}
	results := new(params.ErrorResults)
	err := c.call("AddKeys", p, results)
	return results.Results, err
}

// DeleteKeys deletes the authorised ssh keys for the specified user.
func (c *Client) DeleteKeys(user string, keys ...string) ([]params.ErrorResult, error) {
	p := params.ModifyUserSSHKeys{User: user, Keys: keys}
	results := new(params.ErrorResults)
	err := c.call("DeleteKeys", p, results)
	return results.Results, err
}

// ImportKeys imports the authorised ssh keys with the specified key ids for the specified user.
func (c *Client) ImportKeys(user string, keyIds ...string) ([]params.ErrorResult, error) {
	p := params.ModifyUserSSHKeys{User: user, Keys: keyIds}
	results := new(params.ErrorResults)
	err := c.call("ImportKeys", p, results)
	return results.Results, err
}
