// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/description"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// ModelManagerBackend defines methods provided by a state
// instance used by the model manager apiserver implementation.
// All the interface methods are defined directly on state.State
// and are reproduced here for use in tests.
type ModelManagerBackend interface {
	APIHostPortsGetter
	ToolsStorageGetter
	BlockGetter
	state.CloudAccessor

	ModelUUID() string
	ModelUUIDsForUser(names.UserTag) ([]string, error)
	IsControllerAdmin(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)
	Model() (Model, error)
	AllModelUUIDs() ([]string, error)
	GetModel(string) (Model, func() bool, error)
	GetBackend(string) (ModelManagerBackend, func() bool, error)

	ComposeNewModelConfig(modelAttr map[string]interface{}, regionSpec *environs.RegionSpec) (map[string]interface{}, error)
	ControllerModelUUID() string
	ControllerModelTag() names.ModelTag
	ControllerConfig() (controller.Config, error)
	ModelConfigDefaultValues() (config.ModelDefaultAttributes, error)
	UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, regionSpec *environs.RegionSpec) error
	Unit(name string) (*state.Unit, error)
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	AddModelUser(string, state.UserAccessSpec) (permission.UserAccess, error)
	AddControllerUser(state.UserAccessSpec) (permission.UserAccess, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
	UserAccess(names.UserTag, names.Tag) (permission.UserAccess, error)
	AllMachines() (machines []Machine, err error)
	AllApplications() (applications []Application, err error)
	AllFilesystems() ([]state.Filesystem, error)
	AllVolumes() ([]state.Volume, error)
	ControllerUUID() string
	ControllerTag() names.ControllerTag
	Export() (description.Model, error)
	ExportPartial(state.ExportConfig) (description.Model, error)
	SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error)
	SetModelMeterStatus(string, string) error
	ReloadSpaces(environ environs.Environ) error
	LastModelConnection(user names.UserTag) (time.Time, error)
	LatestMigration() (state.ModelMigration, error)
	DumpAll() (map[string]interface{}, error)
	Close() error

	// Methods required by the metricsender package.
	MetricsManager() (*state.MetricsManager, error)
	MetricsToSend(batchSize int) ([]*state.MetricBatch, error)
	SetMetricBatchesSent(batchUUIDs []string) error
	CountOfUnsentMetrics() (int, error)
	CountOfSentMetrics() (int, error)
	CleanupOldMetrics() error
}

// Model defines methods provided by a state.Model instance.
// All the interface methods are defined directly on state.Model
// and are reproduced here for use in tests.
type Model interface {
	Config() (*config.Config, error)
	Life() state.Life
	ModelTag() names.ModelTag
	Owner() names.UserTag
	Status() (status.StatusInfo, error)
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	Users() ([]permission.UserAccess, error)
	Destroy(state.DestroyModelParams) error
	SLALevel() string
	SLAOwner() string
	MigrationMode() state.MigrationMode
	Name() string
	UUID() string
	ControllerUUID() string
}

var _ ModelManagerBackend = (*modelManagerStateShim)(nil)

type modelManagerStateShim struct {
	*state.State
	pool *state.StatePool
}

// NewModelManagerBackend returns a modelManagerStateShim wrapping the passed
// state, which implements ModelManagerBackend.
func NewModelManagerBackend(st *state.State, pool *state.StatePool) ModelManagerBackend {
	return modelManagerStateShim{st, pool}
}

// NewModel implements ModelManagerBackend.
func (st modelManagerStateShim) NewModel(args state.ModelArgs) (Model, ModelManagerBackend, error) {
	m, otherState, err := st.State.NewModel(args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{m}, modelManagerStateShim{otherState, st.pool}, nil
}

// GetBackend implements ModelManagerBackend.
func (st modelManagerStateShim) GetBackend(modelUUID string) (ModelManagerBackend, func() bool, error) {
	otherState, release, err := st.pool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return modelManagerStateShim{otherState, st.pool}, release, nil
}

// GetModel implements ModelManagerBackend.
func (st modelManagerStateShim) GetModel(modelUUID string) (Model, func() bool, error) {
	model, release, err := st.pool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return modelShim{model}, release, nil
}

// Model implements ModelManagerBackend.
func (st modelManagerStateShim) Model() (Model, error) {
	m, err := st.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

var _ Model = (*modelShim)(nil)

type modelShim struct {
	*state.Model
}

// Users implements ModelManagerBackend.
func (m modelShim) Users() ([]permission.UserAccess, error) {
	stateUsers, err := m.Model.Users()
	if err != nil {
		return nil, err
	}
	users := make([]permission.UserAccess, len(stateUsers))
	for i, user := range stateUsers {
		users[i] = user
	}
	return users, nil
}

type machineShim struct {
	*state.Machine
}

func (st modelManagerStateShim) AllMachines() ([]Machine, error) {
	allStateMachines, err := st.State.AllMachines()
	if err != nil {
		return nil, err
	}
	all := make([]Machine, len(allStateMachines))
	for i, m := range allStateMachines {
		all[i] = machineShim{m}
	}
	return all, nil
}

// Application defines methods provided by a state.Application instance.
type Application interface{}

type applicationShim struct {
	*state.Application
}

func (st modelManagerStateShim) AllApplications() ([]Application, error) {
	allStateApplications, err := st.State.AllApplications()
	if err != nil {
		return nil, err
	}
	all := make([]Application, len(allStateApplications))
	for i, a := range allStateApplications {
		all[i] = applicationShim{a}
	}
	return all, nil
}

func (st modelManagerStateShim) AllFilesystems() ([]state.Filesystem, error) {
	model, err := st.State.IAASModel()
	if err != nil {
		return nil, err
	}
	return model.AllFilesystems()
}

func (st modelManagerStateShim) AllVolumes() ([]state.Volume, error) {
	model, err := st.State.IAASModel()
	if err != nil {
		return nil, err
	}
	return model.AllVolumes()
}
