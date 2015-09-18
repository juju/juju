// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

func (s *upgradeStateContextSuite) TestInstalledBooleanFalseIfInstalling(c *gc.C) {
	oldState := &operation.State{
		Kind:     operation.Install,
		Step:     operation.Pending,
		CharmURL: charm.MustParseURL("local:quantal/charm"),
	}
	err := s.statefile.Write(oldState)
	c.Assert(err, jc.ErrorIsNil)
	err = uniter.AddInstalledToUniterState(s.unitTag, s.datadir)
	c.Assert(err, jc.ErrorIsNil)
	newState := s.readState(c)
	c.Assert(newState.Installed, gc.Equals, false)
}

func (s *upgradeStateContextSuite) TestInstalledBooleanFalseIfRunHookInstalling(c *gc.C) {
	oldState := &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.Install,
		},
	}
	err := s.statefile.Write(oldState)
	c.Assert(err, jc.ErrorIsNil)
	err = uniter.AddInstalledToUniterState(s.unitTag, s.datadir)
	c.Assert(err, jc.ErrorIsNil)
	newState := s.readState(c)
	c.Assert(newState.Installed, gc.Equals, false)
}

func (s *upgradeStateContextSuite) TestInstalledBooleanTrueIfInstalled(c *gc.C) {
	oldState := &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}
	err := s.statefile.Write(oldState)
	c.Assert(err, jc.ErrorIsNil)
	err = uniter.AddInstalledToUniterState(s.unitTag, s.datadir)
	c.Assert(err, jc.ErrorIsNil)
	newState := s.readState(c)
	c.Assert(newState.Installed, gc.Equals, true)
}
