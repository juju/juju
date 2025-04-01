// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/state"
)

type BackendState interface {
	Application(string) (Application, error)
	Charm(curl string) (state.CharmRefFull, error)
	Machine(string) (Machine, error)
}

// Application defines a subset of the functionality provided by the
// state.Application type, as required by the application facade. For
// details on the methods, see the methods on state.Application with
// the same names.
type Application interface {
	AllUnits() ([]Unit, error)
	IsPrincipal() bool
}

// Machine defines a subset of the functionality provided by the
// state.Machine type, as required by the application facade. For
// details on the methods, see the methods on state.Machine with
// the same names.
type Machine interface {
	Constraints() (constraints.Value, error)
}

// Unit defines a subset of the functionality provided by the
// state.Unit type, as required by the application facade. For
// details on the methods, see the methods on state.Unit with
// the same names.
type Unit interface {
	AssignedMachineId() (string, error)
}
