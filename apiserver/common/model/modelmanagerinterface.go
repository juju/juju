// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/state"
)

// ModelManagerBackend defines methods provided by a state
// instance used by the model manager apiserver implementation.
// All the interface methods are defined directly on state.State
// and are reproduced here for use in tests.
type ModelManagerBackend interface {
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)
	Model() (Model, error)
	GetBackend(string) (ModelManagerBackend, func() bool, error)

	ControllerNodes() ([]ControllerNode, error)
	ModelTag() names.ModelTag
	AllMachines() (machines []Machine, err error)
	AllFilesystems() ([]state.Filesystem, error)
	AllVolumes() ([]state.Volume, error)
	Export(store objectstore.ObjectStore) (description.Model, error)
	ExportPartial(state.ExportConfig, objectstore.ObjectStore) (description.Model, error)

	Close() error
	HAPrimaryMachine() (names.MachineTag, error)
}

type ControllerNode interface {
	Id() string
}

type Machine interface {
	Id() string
	ContainerType() instance.ContainerType
	Life() state.Life
	ForceDestroy(time.Duration) error
	Destroy(objectstore.ObjectStore) error
}

// Model defines methods provided by a state.Model instance.
// All the interface methods are defined directly on state.Model
// and are reproduced here for use in tests.
type Model interface {
	Destroy(state.DestroyModelParams) error
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// GetInstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	GetInstanceIDAndName(ctx context.Context, machineUUID machine.UUID) (instance.Id, string, error)
	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (*instance.HardwareCharacteristics, error)
}

// StatusService returns the status of a applications, and units and machines.
type StatusService interface {
	// GetApplicationAndUnitModelStatuses returns the application name and unit
	// count for each model for the model status request.
	GetApplicationAndUnitModelStatuses(ctx context.Context) (map[string]int, error)

	// GetModelStatusInfo returns only basic model information used for
	// displaying model status.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	GetModelStatusInfo(context.Context) (domainstatus.ModelStatusInfo, error)

	// GetAllMachineStatuses returns all the machine statuses for the model, indexed
	// by machine name.
	GetAllMachineStatuses(context.Context) (map[machine.Name]status.StatusInfo, error)
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
	return modelManagerStateShim{
		State: m.State(),
		model: m,
		pool:  pool,
		user:  names.UserTag{},
	}
}

// NewUserAwareModelManagerBackend returns a user-aware modelManagerStateShim
// wrapping the passed state, which implements ModelManagerBackend. The
// returned backend may emit redirect errors when attempting a model lookup for
// a migrated model that this user had been granted access to.
func NewUserAwareModelManagerBackend(m *state.Model, pool *state.StatePool, u names.UserTag) ModelManagerBackend {
	return modelManagerStateShim{
		State: m.State(),
		model: m,
		pool:  pool,
		user:  u,
	}
}

// NewModel implements ModelManagerBackend.
func (st modelManagerStateShim) NewModel(args state.ModelArgs) (Model, ModelManagerBackend, error) {
	aController := state.NewController(st.pool)
	otherModel, otherState, err := aController.NewModel(args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{Model: otherModel}, modelManagerStateShim{
		State: otherState,
		model: otherModel,
		pool:  st.pool,
		user:  st.user,
	}, nil
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

		// TODO(aflynn): Also return this error if, in the migration info, the
		// user had access to the migrated model (JUJU-6669).
		if mig == nil {
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
		State: otherState.State,
		model: otherModel,
		pool:  st.pool,
		user:  st.user,
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

var _ Model = (*modelShim)(nil)

type modelShim struct {
	*state.Model
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

// [TODO: Eric Claude Jones] This method ignores an error for the purpose of
// expediting refactoring for CAAS features (we are avoiding changing method
// signatures so that refactoring doesn't spiral out of control). This method
// should be deleted immediately upon the removal of the ModelTag method from
// state.State.
func (st modelManagerStateShim) ModelTag() names.ModelTag {
	return names.NewModelTag(st.ModelUUID())
}
