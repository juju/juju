// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "ControllerCharm")}
}

type Client struct {
	caller base.FacadeCaller
}

// AddMetricsUser creates a user with the given username and password, and
// grants the new user permission to read the metrics endpoint.
func (c *Client) AddMetricsUser(username, password string) error {
	var result params.AddUserResults

	args := params.AddUsers{[]params.AddUser{{
		Username:    username,
		DisplayName: username,
		Password:    password,
	}}}
	err := c.caller.FacadeCall("AddMetricsUser", args, &result)
	if err != nil {
		return errors.Annotate(err, "making AddMetricsUser facade call")
	}

	if count := len(result.Results); count != 1 {
		return errors.Errorf("expected 1 result, got %d", count)
	}
	if err := result.Results[0].Error; err != nil {
		translatedErr := params.TranslateWellKnownError(err)
		return errors.Annotate(translatedErr, "AddMetricsUser facade call failed")
	}
	return nil
}

// RemoveMetricsUser removes the given user from the controller.
func (c *Client) RemoveMetricsUser(username string) error {
	if !names.IsValidUser(username) {
		return errors.NotValidf("username %q", username)
	}
	tag := names.NewUserTag(username)

	var results params.ErrorResults
	args := params.Entities{
		[]params.Entity{{tag.String()}},
	}
	err := c.caller.FacadeCall("RemoveMetricsUser", args, &results)
	if err != nil {
		return errors.Annotate(err, "making RemoveMetricsUser facade call")
	}
	if err := results.OneError(); err != nil {
		translatedErr := params.TranslateWellKnownError(err)
		return errors.Annotate(translatedErr, "RemoveMetricsUser facade call failed")
	}
	return nil
}
