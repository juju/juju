// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/usermanager"
)

// TODO(mattyw) 2014-03-07 bug #1288750
// Need a SetPassword method.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "UserManager")
	return &Client{ClientFacade: frontend, facade: backend}
}

func (c *Client) AddUser(username, displayName, password string) error {
	if !names.IsValidUser(username) {
		return fmt.Errorf("invalid user name %q", username)
	}
	userArgs := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{Username: username, DisplayName: displayName, Password: password}},
	}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("AddUser", userArgs, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

func (c *Client) RemoveUser(tag string) error {
	u := params.Entity{Tag: tag}
	p := params.Entities{Entities: []params.Entity{u}}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("RemoveUser", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

func (c *Client) UserInfo(username string) (usermanager.UserInfoResult, error) {
	u := params.Entity{Tag: username}
	p := params.Entities{Entities: []params.Entity{u}}
	results := new(usermanager.UserInfoResults)
	err := c.facade.FacadeCall("UserInfo", p, results)
	if err != nil {
		return usermanager.UserInfoResult{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return usermanager.UserInfoResult{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return usermanager.UserInfoResult{}, errors.Trace(err)
	}
	return result, nil
}

func (c *Client) SetPassword(username, password string) error {
	userArgs := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{
			Username: username,
			Password: password}},
	}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("SetPassword", userArgs, results)
	if err != nil {
		return err
	}
	return results.OneError()
}
