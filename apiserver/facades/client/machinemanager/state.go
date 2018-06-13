// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/juju/apiserver/common/storagecommon"
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type Backend interface {
	state.CloudAccessor

	Machine(string) (Machine, error)
	Model() (Model, error)
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)
}

type Pool interface {
	GetModel(string) (Model, func(), error)
}

type Model interface {
	Name() string
	UUID() string
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	Config() (*config.Config, error)
}

type Machine interface {
	Destroy() error
	ForceDestroy() error
	Series() string
	Units() ([]Unit, error)
	SetKeepInstance(keepInstance bool) error
	UpdateMachineSeries(string, bool) error
}

type stateShim struct {
	*state.State
}

func (s stateShim) Machine(name string) (Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return machineShim{m}, nil
}

func (s stateShim) Model() (Model, error) {
	return s.State.Model()
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) GetModel(uuid string) (Model, func(), error) {
	m, ph, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return m, func() { ph.Release() }, nil
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

type Unit interface {
	UnitTag() names.UnitTag
}

type storageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State) (storageInterface, error) {
	m, err := st.Model()
	if err != nil {
		return nil, err
	}
	if m.Type() == state.ModelTypeIAAS {
		im, _ := m.IAASModel()
		storageAccess := &iaasModelShim{Model: m, IAASModel: im}
		return storageAccess, nil
	}
	caasModel, _ := m.CAASModel()
	storageAccess := &caasModelShim{Model: m, CAASModel: caasModel}
	return storageAccess, nil
}

type iaasModelShim struct {
	*state.Model
	*state.IAASModel
}

func (im *iaasModelShim) VolumeAccess() storagecommon.VolumeAccess {
	return im
}

func (im *iaasModelShim) FilesystemAccess() storagecommon.FilesystemAccess {
	return im
}

type caasModelShim struct {
	*state.Model
	*state.CAASModel
}

func (cm *caasModelShim) VolumeAccess() storagecommon.VolumeAccess {
	// CAAS models don't support volume storage yet.
	return nil
}

func (cm *caasModelShim) FilesystemAccess() storagecommon.FilesystemAccess {
	return cm
}
