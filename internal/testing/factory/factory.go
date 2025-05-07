// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

const (
	symbols = "abcdefghijklmopqrstuvwxyz"
)

type Factory struct {
	pool               *state.StatePool
	st                 *state.State
	applicationService *applicationservice.WatchableService
	controllerConfig   controller.Config
}

var index uint32

// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func NewFactory(st *state.State, pool *state.StatePool, controllerConfig controller.Config) *Factory {
	return &Factory{
		st:               st,
		pool:             pool,
		controllerConfig: controllerConfig,
	}
}

// WithApplicationService configures the factory to use the specified application service.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (f *Factory) WithApplicationService(s *applicationservice.WatchableService) *Factory {
	f.applicationService = s
	return f
}

// CharmParams defines the parameters for creating a charm.
type CharmParams struct {
	Name         string
	Series       string
	Revision     string
	Architecture string
	URL          string
}

// MachineParams are for creating a machine.
type MachineParams struct {
	Base            state.Base
	Jobs            []state.MachineJob
	Password        string
	Nonce           string
	Constraints     constraints.Value
	InstanceId      instance.Id
	DisplayName     string
	Characteristics *instance.HardwareCharacteristics
	Addresses       network.SpaceAddresses
	Volumes         []state.HostVolumeParams
	Filesystems     []state.HostFilesystemParams
}

// ApplicationParams is used when specifying parameters for a new application.
type ApplicationParams struct {
	Name                    string
	Charm                   state.CharmRefFull
	CharmURL                string
	CharmOrigin             *state.CharmOrigin
	ApplicationConfig       map[string]interface{}
	ApplicationConfigFields configschema.Fields
	CharmConfig             map[string]interface{}
	Storage                 map[string]state.StorageConstraints
	Constraints             constraints.Value
	EndpointBindings        map[string]string
	Password                string
	Placement               []*instance.Placement
	DesiredScale            int
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
	Endpoints []relation.Endpoint
}

type ModelParams struct {
	Type                    state.ModelType
	UUID                    coremodel.UUID
	Name                    string
	Owner                   names.Tag
	ConfigAttrs             testing.Attrs
	CloudName               string
	CloudRegion             string
	CloudCredential         names.CloudCredentialTag
	StorageProviderRegistry storage.ProviderRegistry
	EnvironVersion          int
}

type SpaceParams struct {
	Name       string
	ProviderID network.Id
	SubnetIDs  []string
}

// RandomSuffix adds a random 5 character suffix to the presented string.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
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

func (factory *Factory) paramsFillDefaults(c *tc.C, params *MachineParams) *MachineParams {
	if params == nil {
		params = &MachineParams{}
	}
	if params.Base.String() == "" {
		params.Base = state.UbuntuBase("12.10")
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
		params.Password, err = password.RandomPassword()
		c.Assert(err, tc.ErrorIsNil)
	}
	if params.Characteristics == nil {
		arch := arch.DefaultArchitecture
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
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (factory *Factory) MakeMachineNested(c *tc.C, parentId string, params *MachineParams) *state.Machine {
	params = factory.paramsFillDefaults(c, params)
	machineTemplate := state.MachineTemplate{
		Base:        params.Base,
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
	c.Assert(err, tc.ErrorIsNil)
	err = m.SetProvisioned(params.InstanceId, params.DisplayName, params.Nonce, params.Characteristics)
	c.Assert(err, tc.ErrorIsNil)
	current := testing.CurrentVersion()
	err = m.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)
	return m
}

// MakeMachine will add a machine with values defined in params. For some
// values in params, if they are missing, some meaningful empty values will be
// set.
// If params is not specified, defaults are used.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (factory *Factory) MakeMachine(c *tc.C, params *MachineParams) *state.Machine {
	machine, _ := factory.MakeMachineReturningPassword(c, params)
	return machine
}

// MakeMachineReturningPassword will add a machine with values defined in
// params. For some values in params, if they are missing, some meaningful
// empty values will be set. If params is not specified, defaults are used.
// The machine and its password are returned.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (factory *Factory) MakeMachineReturningPassword(c *tc.C, params *MachineParams) (*state.Machine, string) {
	params = factory.paramsFillDefaults(c, params)
	return factory.makeMachineReturningPassword(c, params, true)
}

// MakeUnprovisionedMachineReturningPassword will add a machine with values
// defined in params. For some values in params, if they are missing, some
// meaningful empty values will be set. If params is not specified, defaults
// are used. The machine and its password are returned; the machine will not
// be provisioned.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (factory *Factory) MakeUnprovisionedMachineReturningPassword(c *tc.C, params *MachineParams) (*state.Machine, string) {
	if params != nil {
		c.Assert(params.Nonce, tc.Equals, "")
		c.Assert(params.InstanceId, tc.Equals, instance.Id(""))
		c.Assert(params.Characteristics, tc.IsNil)
	}
	params = factory.paramsFillDefaults(c, params)
	params.Nonce = ""
	params.InstanceId = ""
	params.Characteristics = nil
	return factory.makeMachineReturningPassword(c, params, false)
}

func (factory *Factory) makeMachineReturningPassword(c *tc.C, params *MachineParams, setProvisioned bool) (*state.Machine, string) {
	machineTemplate := state.MachineTemplate{
		Base:        params.Base,
		Jobs:        params.Jobs,
		Volumes:     params.Volumes,
		Filesystems: params.Filesystems,
		Constraints: params.Constraints,
	}

	if params.Characteristics != nil {
		machineTemplate.HardwareCharacteristics = *params.Characteristics
	}
	machine, err := factory.st.AddOneMachine(machineTemplate)
	c.Assert(err, tc.ErrorIsNil)
	if setProvisioned {
		err = machine.SetProvisioned(params.InstanceId, params.DisplayName, params.Nonce, params.Characteristics)
		c.Assert(err, tc.ErrorIsNil)
	}
	err = machine.SetPassword(params.Password)
	c.Assert(err, tc.ErrorIsNil)
	if len(params.Addresses) > 0 {
		err = machine.SetProviderAddresses(factory.controllerConfig, params.Addresses...)
		c.Assert(err, tc.ErrorIsNil)
	}
	current := testing.CurrentVersion()
	err = machine.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)
	return machine, params.Password
}

// MakeModel creates an model with specified params,
// filling in sane defaults for missing values. If params is nil,
// defaults are used for all values.
//
// By default the new model shares the same owner as the calling Factory's
// model. TODO(ericclaudejones) MakeModel should return the model itself rather
// than the state.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (factory *Factory) MakeModel(c *tc.C, params *ModelParams) *state.State {
	if params == nil {
		params = new(ModelParams)
	}
	if params.Type == state.ModelType("") {
		params.Type = state.ModelTypeIAAS
	}
	if params.Name == "" {
		params.Name = uniqueString("testmodel")
	}
	if params.CloudName == "" {
		params.CloudName = "dummy"
	}
	if params.CloudRegion == "" && params.CloudName == "dummy" {
		params.CloudRegion = "dummy-region"
	}
	if params.CloudRegion == "<none>" {
		params.CloudRegion = ""
	}
	if params.Owner == nil {
		origEnv, err := factory.st.Model()
		c.Assert(err, tc.ErrorIsNil)
		params.Owner = origEnv.Owner()
	}
	if params.StorageProviderRegistry == nil {
		params.StorageProviderRegistry = provider.CommonStorageProviders()
	}

	// For IAAS models, it only makes sense to make a model with the same provider
	// as the initial model, or things will break elsewhere.
	// For CAAS models, the type is "kubernetes".
	currentCfg := factory.currentCfg(c)
	cfgType := currentCfg.Type()
	if params.Type == state.ModelTypeCAAS {
		cfgType = "kubernetes"
	}

	if params.UUID == coremodel.UUID("") {
		params.UUID = modeltesting.GenModelUUID(c)
	}
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": params.Name,
		"uuid": params.UUID.String(),
		"type": cfgType,
	}.Merge(params.ConfigAttrs))
	controller := state.NewController(factory.pool)
	_, st, err := controller.NewModel(state.ModelArgs{
		Type:            params.Type,
		CloudName:       params.CloudName,
		CloudRegion:     params.CloudRegion,
		CloudCredential: params.CloudCredential,
		Config:          cfg,
		Owner:           params.Owner.(names.UserTag),
		EnvironVersion:  params.EnvironVersion,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = factory.pool.StartWorkers(st)
	c.Assert(err, tc.ErrorIsNil)
	return st
}

// MakeCAASModel creates a CAAS model with specified params,
// filling in sane defaults for missing values. If params is nil,
// defaults are used for all values.
// Deprecated: Testing factory is being removed and should not be used in new
// tests.
func (factory *Factory) MakeCAASModel(c *tc.C, params *ModelParams) *state.State {
	if params == nil {
		params = &ModelParams{}
	}
	params.Type = state.ModelTypeCAAS
	if params.Owner == nil {
		origEnv, err := factory.st.Model()
		c.Assert(err, tc.ErrorIsNil)
		params.Owner = origEnv.Owner()
	}
	if params.CloudName == "" {
		params.CloudName = "caascloud"
	}
	if params.CloudCredential.IsZero() {
		if params.Owner == nil {
			origEnv, err := factory.st.Model()
			c.Assert(err, tc.ErrorIsNil)
			params.Owner = origEnv.Owner()
		}
		tag := names.NewCloudCredentialTag(
			fmt.Sprintf("%s/%s/dummy-credential", params.CloudName, params.Owner.Id()))
		params.CloudCredential = tag
	}
	return factory.MakeModel(c, params)
}

func (factory *Factory) currentCfg(c *tc.C) *config.Config {
	return testing.ModelConfig(c)
}

func NewObjectStore(c *tc.C, modelUUID string, metadataService internalobjectstore.MetadataService, claimer internalobjectstore.Claimer) objectstore.ObjectStore {
	store, err := internalobjectstore.ObjectStoreFactory(
		context.Background(),
		internalobjectstore.DefaultBackendType(),
		modelUUID,
		internalobjectstore.WithRootDir(c.MkDir()),
		internalobjectstore.WithMetadataService(metadataService),
		internalobjectstore.WithClaimer(claimer),
		internalobjectstore.WithLogger(loggertesting.WrapCheckLog(c)),
	)
	c.Assert(err, tc.ErrorIsNil)
	return store
}
