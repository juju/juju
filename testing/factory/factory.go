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

// MakeUser will create a user with values defined by the params.
// For attributes of UserParams that are the default empty values,
// some meaningful valid values are used instead.
func (factory *Factory) MakeUser(vParams ...UserParams) *state.User {
	params := UserParams{}
	if len(vParams) > 0 {
		params = vParams[0]
	}
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
	InstanceId      instance.Id
	Characteristics *instance.HardwareCharacteristics
}

// MakeMachine will add a machine with values defined in params. For some
// values in params, if they are missing, some meaningful empty values will be
// set.
func (factory *Factory) MakeMachine(vParams ...MachineParams) *state.Machine {
	params := MachineParams{}
	if len(vParams) > 0 {
		params = vParams[0]
	}
	if params.Series == "" {
		params.Series = "trusty"
	}
	if params.Nonce == "" {
		params.Nonce = "nonce"
	}
	if len(params.Jobs) == 0 {
		params.Jobs = []state.MachineJob{state.JobHostUnits}
	}
	if params.InstanceId == "" {
		params.InstanceId = instance.Id(factory.UniqueString("id"))
	}
	machine, err := factory.st.AddMachine(params.Series, params.Jobs...)
	factory.c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned(params.InstanceId, params.Nonce, params.Characteristics)
	factory.c.Assert(err, gc.IsNil)
	err = machine.SetPassword(params.Password)
	factory.c.Assert(err, gc.IsNil)
	return machine
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
// Supported charms depend on the github.com/juju/charm/testing package.
// Currently supported charms:
//   all-hooks, category, dummy, format2, logging, monitoring, mysql,
//   mysql-alternative, riak, terracotta, upgrade1, upgrade2, varnish,
//   varnish-alternative, wordpress.
func (factory *Factory) MakeCharm(vParams ...CharmParams) *state.Charm {
	params := CharmParams{}
	if len(vParams) > 0 {
		params = vParams[0]
	}
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

// ServiceParams is used when specifying parameters for a new service.
type ServiceParams struct {
	Name    string
	Charm   *state.Charm
	Creator string
}

// MakeService creates a service with the specified parameters, substituting
// sane defaults for missing values.
func (factory *Factory) MakeService(vParams ...ServiceParams) *state.Service {
	params := ServiceParams{}
	if len(vParams) > 0 {
		params = vParams[0]
	}

	if params.Name == "" {
		params.Name = factory.UniqueString("mysql")
	}
	if params.Charm == nil {
		params.Charm = factory.MakeCharm()
	}
	if params.Creator == "" {
		creator := factory.MakeUser()
		params.Creator = creator.Tag().String()
	}
	service, err := factory.st.AddService(params.Name, params.Creator, params.Charm, nil)
	factory.c.Assert(err, gc.IsNil)
	return service
}

// UnitParams are used to create units.
type UnitParams struct {
	Service *state.Service
	Machine *state.Machine
}

// MakeUnit creates a service unit with specified params, filling in
// sane defaults for missing values.
func (factory *Factory) MakeUnit(vParams ...UnitParams) *state.Unit {
	params := UnitParams{}
	if len(vParams) > 0 {
		params = vParams[0]
	}

	if params.Machine == nil {
		params.Machine = factory.MakeMachine()
	}
	if params.Service == nil {
		params.Service = factory.MakeService()
	}
	unit, err := params.Service.AddUnit()
	factory.c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(params.Machine)
	factory.c.Assert(err, gc.IsNil)
	return unit
}

// RelationParams are used to create relations.
type RelationParams struct {
	Endpoints []state.Endpoint
}

// MakeRelation create a relation with specified params, filling in sane
// defaults for missing values.
func (factory *Factory) MakeRelation(vParams ...RelationParams) *state.Relation {
	params := RelationParams{}
	if len(vParams) > 0 {
		params = vParams[0]
	}

	if len(params.Endpoints) == 0 {
		s1 := factory.MakeService(ServiceParams{
			Charm: factory.MakeCharm(CharmParams{
				Name: "mysql",
			}),
		})
		e1, err := s1.Endpoint("db")
		factory.c.Assert(err, gc.IsNil)

		s2 := factory.MakeService(ServiceParams{
			Charm: factory.MakeCharm(CharmParams{
				Name: "wordpress",
			}),
		})
		e2, err := s2.Endpoint("db")
		factory.c.Assert(err, gc.IsNil)

		params.Endpoints = []state.Endpoint{e1, e2}
	}

	relation, err := factory.st.AddRelation(params.Endpoints...)
	factory.c.Assert(err, gc.IsNil)

	return relation
}
