// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/mongo"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type prereqsSuite struct {
	testbase.LoggingSuite
	tmpdir         string
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

	// Allow non-prereq tests to pass by default.
	isPackageInstalled = func(packageName string) bool {
		return true
	}
}

func (s *prereqsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.tmpdir = c.MkDir()
	s.testMongodPath = filepath.Join(s.tmpdir, "mongod")

	s.PatchEnvironment("PATH", s.tmpdir)

	s.PatchValue(&mongo.JujuMongodPath, "/somewhere/that/wont/exist")

	s.setMongoVersion(c, lowestMongoVersion.Major, lowestMongoVersion.Minor, lowestMongoVersion.Patch)

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "Ubuntu")
	err := ioutil.WriteFile(filepath.Join(s.tmpdir, "lsb_release"), []byte(lsbrelease), 0777)
	c.Assert(err, gc.IsNil)

	// symlink $temp/dpkg-query to /bin/true, to
	// simulate package installation query responses.
	err = os.Symlink("/bin/true", filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, gc.IsNil)
	s.PatchValue(&isPackageInstalled, utils.IsPackageInstalled)
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
echo Thu Feb 13 15:53:58.210 git version: b9925db5eac369d77a3a5f5d98a145eaaacd9673
`

func (s *prereqsSuite) setMongoVersion(c *gc.C, major, minor, patch int) {
	script := fmt.Sprintf(fakeMongoFmt, major, minor, patch)
	err := ioutil.WriteFile(s.testMongodPath, []byte(script), 0777)
	c.Assert(err, gc.IsNil)
}

func (s *prereqsSuite) TestParseMongoVersion(c *gc.C) {
	s.setMongoVersion(c, 2, 2, 2)

	ver, err := mongodVersion(s.testMongodPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ver, gc.Equals, version.Number{2, 2, 2, 0})
}

func (s *prereqsSuite) TestVerifyMongod(c *gc.C) {
	lowver := version.Number{2, 2, 2, 0}
	s.PatchValue(&lowestMongoVersion, lowver)

	s.setMongoVersion(c, 3, 0, 0)
	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(c, 2, 3, 0)
	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(c, 2, 2, 3)
	c.Assert(verifyMongod(), gc.IsNil)

	s.setMongoVersion(c, 2, 2, 2)
	c.Assert(verifyMongod(), gc.IsNil)

	expected := fmt.Sprintf("installed version of mongod .* is not supported by Juju. "+
		"Juju requires version %v or greater.", lowver)

	s.setMongoVersion(c, 2, 2, 1)
	c.Assert(verifyMongod(), gc.ErrorMatches, expected)

	s.setMongoVersion(c, 2, 1, 3)
	c.Assert(verifyMongod(), gc.ErrorMatches, expected)

	s.setMongoVersion(c, 1, 3, 3)
	c.Assert(verifyMongod(), gc.ErrorMatches, expected)
}

func (s *prereqsSuite) TestParseVersion(c *gc.C) {
	data := `
db version v3.2.1
Thu Feb 13 15:53:58.210 git version: b9925db5eac369d77a3a5f5d98a145eaaacd9673
`[1:]
	v, err := parseVersion(data)
	c.Assert(err, gc.IsNil)
	c.Assert(v, gc.Equals, version.Number{3, 2, 1, 0})

	data = "this is total garbage"
	v, err = parseVersion(data)
	c.Assert(err, gc.ErrorMatches, "could not parse mongod version")
	c.Assert(v, gc.Equals, version.Zero)
}

func (s *prereqsSuite) TestMongoPrereq(c *gc.C) {
	err := os.Remove(s.testMongodPath)
	c.Assert(err, gc.IsNil)

	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*MongoDB server must be installed(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install mongodb-server(.|\n)*")

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*MongoDB server must be installed(.|\n)*")
	c.Assert(err, gc.Not(gc.ErrorMatches), "(.|\n)*apt-get install(.|\n)*")

	s.PatchValue(&lowestMongoVersion, version.Number{2, 2, 2, 0})
	s.setMongoVersion(c, 3, 0, 0)
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.IsNil)
}

func (s *prereqsSuite) TestRsyslogGnutlsPrereq(c *gc.C) {
	err := os.Remove(filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, gc.IsNil)
	err = os.Symlink("/bin/false", filepath.Join(s.tmpdir, "dpkg-query"))
	c.Assert(err, gc.IsNil)

	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*rsyslog-gnutls must be installed to enable the local provider(.|\n)*")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*apt-get install rsyslog-gnutls(.|\n)*")

	s.PatchValue(&defaultRsyslogGnutlsPath, filepath.Join(s.tmpdir, "non-existent"))
	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*non-existent: no such file or directory(.|\n)*")
	c.Assert(err, gc.Not(gc.ErrorMatches), "(.|\n)*apt-get install rsyslog-gnutls(.|\n)*")

	err = ioutil.WriteFile(defaultRsyslogGnutlsPath, nil, 0644)
	c.Assert(err, gc.IsNil)
	err = VerifyPrerequisites(instance.LXC)
	c.Assert(err, gc.IsNil)
}
