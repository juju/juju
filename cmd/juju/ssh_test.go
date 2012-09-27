package main

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

var _ = Suite(&SSHSuite{})

type SSHSuite struct {
	testing.JujuConnSuite
	oldpath string
}

// fakessh outputs its arguments to stdout for verification
var fakessh = `#!/bin/bash

echo $@
`

func (s *SSHSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	path := c.MkDir()
	s.oldpath = os.Getenv("PATH")
	os.Setenv("PATH", path+":"+s.oldpath)
	f, err := os.OpenFile(filepath.Join(path, "ssh"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	c.Assert(err, IsNil)
	_, err = f.Write([]byte(fakessh))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)
}

func (s *SSHSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.oldpath)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *SSHSuite) TestFakeSSH(c *C) {
	cmd := exec.Command("ssh", "1", "two", "III")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, "1 two III\n")
}

var sshTests = []struct {
	args   []string
	result string
}{
	{[]string{"0"}, ""},
	{[]string{"mysql/0"}, ""},
	{[]string{"mongodb/1"}, ""},
}

func (s *SSHSuite) TestSSHCommand(c *C) {
	ch := coretesting.Charms.Dir("series", "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mongodb", dummy)
	c.Assert(err, IsNil)

	for _, t := range sshTests {
		c.Logf("testing juju ssh %s", t.args)
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&SSHCommand{}, ctx, t.args)
		c.Check(code, Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), Equals, t.result)
	}
}
