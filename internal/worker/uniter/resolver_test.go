// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
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
	"github.com/juju/juju/internal/worker/uniter/verifycharmprofile"
	"github.com/juju/juju/rpc/params"
)

type baseResolverSuite struct {
	stub           testhelpers.Stub
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

func TestCaasResolverSuite(t *testing.T) { tc.Run(t, &caasResolverSuite{}) }
func TestIaasResolverSuite(t *testing.T) { tc.Run(t, &iaasResolverSuite{}) }
func TestConflictedResolverSuite(t *testing.T) {
	tc.Run(t, &conflictedResolverSuite{})
}

func TestRebootResolverSuite(t *testing.T) { tc.Run(t, &rebootResolverSuite{}) }

const rebootNotDetected = false
const rebootDetected = true

func (s *caasResolverSuite) SetUpTest(c *tc.C) {
	s.resolverSuite.SetUpTest(c, model.CAAS, rebootNotDetected)
}

func (s *iaasResolverSuite) SetUpTest(c *tc.C) {
	s.resolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
}

func (s *conflictedResolverSuite) SetUpTest(_ *tc.C) {
	// NoOp, required to not panic.
}

func (s *rebootResolverSuite) SetUpTest(_ *tc.C) {
	// NoOp, required to not panic.
}

func (s *baseResolverSuite) SetUpTest(c *tc.C, modelType model.ModelType, rebootDetected bool) {
	attachments, err := storage.NewAttachments(c.Context(), &dummyStorageAccessor{}, names.NewUnitTag("u/0"), &fakeRW{}, nil)
	c.Assert(err, tc.ErrorIsNil)
	secretsTracker, err := secrets.NewSecrets(c.Context(), &dummySecretsAccessor{}, names.NewUnitTag("u/0"), &fakeRW{}, nil)
	c.Assert(err, tc.ErrorIsNil)
	logger := loggertesting.WrapCheckLog(c)

	s.workloadEvents = container.NewWorkloadEvents()
	s.firstOptionalResolver = &fakeResolver{}
	s.lastOptionalResolver = &fakeResolver{}
	s.resolverConfig = uniter.ResolverConfig{
		ClearResolved:       func() error { return s.clearResolved() },
		ReportHookError:     func(_ context.Context, info hook.Info) error { return s.reportHookError(info) },
		StartRetryHookTimer: func() { s.stub.AddCall("StartRetryHookTimer") },
		StopRetryHookTimer:  func() { s.stub.AddCall("StopRetryHookTimer") },
		ShouldRetryHooks:    true,
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
			container.NewWorkloadHookResolver(logger, s.workloadEvents, nil),
			s.lastOptionalResolver,
		},
		Logger: logger,
	}

	s.stub = testhelpers.Stub{}
	s.charmURL = "ch:precise/mysql-2"
	s.remoteState = remotestate.Snapshot{
		CharmURL: s.charmURL,
	}
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Logger: loggertesting.WrapCheckLog(c),
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
func (s *resolverSuite) TestStartedNotInstalled(c *tc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

// TestNotStartedNotInstalled tests whether the next operation for an
// uninstalled local state is an install hook operation.
func (s *resolverSuite) TestNotStartedNotInstalled(c *tc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run install hook")
}

func (s *resolverSuite) TestQueuedHookOnAgentRestart(c *tc.C) {
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
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run config-changed hook")
	s.stub.CheckNoCalls(c)
}

func (s *resolverSuite) TestPendingHookOnAgentRestart(c *tc.C) {
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
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(hookError, tc.IsTrue)
	s.stub.CheckNoCalls(c)
}

func (s *resolverSuite) TestHookErrorDoesNotStartRetryTimerIfShouldRetryFalse(c *tc.C) {
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
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckNoCalls(c)
}

func (s *resolverSuite) TestHookErrorStartRetryTimer(c *tc.C) {
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
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	_, err = s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer") // no change
}

func (s *resolverSuite) TestHookErrorStartRetryTimerAgain(c *tc.C) {
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

	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	s.remoteState.RetryHookVersion = 1
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run config-changed hook")
	s.stub.CheckCallNames(c, "StartRetryHookTimer") // no change
	localState.RetryHookVersion = 1

	_, err = s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StartRetryHookTimer")
}

func (s *resolverSuite) TestResolvedRetryHooksStopRetryTimer(c *tc.C) {
	// Resolving a failed hook should stop the retry timer.
	s.testResolveHookErrorStopRetryTimer(c, params.ResolvedRetryHooks)
}

func (s *resolverSuite) TestResolvedNoHooksStopRetryTimer(c *tc.C) {
	// Resolving a failed hook should stop the retry timer.
	s.testResolveHookErrorStopRetryTimer(c, params.ResolvedNoHooks)
}

func (s *resolverSuite) testResolveHookErrorStopRetryTimer(c *tc.C, mode params.ResolvedMode) {
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

	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	s.remoteState.ResolvedMode = mode
	_, err = s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StopRetryHookTimer")
}

func (s *resolverSuite) TestRunHookStopRetryTimer(c *tc.C) {
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

	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer")

	localState.Kind = operation.Continue
	_, err = s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	s.stub.CheckCallNames(c, "StartRetryHookTimer", "StopRetryHookTimer")
}

func (s *resolverSuite) TestRunsConfigChangedIfConfigHashChanges(c *tc.C) {
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

	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestRunsConfigChangedIfTrustHashChanges(c *tc.C) {
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

	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestRunsConfigChangedIfAddressesHashChanges(c *tc.C) {
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

	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run config-changed hook")
}

func (s *resolverSuite) TestNoOperationIfHashesAllMatch(c *tc.C) {
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

	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestUpgradeOperation(c *tc.C) {
	opFactory := setupUpgradeOpFactory(c)
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, fmt.Sprintf("upgrade to %s", s.charmURL))
}

func (s *iaasResolverSuite) TestUpgradeOperationVerifyCPFail(c *tc.C) {
	opFactory := setupUpgradeOpFactory(c)
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	s.remoteState.CharmProfileRequired = true
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestContinueUpgradeOperation(c *tc.C) {
	opFactory := setupUpgradeOpFactory(c)
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	s.setupForceCharmModifiedTrue()
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, fmt.Sprintf("upgrade to %s", s.charmURL))
}

func (s *resolverSuite) TestNoOperationWithOptionalResolvers(c *tc.C) {
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

	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(s.firstOptionalResolver.callCount, tc.Equals, 1)
	c.Assert(s.lastOptionalResolver.callCount, tc.Equals, 1)
}

func (s *resolverSuite) TestOperationWithOptionalResolvers(c *tc.C) {
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

	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op, tc.Equals, s.firstOptionalResolver.op)
	c.Assert(s.firstOptionalResolver.callCount, tc.Equals, 1)
	c.Assert(s.lastOptionalResolver.callCount, tc.Equals, 0)
}

func (s *iaasResolverSuite) TestContinueUpgradeOperationVerifyCPFail(c *tc.C) {
	opFactory := setupUpgradeOpFactory(c)
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
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestRunHookPendingUpgradeOperation(c *tc.C) {
	opFactory := setupUpgradeOpFactory(c)
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
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, fmt.Sprintf("upgrade to %s", s.charmURL))
}

func (s *resolverSuite) TestRunsSecretRotated(c *tc.C) {
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

	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-rotate (secret:9m4e2mr0ui3e8a215n4g) hook")
}

func (s *conflictedResolverSuite) TestNextOpConflicted(c *tc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory(c)
	localState := resolver.LocalState{
		CharmURL:   s.charmURL,
		Conflicted: true,
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.Equals, resolver.ErrWaiting)
}

func (s *conflictedResolverSuite) TestNextOpConflictedVerifyCPFail(c *tc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory(c)
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
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *conflictedResolverSuite) TestNextOpConflictedNewResolvedUpgrade(c *tc.C) {
	s.clearResolved = func() error {
		return nil
	}
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory(c)
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
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, fmt.Sprintf("continue upgrade to %s", s.charmURL))
}

func (s *conflictedResolverSuite) TestNextOpConflictedNewRevertUpgrade(c *tc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootNotDetected)
	opFactory := setupUpgradeOpFactory(c)
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
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, fmt.Sprintf("switch upgrade to %s", s.charmURL))
}

func (s *rebootResolverSuite) TestNopResolverForNonIAASModels(c *tc.C) {
	s.baseResolverSuite.SetUpTest(c, model.CAAS, rebootDetected)

	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *rebootResolverSuite) TestStartHookTriggerPostReboot(c *tc.C) {
	s.baseResolverSuite.SetUpTest(c, model.IAAS, rebootDetected)

	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
			Started:   true, // charm must be started
		},
	}

	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run start hook")

	// Ensure that start-post-reboot is only triggered once
	_, err = s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func setupUpgradeOpFactory(c *tc.C) operation.Factory {
	return operation.NewFactory(operation.FactoryParams{
		Deployer: &fakeDeployer{},
		Logger:   loggertesting.WrapCheckLog(c),
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

func (m *fakeRW) State(context.Context) (params.UnitStateResult, error) {
	return params.UnitStateResult{}, nil
}

func (m *fakeRW) SetState(context.Context, params.SetUnitStateArg) error {
	return nil
}

// fakeDeployer implements the charm.Deployter interface
// so Upgrade operations can call validate deployer not nil.  It doesn't
// need to do anything.
type fakeDeployer struct {
}

func (m *fakeDeployer) Stage(_ context.Context, _ unitercharm.BundleInfo) error {
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
