// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type prereqsSuite struct {
	testing.LoggingSuite
	tmpdir        string
	oldpath       string
	oldmongodpath string
}

var _ = Suite(&prereqsSuite{})

const lsbrelease = `#!/bin/sh
echo $JUJUTEST_LSB_RELEASE_ID
`

func (s *prereqsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.tmpdir = c.MkDir()
	s.oldpath = os.Getenv("PATH")
	s.oldmongodpath = mongodPath
	mongodPath = filepath.Join(s.tmpdir, "mongod")
	os.Setenv("PATH", s.tmpdir)
	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "Ubuntu")
	err := ioutil.WriteFile(filepath.Join(s.tmpdir, "lsb_release"), []byte(lsbrelease), 0777)
	c.Assert(err, IsNil)
}

func (s *prereqsSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.oldpath)
	mongodPath = s.oldmongodpath
	s.LoggingSuite.TearDownTest(c)
}

func (*prereqsSuite) TestSupportedOS(c *C) {
	defer func(old string) {
		goos = old
	}(goos)
	goos = "windows"
	err := VerifyPrerequisites()
	c.Assert(err, ErrorMatches, `Unsupported operating system: windows(.|\n)*`)
}

func (s *prereqsSuite) TestMongoPrereq(c *C) {
	err := VerifyPrerequisites()
	c.Assert(err, ErrorMatches, `(.|\n)*MongoDB server must be installed(.|\n)*`)
	c.Assert(err, ErrorMatches, `(.|\n)*apt-get install mongodb-server(.|\n)*`)

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites()
	c.Assert(err, ErrorMatches, `(.|\n)*MongoDB server must be installed(.|\n)*`)
	c.Assert(err, Not(ErrorMatches), `(.|\n)*apt-get install(.|\n)*`)

	err = ioutil.WriteFile(mongodPath, nil, 0777)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(s.tmpdir, "lxc-ls"), nil, 0777)
	c.Assert(err, IsNil)
	err = VerifyPrerequisites()
	c.Assert(err, IsNil)
}

func (s *prereqsSuite) TestLxcPrereq(c *C) {
	err := ioutil.WriteFile(mongodPath, nil, 0777)
	c.Assert(err, IsNil)

	err = VerifyPrerequisites()
	c.Assert(err, ErrorMatches, `(.|\n)*Linux Containers \(LXC\) userspace tools must be\ninstalled(.|\n)*`)
	c.Assert(err, ErrorMatches, `(.|\n)*apt-get install lxc(.|\n)*`)

	os.Setenv("JUJUTEST_LSB_RELEASE_ID", "NotUbuntu")
	err = VerifyPrerequisites()
	c.Assert(err, ErrorMatches, `(.|\n)*Linux Containers \(LXC\) userspace tools must be installed(.|\n)*`)
	c.Assert(err, Not(ErrorMatches), `(.|\n)*apt-get install(.|\n)*`)

	err = ioutil.WriteFile(filepath.Join(s.tmpdir, "lxc-ls"), nil, 0777)
	c.Assert(err, IsNil)
	err = VerifyPrerequisites()
	c.Assert(err, IsNil)
}
