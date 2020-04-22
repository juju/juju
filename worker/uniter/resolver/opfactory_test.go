// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type ResolverOpFactorySuite struct {
	testing.BaseSuite
	opFactory *mockOpFactory
}

var _ = gc.Suite(&ResolverOpFactorySuite{})

func (s *ResolverOpFactorySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.opFactory = &mockOpFactory{}
}

func (s *ResolverOpFactorySuite) TestInitialState(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	c.Assert(f.LocalState, jc.DeepEquals, &resolver.LocalState{})
	c.Assert(f.RemoteState, jc.DeepEquals, remotestate.Snapshot{})
}

func (s *ResolverOpFactorySuite) TestUpdateStatusChanged(c *gc.C) {
	s.testUpdateStatusChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testUpdateStatusChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) testUpdateStatusChanged(
	c *gc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.UpdateStatusVersion = 1

	op, err := f.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
	c.Assert(err, jc.ErrorIsNil)
	f.RemoteState.UpdateStatusVersion = 2

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	// Local state's UpdateStatusVersion should be set to what
	// RemoteState's UpdateStatusVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.UpdateStatusVersion, gc.Equals, 1)
}

func (s *ResolverOpFactorySuite) TestConfigChanged(c *gc.C) {
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) TestUpgradeSeriesStatusChanged(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)

	// The initial state.
	f.LocalState.UpgradeSeriesStatus = model.UpgradeSeriesNotStarted
	f.RemoteState.UpgradeSeriesStatus = model.UpgradeSeriesPrepareStarted

	op, err := f.NewRunHook(hook.Info{Kind: hooks.PreSeriesUpgrade})
	c.Assert(err, jc.ErrorIsNil)

	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.LocalState.UpgradeSeriesStatus, gc.Equals, model.UpgradeSeriesPrepareStarted)
	f.RemoteState.UpgradeSeriesStatus = model.UpgradeSeriesPrepareCompleted

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.LocalState.UpgradeSeriesStatus, gc.Equals, model.UpgradeSeriesPrepareCompleted)
}

func (s *ResolverOpFactorySuite) TestNewHookError(c *gc.C) {
	s.opFactory.SetErrors(
		errors.New("NewRunHook fails"),
		errors.New("NewSkipHook fails"),
	)
	f := resolver.NewResolverOpFactory(s.opFactory)
	_, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, gc.ErrorMatches, "NewRunHook fails")
	_, err = f.NewSkipHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, gc.ErrorMatches, "NewSkipHook fails")
}

func (s *ResolverOpFactorySuite) testConfigChanged(
	c *gc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ConfigHash = "confighash"
	f.RemoteState.TrustHash = "trusthash"
	f.RemoteState.AddressesHash = "addresseshash"
	f.RemoteState.UpdateStatusVersion = 3

	op, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	f.RemoteState.ConfigHash = "newhash"
	f.RemoteState.TrustHash = "badhash"
	f.RemoteState.AddressesHash = "differenthash"
	f.RemoteState.UpdateStatusVersion = 4

	resultState, err := op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultState, gc.NotNil)

	// Local state's UpdateStatusVersion should be set to what
	// RemoteState's UpdateStatusVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.UpdateStatusVersion, gc.Equals, 3)
	// The hashes need to be set on the result state, because that is
	// written to disk by the executor before the next step is picked.
	c.Assert(resultState.ConfigHash, gc.Equals, "confighash")
	c.Assert(resultState.TrustHash, gc.Equals, "trusthash")
	c.Assert(resultState.AddressesHash, gc.Equals, "addresseshash")
}

func (s *ResolverOpFactorySuite) TestLeaderSettingsChanged(c *gc.C) {
	s.testLeaderSettingsChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testLeaderSettingsChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) testLeaderSettingsChanged(
	c *gc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.LeaderSettingsVersion = 1
	f.RemoteState.UpdateStatusVersion = 3

	op, err := meth(f, hook.Info{Kind: hooks.LeaderSettingsChanged})
	c.Assert(err, jc.ErrorIsNil)
	f.RemoteState.LeaderSettingsVersion = 2
	f.RemoteState.UpdateStatusVersion = 4

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	// Local state's LeaderSettingsVersion should be set to what
	// RemoteState's LeaderSettingsVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.LeaderSettingsVersion, gc.Equals, 1)
	c.Assert(f.LocalState.UpdateStatusVersion, gc.Equals, 3)
}

func (s *ResolverOpFactorySuite) TestUpgrade(c *gc.C) {
	s.testUpgrade(c, resolver.ResolverOpFactory.NewUpgrade)
	s.testUpgrade(c, resolver.ResolverOpFactory.NewRevertUpgrade)
	s.testUpgrade(c, resolver.ResolverOpFactory.NewResolvedUpgrade)
}

func (s *ResolverOpFactorySuite) testUpgrade(
	c *gc.C, meth func(resolver.ResolverOpFactory, *charm.URL) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.Conflicted = true
	curl := charm.MustParseURL("cs:trusty/mysql")
	op, err := meth(f, curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.LocalState.CharmURL, jc.DeepEquals, curl)
	c.Assert(f.LocalState.Conflicted, jc.IsFalse)
}

func (s *ResolverOpFactorySuite) TestRemoteInit(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.OutdatedRemoteCharm = true
	op, err := f.NewRemoteInit(remotestate.ContainerRunningStatus{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.LocalState.OutdatedRemoteCharm, jc.IsFalse)
}

func (s *ResolverOpFactorySuite) TestSkipRemoteInit(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.OutdatedRemoteCharm = true
	op, err := f.NewSkipRemoteInit(false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.LocalState.OutdatedRemoteCharm, jc.IsTrue)
}

func (s *ResolverOpFactorySuite) TestNewUpgradeError(c *gc.C) {
	curl := charm.MustParseURL("cs:trusty/mysql")
	s.opFactory.SetErrors(
		errors.New("NewUpgrade fails"),
		errors.New("NewRevertUpgrade fails"),
		errors.New("NewResolvedUpgrade fails"),
	)
	f := resolver.NewResolverOpFactory(s.opFactory)
	_, err := f.NewUpgrade(curl)
	c.Assert(err, gc.ErrorMatches, "NewUpgrade fails")
	_, err = f.NewRevertUpgrade(curl)
	c.Assert(err, gc.ErrorMatches, "NewRevertUpgrade fails")
	_, err = f.NewResolvedUpgrade(curl)
	c.Assert(err, gc.ErrorMatches, "NewResolvedUpgrade fails")
}

func (s *ResolverOpFactorySuite) TestCommitError(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	curl := charm.MustParseURL("cs:trusty/mysql")
	s.opFactory.op.commit = func(operation.State) (*operation.State, error) {
		return nil, errors.New("commit fails")
	}
	op, err := f.NewUpgrade(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, gc.ErrorMatches, "commit fails")
	// Local state should not have been updated. We use the same code
	// internally for all operations, so it suffices to test just the
	// upgrade case.
	c.Assert(f.LocalState.CharmURL, gc.IsNil)
}

func (s *ResolverOpFactorySuite) TestActionsCommit(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"action 1", "action 2", "action 3"}
	f.LocalState.CompletedActions = map[string]struct{}{}
	op, err := f.NewAction("action 1")
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, gc.DeepEquals, map[string]struct{}{
		"action 1": {},
	})
}

func (s *ResolverOpFactorySuite) TestActionsTrimming(c *gc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"c", "d"}
	f.LocalState.CompletedActions = map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
	}
	op, err := f.NewAction("d")
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, gc.DeepEquals, map[string]struct{}{
		"c": {},
		"d": {},
	})
}
