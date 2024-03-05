// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/state"
)

type BackendModel interface {
	Config() (*config.Config, error)
	ModelTag() names.ModelTag
	CloudRegion() string
	ControllerUUID() string
	Type() state.ModelType
}

type BackendState interface {
	AddCharmMetadata(state.CharmInfo) (*state.Charm, error)
	AllCharms() ([]*state.Charm, error)
	Application(string) (Application, error)
	Charm(curl string) (*state.Charm, error)
	ControllerTag() names.ControllerTag
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	PrepareCharmUpload(curl string) (services.UploadedCharm, error)
	Machine(string) (Machine, error)
	ModelUUID() string
	ModelConstraints() (constraints.Value, error)
}

// Application defines a subset of the functionality provided by the
// state.Application type, as required by the application facade. For
// details on the methods, see the methods on state.Application with
// the same names.
type Application interface {
	AllUnits() ([]Unit, error)
	Constraints() (constraints.Value, error)
	IsPrincipal() bool
}

// Machine defines a subset of the functionality provided by the
// state.Machine type, as required by the application facade. For
// details on the methods, see the methods on state.Machine with
// the same names.
type Machine interface {
	HardwareCharacteristics() (*instance.HardwareCharacteristics, error)
	Constraints() (constraints.Value, error)
}

// Unit defines a subset of the functionality provided by the
// state.Unit type, as required by the application facade. For
// details on the methods, see the methods on state.Unit with
// the same names.
type Unit interface {
	AssignedMachineId() (string, error)
}
