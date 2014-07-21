// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
)

// TODO(mattyw) 2014-03-07 bug #1288750
// Need a SetPassword method.
type Client struct {
	st *api.State
}

var call = func(st *api.State, method string, params, result interface{}) error {
	return st.Call("UserManager", "", method, params, result)
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
	err := call(c.st, "AddUser", userArgs, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

func (c *Client) RemoveUser(tag string) error {
	u := params.Entity{Tag: tag}
	p := params.Entities{Entities: []params.Entity{u}}
	results := new(params.ErrorResults)
	err := call(c.st, "RemoveUser", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

func (c *Client) UserInfo(username string) (UserInfoResult, error) {
	u := params.Entity{Tag: username}
	p := params.Entities{Entities: []params.Entity{u}}
	results := new(UserInfoResults)
	err := call(c.st, "UserInfo", p, results)
	if err != nil {
		return UserInfoResult{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return UserInfoResult{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return UserInfoResult{}, errors.Trace(err)
	}
	return result, nil
}

func (c *Client) SetPassword(username, password string) error {
	userArgs := params.ModifyUsers{
		Changes: []params.ModifyUser{{
			Username: username,
			Password: password}},
	}
	results := new(params.ErrorResults)
	err := call(c.st, "SetPassword", userArgs, results)
	if err != nil {
		return err
	}
	return results.OneError()
}

type UserInfo struct {
	Username       string     `json:username`
	DisplayName    string     `json:display-name`
	CreatedBy      string     `json:created-by`
	DateCreated    time.Time  `json:date-created`
	LastConnection *time.Time `json:last-connection`
}

// UserInfoResult holds the result of a UserInfo call.
type UserInfoResult struct {
	Result *UserInfo     `json:result,omitempty`
	Error  *params.Error `json:error,omitempty`
}

type UserInfoResults struct {
	Results []UserInfoResult
}
