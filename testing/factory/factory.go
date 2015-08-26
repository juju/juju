// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"

	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

const (
	symbols = "abcdefghijklmopqrstuvwxyz"
)

type Factory struct {
	st *state.State
}

var index uint32

func NewFactory(st *state.State) *Factory {
	return &Factory{st: st}
}

type UserParams struct {
	Name        string
	DisplayName string
	Password    string
	Creator     names.Tag
	NoEnvUser   bool
	Disabled    bool
}

// EnvUserParams defines the parameters for creating an environment user.
type EnvUserParams struct {
	User        string
	DisplayName string
	CreatedBy   names.Tag
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
	Addresses       []network.Address
	Volumes         []state.MachineVolumeParams
	Filesystems     []state.MachineFilesystemParams
}

// ServiceParams is used when specifying parameters for a new service.
type ServiceParams struct {
	Name    string
	Charm   *state.Charm
	Creator names.Tag
	Status  *state.StatusInfo
}

// UnitParams are used to create units.
type UnitParams struct {
	Service     *state.Service
	Machine     *state.Machine
	Password    string
	SetCharmURL bool
	Status      *state.StatusInfo
}

// RelationParams are used to create relations.
type RelationParams struct {
	Endpoints []state.Endpoint
}

type MetricParams struct {
	Unit    *state.Unit
	Time    *time.Time
	Metrics []state.Metric
	Sent    bool
}

type EnvParams struct {
	Name        string
	Owner       names.Tag
	ConfigAttrs testing.Attrs
	Prepare     bool
}

// RandomSuffix adds a random 5 character suffix to the presented string.
func (*Factory) RandomSuffix(prefix string) string {
	result := prefix
	for i := 0; i < 5; i++ {
		result += string(symbols[rand.Intn(len(symbols))])
	}
	return result
}

func uniqueInteger() int {
	return int(atomic.AddUint32(&index, 1))
}

func uniqueString(prefix string) string {
	if prefix == "" {
		prefix = "no-prefix"
	}
	return fmt.Sprintf("%s-%d", prefix, uniqueInteger())
}

// MakeUser will create a user with values defined by the params.
// For attributes of UserParams that are the default empty values,
// some meaningful valid values are used instead.
// If params is not specified, defaults are used.
func (factory *Factory) MakeUser(c *gc.C, params *UserParams) *state.User {
	if params == nil {
		params = &UserParams{}
	}
	if params.Name == "" {
		params.Name = uniqueString("username")
	}
	if params.DisplayName == "" {
		params.DisplayName = uniqueString("display name")
	}
	if params.Password == "" {
		params.Password = "password"
	}
	if params.Creator == nil {
		env, err := factory.st.Environment()
		c.Assert(err, jc.ErrorIsNil)
		params.Creator = env.Owner()
	}
	creatorUserTag := params.Creator.(names.UserTag)
	user, err := factory.st.AddUser(
		params.Name, params.DisplayName, params.Password, creatorUserTag.Name())
	c.Assert(err, jc.ErrorIsNil)
	if !params.NoEnvUser {
		_, err := factory.st.AddEnvironmentUser(user.UserTag(), names.NewUserTag(user.CreatedBy()), params.DisplayName)
		c.Assert(err, jc.ErrorIsNil)
	}
	if params.Disabled {
		err := user.Disable()
		c.Assert(err, jc.ErrorIsNil)
	}
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
		user := factory.MakeUser(c, &UserParams{NoEnvUser: true})
		params.User = user.UserTag().Username()
	}
	if params.DisplayName == "" {
		params.DisplayName = uniqueString("display name")
	}
	if params.CreatedBy == nil {
		env, err := factory.st.Environment()
		c.Assert(err, jc.ErrorIsNil)
		params.CreatedBy = env.Owner()
	}
	createdByUserTag := params.CreatedBy.(names.UserTag)
	envUser, err := factory.st.AddEnvironmentUser(names.NewUserTag(params.User), createdByUserTag, params.DisplayName)
	c.Assert(err, jc.ErrorIsNil)
	return envUser
}

func (factory *Factory) paramsFillDefaults(c *gc.C, params *MachineParams) *MachineParams {
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
		params.InstanceId = instance.Id(uniqueString("id"))
	}
	if params.Password == "" {
		var err error
		params.Password, err = utils.RandomPassword()
		c.Assert(err, jc.ErrorIsNil)
	}
	return params
}

func machineParamsToTemplate(p *MachineParams) state.MachineTemplate {
	return state.MachineTemplate{
		Series:     p.Series,
		Nonce:      p.Nonce,
		Jobs:       p.Jobs,
		InstanceId: p.InstanceId,
	}
}

// MakeMachineNested will make a machine nested in the machine with ID given.
func (factory *Factory) MakeMachineNested(c *gc.C, parentId string, params *MachineParams) *state.Machine {
	params = factory.paramsFillDefaults(c, params)
	mTmpl := machineParamsToTemplate(params)
	// Cannot specify an instance id for a new container.
	mTmpl.InstanceId = ""
	// Cannot specify a nonce without an instance ID.
	mTmpl.Nonce = ""

	m, err := factory.st.AddMachineInsideMachine(
		mTmpl,
		parentId,
		instance.LXC,
	)
	c.Assert(err, jc.ErrorIsNil)
	return m
}

// MakeMachine will add a machine with values defined in params. For some
// values in params, if they are missing, some meaningful empty values will be
// set.
// If params is not specified, defaults are used.
func (factory *Factory) MakeMachine(c *gc.C, params *MachineParams) *state.Machine {
	machine, _ := factory.MakeMachineReturningPassword(c, params)
	return machine
}

// MakeMachineReturningPassword will add a machine with values defined in
// params. For some values in params, if they are missing, some meaningful
// empty values will be set. If params is not specified, defaults are used.
// The machine and its password are returned.
func (factory *Factory) MakeMachineReturningPassword(c *gc.C, params *MachineParams) (*state.Machine, string) {
	params = factory.paramsFillDefaults(c, params)
	machineTemplate := state.MachineTemplate{
		Series:      params.Series,
		Jobs:        params.Jobs,
		Volumes:     params.Volumes,
		Filesystems: params.Filesystems,
	}
	machine, err := factory.st.AddOneMachine(machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(params.InstanceId, params.Nonce, params.Characteristics)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(params.Password)
	c.Assert(err, jc.ErrorIsNil)
	if len(params.Addresses) > 0 {
		err := machine.SetProviderAddresses(params.Addresses...)
		c.Assert(err, jc.ErrorIsNil)
	}
	return machine, params.Password
}

// MakeCharm creates a charm with the values specified in params.
// Sensible default values are substituted for missing ones.
// Supported charms depend on the charm/testing package.
// Currently supported charms:
//   all-hooks, category, dummy, format2, logging, monitoring, mysql,
//   mysql-alternative, riak, terracotta, upgrade1, upgrade2, varnish,
//   varnish-alternative, wordpress.
// If params is not specified, defaults are used.
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
		params.Revision = fmt.Sprintf("%d", uniqueInteger())
	}
	if params.URL == "" {
		params.URL = fmt.Sprintf("cs:%s/%s-%s", params.Series, params.Name, params.Revision)
	}

	ch := testcharms.Repo.CharmDir(params.Name)

	curl := charm.MustParseURL(params.URL)
	bundleSHA256 := uniqueString("bundlesha")
	charm, err := factory.st.AddCharm(ch, curl, "fake-storage-path", bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)
	return charm
}

// MakeService creates a service with the specified parameters, substituting
// sane defaults for missing values.
// If params is not specified, defaults are used.
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
	if params.Creator == nil {
		creator := factory.MakeUser(c, nil)
		params.Creator = creator.Tag()
	}
	_ = params.Creator.(names.UserTag)
	service, err := factory.st.AddService(params.Name, params.Creator.String(), params.Charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	if params.Status != nil {
		err = service.SetStatus(params.Status.Status, params.Status.Message, params.Status.Data)
		c.Assert(err, jc.ErrorIsNil)
	}

	return service
}

// MakeUnit creates a service unit with specified params, filling in
// sane defaults for missing values.
// If params is not specified, defaults are used.
func (factory *Factory) MakeUnit(c *gc.C, params *UnitParams) *state.Unit {
	unit, _ := factory.MakeUnitReturningPassword(c, params)
	return unit
}

// MakeUnit creates a service unit with specified params, filling in sane
// defaults for missing values. If params is not specified, defaults are used.
// The unit and its password are returned.
func (factory *Factory) MakeUnitReturningPassword(c *gc.C, params *UnitParams) (*state.Unit, string) {
	if params == nil {
		params = &UnitParams{}
	}
	if params.Machine == nil {
		params.Machine = factory.MakeMachine(c, nil)
	}
	if params.Service == nil {
		params.Service = factory.MakeService(c, nil)
	}
	if params.Password == "" {
		var err error
		params.Password, err = utils.RandomPassword()
		c.Assert(err, jc.ErrorIsNil)
	}
	unit, err := params.Service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(params.Machine)
	c.Assert(err, jc.ErrorIsNil)
	if params.SetCharmURL {
		serviceCharmURL, _ := params.Service.CharmURL()
		err = unit.SetCharmURL(serviceCharmURL)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = unit.SetPassword(params.Password)
	c.Assert(err, jc.ErrorIsNil)

	if params.Status != nil {
		err = unit.SetStatus(params.Status.Status, params.Status.Message, params.Status.Data)
		c.Assert(err, jc.ErrorIsNil)
	}

	return unit, params.Password
}

// MakeMetric makes a metric with specified params, filling in
// sane defaults for missing values.
// If params is not specified, defaults are used.
func (factory *Factory) MakeMetric(c *gc.C, params *MetricParams) *state.MetricBatch {
	now := time.Now().Round(time.Second).UTC()
	if params == nil {
		params = &MetricParams{}
	}
	if params.Unit == nil {
		meteredCharm := factory.MakeCharm(c, &CharmParams{Name: "metered", URL: "cs:quantal/metered"})
		meteredService := factory.MakeService(c, &ServiceParams{Charm: meteredCharm})
		params.Unit = factory.MakeUnit(c, &UnitParams{Service: meteredService, SetCharmURL: true})
	}
	if params.Time == nil {
		params.Time = &now
	}
	if params.Metrics == nil {
		params.Metrics = []state.Metric{{"pings", strconv.Itoa(uniqueInteger()), *params.Time}}
	}

	chURL, ok := params.Unit.CharmURL()
	c.Assert(ok, gc.Equals, true)

	metric, err := params.Unit.AddMetrics(utils.MustNewUUID().String(), *params.Time, chURL.String(), params.Metrics)
	c.Assert(err, jc.ErrorIsNil)
	if params.Sent {
		err := metric.SetSent()
		c.Assert(err, jc.ErrorIsNil)
	}
	return metric
}

// MakeRelation create a relation with specified params, filling in sane
// defaults for missing values.
// If params is not specified, defaults are used.
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
		c.Assert(err, jc.ErrorIsNil)

		s2 := factory.MakeService(c, &ServiceParams{
			Charm: factory.MakeCharm(c, &CharmParams{
				Name: "wordpress",
			}),
		})
		e2, err := s2.Endpoint("db")
		c.Assert(err, jc.ErrorIsNil)

		params.Endpoints = []state.Endpoint{e1, e2}
	}

	relation, err := factory.st.AddRelation(params.Endpoints...)
	c.Assert(err, jc.ErrorIsNil)

	return relation
}

// MakeEnvironment creates an environment with specified params,
// filling in sane defaults for missing values. If params is nil,
// defaults are used for all values.
//
// By default the new enviroment shares the same owner as the calling
// Factory's environment.
func (factory *Factory) MakeEnvironment(c *gc.C, params *EnvParams) *state.State {
	if params == nil {
		params = new(EnvParams)
	}
	if params.Name == "" {
		params.Name = uniqueString("testenv")
	}
	if params.Owner == nil {
		origEnv, err := factory.st.Environment()
		c.Assert(err, jc.ErrorIsNil)
		params.Owner = origEnv.Owner()
	}
	// It only makes sense to make an environment with the same provider
	// as the initial environment, or things will break elsewhere.
	currentCfg, err := factory.st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomEnvironConfig(c, testing.Attrs{
		"name":       params.Name,
		"uuid":       uuid.String(),
		"type":       currentCfg.Type(),
		"state-port": currentCfg.StatePort(),
		"api-port":   currentCfg.APIPort(),
	}.Merge(params.ConfigAttrs))
	_, st, err := factory.st.NewEnvironment(cfg, params.Owner.(names.UserTag))
	c.Assert(err, jc.ErrorIsNil)
	if params.Prepare {
		// Prepare the environment.
		provider, err := environs.Provider(cfg.Type())
		c.Assert(err, jc.ErrorIsNil)
		env, err := provider.PrepareForBootstrap(envtesting.BootstrapContext(c), cfg)
		c.Assert(err, jc.ErrorIsNil)
		// Now save the config back.
		err = st.UpdateEnvironConfig(env.Config().AllAttrs(), nil, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	return st
}
