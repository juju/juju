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
	// jujuMetricsUserPrefix defines the "namespace" in which this facade is
	// allowed to create/remove users.
	jujuMetricsUserPrefix = "juju-metrics-"

	// userCreator is the listed "creator" of metrics users in state.
	userCreator = "admin"
)

// API provides API methods to the controllercharm worker.
type API struct {
	state backend
}

// backend defines the state methods that API needs.
type backend interface {
	AddUser(name string, displayName string, password string, creator string) (*state.User, error)
	RemoveUser(tag names.UserTag) error
	Model() (model, error)
}

// model defines the model methods that API needs.
type model interface {
	AddUser(state.UserAccessSpec) (permission.UserAccess, error)
}

// stateShim allows the real state to implement backend.
type stateShim struct {
	*state.State
}

func (s stateShim) Model() (model, error) {
	return s.State.Model()
}

// AddMetricsUser creates a user with the given username and password, and
// grants the new user permission to read the metrics endpoint.
func (api *API) AddMetricsUser(args params.AddUsers) (params.AddUserResults, error) {
	var results params.AddUserResults
	for _, user := range args.Users {
		err := api.addMetricsUser(user)
		results.Results = append(results.Results, params.AddUserResult{
			Tag:   user.Username,
			Error: apiservererrors.ServerError(err),
		})
	}
	return results, nil
}

func (api *API) addMetricsUser(args params.AddUser) error {
	if !strings.HasPrefix(args.Username, jujuMetricsUserPrefix) {
		return errors.NotValidf("username %q missing prefix %q", args.Username, jujuMetricsUserPrefix)
	}

	if args.Password == "" {
		return errors.NotValidf("empty password for user %q", args.Username)
	}

	_, err := api.state.AddUser(args.Username, args.DisplayName, args.Password, userCreator)
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
		CreatedBy: names.NewUserTag(userCreator),
		Access:    permission.ReadAccess,
	})
	return errors.Annotatef(err, "adding user %q to model %q",
		args.Username, bootstrap.ControllerModelName)
}

// RemoveMetricsUser removes the given user from the controller.
func (api *API) RemoveMetricsUser(entities params.Entities) (params.ErrorResults, error) {
	var results params.ErrorResults
	for _, e := range entities.Entities {
		err := api.removeMetricsUser(e)
		results.Results = append(results.Results, params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		})
	}
	return results, nil
}

func (api *API) removeMetricsUser(e params.Entity) error {
	user, err := names.ParseUserTag(e.Tag)
	if err != nil {
		return errors.Annotatef(err, "couldn't parse user tag %q", e.Tag)
	}

	if !strings.HasPrefix(user.Name(), jujuMetricsUserPrefix) {
		return fmt.Errorf("username %q should have prefix %q", user.Name(), jujuMetricsUserPrefix)
	}

	err = api.state.RemoveUser(user)
	if err != nil && !errors.Is(err, errors.UserNotFound) {
		return errors.Annotatef(err, "failed to delete user %q", user.Name())
	}
	return nil
}
