// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/series"
)

type computedBaseSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&computedBaseSuite{})

func (s *computedBaseSuite) TestBaseToUse(c *gc.C) {
	trusty := series.MustParseBaseFromString("ubuntu@16.04")
	jammy := series.MustParseBaseFromString("ubuntu@22.04")
	focal := series.MustParseBaseFromString("ubuntu@20.04")
	tests := []struct {
		series         series.Base
		supportedBases []series.Base
		baseToUse      series.Base
		err            string
	}{{
		series: series.Base{},
		err:    "base not specified and charm does not define any",
	}, {
		series:    trusty,
		baseToUse: trusty,
	}, {
		series:         trusty,
		supportedBases: []series.Base{focal, trusty},
		baseToUse:      trusty,
	}, {
		series:         series.LatestLTSBase(),
		supportedBases: []series.Base{focal, series.LatestLTSBase(), trusty},
		baseToUse:      series.LatestLTSBase(),
	}, {
		series:         trusty,
		supportedBases: []series.Base{jammy, focal},
		err:            `base "ubuntu@16.04" not supported by charm.*`,
	}}
	for _, test := range tests {
		base, err := BaseForCharm(test.series, test.supportedBases)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		c.Check(base.IsCompatible(test.baseToUse), jc.IsTrue)
	}
}

func (s *computedBaseSuite) TestIsMissingBaseError(c *gc.C) {
	c.Assert(IsMissingBaseError(errMissingBase), jc.IsTrue)
	c.Assert(IsMissingBaseError(errors.New("foo")), jc.IsFalse)
}
