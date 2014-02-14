// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type prereqsSuite struct {
	testbase.LoggingSuite
	tmpdir         string
	oldpath        string
	testMongodPath string
}

var _ = gc.Suite(&prereqsSuite{})

const lsbrelease = `#!/bin/sh
echo $JUJUTEST_LSB_RELEASE_ID
`

func init() {
	// Set the paths to mongod and lxc-ls to
	// something we know exists. This allows
	// all of the non-prereqs tests to pass
	// even when mongodb and lxc-ls can't be
	// found.
	lxclsPath = "/bin/true"
}

func (s *prereqsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.tmpdir = c.MkDir()
	s.oldpath = os.Getenv("PATH")
	s.testMongodPath = filepath.Join(s.tmpdir, "mongod")
	lxclsPath = filepath.Join(s.tmpdir, "lxc-ls")

	path := strings.Join([]string{s.tmpdir, s.oldpath}, string(os.PathListSeparator))

	os.Setenv("PATH", path)
	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "Ubuntu")
	err := ioutil.WriteFile(filepath.Join(s.tmpdir, "lsb_release"), []byte(lsbrelease), 0777)
	c.Assert(err, gc.IsNil)
}

func (s *prereqsSuite) TearDownTest(c *gc.C) {
	os.Setenv("PATH", s.oldpath)
	s.testMongodPath = ""
	lxclsPath = "/bin/true"
	s.LoggingSuite.TearDownTest(c)
}

func (*prereqsSuite) TestSupportedOS(c *gc.C) {
	defer func(old string) {
		goos = old
	}(goos)
	goos = "windows"
	err := VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "Unsupported operating system: windows(.|\n)*")
}

const fakeMongoFmt = `#!/bin/sh
echo db version v%d.%d.%d
echo Thu Feb 13 15:53:58.210 git version: b9925db5eac369d77a3a5f5d98a145eaaacd9673`

func (s *prereqsSuite) setMongoVersion(major, minor, patch int) {
	script := fmt.Sprintf(fakeMongoFmt, major, minor, patch)
	err := ioutil.WriteFile(s.testMongodPath, []byte(script), 0777)

	if err != nil {
		panic(err)
	}
}

func (s *prereqsSuite) TestParseMongoVersion(c *gc.C) {
	s.setMongoVersion(2, 2, 2)

	ver, err := mongodVersion(s.testMongodPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ver, gc.Equals, version.Number{2, 2, 2, 0})
}

func (s *prereqsSuite) TestIsSupportedMongo(c *gc.C) {
	s.PatchValue(&lowestMongoVersion, version.Number{2, 2, 2, 0})

	s.setMongoVersion(3, 0, 0)

	c.Logf("Mongod test path: %v", s.testMongodPath)
	c.Logf("path: %v", os.Getenv("PATH"))
	c.Logf("Mongod actual path: %v", mongodPath())

	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(2, 3, 0)
	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(2, 2, 3)
	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(2, 2, 2)
	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(2, 2, 1)
	c.Assert(verifyMongod(), gc.NotNil)

	s.setMongoVersion(2, 1, 3)
	c.Assert(verifyMongod(), gc.NotNil)

	s.setMongoVersion(1, 3, 3)
	c.Assert(verifyMongod(), gc.NotNil)
}

func (s *prereqsSuite) TestMongoPrereq(c *gc.C) {
	err := VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*MongoDB server must be installed(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install mongodb-server(.|\n)*")

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*MongoDB server must be installed(.|\n)*")
	c.Assert(err, gc.Not(gc.ErrorMatches), "(.|\n)*apt-get install(.|\n)*")

	err = ioutil.WriteFile(s.testMongodPath, nil, 0777)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filepath.Join(s.tmpdir, "lxc-ls"), nil, 0777)
	c.Assert(err, gc.IsNil)
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.IsNil)
}

func (s *prereqsSuite) TestLxcPrereq(c *gc.C) {
	err := ioutil.WriteFile(s.testMongodPath, nil, 0777)
	c.Assert(err, gc.IsNil)

	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*Linux Containers \\(LXC\\) userspace tools must be\ninstalled(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install lxc(.|\n)*")

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*Linux Containers \\(LXC\\) userspace tools must be installed(.|\n)*")
	c.Assert(err, gc.Not(gc.ErrorMatches), "(.|\n)*apt-get install(.|\n)*")

	err = ioutil.WriteFile(lxclsPath, nil, 0777)
	c.Assert(err, gc.IsNil)
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.IsNil)
}
