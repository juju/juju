// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/upgrades/upgradevalidation"
)

type versionSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&versionSuite{})

type versionCheckTC struct {
	from    string
	to      string
	allowed bool
	minVers string
	err     string
}

func (s *versionSuite) TestUpgradeControllerAllowed(c *gc.C) {
	for i, t := range []versionCheckTC{
		{
			from:    "2.8.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `upgrading controller to "4.0.0" is not supported from "2.8.0"`,
		}, {
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `upgrading controller to "4.0.0" is not supported from "2.9.0"`,
		}, {
			from:    "2.9.65",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `upgrading controller to "4.0.0" is not supported from "2.9.65"`,
		}, {
			from:    "2.9.37",
			to:      "3.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `upgrading controller to "3.0.0" is not supported from "2.9.37"`,
		}, {
			from:    "3.0.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "3.1.0",
		}, {
			from:    "3.0.1",
			to:      "4.0.0",
			allowed: false,
			minVers: "3.1.0",
		}, {
			from:    "3.1.0",
			to:      "4.0.0",
			allowed: true,
			minVers: "3.1.0",
		}, {
			from:    "3.3.0",
			to:      "4.0.0",
			allowed: true,
			minVers: "3.1.0",
		}, {
			from:    "4.0.0",
			to:      "3.1.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `downgrade is not allowed`,
		}, {
			from:    "5.0.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `downgrade is not allowed`,
		},
	} {
		s.assertUpgradeControllerAllowed(c, i, t)
	}
}

func (s *versionSuite) assertUpgradeControllerAllowed(c *gc.C, i int, t versionCheckTC) {
	c.Logf("testing %d", i)

	from := version.MustParse(t.from)
	to := version.MustParse(t.to)
	minVers := version.MustParse(t.minVers)
	allowed, vers, err := upgradevalidation.UpgradeControllerAllowed(from, to)
	c.Check(allowed, gc.Equals, t.allowed)
	c.Check(vers, gc.DeepEquals, minVers)
	if t.err == "" {
		c.Check(err, jc.ErrorIsNil)
	} else {
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *versionSuite) TestMigrateToAllowed(c *gc.C) {
	for i, t := range []versionCheckTC{
		{
			from:    "2.8.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `migrate to "4.0.0" is not supported from "2.8.0"`,
		}, {
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `migrate to "4.0.0" is not supported from "2.9.0"`,
		}, {
			from:    "2.9.65",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `migrate to "4.0.0" is not supported from "2.9.65"`,
		}, {
			from:    "2.9.37",
			to:      "3.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `migrate to "3.0.0" is not supported from "2.9.37"`,
		}, {
			from:    "3.0.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "3.1.0",
		}, {
			from:    "3.0.1",
			to:      "4.0.0",
			allowed: false,
			minVers: "3.1.0",
		}, {
			from:    "3.1.0",
			to:      "4.0.0",
			allowed: true,
			minVers: "3.1.0",
		}, {
			from:    "3.3.0",
			to:      "4.0.0",
			allowed: true,
			minVers: "3.1.0",
		}, {
			from:    "4.0.0",
			to:      "3.1.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `downgrade is not allowed`,
		}, {
			from:    "5.0.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `downgrade is not allowed`,
		},
	} {
		s.assertMigrateToAllowed(c, i, t)
	}
}

func (s *versionSuite) assertMigrateToAllowed(c *gc.C, i int, t versionCheckTC) {
	c.Logf("testing %d", i)

	from := version.MustParse(t.from)
	to := version.MustParse(t.to)
	minVers := version.MustParse(t.minVers)
	allowed, vers, err := upgradevalidation.MigrateToAllowed(from, to)
	c.Check(allowed, gc.Equals, t.allowed)
	c.Check(vers, gc.DeepEquals, minVers)
	if t.err == "" {
		c.Check(err, jc.ErrorIsNil)
	} else {
		c.Check(err, gc.ErrorMatches, t.err)
	}
}
