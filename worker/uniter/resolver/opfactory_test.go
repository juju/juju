// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

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
	c.Assert(f.LocalState, jc.DeepEquals, resolver.LocalState{})
	c.Assert(f.RemoteState, jc.DeepEquals, remotestate.Snapshot{})
}

func (s *ResolverOpFactorySuite) TestConfigChanged(c *gc.C) {
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewSkipHook)
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
	f.RemoteState.ConfigVersion = 1

	op, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	f.RemoteState.ConfigVersion = 2

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	// Local state's ConfigVersion should be set to what
	// RemoteState's ConfigVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.ConfigVersion, gc.Equals, 1)
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

	op, err := meth(f, hook.Info{Kind: hooks.LeaderSettingsChanged})
	c.Assert(err, jc.ErrorIsNil)
	f.RemoteState.LeaderSettingsVersion = 2

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	// Local state's LeaderSettingsVersion should be set to what
	// RemoteState's LeaderSettingsVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.LeaderSettingsVersion, gc.Equals, 1)
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
	curl := charm.MustParseURL("cs:trusty/mysql")
	op, err := meth(f, curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.LocalState.CharmURL, jc.DeepEquals, curl)
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
		return nil, errors.New("Commit fails")
	}
	op, err := f.NewUpgrade(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, gc.ErrorMatches, "Commit fails")
	// Local state should not have been updated. We use the same code
	// internally for all operations, so it suffices to test just the
	// upgrade case.
	c.Assert(f.LocalState.CharmURL, gc.IsNil)
}
