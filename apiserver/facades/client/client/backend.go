// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"

	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	AllApplications() ([]*state.Application, error)
	AllUnits() ([]*state.Unit, error)
	AllRemoteApplications() ([]commoncrossmodel.RemoteApplication, error)
	AllMachines() ([]*state.Machine, error)
	AllIPAddresses() ([]*state.Address, error)
	AllLinkLayerDevices() ([]*state.LinkLayerDevice, error)
	AllEndpointBindings() (map[string]*state.Bindings, error)
	AllStatus() (*state.AllStatus, error)
	ControllerNodes() ([]state.ControllerNode, error)
	ControllerTimestamp() (*time.Time, error)
	HAPrimaryMachine() (names.MachineTag, error)
	Machine(string) (*state.Machine, error)
	MachineConstraints() (*state.MachineConstraints, error)
}

// MongoSession provides a way to get the status for the mongo replicaset.
type MongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
}

// Unit represents a state.Unit.
type Unit interface {
	Life() state.Life
	IsPrincipal() bool
	PublicAddress() (network.SpaceAddress, error)
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	cmrBackend commoncrossmodel.Backend
}

func (s *stateShim) AllRemoteApplications() ([]commoncrossmodel.RemoteApplication, error) {
	return s.cmrBackend.AllRemoteApplications()
}

func (s stateShim) ControllerNodes() ([]state.ControllerNode, error) {
	nodes, err := s.State.ControllerNodes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]state.ControllerNode, len(nodes))
	for i, n := range nodes {
		result[i] = n
	}
	return result, nil
}

type StorageInterface interface {
	storagecommon.StorageAccess
	storagecommon.VolumeAccess
	storagecommon.FilesystemAccess

	AllStorageInstances() ([]state.StorageInstance, error)
	AllFilesystems() ([]state.Filesystem, error)
	AllVolumes() ([]state.Volume, error)

	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)
	FilesystemAttachments(names.FilesystemTag) ([]state.FilesystemAttachment, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)
}

var getStorageState = func(st *state.State) (StorageInterface, error) {
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	return sb, nil
}
