// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"
	"net/url"

	"github.com/juju/charm"
	charmtesting "github.com/juju/charm/testing"
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

// CharmParams defines the parameters for creating a charm.
type CharmParams struct {
	Name     string
	Series   string
	Revision string
	URL      string
}

// MakeCharm creates a charm with the values specified in params.
// Sensible default values are substituted for missing ones.
func (factory *Factory) MakeCharm(params CharmParams) *state.Charm {
	if params.Name == "" {
		params.Name = "mysql"
	}
	if params.Series == "" {
		params.Series = "quantal"
	}
	if params.Revision == "" {
		params.Revision = fmt.Sprintf("%d", factory.UniqueInteger())
	}
	if params.URL == "" {
		params.URL = fmt.Sprintf("cs:%s/%s-%s", params.Series, params.Name, params.Revision)
	}

	ch := charmtesting.Charms.Dir(params.Name)

	curl := charm.MustParseURL(params.URL)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	bundleSHA256 := factory.UniqueString("bundlesha")
	factory.c.Assert(err, gc.IsNil)

	charm, err := factory.st.AddCharm(ch, curl, bundleURL, bundleSHA256)
	factory.c.Assert(err, gc.IsNil)
	return charm
}

// MakeAnyCharm will create a charm with no parameters specified.
func (factory *Factory) MakeAnyCharm() *state.Charm {
	return factory.MakeCharm(CharmParams{})
}

// ServiceParams is used when specifying parameters for a new service.
type ServiceParams struct {
	Name    string
	Charm   *state.Charm
	Creator string
}

// MakeService creates a service with the specified parameters, substituting
// sane defaults for missing values.
func (factory *Factory) MakeService(params ServiceParams) *state.Service {
	if params.Name == "" {
		params.Name = factory.UniqueString("mysql")
	}
	if params.Charm == nil {
		params.Charm = factory.MakeAnyCharm()
	}
	if params.Creator == "" {
		creator := factory.MakeAnyUser()
		params.Creator = creator.Tag().String()
	}
	service, err := factory.st.AddService(params.Name, params.Creator, params.Charm, nil)
	factory.c.Assert(err, gc.IsNil)
	return service
}

// MakeAnyService creates a service with an empty params struct.
func (factory *Factory) MakeAnyService() *state.Service {
	return factory.MakeService(ServiceParams{})
}

// UnitParams are used to create units.
type UnitParams struct {
	Service *state.Service
}

// MakeUnit creates a service unit with specified params, filling in
// sane defaults for missing values.
func (factory *Factory) MakeUnit(params UnitParams) *state.Unit {
	if params.Service == nil {
		params.Service = factory.MakeAnyService()
	}
	unit, err := params.Service.AddUnit()
	factory.c.Assert(err, gc.IsNil)
	return unit
}

// MakeAnyUnit creates a unit with empty params.
func (factory *Factory) MakeAnyUnit() *state.Unit {
	return factory.MakeUnit(UnitParams{})
}
