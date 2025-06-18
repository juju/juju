// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"fmt"
	"math/rand"
	"sync/atomic"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/configschema"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
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

	if params.UUID == coremodel.UUID("") {
		params.UUID = modeltesting.GenModelUUID(c)
	}
	controller := state.NewController(factory.pool)
	_, st, err := controller.NewModel(state.ModelArgs{
		Name:            params.Name,
		UUID:            params.UUID,
		Type:            params.Type,
		CloudName:       params.CloudName,
		CloudRegion:     params.CloudRegion,
		CloudCredential: params.CloudCredential,
		Owner:           params.Owner.(names.UserTag),
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

func NewObjectStore(c *tc.C, modelUUID string, metadataService internalobjectstore.MetadataService, claimer internalobjectstore.Claimer) objectstore.ObjectStore {
	store, err := internalobjectstore.ObjectStoreFactory(
		c.Context(),
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
