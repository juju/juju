// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type ResolverOpFactorySuite struct {
	testing.BaseSuite
	opFactory *mockOpFactory
}

func TestResolverOpFactorySuite(t *stdtesting.T) { tc.Run(t, &ResolverOpFactorySuite{}) }
func (s *ResolverOpFactorySuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.opFactory = &mockOpFactory{}
}

func (s *ResolverOpFactorySuite) TestInitialState(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	c.Assert(f.LocalState, tc.DeepEquals, &resolver.LocalState{})
	c.Assert(f.RemoteState, tc.DeepEquals, remotestate.Snapshot{})
}

func (s *ResolverOpFactorySuite) TestUpdateStatusChanged(c *tc.C) {
	s.testUpdateStatusChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testUpdateStatusChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) testUpdateStatusChanged(
	c *tc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.UpdateStatusVersion = 1

	op, err := f.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
	c.Assert(err, tc.ErrorIsNil)
	f.RemoteState.UpdateStatusVersion = 2

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	// Local state's UpdateStatusVersion should be set to what
	// RemoteState's UpdateStatusVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.UpdateStatusVersion, tc.Equals, 1)
}

func (s *ResolverOpFactorySuite) TestConfigChanged(c *tc.C) {
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) TestNewHookError(c *tc.C) {
	s.opFactory.SetErrors(
		errors.New("NewRunHook fails"),
		errors.New("NewSkipHook fails"),
	)
	f := resolver.NewResolverOpFactory(s.opFactory)
	_, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorMatches, "NewRunHook fails")
	_, err = f.NewSkipHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorMatches, "NewSkipHook fails")
}

func (s *ResolverOpFactorySuite) testConfigChanged(
	c *tc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ConfigHash = "confighash"
	f.RemoteState.TrustHash = "trusthash"
	f.RemoteState.AddressesHash = "addresseshash"
	f.RemoteState.UpdateStatusVersion = 3

	op, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)
	f.RemoteState.ConfigHash = "newhash"
	f.RemoteState.TrustHash = "badhash"
	f.RemoteState.AddressesHash = "differenthash"
	f.RemoteState.UpdateStatusVersion = 4

	resultState, err := op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resultState, tc.NotNil)

	// Local state's UpdateStatusVersion should be set to what
	// RemoteState's UpdateStatusVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.UpdateStatusVersion, tc.Equals, 3)
	// The hashes need to be set on the result state, because that is
	// written to disk by the executor before the next step is picked.
	c.Assert(resultState.ConfigHash, tc.Equals, "confighash")
	c.Assert(resultState.TrustHash, tc.Equals, "trusthash")
	c.Assert(resultState.AddressesHash, tc.Equals, "addresseshash")
}

func (s *ResolverOpFactorySuite) TestUpgrade(c *tc.C) {
	s.testUpgrade(c, resolver.ResolverOpFactory.NewUpgrade)
	s.testUpgrade(c, resolver.ResolverOpFactory.NewRevertUpgrade)
	s.testUpgrade(c, resolver.ResolverOpFactory.NewResolvedUpgrade)
}

func (s *ResolverOpFactorySuite) testUpgrade(
	c *tc.C, meth func(resolver.ResolverOpFactory, string) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.Conflicted = true
	curl := "ch:trusty/mysql"
	op, err := meth(f, curl)
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CharmURL, tc.DeepEquals, curl)
	c.Assert(f.LocalState.Conflicted, tc.IsFalse)
}

func (s *ResolverOpFactorySuite) TestNewUpgradeError(c *tc.C) {
	curl := "ch:trusty/mysql"
	s.opFactory.SetErrors(
		errors.New("NewUpgrade fails"),
		errors.New("NewRevertUpgrade fails"),
		errors.New("NewResolvedUpgrade fails"),
	)
	f := resolver.NewResolverOpFactory(s.opFactory)
	_, err := f.NewUpgrade(curl)
	c.Assert(err, tc.ErrorMatches, "NewUpgrade fails")
	_, err = f.NewRevertUpgrade(curl)
	c.Assert(err, tc.ErrorMatches, "NewRevertUpgrade fails")
	_, err = f.NewResolvedUpgrade(curl)
	c.Assert(err, tc.ErrorMatches, "NewResolvedUpgrade fails")
}

func (s *ResolverOpFactorySuite) TestCommitError(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)

	s.opFactory.op.commit = func(operation.State) (*operation.State, error) {
		return nil, errors.New("commit fails")
	}
	op, err := f.NewUpgrade("ch:trusty/mysql")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorMatches, "commit fails")
	// Local state should not have been updated. We use the same code
	// internally for all operations, so it suffices to test just the
	// upgrade case.
	c.Assert(f.LocalState.CharmURL, tc.Equals, "")
}

func (s *ResolverOpFactorySuite) TestActionsCommit(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"action 1", "action 2", "action 3"}
	f.LocalState.CompletedActions = map[string]struct{}{}
	op, err := f.NewAction(c.Context(), "action 1")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"action 1": {},
	})
}

func (s *ResolverOpFactorySuite) TestActionsTrimming(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"c", "d"}
	f.LocalState.CompletedActions = map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
	}
	op, err := f.NewAction(c.Context(), "d")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"c": {},
		"d": {},
	})
}

func (s *ResolverOpFactorySuite) TestFailActionsCommit(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"action 1", "action 2", "action 3"}
	f.LocalState.CompletedActions = map[string]struct{}{}
	op, err := f.NewFailAction("action 1")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"action 1": {},
	})
}

func (s *ResolverOpFactorySuite) TestFailActionsTrimming(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"c", "d"}
	f.LocalState.CompletedActions = map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
	}
	op, err := f.NewFailAction("d")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"c": {},
		"d": {},
	})
}
