// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"
	"strings"

	"gopkg.in/macaroon.v1"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.usermanager")

// Client provides methods that the Juju client command uses to interact
// with users stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "UserManager")
	return &Client{ClientFacade: frontend, facade: backend}
}

// AddUser creates a new local user in the controller, sharing with that user any specified models.
func (c *Client) AddUser(
	username, displayName, password, access string, modelUUIDs ...string,
) (_ names.UserTag, secretKey []byte, _ error) {
	if !names.IsValidUser(username) {
		return names.UserTag{}, nil, fmt.Errorf("invalid user name %q", username)
	}
	modelTags := make([]string, len(modelUUIDs))
	for i, uuid := range modelUUIDs {
		modelTags[i] = names.NewModelTag(uuid).String()
	}

	var accessPermission params.ModelAccessPermission
	var err error
	if len(modelTags) > 0 {
		accessPermission, err = modelmanager.ParseModelAccess(access)
		if err != nil {
			return names.UserTag{}, nil, errors.Trace(err)
		}
	}

	userArgs := params.AddUsers{
		Users: []params.AddUser{{
			Username:        username,
			DisplayName:     displayName,
			Password:        password,
			SharedModelTags: modelTags,
			ModelAccess:     accessPermission}},
	}
	var results params.AddUserResults
	err = c.facade.FacadeCall("AddUser", userArgs, &results)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		logger.Errorf("expected 1 result, got %#v", results)
		return names.UserTag{}, nil, errors.Errorf("expected 1 result, got %d", count)
	}
	result := results.Results[0]
	if result.Error != nil {
		return names.UserTag{}, nil, errors.Trace(result.Error)
	}
	tag, err := names.ParseUserTag(result.Tag)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}
	return tag, result.SecretKey, nil
}

func (c *Client) userCall(username string, methodCall string) error {
	if !names.IsValidUser(username) {
		return errors.Errorf("%q is not a valid username", username)
	}
	tag := names.NewUserTag(username)

	var results params.ErrorResults
	args := params.Entities{
		[]params.Entity{{tag.String()}},
	}
	err := c.facade.FacadeCall(methodCall, args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// DisableUser disables a user.  If the user is already disabled, the action
// is consided a success.
func (c *Client) DisableUser(username string) error {
	return c.userCall(username, "DisableUser")
}

// EnableUser enables a users.  If the user is already enabled, the action is
// consided a success.
func (c *Client) EnableUser(username string) error {
	return c.userCall(username, "EnableUser")
}

// IncludeDisabled is a type alias to avoid bare true/false values
// in calls to the client method.
type IncludeDisabled bool

var (
	// ActiveUsers indicates to only return active users.
	ActiveUsers IncludeDisabled = false
	// AllUsers indicates that both enabled and disabled users should be
	// returned.
	AllUsers IncludeDisabled = true
)

// UserInfo returns information about the specified users.  If no users are
// specified, the call should return all users.  If includeDisabled is set to
// ActiveUsers, only enabled users are returned.
func (c *Client) UserInfo(usernames []string, all IncludeDisabled) ([]params.UserInfo, error) {
	var results params.UserInfoResults
	var entities []params.Entity
	for _, username := range usernames {
		if !names.IsValidUser(username) {
			return nil, errors.Errorf("%q is not a valid username", username)
		}
		tag := names.NewUserTag(username)
		entities = append(entities, params.Entity{Tag: tag.String()})
	}
	args := params.UserInfoRequest{
		Entities:        entities,
		IncludeDisabled: bool(all),
	}
	err := c.facade.FacadeCall("UserInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Only need to look for errors if users were explicitly specified, because
	// if we didn't ask for any, we should get all, and we shouldn't get any
	// errors for listing all.  We care here because we index into the users
	// slice.
	if len(results.Results) == len(usernames) {
		var errorStrings []string
		for i, result := range results.Results {
			if result.Error != nil {
				annotated := errors.Annotate(result.Error, usernames[i])
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

// SetPassword changes the password for the specified user.
func (c *Client) SetPassword(username, password string) error {
	if !names.IsValidUser(username) {
		return errors.Errorf("%q is not a valid username", username)
	}
	tag := names.NewUserTag(username)
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

// CreateLocalLoginMacaroon creates a local login macaroon for the
// authenticated user.
func (c *Client) CreateLocalLoginMacaroon(tag names.UserTag) (*macaroon.Macaroon, error) {
	args := params.Entities{Entities: []params.Entity{{tag.String()}}}
	var results params.MacaroonResults
	if err := c.facade.FacadeCall("CreateLocalLoginMacaroon", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		logger.Errorf("expected 1 result, got %#v", results)
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return result.Result, nil
}
