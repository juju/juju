// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
)

type windowsVersionSuite struct{}

var _ = gc.Suite(&windowsVersionSuite{})

func (s *windowsVersionSuite) TestOSVersion(c *gc.C) {
	knownSeries := set.NewStrings()
	for _, series := range windowsVersions {
		knownSeries.Add(series)
	}
	version, err := osVersion()
	c.Assert(err, gc.IsNil)
	c.Check(version, jc.Satisfies, knownSeries.Contains)
}
