// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// remote firewaller facade.
type State interface {
	// ModelUUID returns the model UUID for the model
	// controlled by this state instance.
	ModelUUID() string

	// WatchSubnets returns a StringsWatcher that notifies of changes to
	// the lifecycles of the subnets in the model.
	WatchSubnets() state.StringsWatcher
}

type stateShim struct {
	*state.State
}
