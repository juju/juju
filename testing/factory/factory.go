// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"
	"math/rand"
	"net/url"

	"github.com/juju/names"
	"github.com/juju/utils"
	charm "gopkg.in/juju/charm.v2"
	charmtesting "gopkg.in/juju/charm.v2/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/utils"
)

const (
	symbols = "abcdefghijklmopqrstuvwxyz"
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
	Name        string
	DisplayName string
	Password    string
	Creator     string
}

type EnvUserParams struct {
	User        string
	Alias       string
	DisplayName string
	CreatedBy   string
}

// CharmParams defines the parameters for creating a charm.
type CharmParams struct {
	Name     string
	Series   string
	Revision string
	URL      string
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

// ServiceParams is used when specifying parameters for a new service.
type ServiceParams struct {
	Name    string
	Charm   *state.Charm
	Creator string
}

// UnitParams are used to create units.
type UnitParams struct {
	Service *state.Service
	Machine *state.Machine
}

// RelationParams are used to create relations.
type RelationParams struct {
	Endpoints []state.Endpoint
}

// RandomSuffix adds a random 5 character suffix to the presented string.
func (*Factory) RandomSuffix(prefix string) string {
	result := prefix
	for i := 0; i < 5; i++ {
		result += string(symbols[rand.Intn(len(symbols))])
	}
	return result
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
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeUser(vParams ...UserParams) *state.User {
	params := UserParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
	}
	if params.Name == "" {
		params.Name = factory.UniqueString("username")
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
		params.Name, params.DisplayName, params.Password, params.Creator)
	factory.c.Assert(err, gc.IsNil)
	return user
}

// MakeEnvUser will create a envUser with values defined by the params.
// For attributes of EnvUserParams that are the default empty values,
// some meaningful valid values are used instead.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeEnvUser(vParams ...EnvUserParams) *state.EnvironmentUser {
	params := EnvUserParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
	}
	if params.User == "" {
		params.User = factory.UniqueString("user")
	}
	if params.Alias == "" {
		params.Alias = "alias"
	}
	if params.DisplayName == "" {
		params.DisplayName = factory.UniqueString("display name")
	}
	if params.CreatedBy == "" {
		params.CreatedBy = "created-by"
	}
	envUser, err := factory.st.AddEnvironmentUser(params.User, params.DisplayName, params.Alias, params.CreatedBy)
	factory.c.Assert(err, gc.IsNil)
	return envUser
}

// MakeMachine will add a machine with values defined in params. For some
// values in params, if they are missing, some meaningful empty values will be
// set.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeMachine(vParams ...MachineParams) *state.Machine {
	params := MachineParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
	}
	if params.Series == "" {
		params.Series = "quantal"
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
	if params.Password == "" {
		var err error
		params.Password, err = utils.RandomPassword()
		factory.c.Assert(err, gc.IsNil)
	}
	machine, err := factory.st.AddMachine(params.Series, params.Jobs...)
	factory.c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned(params.InstanceId, params.Nonce, params.Characteristics)
	factory.c.Assert(err, gc.IsNil)
	err = machine.SetPassword(params.Password)
	factory.c.Assert(err, gc.IsNil)
	return machine
}

// MakeCharm creates a charm with the values specified in params.
// Sensible default values are substituted for missing ones.
// Supported charms depend on the charm/testing package.
// Currently supported charms:
//   all-hooks, category, dummy, format2, logging, monitoring, mysql,
//   mysql-alternative, riak, terracotta, upgrade1, upgrade2, varnish,
//   varnish-alternative, wordpress.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeCharm(vParams ...CharmParams) *state.Charm {
	params := CharmParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
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

	ch := charmtesting.Charms.CharmDir(params.Name)

	curl := charm.MustParseURL(params.URL)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	bundleSHA256 := factory.UniqueString("bundlesha")
	factory.c.Assert(err, gc.IsNil)
	charm, err := factory.st.AddCharm(ch, curl, bundleURL, bundleSHA256)

	factory.c.Assert(err, gc.IsNil)
	return charm
}

// MakeService creates a service with the specified parameters, substituting
// sane defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeService(vParams ...ServiceParams) *state.Service {
	params := ServiceParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
	}

	if params.Charm == nil {
		params.Charm = factory.MakeCharm()
	}
	if params.Name == "" {
		params.Name = params.Charm.Meta().Name
	}
	if params.Creator == "" {
		creator := factory.MakeUser()
		params.Creator = creator.Tag().String()
	}
	service, err := factory.st.AddService(params.Name, params.Creator, params.Charm, nil)
	factory.c.Assert(err, gc.IsNil)
	return service
}

// MakeUnit creates a service unit with specified params, filling in
// sane defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeUnit(vParams ...UnitParams) *state.Unit {
	params := UnitParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
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

// MakeRelation create a relation with specified params, filling in sane
// defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeRelation(vParams ...RelationParams) *state.Relation {
	params := RelationParams{}
	if len(vParams) == 1 {
		params = vParams[0]
	} else if len(vParams) > 1 {
		panic("expecting 1 parameter or none")
	}

	if len(params.Endpoints) == 0 {
		s1 := factory.MakeService(ServiceParams{
			Charm: factory.MakeCharm(CharmParams{
				Name: "mysql",
			}),
		})
		e1, err := s1.Endpoint("server")
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

// Returns a new uuid
func (factory *Factory) NewUUID() string {
	uuid, err := utils.NewUUID()
	factory.c.Assert(err, gc.IsNil)
	return uuid.String()
}

// EnvironTag returns the environtag for the state this factory uses
func (factory *Factory) EnvironTag() names.EnvironTag {
	return factory.st.EnvironTag()
}
