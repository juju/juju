// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/worker/uniter/runner/context"
)

type StateTrackerForTestConfig struct {
	St                StateTrackerState
	Unit              Unit
	LeadershipContext context.LeadershipContext
	Subordinate       bool
	PrincipalName     string
	CharmDir          string
	StateManager      StateManager
	NewRelationerFunc func(RelationUnit, StateManager, UnitGetter, Logger) Relationer
	Relationers       map[int]Relationer
	RemoteAppName     map[int]string
}

func NewStateTrackerForTest(cfg StateTrackerForTestConfig) (RelationStateTracker, error) {
	rst := &relationStateTracker{
		st:              cfg.St,
		unit:            cfg.Unit,
		leaderCtx:       cfg.LeadershipContext,
		abort:           make(chan struct{}),
		subordinate:     cfg.Subordinate,
		principalName:   cfg.PrincipalName,
		charmDir:        cfg.CharmDir,
		relationers:     make(map[int]Relationer),
		remoteAppName:   make(map[int]string),
		relationCreated: make(map[int]bool),
		isPeerRelation:  make(map[int]bool),
		stateMgr:        cfg.StateManager,
		logger:          loggo.GetLogger("test"),
		newRelationer:   cfg.NewRelationerFunc,
	}

	return rst, rst.loadInitialState()
}

func NewStateTrackerForSyncScopesTest(cfg StateTrackerForTestConfig) (RelationStateTracker, error) {
	return &relationStateTracker{
		st:              cfg.St,
		unit:            cfg.Unit,
		leaderCtx:       cfg.LeadershipContext,
		abort:           make(chan struct{}),
		relationers:     cfg.Relationers,
		remoteAppName:   cfg.RemoteAppName,
		relationCreated: make(map[int]bool),
		isPeerRelation:  make(map[int]bool),
		stateMgr:        cfg.StateManager,
		logger:          loggo.GetLogger("test"),
		newRelationer:   cfg.NewRelationerFunc,
		charmDir:        cfg.CharmDir,
	}, nil
}
