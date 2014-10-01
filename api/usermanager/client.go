// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.usermanager")

type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "UserManager")
	return &Client{ClientFacade: frontend, facade: backend}
}

func (c *Client) AddUser(username, displayName, password string) (names.UserTag, error) {
	if !names.IsValidUser(username) {
		return names.UserTag{}, fmt.Errorf("invalid user name %q", username)
	}
	userArgs := params.AddUsers{
		Users: []params.AddUser{{Username: username, DisplayName: displayName, Password: password}},
	}
	var results params.AddUserResults
	err := c.facade.FacadeCall("AddUser", userArgs, &results)
	if err != nil {
		return names.UserTag{}, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		logger.Errorf("expected 1 result, got %#v", results)
		return names.UserTag{}, errors.Errorf("expected 1 result, got %d", count)
	}
	result := results.Results[0]
	if result.Error != nil {
		return names.UserTag{}, errors.Trace(result.Error)
	}
	tag, err := names.ParseUserTag(result.Tag)
	if err != nil {
		return names.UserTag{}, errors.Trace(err)
	}
	logger.Infof("created user %s", result.Tag)
	return tag, nil
}

func (c *Client) deactivateUser(tag names.UserTag, deactivate bool) error {
	var results params.ErrorResults
	args := params.DeactivateUsers{
		Users: []params.DeactivateUser{{Tag: tag.String(), Deactivate: deactivate}},
	}
	err := c.facade.FacadeCall("DeactivateUser", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

func (c *Client) DeactivateUser(tag names.UserTag) error {
	return c.deactivateUser(tag, true)
}

func (c *Client) ActivateUser(tag names.UserTag) error {
	return c.deactivateUser(tag, false)
}

func (c *Client) UserInfo(tags []names.UserTag, includeDeactivated bool) ([]params.UserInfo, error) {
	var results params.UserInfoResults
	var entities []params.Entity
	for _, tag := range tags {
		entities = append(entities, params.Entity{Tag: tag.String()})
	}
	args := params.UserInfoRequest{
		Entities:           entities,
		IncludeDeactivated: includeDeactivated,
	}
	err := c.facade.FacadeCall("UserInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Only need to look for errors if tags were explicitly specfied, because
	// if we didn't ask for any, we should get all, and we shouldn't get any
	// errors for listing all.
	if len(tags) > 0 {
		var errorStrings []string
		for i, result := range results.Results {
			if result.Error != nil {
				annotated := errors.Annotate(result.Error, tags[i].Name())
				errorStrings = append(errorStrings, annotated.Error())
			}
		}
		if len(errorStrings) > 0 {
			return nil, errors.New(strings.Join(errorStrings, ", "))
		}
	}
	info := []params.UserInfo{}
	for i, result := range results.Results {
		if result.Result == nil {
			return nil, errors.Errorf("unexpected nil result at position %d", i)
		}
		info = append(info, *result.Result)
	}
	return info, nil
}

func (c *Client) SetPassword(tag names.UserTag, password string) error {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      tag.String(),
			Password: password}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("SetPassword", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}
