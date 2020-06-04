// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile_test

import (
	"github.com/juju/charm/v7"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/verifycharmprofile"
)

type verifySuite struct{}

var _ = gc.Suite(&verifySuite{})

func (s *verifySuite) TestNextOpNotInstallNorUpgrade(c *gc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.RunAction},
	}
	remote := remotestate.Snapshot{}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func (s *verifySuite) TestNextOpInstallProfileNotRequired(c *gc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Install},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: false,
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func (s *verifySuite) TestNextOpInstallProfileRequiredEmptyName(c *gc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Install},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, gc.Equals, resolver.ErrDoNotProceed)
	c.Assert(op, gc.IsNil)
}

func (s *verifySuite) TestNextOpMisMatchCharmRevisions(c *gc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Upgrade},
	}
	curl, err := charm.ParseURL("cs:wordpress-75")
	c.Assert(err, jc.ErrorIsNil)
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-74",
		CharmURL:             curl,
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, gc.Equals, resolver.ErrDoNotProceed)
	c.Assert(op, gc.IsNil)
}

func (s *verifySuite) TestNextOpMatchingCharmRevisions(c *gc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Upgrade},
	}
	curl, err := charm.ParseURL("cs:wordpress-75")
	c.Assert(err, jc.ErrorIsNil)
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-75",
		CharmURL:             curl,
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func newVerifyCharmProfileResolver() resolver.Resolver {
	return verifycharmprofile.NewResolver(&fakelogger{})
}

type fakelogger struct{}

func (*fakelogger) Debugf(string, ...interface{}) {}

func (*fakelogger) Tracef(string, ...interface{}) {}
