// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	"github.com/juju/collections/transform"
	jujuos "github.com/juju/os/v2"
	jujuseries "github.com/juju/os/v2/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type SupportedSeriesLinuxSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SupportedSeriesLinuxSuite{})

func (s *SupportedSeriesLinuxSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&LocalSeriesVersionInfo, func() (jujuos.OSType, map[string]jujuseries.SeriesVersionInfo, error) {
		return jujuos.Ubuntu, map[string]jujuseries.SeriesVersionInfo{
			"hairy": {},
		}, nil
	})
}

func (s *SupportedSeriesLinuxSuite) TestWorkloadBases(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	s.PatchValue(&UbuntuDistroInfo, tmpFile.Name())

	bases, err := WorkloadBases(time.Time{}, Base{}, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bases, gc.DeepEquals, transform.Slice([]string{
		"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04",
	}, MustParseBaseFromString))
}
