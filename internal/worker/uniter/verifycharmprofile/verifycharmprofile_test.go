// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/verifycharmprofile"
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
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-74",
		CharmURL:             "ch:wordpress-75",
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
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-75",
		CharmURL:             "ch:wordpress-75",
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func (s *verifySuite) TestNewResolverCAAS(c *gc.C) {
	r := verifycharmprofile.NewResolver(&fakelogger{}, model.CAAS)
	op, err := r.NextOp(resolver.LocalState{}, remotestate.Snapshot{}, nil)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(op, jc.ErrorIsNil)
}

func newVerifyCharmProfileResolver() resolver.Resolver {
	return verifycharmprofile.NewResolver(&fakelogger{}, model.IAAS)
}

type fakelogger struct{}

func (*fakelogger) Debugf(string, ...interface{}) {}

func (*fakelogger) Tracef(string, ...interface{}) {}
