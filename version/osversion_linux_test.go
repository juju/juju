// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type linuxVersionSuite struct {
	testing.BaseSuite
}

var futureReleaseFileContents = `NAME="Ubuntu"
VERSION="99.04 LTS, Star Trek"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu spock (99.04 LTS)"
VERSION_ID="99.04"
`

var distroInfoContents = `version,codename,series,created,release,eol,eol-server
12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26
99.04,Star Trek,spock,2364-04-25,2364-10-17,2365-07-17
`

var _ = gc.Suite(&linuxVersionSuite{})

func (s *linuxVersionSuite) SetUpTest(c *gc.C) {
	cleanup := version.SetSeriesVersions(make(map[string]string))
	s.AddCleanup(func(*gc.C) { cleanup() })
}

func (s *linuxVersionSuite) TestOSVersion(c *gc.C) {
	// Set up fake /etc/os-release file from the future.
	d := c.MkDir()
	release := filepath.Join(d, "future-release")
	s.PatchValue(version.OSReleaseFile, release)
	err := ioutil.WriteFile(release, []byte(futureReleaseFileContents), 0666)
	c.Assert(err, jc.ErrorIsNil)

	// Set up fake /usr/share/distro-info/ubuntu.csv, also from the future.
	distroInfo := filepath.Join(d, "ubuntu.csv")
	err = ioutil.WriteFile(distroInfo, []byte(distroInfoContents), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(version.DistroInfo, distroInfo)

	// Ensure the future series can be read even though Juju doesn't
	// know about it.
	version, err := version.ReadSeries()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version, gc.Equals, "spock")
}
