// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"github.com/juju/charm/v12"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
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
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
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
		charmResolver: func(string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithDowngrades(c *gc.C) {
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
		charmResolver: func(string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, gc.ErrorMatches, `application "ubuntu": downgrades are not currently supported: deployed revision 2 is newer than requested revision 1`)
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
		charmResolver: func(string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithDifferentChannel(c *gc.C) {
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
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, gc.ErrorMatches, `^application "ubuntu": upgrades not supported across channels \(existing: "stable", requested: "edge"\); use --force to override`)
	c.Assert(ok, jc.IsFalse)
}

func (s *resolverSuite) TestAllowUpgradeWithNoBundleChannel(c *gc.C) {
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
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, gc.ErrorMatches, `^application "ubuntu": upgrades not supported across channels \(existing: "stable", resolved: ""\); use --force to override`)
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
		charmResolver: func(string, corebase.Base, string, string, int) (string, int, error) {
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
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
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, gc.ErrorMatches, `^upgrades not supported when the channel for "" is unknown; use --force to override`)
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
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *resolverSuite) TestAllowUpgradeWithBundleDefaultBase(c *gc.C) {
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

	defaultBase, err := corebase.ParseBaseFromString("ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)

	computedBase, err := computeApplicationBase(requested, defaultBase)
	c.Assert(err, jc.ErrorIsNil)

	var resolvedBase corebase.Base
	r := resolver{
		force: true,
		charmResolver: func(_ string, base corebase.Base, _ string, _ string, _ int) (string, int, error) {
			resolvedBase = base
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, computedBase)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(resolvedBase.String(), gc.Equals, "ubuntu@22.04/stable")
}

func (s *resolverSuite) TestAllowUpgradeWithExistingAppBaseFallback(c *gc.C) {
	existingBase, err := corebase.ParseBaseFromString("ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)

	existing := &Application{
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 0,
		Base:     existingBase,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	var resolvedBase corebase.Base
	r := resolver{
		force: true,
		charmResolver: func(_ string, base corebase.Base, _ string, _ string, _ int) (string, int, error) {
			resolvedBase = base
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, corebase.Base{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(resolvedBase.String(), gc.Equals, "ubuntu@22.04/stable")
}

func (s *resolverSuite) TestAllowUpgradePrefersAppBaseOverDefaultBase(c *gc.C) {
	existingBase, err := corebase.ParseBaseFromString("ubuntu@20.04")
	c.Assert(err, jc.ErrorIsNil)

	existing := &Application{
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 0,
		Base:     existingBase,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Base:    "ubuntu@24.04",
		Channel: "stable",
	}
	requestedArch := "amd64"

	defaultBase, err := corebase.ParseBaseFromString("ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)

	computedBase, err := computeApplicationBase(requested, defaultBase)
	c.Assert(err, jc.ErrorIsNil)

	var resolvedBase corebase.Base
	r := resolver{
		force: true,
		charmResolver: func(_ string, base corebase.Base, _ string, _ string, _ int) (string, int, error) {
			resolvedBase = base
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, computedBase)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(resolvedBase.String(), gc.Equals, "ubuntu@24.04/stable")
}

func (s *resolverSuite) TestAllowUpgradePrefersDefaultBaseOverExistingAppBase(c *gc.C) {
	existingBase, err := corebase.ParseBaseFromString("ubuntu@20.04")
	c.Assert(err, jc.ErrorIsNil)

	existing := &Application{
		Charm:    "ch:ubuntu",
		Channel:  "stable",
		Revision: 0,
		Base:     existingBase,
	}
	requested := &charm.ApplicationSpec{
		Charm:   "ch:ubuntu",
		Channel: "stable",
	}
	requestedArch := "amd64"

	defaultBase, err := corebase.ParseBaseFromString("ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)

	computedBase, err := computeApplicationBase(requested, defaultBase)
	c.Assert(err, jc.ErrorIsNil)

	var resolvedBase corebase.Base
	r := resolver{
		force: true,
		charmResolver: func(_ string, base corebase.Base, _ string, _ string, _ int) (string, int, error) {
			resolvedBase = base
			return "stable", 1, nil
		},
	}
	ok, err := r.allowCharmUpgrade(existing, requested, requestedArch, computedBase)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(resolvedBase.String(), gc.Equals, "ubuntu@22.04/stable")
}
