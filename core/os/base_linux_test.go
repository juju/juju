// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	corebase "github.com/juju/juju/core/base"
)

type linuxBaseSuite struct {
	testing.CleanupSuite
}

var _ = tc.Suite(&linuxBaseSuite{})

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
	`NAME="CentOS Linux"
ID="centos"
VERSION_ID="7"
`,
	corebase.MustParseBaseFromString("centos@7"),
	"",
}, {
	`NAME="openSUSE Leap"
ID=opensuse
VERSION_ID="42.2"
`,
	corebase.MustParseBaseFromString("opensuse@42.2"),
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
}, {
	`NAME=Fedora
VERSION="24 (Twenty Four)"
ID=fedora
VERSION_ID=24
PRETTY_NAME="Fedora 24 (Twenty Four)"
CPE_NAME="cpe:/o:fedoraproject:fedora:24"
HOME_URL="https://fedoraproject.org/"
BUG_REPORT_URL="https://bugzilla.redhat.com/"
`,
	corebase.MustParseBaseFromString("fedora@24"),
	"",
}, {

	"",
	corebase.Base{},
	"OS release file is missing ID",
}, {
	`NAME="CentOS Linux"
ID="centos"
`,
	corebase.Base{},
	"OS release file is missing VERSION_ID",
}, {
	`NAME=openSUSE
ID=opensuse
VERSION_ID="42.3"`,
	corebase.MustParseBaseFromString("opensuse@42.3"),
	"",
},
}

func (s *linuxBaseSuite) TestReadSeries(c *tc.C) {
	d := c.MkDir()
	f := filepath.Join(d, "foo")
	s.PatchValue(&osReleaseFile, f)
	for i, t := range readBaseTests {
		c.Logf("test %d", i)
		err := ioutil.WriteFile(f, []byte(t.contents), 0666)
		c.Assert(err, jc.ErrorIsNil)
		b, err := readBase()
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(b, tc.Equals, t.base)
		} else {
			c.Assert(err, tc.ErrorMatches, t.err)
		}
	}
}
