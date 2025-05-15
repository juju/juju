// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/verifycharmprofile"
)

type verifySuite struct{}

var _ = tc.Suite(&verifySuite{})

func (s *verifySuite) TestNextOpNotInstallNorUpgrade(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.RunAction},
	}
	remote := remotestate.Snapshot{}
	res := newVerifyCharmProfileResolver(c)

	op, err := res.NextOp(c.Context(), local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpInstallProfileNotRequired(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Install},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: false,
	}
	res := newVerifyCharmProfileResolver(c)

	op, err := res.NextOp(c.Context(), local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpInstallProfileRequiredEmptyName(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Install},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
	}
	res := newVerifyCharmProfileResolver(c)

	op, err := res.NextOp(c.Context(), local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrDoNotProceed)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpMisMatchCharmRevisions(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Upgrade},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-74",
		CharmURL:             "ch:wordpress-75",
	}
	res := newVerifyCharmProfileResolver(c)

	op, err := res.NextOp(c.Context(), local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrDoNotProceed)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpMatchingCharmRevisions(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Upgrade},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-75",
		CharmURL:             "ch:wordpress-75",
	}
	res := newVerifyCharmProfileResolver(c)

	op, err := res.NextOp(c.Context(), local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNewResolverCAAS(c *tc.C) {
	r := verifycharmprofile.NewResolver(loggertesting.WrapCheckLog(c), model.CAAS)
	op, err := r.NextOp(c.Context(), resolver.LocalState{}, remotestate.Snapshot{}, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.ErrorIsNil)
}

func newVerifyCharmProfileResolver(c *tc.C) resolver.Resolver {
	return verifycharmprofile.NewResolver(loggertesting.WrapCheckLog(c), model.IAAS)
}
