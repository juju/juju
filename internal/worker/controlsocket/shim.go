// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

// State defines the state methods that the controlsocket worker needs.
type State interface {
	User(tag names.UserTag) (user, error)
	AddUser(name string, displayName string, password string, creator string) (user, error)
	RemoveUser(tag names.UserTag) error
	Model() (model, error)
}

// stateShim allows the real state to implement State.
type stateShim struct {
	st *state.State
}

func (s stateShim) User(tag names.UserTag) (user, error) {
	u, err := s.st.User(tag)
	return u, errors.Trace(err)
}

func (s stateShim) AddUser(name, displayName, password, creator string) (user, error) {
	u, err := s.st.AddUser(name, displayName, password, creator)
	return u, errors.Trace(err)
}

func (s stateShim) Model() (model, error) {
	m, err := s.st.Model()
	return m, errors.Trace(err)
}

func (s stateShim) RemoveUser(tag names.UserTag) error {
	return errors.Trace(s.st.RemoveUser(tag))
}

// model defines the model methods that the controlsocket worker needs.
type model interface {
	AddUser(state.UserAccessSpec) (permission.UserAccess, error)
}

// user defines the user methods that the controlsocket worker needs.
type user interface {
	Name() string
	CreatedBy() string
	UserTag() names.UserTag
	PasswordValid(string) bool
}
