// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"github.com/juju/charm/v8"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type resolverSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&resolverSuite{})

func (s *resolverSuite) TestAllowUpgrade(c *gc.C) {
	existing := &Application{
		Charm: "ch:ubuntu",
	}
	requested := &charm.ApplicationSpec{
		Charm: "ch:mysql",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithSameChannel(c *gc.C) {
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
		charmResolver: func(charm, series, channel, arch string) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithDowngrades(c *gc.C) {
	existing := &Application{
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
		charmResolver: func(charm, series, channel, arch string) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, gc.ErrorMatches, `downgrades are not currently supported`)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithSameRevision(c *gc.C) {
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
		charmResolver: func(charm, series, channel, arch string) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithDifferentChannel(c *gc.C) {
	existing := &Application{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "edge",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, gc.ErrorMatches, `^upgrades not supported across channels \(existing: "stable", requested: "edge"\); use --force to override`)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithNoBundleChannel(c *gc.C) {
	existing := &Application{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requested := &charm.ApplicationSpec{
		Charm: "ch:ubuntu",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, gc.ErrorMatches, `^upgrades not supported across channels \(existing: "stable", resolved: ""\); use --force to override`)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithDifferentChannelAndForce(c *gc.C) {
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
		charmResolver: func(charm, series, channel, arch string) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithNoExistingChannel(c *gc.C) {
	existing := &Application{
		Charm: "ch:ubuntu",
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	r := resolver{}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, gc.ErrorMatches, `^upgrades not supported when the channel for the deployed application is unknown; use --force to override`)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithNoExistingChannelWithForce(c *gc.C) {
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
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}
