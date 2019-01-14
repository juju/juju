// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/lease"
)

// SingularClaimer returns a lease.Claimer representing the exclusive right to
// manage the model.
func (st *State) SingularClaimer() lease.Claimer {
	return lazyLeaseClaimer{func() (lease.Claimer, error) {
		manager := st.workers.singularManager()
		return manager.Claimer(singularControllerNamespace, st.modelUUID())
	}}
}
