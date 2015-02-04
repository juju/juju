// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
)

type prereqsSuite struct {
	testing.BaseSuite
	tmpdir string
}

var _ = gc.Suite(&prereqsSuite{})

const lsbrelease = `#!/bin/sh
echo $JUJUTEST_LSB_RELEASE_ID
`

func init() {
	// Set the path to lxc-ls to
	// something we know exists. This allows
	// all of the non-prereqs tests to pass
	// even when lxc-ls can't be
	// found.
	lxclsPath = "/bin/true"

	// Allow non-prereq tests to pass by default.
	isPackageInstalled = func(packageName string) bool {
		return true
	}
}

func (s *prereqsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.tmpdir = c.MkDir()

	s.PatchEnvironment("PATH", s.tmpdir)

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "Ubuntu")
	err := ioutil.WriteFile(filepath.Join(s.tmpdir, "lsb_release"), []byte(lsbrelease), 0777)
	c.Assert(err, jc.ErrorIsNil)

	// symlink $temp/dpkg-query to /bin/true, to
	// simulate package installation query responses.
	err = symlink.New("/bin/true", filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&isPackageInstalled, apt.IsPackageInstalled)
}

func (*prereqsSuite) TestSupportedOS(c *gc.C) {
	defer func(old string) {
		goos = old
	}(goos)
	goos = "windows"
	err := VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "Unsupported operating system: windows(.|\n)*")
}

func (s *prereqsSuite) TestLxcPrereq(c *gc.C) {
	s.PatchValue(&lxclsPath, filepath.Join(s.tmpdir, "non-existent"))

	err := VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*Linux Containers \\(LXC\\) userspace tools must be\ninstalled(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install lxc(.|\n)*")

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*Linux Containers \\(LXC\\) userspace tools must be installed(.|\n)*")
	c.Assert(err, gc.Not(gc.ErrorMatches), "(.|\n)*apt-get install(.|\n)*")

	err = ioutil.WriteFile(lxclsPath, nil, 0777)
	c.Assert(err, jc.ErrorIsNil)
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
}

const jujuLocalInstalled = `#!/bin/sh
if [ "$2" = "juju-local" ]; then return 0; else return 1; fi
`

func (s *prereqsSuite) TestCloudImageUtilsPrereq(c *gc.C) {
	err := os.Remove(filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(s.tmpdir, "dpkg-query"), []byte(jujuLocalInstalled), 0777)
	c.Assert(err, jc.ErrorIsNil)

	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*cloud-image-utils must be installed(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install cloud-image-utils(.|\n)*")
}

func (s *prereqsSuite) TestJujuLocalPrereq(c *gc.C) {
	err := os.Remove(filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, jc.ErrorIsNil)
	err = symlink.New("/bin/false", filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, jc.ErrorIsNil)

	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*juju-local must be installed to enable the local provider(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install juju-local(.|\n)*")
}
