// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_statetracker.go github.com/juju/juju/worker/uniter/relation RelationStateTracker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_relationer.go github.com/juju/juju/worker/uniter/relation Relationer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_subordinate_destroyer.go github.com/juju/juju/worker/uniter/relation SubordinateDestroyer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_state_tracker_state.go github.com/juju/juju/worker/uniter/relation StateTrackerState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_uniter_api.go github.com/juju/juju/worker/uniter/relation Unit,Relation,RelationUnit
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_state_manager.go github.com/juju/juju/worker/uniter/relation StateManager
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mock_unit_getter.go github.com/juju/juju/worker/uniter/relation UnitGetter
