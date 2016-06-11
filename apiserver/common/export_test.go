// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "github.com/juju/juju/apiserver/facade"

var (
	MachineJobFromParams    = machineJobFromParams
	ValidateNewFacade       = validateNewFacade
	WrapNewFacade           = wrapNewFacade
	EnvtoolsFindTools       = &envtoolsFindTools
	SendMetrics             = &sendMetrics
	MockableDestroyMachines = destroyMachines
	IsUnknownModelError     = isUnknownModelError
)

type Patcher interface {
	PatchValue(dest, value interface{})
}

// SanitizeFacades patches Facades so that for the lifetime of the test we get
// a clean slate to work from, and will not accidentally overrite/mutate the
// real facade registry.
func SanitizeFacades(patcher Patcher) {
	emptyFacades := &facade.Registry{}
	patcher.PatchValue(&Facades, emptyFacades)
}
