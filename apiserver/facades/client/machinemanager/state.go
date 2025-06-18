// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

type Backend interface {
	Machine(string) (Machine, error)
	AllMachines() ([]Machine, error)
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
}

type Pool interface {
	SystemState() (ControllerBackend, error)
}

type Machine interface {
	Id() string
	Tag() names.Tag
	Destroy(objectstore.ObjectStore) error
	ForceDestroy(time.Duration) error
	Base() state.Base
	Containers() ([]string, error)
	Principals() []string
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(sInfo status.StatusInfo) error
}

type stateShim struct {
	*state.State
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
	m, err := s.State.AddOneMachine(template)
	return machineShim{Machine: m}, err
}

func (s stateShim) AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (Machine, error) {
	m, err := s.State.AddMachineInsideNewMachine(template, parentTemplate, containerType)
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

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) SystemState() (ControllerBackend, error) {
	return p.pool.SystemState()
}

type machineShim struct {
	*state.Machine
}

type StorageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State, modelType coremodel.ModelType) (StorageInterface, error) {
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
	if modelType == coremodel.CAAS {
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
