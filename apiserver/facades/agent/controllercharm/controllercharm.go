// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

const (
	// JujuMetricsUserPrefix defines the "namespace" in which this facade is
	// allowed to create/remove users.
	JujuMetricsUserPrefix = "juju-metrics-"

	// UserCreator is the listed "creator" of metrics users in state.
	UserCreator = "admin"
)

// ControllerCharmAPI provides API methods to the controllercharm worker.
type ControllerCharmAPI struct {
	state backend
}

// backend defines the state methods that ControllerCharmAPI needs.
type backend interface {
	AddUser(name string, displayName string, password string, creator string) (*state.User, error)
	RemoveUser(tag names.UserTag) error
	Model() (*state.Model, error)
}

// AddMetricsUser creates a user with the given username and password, and
// grants the new user permission to read the metrics endpoint.
func (api *ControllerCharmAPI) AddMetricsUser(args params.AddUsers) (params.AddUserResults, error) {
	var results params.AddUserResults
	for _, user := range args.Users {
		var tag string
		err := api.addMetricsUser(user)
		if err == nil {
			tag = user.Username
		}

		results.Results = append(results.Results, params.AddUserResult{
			Tag:   tag,
			Error: apiservererrors.ServerError(err),
		})
	}
	return results, nil
}

func (api *ControllerCharmAPI) addMetricsUser(args params.AddUser) error {
	if !strings.HasPrefix(args.Username, JujuMetricsUserPrefix) {
		return errors.NotValidf("username %q missing prefix %q", args.Username, JujuMetricsUserPrefix)
	}

	if args.Password == "" {
		return errors.NotValidf("empty password for user %q", args.Username)
	}

	_, err := api.state.AddUser(args.Username, args.DisplayName, args.Password, UserCreator)
	if err != nil {
		return errors.Annotatef(err, "failed to create user %q", args.Username)
	}

	model, err := api.state.Model()
	if err != nil {
		return errors.Annotatef(err, "retrieving current model")
	}

	userTag := names.NewUserTag(args.Username)
	_, err = model.AddUser(state.UserAccessSpec{
		User:      userTag,
		CreatedBy: names.NewUserTag(UserCreator),
		Access:    permission.ReadAccess, // TODO: restrict access further?
	})
	return errors.Annotatef(err, "adding user %q to model %q",
		args.Username, bootstrap.ControllerModelName)
}

// RemoveMetricsUser removes the given user from the controller.
func (api *ControllerCharmAPI) RemoveMetricsUser(entities params.Entities) (params.ErrorResults, error) {
	var results params.ErrorResults
	for _, e := range entities.Entities {
		err := api.removeMetricsUser(e)
		results.Results = append(results.Results, params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		})
	}
	return results, nil
}

func (api *ControllerCharmAPI) removeMetricsUser(e params.Entity) error {
	user, err := names.ParseUserTag(e.Tag)
	if err != nil {
		return errors.Annotatef(err, "couldn't parse user tag %q", e.Tag)
	}

	if !strings.HasPrefix(user.Name(), JujuMetricsUserPrefix) {
		return fmt.Errorf("username %q should have prefix %q", user.Name(), JujuMetricsUserPrefix)
	}

	err = api.state.RemoveUser(user)
	if err != nil && !errors.Is(err, errors.UserNotFound) {
		return errors.Annotatef(err, "failed to delete user %q", user.Name())
	}
	return nil
}
