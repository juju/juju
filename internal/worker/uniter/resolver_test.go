// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"fmt"

	"github.com/juju/charm/v13/hooks"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter"
	uniteractions "github.com/juju/juju/internal/worker/uniter/actions"
	unitercharm "github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/container"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/leadership"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/reboot"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/secrets"
	"github.com/juju/juju/internal/worker/uniter/storage"
	"github.com/juju/juju/internal/worker/uniter/upgradeseries"
	"github.com/juju/juju/internal/worker/uniter/verifycharmprofile"
	"github.com/juju/juju/rpc/params"
)

type baseResolverSuite struct {
	stub           testing.Stub
	charmURL       string
	remoteState    remotestate.Snapshot
	opFactory      operation.Factory
	resolver       resolver.Resolver
	resolverConfig uniter.ResolverConfig

	clearResolved   func() error
	reportHookError func(hook.Info) error

	workloadEvents        container.WorkloadEvents
	firstOptionalResolver *fakeResolver
	lastOptionalResolver  *fakeResolver
}

type resolverSuite struct {
	baseResolverSuite
}

type caasResolverSuite struct {
	resolverSuite
}

type iaasResolverSuite struct {
	resolverSuite
}

type conflictedResolverSuite struct {
	baseResolverSuite
}

type rebootResolverSuite struct {
	baseResolverSuite
}

var _ = gc.Suite(&caasResolverSuite{})
var _ = gc.Suite(&iaasResolverSuite{})
var _ = gc.Suite(&conflictedResolverSuite{})
var _ = gc.Suite(&rebootResolverSuite{})

const rebootNotDetected = false
const rebootDetected = true

func (s *caasResolverSuite) SetUpTest(c *gc.C) {
	s.resolverSuite.SetUpTest(c, model.CAAS, rebootNotDetected)
}

func (s *iaasResolverSuite) SetUpTest(c *gc.C) {
	s.resolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
}

func (s *conflictedResolverSuite) SetUpTest(_ *gc.C) {
	// NoOp, required to not panic.
}

func (s *rebootResolverSuite) SetUpTest(_ *gc.C) {
	// NoOp, required to not panic.
}

func (s *baseResolverSuite) SetUpTest(c *gc.C, modelType model.ModelType, rebootDetected bool) {
	attachments, err := storage.NewAttachments(&dummyStorageAccessor{}, names.NewUnitTag("u/0"), &fakeRW{}, nil)
	c.Assert(err, jc.ErrorIsNil)
	secretsTracker, err := secrets.NewSecrets(&dummySecretsAccessor{}, names.NewUnitTag("u/0"), &fakeRW{}, nil)
	c.Assert(err, jc.ErrorIsNil)
	logger := loggo.GetLogger("test")

	s.workloadEvents = container.NewWorkloadEvents()
	s.firstOptionalResolver = &fakeResolver{}
	s.lastOptionalResolver = &fakeResolver{}
	s.resolverConfig = uniter.ResolverConfig{
		ClearResolved:       func() error { return s.clearResolved() },
		ReportHookError:     func(info hook.Info) error { return s.reportHookError(info) },
		StartRetryHookTimer: func() { s.stub.AddCall("StartRetryHookTimer") },
		StopRetryHookTimer:  func() { s.stub.AddCall("StopRetryHookTimer") },
		ShouldRetryHooks:    true,
		UpgradeSeries:       upgradeseries.NewResolver(logger),
		Secrets:             secrets.NewSecretsResolver(logger, secretsTracker, func(_ string) {}, func(_ string) {}, func(_ []string) {}),
		Reboot:              reboot.NewResolver(logger, rebootDetected),
		Leadership:          leadership.NewResolver(logger),
		Actions:             uniteractions.NewResolver(logger),
		VerifyCharmProfile:  verifycharmprofile.NewResolver(logger, modelType),
		CreatedRelations:    nopResolver{},
		Relations:           nopResolver{},
		Storage:             storage.NewResolver(logger, attachments, modelType),
		Commands:            nopResolver{},
		ModelType:           modelType,
		OptionalResolvers: []resolver.Resolver{
			s.firstOptionalResolver,
			container.NewRemoteContainerInitResolver(),
			container.NewWorkloadHookResolver(logger, s.workloadEvents, nil),
			s.lastOptionalResolver,
		},
		Logger: logger,
	}

	s.stub = testing.Stub{}
	s.charmURL = "ch:precise/mysql-2"
	s.remoteState = remotestate.Snapshot{
		CharmURL: s.charmURL,
	}
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Logger: loggo.GetLogger("test"),
	})

	if s.clearResolved == nil {
		s.clearResolved = func() error {
			return errors.New("unexpected resolved")
		}
	}

	s.reportHookError = func(hook.Info) error {
		return nil
		//return errors.New("unexpected report hook error")
	}

	s.resolver = uniter.NewUniterResolver(s.resolverConfig)
}

// TestStartedNotInstalled tests whether the Started flag overrides the
// Installed flag being unset, in the event of an unexpected inconsistency in
// local state.
func (s *resolverSuite) TestStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

// TestNotStartedNotInstalled tests whether the next operation for an
// uninstalled local state is an install hook operation.
func (s *resolverSuite) TestNotStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run install hook")
}

func (s *iaasResolverSuite) TestUpgradeSeriesPrepareStatusChanged(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL:             s.charmURL,
		UpgradeMachineStatus: model.UpgradeSeriesNotStarted,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}

	s.remoteState.UpgradeMachineStatus = model.UpgradeSeriesPrepareStarted
	s.remoteState.UpgradeMachineTarget = "ubuntu@20.04"

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run pre-series-upgrade hook")
}

func (s *iaasResolverSuite) TestPostSeriesUpgradeHookRunsWhenConditionsAreMet(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL:              s.charmURL,
		UpgradeMachineStatus:  model.UpgradeSeriesNotStarted,
		LeaderSettingsVersion: 1,
		State: operation.State{
			Kind:       operation.Continue,
			Installed:  true,
			Started:    true,
			ConfigHash: "version1",
		},
	}
	s.remoteState.UpgradeMachineStatus = model.UpgradeSeriesCompleteStarted

	// Bumping the remote state versions verifies that the upgrade-series
	// completion hook takes precedence.
	s.remoteState.ConfigHash = "version2"
	s.remoteState.LeaderSettingsVersion = 2

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run post-series-upgrade hook")
}

func (s *iaasResolverSuite) TestRunsOperationToResetLocalUpgradeSeriesStateWhenConditionsAreMet(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL:             s.charmURL,
		UpgradeMachineStatus: model.UpgradeSeriesCompleted,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.UpgradeMachineStatus = model.UpgradeSeriesNotStarted
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "complete upgrade series")
}

func (s *iaasResolverSuite) TestUniterIdlesWhenRemoteStateIsUpgradeSeriesCompleted(c *gc.C) {
	localState := resolver.LocalState{
		UpgradeMachineStatus: model.UpgradeSeriesNotStarted,
		CharmURL:             s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.UpgradeMachineStatus = model.UpgradeSeriesPrepareCompleted
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestQueuedHookOnAgentRestart(c *gc.C) {
	s.resolver = uniter.NewUniterResolver(s.resolverConfig)
	s.reportHookError = func(hook.Info) error { return errors.New("unexpected") }
	queued := operation.Queued
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
			HookStep: &queued,
		},
	}
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
	s.stub.CheckNoCalls(c)
}

func (s *resolverSuite) TestPendingHookOnAgentRestart(c *gc.C) {
	s.resolverConfig.ShouldRetryHooks = false
	s.resolver = uniter.NewUniterResolver(s.resolverConfig)
	hookError := false
	s.reportHookError = func(hook.Info) error {
		hookError = true
		return nil
	}
	queued := operation.Pending
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
			HookStep: &queued,
		},
	}
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(hookError, jc.IsTrue)
	s.stub.CheckNoCalls(c)
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
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckNoCalls(c)
}

func (s *resolverSuite) TestHookErrorStartRetryTimer(c *gc.C) {
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
	// Run the resolver twice; we should start the hook retry
	// timer on the first time through, no change on the second.
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	_, err = s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer") // no change
}

func (s *resolverSuite) TestHookErrorStartRetryTimerAgain(c *gc.C) {
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

	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	s.remoteState.RetryHookVersion = 1
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
	s.stub.CheckCallNames(c, "StartRetryHookTimer") // no change
	localState.RetryHookVersion = 1

	_, err = s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
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

	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	s.remoteState.ResolvedMode = mode
	_, err = s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StopRetryHookTimer")
}

func (s *resolverSuite) TestRunHookStopRetryTimer(c *gc.C) {
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

	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	localState.Kind = operation.Continue
	_, err = s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StopRetryHookTimer")
}

func (s *resolverSuite) TestRunsConfigChangedIfConfigHashChanges(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:       operation.Continue,
			Installed:  true,
			Started:    true,
			ConfigHash: "somehash",
		},
	}
	s.remoteState.ConfigHash = "differenthash"

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestRunsConfigChangedIfTrustHashChanges(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
			TrustHash: "somehash",
		},
	}
	s.remoteState.TrustHash = "differenthash"

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestRunsConfigChangedIfAddressesHashChanges(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:          operation.Continue,
			Installed:     true,
			Started:       true,
			AddressesHash: "somehash",
		},
	}
	s.remoteState.AddressesHash = "differenthash"

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestNoOperationIfHashesAllMatch(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
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

	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestUpgradeOperation(c *gc.C) {
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, fmt.Sprintf("upgrade to %s", s.charmURL))
}

func (s *iaasResolverSuite) TestUpgradeOperationVerifyCPFail(c *gc.C) {
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.CharmProfileRequired = true
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestContinueUpgradeOperation(c *gc.C) {
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	s.setupForceCharmModifiedTrue()
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, fmt.Sprintf("upgrade to %s", s.charmURL))
}

func (s *resolverSuite) TestNoOperationWithOptionalResolvers(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
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

	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(s.firstOptionalResolver.callCount, gc.Equals, 1)
	c.Assert(s.lastOptionalResolver.callCount, gc.Equals, 1)
}

func (s *resolverSuite) TestOperationWithOptionalResolvers(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
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

	s.firstOptionalResolver.op = &fakeNoOp{}

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, gc.Equals, s.firstOptionalResolver.op)
	c.Assert(s.firstOptionalResolver.callCount, gc.Equals, 1)
	c.Assert(s.lastOptionalResolver.callCount, gc.Equals, 0)
}

func (s *iaasResolverSuite) TestContinueUpgradeOperationVerifyCPFail(c *gc.C) {
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	s.setupForceCharmModifiedTrue()
	s.remoteState.CharmProfileRequired = true
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestRunHookPendingUpgradeOperation(c *gc.C) {
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.RunHook,
			Hook:      &hook.Info{Kind: hooks.ConfigChanged},
			Installed: true,
			Started:   true,
			Step:      operation.Pending,
		},
	}
	s.setupForceCharmModifiedTrue()
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, fmt.Sprintf("upgrade to %s", s.charmURL))
}

func (s *resolverSuite) TestRunsSecretRotated(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
			Leader:    true,
		},
	}
	s.remoteState.Leader = true
	s.remoteState.SecretRotations = []string{"secret:9m4e2mr0ui3e8a215n4g"}

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run secret-rotate (secret:9m4e2mr0ui3e8a215n4g) hook")
}

func (s *conflictedResolverSuite) TestNextOpConflicted(c *gc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL:   s.charmURL,
		Conflicted: true,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, gc.Equals, resolver.ErrWaiting)
}

func (s *conflictedResolverSuite) TestNextOpConflictedVerifyCPFail(c *gc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL:   s.charmURL,
		Conflicted: true,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.CharmProfileRequired = true
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *conflictedResolverSuite) TestNextOpConflictedNewResolvedUpgrade(c *gc.C) {
	s.clearResolved = func() error {
		return nil
	}
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL:   s.charmURL,
		Conflicted: true,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.ResolvedMode = params.ResolvedRetryHooks
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, fmt.Sprintf("continue upgrade to %s", s.charmURL))
}

func (s *conflictedResolverSuite) TestNextOpConflictedNewRevertUpgrade(c *gc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory()
	localState := resolver.LocalState{
		CharmURL:   s.charmURL,
		Conflicted: true,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	s.setupForceCharmModifiedTrue()
	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, fmt.Sprintf("switch upgrade to %s", s.charmURL))
}

func (s *rebootResolverSuite) TestNopResolverForNonIAASModels(c *gc.C) {
	s.baseResolverSuite.SetUpTest(c, model.CAAS, rebootDetected)

	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *rebootResolverSuite) TestStartHookTriggerPostReboot(c *gc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootDetected)

	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true, // charm must be started
		},
	}
	s.remoteState.UpgradeMachineStatus = model.UpgradeSeriesNotStarted

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run start hook")

	// Ensure that start-post-reboot is only triggered once
	_, err = s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *rebootResolverSuite) TestStartHookDeferredWhenUpgradeIsInProgress(c *gc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootDetected)

	localState := resolver.LocalState{
		CharmURL:             s.charmURL,
		UpgradeMachineStatus: model.UpgradeSeriesNotStarted,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true, // charm must be started
		},
	}

	// Controller indicates a series upgrade is in progress
	statusChecks := []struct {
		status model.UpgradeSeriesStatus
		expOp  string
		expErr error
	}{
		{
			status: model.UpgradeSeriesPrepareStarted,
			expOp:  "run pre-series-upgrade hook",
		},
		{
			status: model.UpgradeSeriesPrepareRunning,
			expErr: resolver.ErrNoOperation,
		},
		{
			status: model.UpgradeSeriesPrepareCompleted,
			expErr: resolver.ErrNoOperation,
		},
		{
			status: model.UpgradeSeriesCompleteStarted,
			expOp:  "run post-series-upgrade hook",
		},
		{
			status: model.UpgradeSeriesCompleteRunning,
			expErr: resolver.ErrNoOperation,
		},
		{
			status: model.UpgradeSeriesCompleted,
			expErr: resolver.ErrNoOperation,
		},
	}

	for _, statusTest := range statusChecks {
		c.Logf("triggering resolver with upgrade status: %s", statusTest.status)
		s.remoteState.UpgradeMachineStatus = statusTest.status
		s.remoteState.UpgradeMachineTarget = "ubuntu@20.04"

		op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
		if statusTest.expErr != nil {
			c.Assert(err, gc.Equals, statusTest.expErr)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(op.String(), gc.Equals, statusTest.expOp)
		}
	}

	// Mutate remote state to indicate that upgrade has not been started
	// and run the resolver again. This time, we should get back the
	// deferred start hook.
	s.remoteState.UpgradeMachineStatus = model.UpgradeSeriesNotStarted

	op, err := s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run start hook")

	// Ensure that start-post-reboot is only triggered once
	_, err = s.resolver.NextOp(context.Background(), localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func setupUpgradeOpFactory() operation.Factory {
	return operation.NewFactory(operation.FactoryParams{
		Deployer: &fakeDeployer{},
		Logger:   loggo.GetLogger("test"),
	})
}

func (s *baseResolverSuite) setupForceCharmModifiedTrue() {
	s.remoteState.ForceCharmUpgrade = true
	s.remoteState.CharmModifiedVersion = 3
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

// fakeDeployer implements the charm.Deployter interface
// so Upgrade operations can call validate deployer not nil.  It doesn't
// need to do anything.
type fakeDeployer struct {
}

func (m *fakeDeployer) Stage(_ unitercharm.BundleInfo, _ <-chan struct{}) error {
	return nil
}

func (m *fakeDeployer) Deploy() error {
	return nil
}

type fakeResolver struct {
	callCount int
	op        operation.Operation
}

func (r *fakeResolver) NextOp(
	_ context.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	r.callCount++
	if r.op != nil {
		return r.op, nil
	}
	return nil, resolver.ErrNoOperation
}

type fakeNoOp struct {
	operation.Operation
}
