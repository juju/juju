// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

type StateTrackerForTestConfig struct {
	Client            StateTrackerClient
	Unit              api.Unit
	LeadershipContext context.LeadershipContext
	Subordinate       bool
	PrincipalName     string
	CharmDir          string
	StateManager      StateManager
	NewRelationerFunc func(api.RelationUnit, StateManager, UnitGetter, logger.Logger) Relationer
	Relationers       map[int]Relationer
	RemoteAppName     map[int]string
}

func NewStateTrackerForTest(c *tc.C, cfg StateTrackerForTestConfig) (RelationStateTracker, error) {
	rst := &relationStateTracker{
		client:          cfg.Client,
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
		logger:          loggertesting.WrapCheckLogWithLevel(c, logger.DEBUG),
		newRelationer:   cfg.NewRelationerFunc,
	}
	err := rst.loadInitialState(c.Context())
	return rst, err
}

func NewStateTrackerForSyncScopesTest(c *tc.C, cfg StateTrackerForTestConfig) (RelationStateTracker, error) {
	return &relationStateTracker{
		client:          cfg.Client,
		unit:            cfg.Unit,
		leaderCtx:       cfg.LeadershipContext,
		abort:           make(chan struct{}),
		relationers:     cfg.Relationers,
		remoteAppName:   cfg.RemoteAppName,
		relationCreated: make(map[int]bool),
		isPeerRelation:  make(map[int]bool),
		stateMgr:        cfg.StateManager,
		logger:          loggertesting.WrapCheckLogWithLevel(c, logger.DEBUG),
		newRelationer:   cfg.NewRelationerFunc,
		charmDir:        cfg.CharmDir,
	}, nil
}
