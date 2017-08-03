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
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdmin(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)

	ComposeNewModelConfig(modelAttr map[string]interface{}, regionSpec *environs.RegionSpec) (map[string]interface{}, error)
	ControllerModelUUID() string
	ControllerModelTag() names.ModelTag
	ControllerConfig() (controller.Config, error)
	ForModel(tag names.ModelTag) (ModelManagerBackend, error)
	GetModel(names.ModelTag) (Model, error)
	Model() (Model, error)
	ModelConfigDefaultValues() (config.ModelDefaultAttributes, error)
	UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, regionSpec *environs.RegionSpec) error
	Unit(name string) (*state.Unit, error)
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	AllModels() ([]Model, error)
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

// ForModel implements ModelManagerBackend.
func (st modelManagerStateShim) ForModel(tag names.ModelTag) (ModelManagerBackend, error) {
	otherState, err := st.State.ForModel(tag)
	if err != nil {
		return nil, err
	}
	return modelManagerStateShim{otherState, st.pool}, nil
}

// GetModel implements ModelManagerBackend.
func (st modelManagerStateShim) GetModel(tag names.ModelTag) (Model, error) {
	m, err := st.State.GetModel(tag)
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

// Model implements ModelManagerBackend.
func (st modelManagerStateShim) Model() (Model, error) {
	m, err := st.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

// AllModels implements ModelManagerBackend.
func (st modelManagerStateShim) AllModels() ([]Model, error) {
	modelUUIDs, err := st.State.AllModelUUIDs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]Model, 0, len(modelUUIDs))
	for _, modelUUID := range modelUUIDs {
		st, release, err := st.pool.Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer release()
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		out = append(out, model)
	}
	return out, nil
}

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

// BackendPool provides access to a pool of ModelManagerBackends.
type BackendPool interface {
	Get(modelUUID string) (ModelManagerBackend, func(), error)
}

// NewBackendPool returns a BackendPool wrapping the passed StatePool.
func NewBackendPool(pool *state.StatePool) BackendPool {
	return &statePoolShim{pool: pool}
}

type statePoolShim struct {
	pool *state.StatePool
}

// Get implements BackendPool.
func (p *statePoolShim) Get(modelUUID string) (ModelManagerBackend, func(), error) {
	st, releaser, err := p.pool.Get(modelUUID)
	closer := func() {
		releaser()
	}
	return NewModelManagerBackend(st, p.pool), closer, err
}
