// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	coretesting "launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&SCPSuite{})

type SCPSuite struct {
	SSHCommonSuite
}

var scpTests = []struct {
	about  string
	args   []string
	result string
}{
	{
		"scp from machine 0 to current dir",
		[]string{"0:foo", "."},
		commonArgs + "ubuntu@dummyenv-0.dns:foo .\n",
	},
	{
		"scp from machine 0 to current dir with extra args before",
		[]string{"-- -rv", "0:foo", "."},
		commonArgs + "-rv ubuntu@dummyenv-0.dns:foo .\n",
	},
	{
		"scp from machine 0 to current dir with extra args after",
		[]string{"0:foo", ".", "-- -rv"},
		commonArgs + "-rv ubuntu@dummyenv-0.dns:foo .\n",
	},
	{
		"scp from current dir to machine 0",
		[]string{"foo", "0:"},
		commonArgs + "foo ubuntu@dummyenv-0.dns:\n",
	},
	{
		"scp from current dir to machine 0 with extra args in the middle",
		[]string{"foo", "-- -r -v", "0:"},
		commonArgs + "-r -v foo ubuntu@dummyenv-0.dns:\n",
	},
	{
		"scp from machine 0 to unit mysql/0",
		[]string{"0:foo", "mysql/0:/foo"},
		commonArgs + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	},
	{
		"scp from machine 0 to unit mysql/0 and extra args",
		[]string{"-- -q", "0:foo", "mysql/0:/foo"},
		commonArgs + "-q ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	},
	{
		"scp two local files to unit mysql/0",
		[]string{"file1", "file2", "mysql/0:/foo/"},
		commonArgs + "file1 file2 ubuntu@dummyenv-0.dns:/foo/\n",
	},
	{
		"scp from unit mongodb/1 to unit mongodb/0 and multiple extra args",
		[]string{"-- -r", "mongodb/1:foo", "-- -v", "mongodb/0:", "-- -q -l5"},
		commonArgs + "-r -v -q -l5 ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns:\n",
	},
}

func (s *SCPSuite) TestSCPCommand(c *gc.C) {
	m := s.makeMachines(3, c, true)
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)
	srv := s.AddTestingService(c, "mysql", dummy)
	s.addUnit(srv, m[0], c)

	srv = s.AddTestingService(c, "mongodb", dummy)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)

	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		ctx := coretesting.Context(c)
		scpcmd := &SCPCommand{}

		err := scpcmd.Init(t.args)
		c.Check(err, gc.IsNil)
		err = scpcmd.Run(ctx)
		c.Check(err, gc.IsNil)
		// we suppress stdout from scp
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
		data, err := ioutil.ReadFile(filepath.Join(s.bin, "scp.args"))
		c.Assert(err, gc.IsNil)
		c.Assert(string(data), gc.Equals, t.result)
	}
}
