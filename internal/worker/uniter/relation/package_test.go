// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_statetracker.go github.com/juju/juju/internal/worker/uniter/relation RelationStateTracker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_relationer.go github.com/juju/juju/internal/worker/uniter/relation Relationer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_subordinate_destroyer.go github.com/juju/juju/internal/worker/uniter/relation SubordinateDestroyer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_state_tracker.go github.com/juju/juju/internal/worker/uniter/relation StateTrackerClient
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_state_manager.go github.com/juju/juju/internal/worker/uniter/relation StateManager
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_unit_getter.go github.com/juju/juju/internal/worker/uniter/relation UnitGetter
