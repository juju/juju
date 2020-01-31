// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/description"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// ModelManagerBackend defines methods provided by a state
// instance used by the model manager apiserver implementation.
// All the interface methods are defined directly on state.State
// and are reproduced here for use in tests.
type ModelManagerBackend interface {
	APIHostPortsForAgentsGetter
	ToolsStorageGetter
	BlockGetter
	state.CloudAccessor

	ModelUUID() string
	ModelUUIDsForUser(names.UserTag) ([]string, error)
	ModelBasicInfoForUser(user names.UserTag) ([]state.ModelAccessInfo, error)
	ModelSummariesForUser(user names.UserTag, all bool) ([]state.ModelSummary, error)
	IsControllerAdmin(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)
	Model() (Model, error)
	AllModelUUIDs() ([]string, error)
	GetModel(string) (Model, func() bool, error)
	GetBackend(string) (ModelManagerBackend, func() bool, error)

	ComposeNewModelConfig(modelAttr map[string]interface{}, regionSpec *environs.CloudRegionSpec) (map[string]interface{}, error)
	ControllerModelUUID() string
	ControllerModelTag() names.ModelTag
	IsController() bool
	ControllerConfig() (controller.Config, error)
	ControllerNodes() ([]ControllerNode, error)
	ModelConfigDefaultValues(cloudName string) (config.ModelDefaultAttributes, error)
	UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, regionSpec *environs.CloudRegionSpec) error
	Unit(name string) (*state.Unit, error)
	Name() string
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	AddControllerUser(state.UserAccessSpec) (permission.UserAccess, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
	UserAccess(names.UserTag, names.Tag) (permission.UserAccess, error)
	GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error)
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
	ReloadSpaces(environ environs.BootstrapEnviron) error
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
	Type() state.ModelType
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
	LastModelConnection(user names.UserTag) (time.Time, error)
	AddUser(state.UserAccessSpec) (permission.UserAccess, error)
	AutoConfigureContainerNetworking(environ environs.BootstrapEnviron) error
	SetCloudCredential(tag names.CloudCredentialTag) (bool, error)
}

var _ ModelManagerBackend = (*modelManagerStateShim)(nil)

type modelManagerStateShim struct {
	*state.State
	model *state.Model
	pool  *state.StatePool
	user  names.UserTag
}

// NewModelManagerBackend returns a modelManagerStateShim wrapping the passed
// state, which implements ModelManagerBackend.
func NewModelManagerBackend(m *state.Model, pool *state.StatePool) ModelManagerBackend {
	return modelManagerStateShim{m.State(), m, pool, names.UserTag{}}
}

// NewUserAwareModelManagerBackend returns a user-aware modelManagerStateShim
// wrapping the passed state, which implements ModelManagerBackend. The
// returned backend may emit redirect errors when attempting a model lookup for
// a migrated model that this user had been granted access to.
func NewUserAwareModelManagerBackend(m *state.Model, pool *state.StatePool, u names.UserTag) ModelManagerBackend {
	return modelManagerStateShim{m.State(), m, pool, u}
}

// NewModel implements ModelManagerBackend.
func (st modelManagerStateShim) NewModel(args state.ModelArgs) (Model, ModelManagerBackend, error) {
	controller := state.NewController(st.pool)
	otherModel, otherState, err := controller.NewModel(args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{otherModel}, modelManagerStateShim{otherState, otherModel, st.pool, st.user}, nil
}

func (st modelManagerStateShim) ModelConfigDefaultValues(cloudName string) (config.ModelDefaultAttributes, error) {
	return st.State.ModelConfigDefaultValues(cloudName)
}

// UpdateModelConfigDefaultValues implements the ModelManagerBackend method.
func (st modelManagerStateShim) UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, regionSpec *environs.CloudRegionSpec) error {
	return st.State.UpdateModelConfigDefaultValues(update, remove, regionSpec)
}

// ControllerTag exposes Model ControllerTag for ModelManagerBackend inteface
func (st modelManagerStateShim) ControllerTag() names.ControllerTag {
	return st.model.ControllerTag()
}

// GetBackend implements ModelManagerBackend.
func (st modelManagerStateShim) GetBackend(modelUUID string) (ModelManagerBackend, func() bool, error) {
	otherState, err := st.pool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	otherModel, err := otherState.Model()
	if err != nil {
		defer otherState.Release()
		if !errors.IsNotFound(err) || st.user.Id() == "" {
			return nil, nil, err
		}

		// Check if this model has been migrated and this user had
		// access to it before its migration.
		mig, mErr := otherState.CompletedMigration()
		if mErr != nil && !errors.IsNotFound(mErr) {
			return nil, nil, errors.Trace(mErr)
		}

		if mig == nil || mig.ModelUserAccess(st.user) == permission.NoAccess {
			return nil, nil, errors.Trace(err) // return original NotFound error
		}

		target, mErr := mig.TargetInfo()
		if mErr != nil {
			return nil, nil, errors.Trace(mErr)
		}

		hps, mErr := network.ParseProviderHostPorts(target.Addrs...)
		if mErr != nil {
			return nil, nil, errors.Trace(mErr)
		}

		return nil, nil, &RedirectError{
			Servers:         []network.ProviderHostPorts{hps},
			CACert:          target.CACert,
			ControllerAlias: target.ControllerAlias,
		}
	}
	return modelManagerStateShim{otherState.State, otherModel, st.pool, st.user}, otherState.Release, nil
}

// GetModel implements ModelManagerBackend.
func (st modelManagerStateShim) GetModel(modelUUID string) (Model, func() bool, error) {
	model, hp, err := st.pool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return modelShim{model}, hp.Release, nil
}

// Model implements ModelManagerBackend.
func (st modelManagerStateShim) Model() (Model, error) {
	return modelShim{st.model}, nil
}

// Name implements ModelManagerBackend.
func (st modelManagerStateShim) Name() string {
	return st.model.Name()
}

func (st modelManagerStateShim) ControllerNodes() ([]ControllerNode, error) {
	nodes, err := st.State.ControllerNodes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]ControllerNode, len(nodes))
	for i, n := range nodes {
		result[i] = n
	}
	return result, nil
}

func (st modelManagerStateShim) IsController() bool {
	return st.State.IsController()
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
type Application interface {
	UnitCount() int
}

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
	sb, err := state.NewStorageBackend(st.State)
	if err != nil {
		return nil, err
	}
	return sb.AllFilesystems()
}

func (st modelManagerStateShim) AllVolumes() ([]state.Volume, error) {
	sb, err := state.NewStorageBackend(st.State)
	if err != nil {
		return nil, err
	}
	return sb.AllVolumes()
}

// ModelConfig returns the underlying model's config. Exposed here to satisfy the
// ModelBackend interface.
func (st modelManagerStateShim) ModelConfig() (*config.Config, error) {
	model, err := st.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model.ModelConfig()
}

// [TODO: Eric Claude Jones] This method ignores an error for the purpose of
// expediting refactoring for CAAS features (we are avoiding changing method
// signatures so that refactoring doesn't spiral out of control). This method
// should be deleted immediately upon the removal of the ModelTag method from
// state.State.
func (st modelManagerStateShim) ModelTag() names.ModelTag {
	return names.NewModelTag(st.ModelUUID())
}
