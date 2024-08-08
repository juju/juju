// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
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

	ModelUUID() string
	IsControllerAdmin(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)
	Model() (Model, error)
	AllModelUUIDs() ([]string, error)
	GetModel(string) (Model, func() bool, error)
	GetBackend(string) (ModelManagerBackend, func() bool, error)

	ComposeNewModelConfig(
		config.ConfigSchemaSourceGetter,
		map[string]any,
		*environscloudspec.CloudRegionSpec,
	) (map[string]any, error)
	ControllerModelUUID() string
	ControllerModelTag() names.ModelTag
	IsController() bool
	ControllerNodes() ([]ControllerNode, error)
	ModelConfigDefaultValues(cloudName string) (config.ModelDefaultAttributes, error)
	UpdateModelConfigDefaultValues(
		update map[string]any,
		remove []string,
		regionSpec *environscloudspec.CloudRegionSpec,
	) error
	Unit(name string) (*state.Unit, error)
	Name() string
	ModelTag() names.ModelTag
	ModelConfig(context.Context) (*config.Config, error)
	AddControllerUser(state.UserAccessSpec) (permission.UserAccess, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
	UserAccess(names.UserTag, names.Tag) (permission.UserAccess, error)
	AllMachines() (machines []Machine, err error)
	AllApplications() (applications []Application, err error)
	AllFilesystems() ([]state.Filesystem, error)
	AllVolumes() ([]state.Volume, error)
	ControllerTag() names.ControllerTag
	Export(leaders map[string]string, store objectstore.ObjectStore) (description.Model, error)
	ExportPartial(state.ExportConfig, objectstore.ObjectStore) (description.Model, error)
	SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error)
	ConstraintsBySpaceName(string) ([]*state.Constraints, error)

	LatestMigration() (state.ModelMigration, error)
	DumpAll() (map[string]interface{}, error)
	Close() error
	HAPrimaryMachine() (names.MachineTag, error)
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
	CloudName() string
	CloudCredentialTag() (names.CloudCredentialTag, bool)
	CloudRegion() string
	Destroy(state.DestroyModelParams) error
	MigrationMode() state.MigrationMode
	Name() string
	UUID() string
	// TODO(aflynn): ControllerUUID is only here because the EnvironConfigGetter
	// needs a Model with this model. Once this is gone ControllerUUID can be
	// removed from this interface.
	ControllerUUID() string
	AddUser(state.UserAccessSpec) (permission.UserAccess, error)
	SetCloudCredential(tag names.CloudCredentialTag) (bool, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// WatchCloud returns a watcher that observes changes to the specified cloud.
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

var _ ModelManagerBackend = (*modelManagerStateShim)(nil)

type modelManagerStateShim struct {
	*state.State
	configSchemaSourceGetter config.ConfigSchemaSourceGetter
	model                    *state.Model
	pool                     *state.StatePool
	user                     names.UserTag
}

// NewModelManagerBackend returns a modelManagerStateShim wrapping the passed
// state, which implements ModelManagerBackend.
func NewModelManagerBackend(configSchemaSourceGetter config.ConfigSchemaSourceGetter, m *state.Model, pool *state.StatePool) ModelManagerBackend {
	return modelManagerStateShim{
		State:                    m.State(),
		configSchemaSourceGetter: configSchemaSourceGetter,
		model:                    m,
		pool:                     pool,
		user:                     names.UserTag{},
	}
}

// NewUserAwareModelManagerBackend returns a user-aware modelManagerStateShim
// wrapping the passed state, which implements ModelManagerBackend. The
// returned backend may emit redirect errors when attempting a model lookup for
// a migrated model that this user had been granted access to.
func NewUserAwareModelManagerBackend(configSchemaSourceGetter config.ConfigSchemaSourceGetter, m *state.Model, pool *state.StatePool, u names.UserTag) ModelManagerBackend {
	return modelManagerStateShim{
		State:                    m.State(),
		configSchemaSourceGetter: configSchemaSourceGetter,
		model:                    m,
		pool:                     pool,
		user:                     u,
	}
}

// NewModel implements ModelManagerBackend.
func (st modelManagerStateShim) NewModel(args state.ModelArgs) (Model, ModelManagerBackend, error) {
	aController := state.NewController(st.pool)
	otherModel, otherState, err := aController.NewModel(st.configSchemaSourceGetter, args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{Model: otherModel}, modelManagerStateShim{
		State:                    otherState,
		configSchemaSourceGetter: st.configSchemaSourceGetter,
		model:                    otherModel,
		pool:                     st.pool,
		user:                     st.user,
	}, nil
}

func (st modelManagerStateShim) ModelConfigDefaultValues(cloudName string) (config.ModelDefaultAttributes, error) {
	return st.State.ModelConfigDefaultValues(st.configSchemaSourceGetter, cloudName)
}

// UpdateModelConfigDefaultValues implements the ModelManagerBackend method.
func (st modelManagerStateShim) UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, regionSpec *environscloudspec.CloudRegionSpec) error {
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
		if !errors.Is(err, errors.NotFound) || st.user.Id() == "" {
			return nil, nil, err
		}

		// Check if this model has been migrated and this user had
		// access to it before its migration.
		mig, mErr := otherState.CompletedMigration()
		if mErr != nil && !errors.Is(mErr, errors.NotFound) {
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

		return nil, nil, &apiservererrors.RedirectError{
			Servers:         []network.ProviderHostPorts{hps},
			CACert:          target.CACert,
			ControllerAlias: target.ControllerAlias,
		}
	}
	return modelManagerStateShim{
		State:                    otherState.State,
		configSchemaSourceGetter: st.configSchemaSourceGetter,
		model:                    otherModel,
		pool:                     st.pool,
		user:                     st.user,
	}, otherState.Release, nil
}

// GetModel implements ModelManagerBackend.
func (st modelManagerStateShim) GetModel(modelUUID string) (Model, func() bool, error) {
	model, hp, err := st.pool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return modelShim{Model: model}, hp.Release, nil
}

// Model implements ModelManagerBackend.
func (st modelManagerStateShim) Model() (Model, error) {
	return modelShim{Model: st.model}, nil
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
	Name() string
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
func (st modelManagerStateShim) ModelConfig(ctx context.Context) (*config.Config, error) {
	model, err := st.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model.ModelConfig(ctx)
}

// [TODO: Eric Claude Jones] This method ignores an error for the purpose of
// expediting refactoring for CAAS features (we are avoiding changing method
// signatures so that refactoring doesn't spiral out of control). This method
// should be deleted immediately upon the removal of the ModelTag method from
// state.State.
func (st modelManagerStateShim) ModelTag() names.ModelTag {
	return names.NewModelTag(st.ModelUUID())
}
