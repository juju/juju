// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// CAASUnitProvisionerState provides the subset of global state
// required by the CAAS operator facade.
type CAASUnitProvisionerState interface {
	ControllerConfig() (controller.Config, error)
	Application(string) (Application, error)
	FindEntity(names.Tag) (state.Entity, error)
	Model() (Model, error)
	WatchApplications() state.StringsWatcher
}

// StorageBackend provides the subset of backend storage
// functionality needed by the CAAS operator facade.
type StorageBackend interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	FilesystemAttachment(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	UnitStorageAttachments(unit names.UnitTag) ([]state.StorageAttachment, error)
}

// Model provides the subset of CAAS model state required
// by the CAAS operator facade.
type Model interface {
	ModelConfig() (*config.Config, error)
	PodSpec(tag names.ApplicationTag) (string, error)
	WatchPodSpec(tag names.ApplicationTag) (state.NotifyWatcher, error)
}

// Application provides the subset of application state
// required by the CAAS operator facade.
type Application interface {
	WatchUnits() state.StringsWatcher
	ApplicationConfig() (application.ConfigAttributes, error)
	AllUnits() (units []Unit, err error)
	AddOperation(state.UnitUpdateProperties) *state.AddUnitOperation
	UpdateUnits(*state.UpdateUnitsOperation) error
	UpdateCloudService(providerId string, addreses []network.Address) error
	Life() state.Life
	Name() string
}

type stateShim struct {
	*state.State
}

func (s stateShim) Application(id string) (Application, error) {
	app, err := s.State.Application(id)
	if err != nil {
		return nil, err
	}
	return applicationShim{app}, nil
}

func (s stateShim) Model() (Model, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return model.CAASModel()
}

type applicationShim struct {
	*state.Application
}

func (a applicationShim) AllUnits() ([]Unit, error) {
	all, err := a.Application.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]Unit, len(all))
	for i, u := range all {
		result[i] = u
	}
	return result, nil
}

type Unit interface {
	Name() string
	Life() state.Life
	UnitTag() names.UnitTag
	ContainerInfo() (state.CloudContainer, error)
	AgentStatus() (status.StatusInfo, error)
	UpdateOperation(props state.UnitUpdateProperties) *state.UpdateUnitOperation
	DestroyOperation() *state.DestroyUnitOperation
}
