// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// agentAuth implements facade.Authorizer for use in the tests.
type agentAuth struct {
	facade.Authorizer
	machine     bool
	unit        bool
	application bool
}

// AuthMachineAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthMachineAgent() bool {
	return auth.machine
}

// AuthUnitAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthUnitAgent() bool {
	return auth.unit
}

// AuthApplicationAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthApplicationAgent() bool {
	return auth.application
}

// entities is a convenience constructor for params.Entities.
func entities(tags ...string) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag
	}
	return entities
}

// authOK will always authenticate successfully.
var authOK = agentAuth{machine: true}

// unknownModel is expected to induce a permissions error.
const unknownModel = "model-01234567-89ab-cdef-0123-456789abcdef"
