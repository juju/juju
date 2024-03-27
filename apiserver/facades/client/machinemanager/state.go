// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"time"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

// StateBackend wraps a state.
// TODO(juju3) - move to export_test
// It's here because we need to for the client
// facade for backwards compatibility.
func StateBackend1(st *state.State, prechecker environs.InstancePrechecker) Backend {
	return &stateShim{State: st, prechcker: prechecker}
}

type Backend interface {
	network.SpaceLookup

	// Application returns a application state by name.
	Application(string) (Application, error)
	Machine(string) (Machine, error)
	AllMachines() ([]Machine, error)
	Unit(string) (Unit, error)
	Model() (Model, error)
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (Machine, error)
	ToolsStorage(objectstore.ObjectStore) (binarystorage.StorageCloser, error)
}

type BackendState interface {
	Backend
	MachineFromTag(string) (Machine, error)
}

type ControllerBackend interface {
	ControllerTag() names.ControllerTag
	ControllerConfig() (controller.Config, error)
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
}

type Pool interface {
	GetModel(string) (Model, func(), error)
	SystemState() (ControllerBackend, error)
}

type Model interface {
	Name() string
	UUID() string
	ModelTag() names.ModelTag
	ControllerUUID() string
	Type() state.ModelType
	CloudCredentialTag() (names.CloudCredentialTag, bool)
	CloudName() string
	CloudRegion() string
	Config() (*config.Config, error)
}

type Machine interface {
	Id() string
	Tag() names.Tag
	SetPassword(string) error
	HardwareCharacteristics() (*instance.HardwareCharacteristics, error)
	Destroy(objectstore.ObjectStore) error
	ForceDestroy(time.Duration) error
	Base() state.Base
	Containers() ([]string, error)
	Units() ([]Unit, error)
	SetKeepInstance(keepInstance bool) error
	CreateUpgradeSeriesLock([]string, state.Base) error
	RemoveUpgradeSeriesLock() error
	CompleteUpgradeSeries() error
	Principals() []string
	WatchUpgradeSeriesNotifications() (state.NotifyWatcher, error)
	GetUpgradeSeriesMessages() ([]string, bool, error)
	IsManager() bool
	IsLockedForSeriesUpgrade() (bool, error)
	UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error)
	SetUpgradeSeriesStatus(model.UpgradeSeriesStatus, string) error
	ApplicationNames() ([]string, error)
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(sInfo status.StatusInfo, recorder status.StatusHistoryRecorder) error
}

type Application interface {
	Name() string
	Charm() (Charm, bool, error)
	CharmOrigin() *state.CharmOrigin
}

type Charm interface {
	Meta() *charm.Meta
	Manifest() *charm.Manifest
}

type stateShim struct {
	*state.State
	prechcker environs.InstancePrechecker
}

func (s stateShim) Application(name string) (Application, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationShim{
		Application: a,
	}, nil
}

func (s stateShim) Machine(name string) (Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machineShim{
		Machine: m,
	}, nil
}

func (s stateShim) AddOneMachine(template state.MachineTemplate) (Machine, error) {
	m, err := s.State.AddOneMachine(s.prechcker, template)
	return machineShim{Machine: m}, err
}

func (s stateShim) AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (Machine, error) {
	m, err := s.State.AddMachineInsideNewMachine(s.prechcker, template, parentTemplate, containerType)
	return machineShim{Machine: m}, err
}

func (s stateShim) AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (Machine, error) {
	m, err := s.State.AddMachineInsideMachine(template, parentId, containerType)
	return machineShim{Machine: m}, err
}

func (s stateShim) AllMachines() ([]Machine, error) {
	all, err := s.State.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]Machine, len(all))
	for i, m := range all {
		result[i] = machineShim{Machine: m}
	}
	return result, nil
}

func (s stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return unitShim{
		Unit: u,
	}, nil
}

func (s stateShim) Model() (Model, error) {
	return s.State.Model()
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) SystemState() (ControllerBackend, error) {
	return p.pool.SystemState()
}

func (p *poolShim) GetModel(uuid string) (Model, func(), error) {
	m, ph, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return m, func() { ph.Release() }, nil
}

type applicationShim struct {
	*state.Application
}

func (a applicationShim) Charm() (Charm, bool, error) {
	ch, force, err := a.Application.Charm()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return ch, force, nil
}

type machineShim struct {
	*state.Machine
}

func (m machineShim) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, err
	}
	out := make([]Unit, len(units))
	for i, u := range units {
		out[i] = u
	}
	return out, nil
}

type unitShim struct {
	*state.Unit
}

type Unit interface {
	UnitTag() names.UnitTag
	Name() string
	AgentStatus() (status.StatusInfo, error)
	Status() (status.StatusInfo, error)
}

type StorageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State) (StorageInterface, error) {
	m, err := st.Model()
	if err != nil {
		return nil, err
	}
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	storageAccess := &storageShim{
		StorageAccess: sb,
		va:            sb,
		fa:            sb,
	}
	// CAAS models don't support volume storage yet.
	if m.Type() == state.ModelTypeCAAS {
		storageAccess.va = nil
	}
	return storageAccess, nil
}

type storageShim struct {
	storagecommon.StorageAccess
	fa storagecommon.FilesystemAccess
	va storagecommon.VolumeAccess
}

func (s *storageShim) VolumeAccess() storagecommon.VolumeAccess {
	return s.va
}

func (s *storageShim) FilesystemAccess() storagecommon.FilesystemAccess {
	return s.fa
}
