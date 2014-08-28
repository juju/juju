// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"
	"math/rand"
	"net/url"
	"time"

	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v3"
	charmtesting "gopkg.in/juju/charm.v3/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

const (
	symbols = "abcdefghijklmopqrstuvwxyz"
)

type Factory struct {
	st    *state.State
	index int
}

func NewFactory(st *state.State) *Factory {
	return &Factory{st: st}
}

type UserParams struct {
	Name        string
	DisplayName string
	Password    string
	Creator     string
}

// EnvUserParams defines the parameters for creating an environment user.
type EnvUserParams struct {
	User        string
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

type MetricParams struct {
	Unit    *state.Unit
	Time    *time.Time
	Metrics []*state.Metric
	Sent    bool
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
func (factory *Factory) MakeUser(c *gc.C, params *UserParams) *state.User {
	if params == nil {
		params = &UserParams{}
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
	c.Assert(err, gc.IsNil)
	return user
}

// MakeEnvUser will create a envUser with values defined by the params. For
// attributes of EnvUserParams that are the default empty values, some
// meaningful valid values are used instead. If params is not specified,
// defaults are used.
func (factory *Factory) MakeEnvUser(c *gc.C, params *EnvUserParams) *state.EnvironmentUser {
	if params == nil {
		params = &EnvUserParams{}
	}
	if params.User == "" {
		user := factory.MakeUser(c, nil)
		params.User = user.UserTag().Username()
	}
	if params.DisplayName == "" {
		params.DisplayName = factory.UniqueString("display name")
	}
	if params.CreatedBy == "" {
		user := factory.MakeUser(c, nil)
		params.CreatedBy = user.UserTag().Username()
	}

	envUser, err := factory.st.AddEnvironmentUser(names.NewUserTag(params.User), names.NewUserTag(params.CreatedBy), params.DisplayName)
	c.Assert(err, gc.IsNil)
	return envUser
}

// MakeMachine will add a machine with values defined in params. For some
// values in params, if they are missing, some meaningful empty values will be
// set.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeMachine(c *gc.C, params *MachineParams) *state.Machine {
	if params == nil {
		params = &MachineParams{}
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
		c.Assert(err, gc.IsNil)
	}
	machine, err := factory.st.AddMachine(params.Series, params.Jobs...)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned(params.InstanceId, params.Nonce, params.Characteristics)
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(params.Password)
	c.Assert(err, gc.IsNil)
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
func (factory *Factory) MakeCharm(c *gc.C, params *CharmParams) *state.Charm {
	if params == nil {
		params = &CharmParams{}
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
	c.Assert(err, gc.IsNil)
	charm, err := factory.st.AddCharm(ch, curl, bundleURL, bundleSHA256)

	c.Assert(err, gc.IsNil)
	return charm
}

// MakeService creates a service with the specified parameters, substituting
// sane defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeService(c *gc.C, params *ServiceParams) *state.Service {
	if params == nil {
		params = &ServiceParams{}
	}
	if params.Charm == nil {
		params.Charm = factory.MakeCharm(c, nil)
	}
	if params.Name == "" {
		params.Name = params.Charm.Meta().Name
	}
	if params.Creator == "" {
		creator := factory.MakeUser(c, nil)
		params.Creator = creator.Tag().String()
	}
	service, err := factory.st.AddService(params.Name, params.Creator, params.Charm, nil)
	c.Assert(err, gc.IsNil)
	return service
}

// MakeUnit creates a service unit with specified params, filling in
// sane defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeUnit(c *gc.C, params *UnitParams) *state.Unit {
	if params == nil {
		params = &UnitParams{}
	}
	if params.Machine == nil {
		params.Machine = factory.MakeMachine(c, nil)
	}
	if params.Service == nil {
		params.Service = factory.MakeService(c, nil)
	}
	unit, err := params.Service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(params.Machine)
	c.Assert(err, gc.IsNil)
	serviceCharmURL, _ := params.Service.CharmURL()
	err = unit.SetCharmURL(serviceCharmURL)
	c.Assert(err, gc.IsNil)
	return unit
}

// MakeMetric makes a metric with specified params, filling in
// sane defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params stuct is passed to the function, it panics.
func (factory *Factory) MakeMetric(c *gc.C, params *MetricParams) *state.MetricBatch {
	now := time.Now().Round(time.Second).UTC()
	if params == nil {
		params = &MetricParams{}
	}
	if params.Unit == nil {
		params.Unit = factory.MakeUnit(c, nil)
	}
	if params.Time == nil {
		params.Time = &now
	}
	if params.Metrics == nil {
		params.Metrics = []*state.Metric{state.NewMetric(factory.UniqueString("metric"), factory.UniqueString(""), now, []byte("creds"))}
	}

	metric, err := params.Unit.AddMetrics(params.Metrics)
	c.Assert(err, gc.IsNil)
	if params.Sent {
		err := metric.SetSent()
		c.Assert(err, gc.IsNil)
	}
	return metric
}

// MakeRelation create a relation with specified params, filling in sane
// defaults for missing values.
// If params is not specified, defaults are used. If more than one
// params struct is passed to the function, it panics.
func (factory *Factory) MakeRelation(c *gc.C, params *RelationParams) *state.Relation {
	if params == nil {
		params = &RelationParams{}
	}
	if len(params.Endpoints) == 0 {
		s1 := factory.MakeService(c, &ServiceParams{
			Charm: factory.MakeCharm(c, &CharmParams{
				Name: "mysql",
			}),
		})
		e1, err := s1.Endpoint("server")
		c.Assert(err, gc.IsNil)

		s2 := factory.MakeService(c, &ServiceParams{
			Charm: factory.MakeCharm(c, &CharmParams{
				Name: "wordpress",
			}),
		})
		e2, err := s2.Endpoint("db")
		c.Assert(err, gc.IsNil)

		params.Endpoints = []state.Endpoint{e1, e2}
	}

	relation, err := factory.st.AddRelation(params.Endpoints...)
	c.Assert(err, gc.IsNil)

	return relation
}
