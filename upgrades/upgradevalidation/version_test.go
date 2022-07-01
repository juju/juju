// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/upgrades/upgradevalidation"
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
	patch   bool
}

func (s *versionSuite) TestUpgradeToAllowed(c *gc.C) {
	for i, t := range []versionCheckTC{
		{
			from:    "2.8.0",
			to:      "3.0.0",
			allowed: false,
			minVers: "2.9.33",
			patch:   true,
		}, {
			from:    "2.9.33",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.33",
			patch:   true,
		}, {
			from:    "2.9.34",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.33",
			patch:   true,
		}, {
			from:    "2.9.34",
			to:      "3.0.0",
			allowed: false,
			minVers: "0.0.0",
			patch:   false, // We disallow upgrading to 3 for now.
			err:     `"3.0.0" is not a supported version`,
		},
		{
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			patch:   true,
			err:     `"4.0.0" is not a supported version`,
		},
		{
			from:    "3.0.0",
			to:      "2.0.0",
			allowed: false,
			minVers: "0.0.0",
			patch:   true,
			err:     `downgrade is not allowed`,
		},
	} {
		s.assertUpgradeToAllowed(c, i, t)
	}
}

func (s *versionSuite) assertUpgradeToAllowed(c *gc.C, i int, t versionCheckTC) {
	c.Logf("testing %d", i)
	if t.patch {
		restore := jujutesting.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
			3: version.MustParse("2.9.33"),
		})
		defer restore()
	}

	from := version.MustParse(t.from)
	to := version.MustParse(t.to)
	minVers := version.MustParse(t.minVers)
	allowed, vers, err := upgradevalidation.UpgradeToAllowed(from, to)
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
			to:      "3.0.0",
			allowed: false,
			minVers: "2.9.32",
		}, {
			from:    "2.9.32",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.32",
		}, {
			from:    "2.9.34",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.32",
		},
		{
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `"4.0.0" is not a supported version`,
		},
		{
			from:    "3.0.0",
			to:      "2.0.0",
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
