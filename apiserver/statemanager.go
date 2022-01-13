// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/clock"

	"github.com/juju/juju/worker/statemanager"
)

// stateManagerMediator encapsulates DB related capabilities to the facades.
type stateManagerMediator struct {
	stateManager statemanager.Overlord
	logger       Logger
	clock        clock.Clock
}
