// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/testhelpers"
)

type linuxBaseSuite struct {
	testhelpers.CleanupSuite
}

func TestLinuxBaseSuite(t *testing.T) {
	tc.Run(t, &linuxBaseSuite{})
}

var readBaseTests = []struct {
	contents string
	base     corebase.Base
	err      string
}{{
	`NAME="Ubuntu"
VERSION="99.04 LTS, Star Trek"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu spock (99.04 LTS)"
VERSION_ID="99.04"
`,
	corebase.MustParseBaseFromString("ubuntu@99.04"),
	"",
}, {
	`NAME="Ubuntu"
VERSION="12.04.5 LTS, Precise Pangolin"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu precise (12.04.5 LTS)"
VERSION_ID="12.04"
`,
	corebase.MustParseBaseFromString("ubuntu@12.04"),
	"",
}, {
	`NAME="Ubuntu"
ID=ubuntu
VERSION_ID= "12.04" `,
	corebase.MustParseBaseFromString("ubuntu@12.04"),
	"",
}, {
	`NAME='Ubuntu'
ID='ubuntu'
VERSION_ID='12.04'
`,

	corebase.MustParseBaseFromString("ubuntu@12.04"),
	"",
}, {
	`NAME="Ubuntu"
VERSION="14.04.1 LTS, Trusty Tahr"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 14.04.1 LTS"
VERSION_ID="14.04"
HOME_URL="http://www.ubuntu.com/"
SUPPORT_URL="http://help.ubuntu.com/"
BUG_REPORT_URL="http://bugs.launchpad.net/ubuntu/"
`,
	corebase.MustParseBaseFromString("ubuntu@14.04"),
	"",
}}

func (s *linuxBaseSuite) TestReadSeries(c *tc.C) {
	d := c.MkDir()
	f := filepath.Join(d, "foo")
	s.PatchValue(&osReleaseFile, f)
	for i, t := range readBaseTests {
		c.Logf("test %d", i)
		err := ioutil.WriteFile(f, []byte(t.contents), 0666)
		c.Assert(err, tc.ErrorIsNil)
		b, err := readBase()
		if t.err == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(b, tc.Equals, t.base)
		} else {
			c.Assert(err, tc.ErrorMatches, t.err)
		}
	}
}
