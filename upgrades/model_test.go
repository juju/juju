// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type ModelSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ModelSuite{})

func (s *ModelSuite) TestUpgradeAllowed(c *gc.C) {
	for _, t := range []struct {
		from    string
		to      string
		allowed bool
		minVers string
		err     string
	}{
		{
			from:    "2.8.0",
			to:      "3.0.0",
			allowed: false,
			minVers: upgrades.MinMajorUpgradeVersionValue[3],
		}, {
			from:    "2.9-rc1",
			to:      "3.0.0",
			allowed: true,
			minVers: upgrades.MinMajorUpgradeVersionValue[3],
		}, {
			from:    "2.9.0",
			to:      "3.0.0",
			allowed: true,
			minVers: upgrades.MinMajorUpgradeVersionValue[3],
		}, {
			from:    "2.9.1",
			to:      "3.0.0",
			allowed: true,
			minVers: upgrades.MinMajorUpgradeVersionValue[3],
		}, {
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `unknown version "4.0.0"`,
		},
	} {
		from := version.MustParse(t.from)
		to := version.MustParse(t.to)
		minVers := version.MustParse(t.minVers)
		allowed, vers, err := upgrades.UpgradeAllowed(from, to)
		c.Assert(allowed, gc.Equals, t.allowed)
		c.Assert(vers, gc.DeepEquals, minVers)
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}
