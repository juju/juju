// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type Factory struct {
	st    *state.State
	c     *gc.C
	index int
}

func NewFactory(st *state.State, c *gc.C) *Factory {
	return &Factory{st: st, c: c}
}

type UserParams struct {
	Username    string
	DisplayName string
	Password    string
	Creator     string
}

// IdentityParams provides the optional values for the Factory.MakeIdentity method.
type IdentityParams struct {
	Name        string
	DisplayName string
	Password    string
	Creator     string
}

func (factory *Factory) UniqueInteger() int {
	factory.index++
	return factory.index
}

func (factory *Factory) UniqueString(prefix string) string {
	if prefix == "" {
		prefix = "no-prefix"
	}
	return fmt.Sprintf("%s-%d", prefix, factory.UniqueInteger())
}

// MakeAnyUser will create a user with no specified values.
func (factory *Factory) MakeAnyUser() *state.User {
	return factory.MakeUser(UserParams{})
}

// MakeUser will create a user with values defined by the params.
// For attributes of UserParams that are the default empty values,
// some meaningful valid values are used instead.
func (factory *Factory) MakeUser(params UserParams) *state.User {
	if params.Username == "" {
		params.Username = factory.UniqueString("username")
	}
	if params.DisplayName == "" {
		params.DisplayName = factory.UniqueString("display name")
	}
	if params.Password == "" {
		params.Password = "password"
	}
	if params.Creator == "" {
		params.Creator = "admin"
	}
	user, err := factory.st.AddUser(
		params.Username, params.DisplayName, params.Password, params.Creator)
	factory.c.Assert(err, gc.IsNil)
	return user
}

// MakeAnyIdentity will create an identity with no specified values.
func (factory *Factory) MakeAnyIdentity() *state.Identity {
	return factory.MakeIdentity(IdentityParams{})
}

// MakeIdentity will create an identity with values defined by the params.
// For attributes of IdentityParams that are the default empty values,
// some meaningful valid values are used instead.
func (factory *Factory) MakeIdentity(params IdentityParams) *state.Identity {
	if params.Name == "" {
		params.Name = factory.UniqueString("name")
	}
	if params.DisplayName == "" {
		params.DisplayName = factory.UniqueString("display name")
	}
	if params.Password == "" {
		params.Password = "password"
	}
	if params.Creator == "" {
		params.Creator = state.AdminIdentity
	}
	identity, err := factory.st.AddIdentity(
		params.Name, params.DisplayName, params.Password, params.Creator)
	factory.c.Assert(err, gc.IsNil)
	return identity
}

// Params for creating a machine.
type MachineParams struct {
	Series          string
	Jobs            []state.MachineJob
	Password        string
	Nonce           string
	Id              instance.Id
	Characteristics *instance.HardwareCharacteristics
}

// MakeMachine will add a machine with values defined in params. For some
// values in params, if they are missing, some meaningful empty values will be
// set.
func (factory *Factory) MakeMachine(params MachineParams) *state.Machine {
	if params.Series == "" {
		params.Series = "precise"
	}
	if params.Nonce == "" {
		params.Nonce = "nonce"
	}
	if len(params.Jobs) == 0 {
		params.Jobs = []state.MachineJob{state.JobHostUnits}
	}
	if params.Id == "" {
		params.Id = instance.Id(factory.UniqueString("id"))
	}
	machine, err := factory.st.AddMachine(params.Series, params.Jobs...)
	factory.c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned(params.Id, params.Nonce, params.Characteristics)
	factory.c.Assert(err, gc.IsNil)
	err = machine.SetPassword(params.Password)
	factory.c.Assert(err, gc.IsNil)
	return machine
}

// MakeAnyMachine will create a machine with no params specified.
func (factory *Factory) MakeAnyMachine() *state.Machine {
	return factory.MakeMachine(MachineParams{})
}
