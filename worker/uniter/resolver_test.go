// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter"
	uniteractions "github.com/juju/juju/worker/uniter/actions"
	"github.com/juju/juju/worker/uniter/container"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/leadership"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/storage"
	"github.com/juju/juju/worker/uniter/upgradeseries"
)

type resolverSuite struct {
	stub                 testing.Stub
	charmModifiedVersion int
	charmURL             *charm.URL
	remoteState          remotestate.Snapshot
	opFactory            operation.Factory
	resolver             resolver.Resolver
	resolverConfig       uniter.ResolverConfig

	clearResolved   func() error
	reportHookError func(hook.Info) error
}

type caasResolverSuite struct {
	resolverSuite
}

type iaasResolverSuite struct {
	resolverSuite
}

var _ = gc.Suite(&caasResolverSuite{})
var _ = gc.Suite(&iaasResolverSuite{})

func (s *caasResolverSuite) SetUpTest(c *gc.C) {
	attachments, err := storage.NewAttachments(&dummyStorageAccessor{}, names.NewUnitTag("u/0"), &fakeRW{}, nil)
	c.Assert(err, jc.ErrorIsNil)
	logger := loggo.GetLogger("test")
	s.resolverConfig = uniter.ResolverConfig{
		ClearResolved:       func() error { return s.clearResolved() },
		ReportHookError:     func(info hook.Info) error { return s.reportHookError(info) },
		StartRetryHookTimer: func() { s.stub.AddCall("StartRetryHookTimer") },
		StopRetryHookTimer:  func() { s.stub.AddCall("StopRetryHookTimer") },
		ShouldRetryHooks:    true,
		UpgradeSeries:       upgradeseries.NewResolver(),
		Leadership:          leadership.NewResolver(logger),
		Actions:             uniteractions.NewResolver(logger),
		CreatedRelations:    nopResolver{},
		Relations:           nopResolver{},
		Storage:             storage.NewResolver(attachments, model.CAAS),
		Commands:            nopResolver{},
		ModelType:           model.CAAS,
		Container:           container.NewResolver(),
		Logger:              logger,
	}
	s.resolverSuite.SetUpTest(c)
}

func (s *iaasResolverSuite) SetUpTest(c *gc.C) {
	attachments, err := storage.NewAttachments(&dummyStorageAccessor{}, names.NewUnitTag("u/0"), &fakeRW{}, nil)
	c.Assert(err, jc.ErrorIsNil)
	logger := loggo.GetLogger("test")
	s.resolverConfig = uniter.ResolverConfig{
		ClearResolved:       func() error { return s.clearResolved() },
		ReportHookError:     func(info hook.Info) error { return s.reportHookError(info) },
		StartRetryHookTimer: func() { s.stub.AddCall("StartRetryHookTimer") },
		StopRetryHookTimer:  func() { s.stub.AddCall("StopRetryHookTimer") },
		ShouldRetryHooks:    true,
		UpgradeSeries:       upgradeseries.NewResolver(),
		Leadership:          leadership.NewResolver(logger),
		Actions:             uniteractions.NewResolver(logger),
		CreatedRelations:    nopResolver{},
		Relations:           nopResolver{},
		Storage:             storage.NewResolver(attachments, model.IAAS),
		Commands:            nopResolver{},
		ModelType:           model.IAAS,
		Logger:              logger,
	}
	s.resolverSuite.SetUpTest(c)
	s.resolver = uniter.NewUniterResolver(s.resolverConfig)
}

func (s *resolverSuite) SetUpTest(c *gc.C) {
	s.stub = testing.Stub{}
	s.charmURL = charm.MustParseURL("cs:precise/mysql-2")
	s.remoteState = remotestate.Snapshot{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
	}
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Logger: loggo.GetLogger("test"),
	})

	s.clearResolved = func() error {
		return errors.New("unexpected resolved")
	}

	s.reportHookError = func(hook.Info) error {
		return errors.New("unexpected report hook error")
	}

	s.resolver = uniter.NewUniterResolver(s.resolverConfig)
}

// TestStartedNotInstalled tests whether the Started flag overrides the
// Installed flag being unset, in the event of an unexpected inconsistency in
// local state.
func (s *resolverSuite) TestStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

// TestNotStartedNotInstalled tests whether the next operation for an
// uninstalled local state is an install hook operation.
func (s *resolverSuite) TestNotStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run install hook")
}

func (s *iaasResolverSuite) TestUpgradeSeriesPrepareStatusChanged(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		UpgradeSeriesStatus:  model.UpgradeSeriesNotStarted,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.UpgradeSeriesStatus = model.UpgradeSeriesPrepareStarted
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run pre-series-upgrade hook")
}

func (s *iaasResolverSuite) TestPostSeriesUpgradeHookRunsWhenConditionsAreMet(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion:  s.charmModifiedVersion,
		CharmURL:              s.charmURL,
		UpgradeSeriesStatus:   model.UpgradeSeriesNotStarted,
		LeaderSettingsVersion: 1,
		State: operation.State{
			Kind:       operation.Continue,
			Installed:  true,
			Started:    true,
			ConfigHash: "version1",
		},
	}
	s.remoteState.UpgradeSeriesStatus = model.UpgradeSeriesCompleteStarted

	// Bumping the remote state versions verifies that the upgrade-series
	// completion hook takes precedence.
	s.remoteState.ConfigHash = "version2"
	s.remoteState.LeaderSettingsVersion = 2

	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run post-series-upgrade hook")
}

func (s *iaasResolverSuite) TestRunsOperationToResetLocalUpgradeSeriesStateWhenConditionsAreMet(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		UpgradeSeriesStatus:  model.UpgradeSeriesCompleted,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.UpgradeSeriesStatus = model.UpgradeSeriesNotStarted
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "complete upgrade series")
}

func (s *iaasResolverSuite) TestUniterIdlesWhenRemoteStateIsUpgradeSeriesCompleted(c *gc.C) {
	localState := resolver.LocalState{
		UpgradeSeriesStatus: model.UpgradeSeriesNotStarted,
		CharmURL:            s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.UpgradeSeriesStatus = model.UpgradeSeriesPrepareCompleted
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestHookErrorDoesNotStartRetryTimerIfShouldRetryFalse(c *gc.C) {
	s.resolverConfig.ShouldRetryHooks = false
	s.resolver = uniter.NewUniterResolver(s.resolverConfig)
	s.reportHookError = func(hook.Info) error { return nil }
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.RunHook,
			Step:      operation.Pending,
			Installed: true,
			Started:   true,
			Hook: &hook.Info{
				Kind: hooks.ConfigChanged,
			},
		},
	}
	// Run the resolver; we should not attempt a hook retry
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckNoCalls(c)
}

func (s *resolverSuite) TestHookErrorStartRetryTimer(c *gc.C) {
	s.reportHookError = func(hook.Info) error { return nil }
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.RunHook,
			Step:      operation.Pending,
			Installed: true,
			Started:   true,
			Hook: &hook.Info{
				Kind: hooks.ConfigChanged,
			},
		},
	}
	// Run the resolver twice; we should start the hook retry
	// timer on the first time through, no change on the second.
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	_, err = s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer") // no change
}

func (s *resolverSuite) TestHookErrorStartRetryTimerAgain(c *gc.C) {
	s.reportHookError = func(hook.Info) error { return nil }
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.RunHook,
			Step:      operation.Pending,
			Installed: true,
			Started:   true,
			Hook: &hook.Info{
				Kind: hooks.ConfigChanged,
			},
		},
	}

	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	s.remoteState.RetryHookVersion = 1
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
	s.stub.CheckCallNames(c, "StartRetryHookTimer") // no change
	localState.RetryHookVersion = 1

	_, err = s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StartRetryHookTimer")
}

func (s *resolverSuite) TestResolvedRetryHooksStopRetryTimer(c *gc.C) {
	// Resolving a failed hook should stop the retry timer.
	s.testResolveHookErrorStopRetryTimer(c, params.ResolvedRetryHooks)
}

func (s *resolverSuite) TestResolvedNoHooksStopRetryTimer(c *gc.C) {
	// Resolving a failed hook should stop the retry timer.
	s.testResolveHookErrorStopRetryTimer(c, params.ResolvedNoHooks)
}

func (s *resolverSuite) testResolveHookErrorStopRetryTimer(c *gc.C, mode params.ResolvedMode) {
	s.stub.ResetCalls()
	s.clearResolved = func() error { return nil }
	s.reportHookError = func(hook.Info) error { return nil }
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.RunHook,
			Step:      operation.Pending,
			Installed: true,
			Started:   true,
			Hook: &hook.Info{
				Kind: hooks.ConfigChanged,
			},
		},
	}

	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	s.remoteState.ResolvedMode = mode
	_, err = s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StopRetryHookTimer")
}

func (s *resolverSuite) TestRunHookStopRetryTimer(c *gc.C) {
	s.reportHookError = func(hook.Info) error { return nil }
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.RunHook,
			Step:      operation.Pending,
			Installed: true,
			Started:   true,
			Hook: &hook.Info{
				Kind: hooks.ConfigChanged,
			},
		},
	}

	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	localState.Kind = operation.Continue
	_, err = s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StopRetryHookTimer")
}

func (s *resolverSuite) TestRunsConfigChangedIfConfigHashChanges(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:       operation.Continue,
			Installed:  true,
			Started:    true,
			ConfigHash: "somehash",
		},
	}
	s.remoteState.ConfigHash = "differenthash"

	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestRunsConfigChangedIfTrustHashChanges(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
			TrustHash: "somehash",
		},
	}
	s.remoteState.TrustHash = "differenthash"

	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestRunsConfigChangedIfAddressesHashChanges(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:          operation.Continue,
			Installed:     true,
			Started:       true,
			AddressesHash: "somehash",
		},
	}
	s.remoteState.AddressesHash = "differenthash"

	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestNoOperationIfHashesAllMatch(c *gc.C) {
	localState := resolver.LocalState{
		CharmModifiedVersion: s.charmModifiedVersion,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:          operation.Continue,
			Installed:     true,
			Started:       true,
			ConfigHash:    "config",
			TrustHash:     "trust",
			AddressesHash: "addresses",
		},
	}
	s.remoteState.ConfigHash = "config"
	s.remoteState.TrustHash = "trust"
	s.remoteState.AddressesHash = "addresses"

	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

// fakeRW implements the storage.UnitStateReadWriter interface
// so SetUpTests can call storage.NewAttachments.  It doesn't
// need to do anything.
type fakeRW struct {
}

func (m *fakeRW) State() (params.UnitStateResult, error) {
	return params.UnitStateResult{}, nil
}

func (m *fakeRW) SetState(_ params.SetUnitStateArg) error {
	return nil
}
