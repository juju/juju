// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/version"
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

// UserParams defines the parameters for creating a user with MakeUser.
type UserParams struct {
	Name        string
	DisplayName string
	Password    string
	Creator     names.Tag
	NoModelUser bool
	Disabled    bool
	Access      permission.Access
}

// ModelUserParams defines the parameters for creating an environment user.
type ModelUserParams struct {
	User        string
	DisplayName string
	CreatedBy   names.Tag
	Access      permission.Access
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
	Constraints     constraints.Value
	InstanceId      instance.Id
	Characteristics *instance.HardwareCharacteristics
	Addresses       []network.Address
	Volumes         []state.MachineVolumeParams
	Filesystems     []state.MachineFilesystemParams
}

// ApplicationParams is used when specifying parameters for a new application.
type ApplicationParams struct {
	Name        string
	Charm       *state.Charm
	Status      *status.StatusInfo
	Settings    map[string]interface{}
	Storage     map[string]state.StorageConstraints
	Constraints constraints.Value
}

// UnitParams are used to create units.
type UnitParams struct {
	Application *state.Application
	Machine     *state.Machine
	Password    string
	SetCharmURL bool
	Status      *status.StatusInfo
	Constraints constraints.Value
}

// RelationParams are used to create relations.
type RelationParams struct {
	Endpoints []state.Endpoint
}

type MetricParams struct {
	Unit       *state.Unit
	Time       *time.Time
	Metrics    []state.Metric
	Sent       bool
	DeleteTime *time.Time
}

type ModelParams struct {
	Name                    string
	Owner                   names.Tag
	ConfigAttrs             testing.Attrs
	CloudName               string
	CloudRegion             string
	CloudCredential         names.CloudCredentialTag
	StorageProviderRegistry storage.ProviderRegistry
}

type SpaceParams struct {
	Name       string
	ProviderID network.Id
	Subnets    []string
	IsPublic   bool
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
// If params.NoModelUser is false, the user will also be created
// in the current model.
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
		env, err := factory.st.Model()
		c.Assert(err, jc.ErrorIsNil)
		params.Creator = env.Owner()
	}
	if params.Access == permission.NoAccess {
		params.Access = permission.AdminAccess
	}
	creatorUserTag := params.Creator.(names.UserTag)
	user, err := factory.st.AddUser(
		params.Name, params.DisplayName, params.Password, creatorUserTag.Name())
	c.Assert(err, jc.ErrorIsNil)
	if !params.NoModelUser {
		_, err := factory.st.AddModelUser(factory.st.ModelUUID(), state.UserAccessSpec{
			User:        user.UserTag(),
			CreatedBy:   names.NewUserTag(user.CreatedBy()),
			DisplayName: params.DisplayName,
			Access:      params.Access,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	if params.Disabled {
		err := user.Disable()
		c.Assert(err, jc.ErrorIsNil)
	}
	return user
}

// MakeModelUser will create a modelUser with values defined by the params. For
// attributes of ModelUserParams that are the default empty values, some
// meaningful valid values are used instead. If params is not specified,
// defaults are used.
func (factory *Factory) MakeModelUser(c *gc.C, params *ModelUserParams) permission.UserAccess {
	if params == nil {
		params = &ModelUserParams{}
	}
	if params.User == "" {
		user := factory.MakeUser(c, &UserParams{NoModelUser: true})
		params.User = user.UserTag().Id()
	}
	if params.DisplayName == "" {
		params.DisplayName = uniqueString("display name")
	}
	if params.Access == permission.NoAccess {
		params.Access = permission.AdminAccess
	}
	if params.CreatedBy == nil {
		env, err := factory.st.Model()
		c.Assert(err, jc.ErrorIsNil)
		params.CreatedBy = env.Owner()
	}
	createdByUserTag := params.CreatedBy.(names.UserTag)
	modelUser, err := factory.st.AddModelUser(factory.st.ModelUUID(), state.UserAccessSpec{
		User:        names.NewUserTag(params.User),
		CreatedBy:   createdByUserTag,
		DisplayName: params.DisplayName,
		Access:      params.Access,
	})
	c.Assert(err, jc.ErrorIsNil)
	return modelUser
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
	if params.Characteristics == nil {
		arch := "amd64"
		mem := uint64(64 * 1024 * 1024 * 1024)
		hardware := instance.HardwareCharacteristics{
			Arch: &arch,
			Mem:  &mem,
		}
		params.Characteristics = &hardware
	}

	return params
}

// MakeMachineNested will make a machine nested in the machine with ID given.
func (factory *Factory) MakeMachineNested(c *gc.C, parentId string, params *MachineParams) *state.Machine {
	params = factory.paramsFillDefaults(c, params)
	machineTemplate := state.MachineTemplate{
		Series:      params.Series,
		Jobs:        params.Jobs,
		Volumes:     params.Volumes,
		Filesystems: params.Filesystems,
		Constraints: params.Constraints,
	}

	m, err := factory.st.AddMachineInsideMachine(
		machineTemplate,
		parentId,
		instance.LXD,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned(params.InstanceId, params.Nonce, params.Characteristics)
	c.Assert(err, jc.ErrorIsNil)
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	err = m.SetAgentVersion(current)
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
	return factory.makeMachineReturningPassword(c, params, true)
}

// MakeUnprovisionedMachineReturningPassword will add a machine with values
// defined in params. For some values in params, if they are missing, some
// meaningful empty values will be set. If params is not specified, defaults
// are used. The machine and its password are returned; the machine will not
// be provisioned.
func (factory *Factory) MakeUnprovisionedMachineReturningPassword(c *gc.C, params *MachineParams) (*state.Machine, string) {
	if params != nil {
		c.Assert(params.Nonce, gc.Equals, "")
		c.Assert(params.InstanceId, gc.Equals, instance.Id(""))
		c.Assert(params.Characteristics, gc.IsNil)
	}
	params = factory.paramsFillDefaults(c, params)
	params.Nonce = ""
	params.InstanceId = ""
	params.Characteristics = nil
	return factory.makeMachineReturningPassword(c, params, false)
}

func (factory *Factory) makeMachineReturningPassword(c *gc.C, params *MachineParams, setProvisioned bool) (*state.Machine, string) {
	machineTemplate := state.MachineTemplate{
		Series:      params.Series,
		Jobs:        params.Jobs,
		Volumes:     params.Volumes,
		Filesystems: params.Filesystems,
		Constraints: params.Constraints,
	}
	machine, err := factory.st.AddOneMachine(machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	if setProvisioned {
		err = machine.SetProvisioned(params.InstanceId, params.Nonce, params.Characteristics)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = machine.SetPassword(params.Password)
	c.Assert(err, jc.ErrorIsNil)
	if len(params.Addresses) > 0 {
		err := machine.SetProviderAddresses(params.Addresses...)
		c.Assert(err, jc.ErrorIsNil)
	}
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	err = machine.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)
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
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "fake-storage-path",
		SHA256:      bundleSHA256,
	}
	charm, err := factory.st.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	return charm
}

// MakeApplication creates an application with the specified parameters, substituting
// sane defaults for missing values.
// If params is not specified, defaults are used.
func (factory *Factory) MakeApplication(c *gc.C, params *ApplicationParams) *state.Application {
	if params == nil {
		params = &ApplicationParams{}
	}
	if params.Charm == nil {
		params.Charm = factory.MakeCharm(c, nil)
	}
	if params.Name == "" {
		params.Name = params.Charm.Meta().Name
	}
	application, err := factory.st.AddApplication(state.AddApplicationArgs{
		Name:        params.Name,
		Charm:       params.Charm,
		Settings:    charm.Settings(params.Settings),
		Storage:     params.Storage,
		Constraints: params.Constraints,
	})
	c.Assert(err, jc.ErrorIsNil)

	if params.Status != nil {
		now := time.Now()
		s := status.StatusInfo{
			Status:  params.Status.Status,
			Message: params.Status.Message,
			Data:    params.Status.Data,
			Since:   &now,
		}
		err = application.SetStatus(s)
		c.Assert(err, jc.ErrorIsNil)
	}

	return application
}

// MakeUnit creates an application unit with specified params, filling in
// sane defaults for missing values.
// If params is not specified, defaults are used.
func (factory *Factory) MakeUnit(c *gc.C, params *UnitParams) *state.Unit {
	unit, _ := factory.MakeUnitReturningPassword(c, params)
	return unit
}

// MakeUnit creates an application unit with specified params, filling in sane
// defaults for missing values. If params is not specified, defaults are used.
// The unit and its password are returned.
func (factory *Factory) MakeUnitReturningPassword(c *gc.C, params *UnitParams) (*state.Unit, string) {
	if params == nil {
		params = &UnitParams{}
	}
	if params.Machine == nil {
		params.Machine = factory.MakeMachine(c, nil)
	}
	if params.Application == nil {
		params.Application = factory.MakeApplication(c, &ApplicationParams{
			Constraints: params.Constraints,
		})
	}
	if params.Password == "" {
		var err error
		params.Password, err = utils.RandomPassword()
		c.Assert(err, jc.ErrorIsNil)
	}
	unit, err := params.Application.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(params.Machine)
	c.Assert(err, jc.ErrorIsNil)

	agentTools := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: params.Application.Series(),
	}
	err = unit.SetAgentVersion(agentTools)
	c.Assert(err, jc.ErrorIsNil)
	if params.SetCharmURL {
		applicationCharmURL, _ := params.Application.CharmURL()
		err = unit.SetCharmURL(applicationCharmURL)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = unit.SetPassword(params.Password)
	c.Assert(err, jc.ErrorIsNil)

	if params.Status != nil {
		now := time.Now()
		s := status.StatusInfo{
			Status:  params.Status.Status,
			Message: params.Status.Message,
			Data:    params.Status.Data,
			Since:   &now,
		}
		err = unit.SetStatus(s)
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
		meteredApplication := factory.MakeApplication(c, &ApplicationParams{Charm: meteredCharm})
		params.Unit = factory.MakeUnit(c, &UnitParams{Application: meteredApplication, SetCharmURL: true})
	}
	if params.Time == nil {
		params.Time = &now
	}
	if params.Metrics == nil {
		params.Metrics = []state.Metric{{"pings", strconv.Itoa(uniqueInteger()), *params.Time}}
	}

	chURL, ok := params.Unit.CharmURL()
	c.Assert(ok, gc.Equals, true)

	metric, err := factory.st.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  *params.Time,
			CharmURL: chURL.String(),
			Metrics:  params.Metrics,
			Unit:     params.Unit.UnitTag(),
		})
	c.Assert(err, jc.ErrorIsNil)
	if params.Sent {
		t := now
		if params.DeleteTime != nil {
			t = *params.DeleteTime
		}
		err := metric.SetSent(t)
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
		s1 := factory.MakeApplication(c, &ApplicationParams{
			Charm: factory.MakeCharm(c, &CharmParams{
				Name: "mysql",
			}),
		})
		e1, err := s1.Endpoint("server")
		c.Assert(err, jc.ErrorIsNil)

		s2 := factory.MakeApplication(c, &ApplicationParams{
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

// MakeModel creates an model with specified params,
// filling in sane defaults for missing values. If params is nil,
// defaults are used for all values.
//
// By default the new model shares the same owner as the calling
// Factory's model.
func (factory *Factory) MakeModel(c *gc.C, params *ModelParams) *state.State {
	if params == nil {
		params = new(ModelParams)
	}
	if params.Name == "" {
		params.Name = uniqueString("testenv")
	}
	if params.CloudName == "" {
		params.CloudName = "dummy"
	}
	if params.CloudRegion == "" {
		params.CloudRegion = "dummy-region"
	}
	if params.Owner == nil {
		origEnv, err := factory.st.Model()
		c.Assert(err, jc.ErrorIsNil)
		params.Owner = origEnv.Owner()
	}
	if params.StorageProviderRegistry == nil {
		params.StorageProviderRegistry = provider.CommonStorageProviders()
	}
	// It only makes sense to make an model with the same provider
	// as the initial model, or things will break elsewhere.
	currentCfg, err := factory.st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": params.Name,
		"uuid": uuid.String(),
		"type": currentCfg.Type(),
	}.Merge(params.ConfigAttrs))
	_, st, err := factory.st.NewModel(state.ModelArgs{
		CloudName:       params.CloudName,
		CloudRegion:     params.CloudRegion,
		CloudCredential: params.CloudCredential,
		Config:          cfg,
		Owner:           params.Owner.(names.UserTag),
		StorageProviderRegistry: params.StorageProviderRegistry,
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

// MakeSpace will create a new space with the specified params. If the space
// name is not set, a unique space name is created.
func (factory *Factory) MakeSpace(c *gc.C, params *SpaceParams) *state.Space {
	if params == nil {
		params = new(SpaceParams)
	}
	if params.Name == "" {
		params.Name = uniqueString("space-")
	}
	space, err := factory.st.AddSpace(params.Name, params.ProviderID, params.Subnets, params.IsPublic)
	c.Assert(err, jc.ErrorIsNil)
	return space
}
