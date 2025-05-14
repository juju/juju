// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"context"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type resolverSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&resolverSuite{})

func (s *resolverSuite) TestAllowUpgrade(c *tc.C) {
	existing := &Application{
		Charm: "ch:ubuntu",
	}
	requested := &charm.ApplicationSpec{
		Charm: "ch:mysql",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithSameChannel(c *tc.C) {
	existing := &Application{
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 0,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	r := resolver{
		force: true,
		charmResolver: func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithDowngrades(c *tc.C) {
	existing := &Application{
		Name:     "ubuntu",
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 2,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	r := resolver{
		force: true,
		charmResolver: func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorMatches, `application "ubuntu": downgrades are not currently supported: deployed revision 2 is newer than requested revision 1`)
	c.Assert(ok, tc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithSameRevision(c *tc.C) {
	existing := &Application{
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 1,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	r := resolver{
		force: true,
		charmResolver: func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithDifferentChannel(c *tc.C) {
	existing := &Application{
		Name:    "ubuntu",
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "edge",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorMatches, `^application "ubuntu": upgrades not supported across channels \(existing: "stable", requested: "edge"\); use --force to override`)
	c.Assert(ok, tc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithNoBundleChannel(c *tc.C) {
	existing := &Application{
		Name:    "ubuntu",
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requested := &charm.ApplicationSpec{
		Charm: "ch:ubuntu",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorMatches, `^application "ubuntu": upgrades not supported across channels \(existing: "stable", resolved: ""\); use --force to override`)
	c.Assert(ok, tc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithDifferentChannelAndForce(c *tc.C) {
	existing := &Application{
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 0,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "edge",
	}
	requestedArch := "amd64"

	r := resolver{
		force: true,
		charmResolver: func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithNoExistingChannel(c *tc.C) {
	existing := &Application{
		Charm: "ch:ubuntu",
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorMatches, `^upgrades not supported when the channel for "" is unknown; use --force to override`)
	c.Assert(ok, tc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithNoExistingChannelWithForce(c *tc.C) {
	existing := &Application{
		Charm: "ch:ubuntu",
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	r := resolver{
		force: true,
	}
	ok, err := r.allowCharmUpgrade(c.Context(), existing, requested, requestedArch)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}
