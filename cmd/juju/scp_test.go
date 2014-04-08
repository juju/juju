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
	"launchpad.net/juju-core/instance"
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
	proxy  bool
	error  string
}{
	{
		about:  "scp from machine 0 to current dir",
		args:   []string{"0:foo", "."},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo .\n",
	},
	{
		about:  "scp from machine 0 to current dir with extra args",
		args:   []string{"0:foo", ".", "-rv -o SomeOption"},
		result: commonArgsNoProxy + "-rv -o SomeOption ubuntu@dummyenv-0.dns:foo .\n",
	},
	{
		about:  "scp from current dir to machine 0",
		args:   []string{"foo", "0:"},
		result: commonArgsNoProxy + "foo ubuntu@dummyenv-0.dns:\n",
	},
	{
		about:  "scp from current dir to machine 0 with extra args",
		args:   []string{"foo", "0:", "-r -v"},
		result: commonArgsNoProxy + "-r -v foo ubuntu@dummyenv-0.dns:\n",
	},
	{
		about:  "scp from machine 0 to unit mysql/0",
		args:   []string{"0:foo", "mysql/0:/foo"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	},
	{
		about:  "scp from machine 0 to unit mysql/0 and extra args",
		args:   []string{"0:foo", "mysql/0:/foo", "-q"},
		result: commonArgsNoProxy + "-q ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	},
	{
		about: "scp from machine 0 to unit mysql/0 and extra args before",
		args:  []string{"-q", "-r", "0:foo", "mysql/0:/foo"},
		error: `unexpected argument "-q"; extra arguments must be last`,
	},
	{
		about:  "scp two local files to unit mysql/0",
		args:   []string{"file1", "file2", "mysql/0:/foo/"},
		result: commonArgsNoProxy + "file1 file2 ubuntu@dummyenv-0.dns:/foo/\n",
	},
	{
		about:  "scp from unit mongodb/1 to unit mongodb/0 and multiple extra args",
		args:   []string{"mongodb/1:foo", "mongodb/0:", "-r -v -q -l5"},
		result: commonArgsNoProxy + "-r -v -q -l5 ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns:\n",
	},
	{
		about:  "scp works with IPv6 addresses",
		args:   []string{"ipv6-svc/0:foo", "bar"},
		result: commonArgsNoProxy + `ubuntu@\[2001:db8::\]:foo bar` + "\n",
	},
	{
		about:  "scp from machine 0 to unit mysql/0 with proxy",
		args:   []string{"0:foo", "mysql/0:/foo"},
		result: commonArgs + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
		proxy:  true,
	},
}

func (s *SCPSuite) TestSCPCommand(c *gc.C) {
	m := s.makeMachines(4, c, true)
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	dummyCharm, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)
	srv := s.AddTestingService(c, "mysql", dummyCharm)
	s.addUnit(srv, m[0], c)

	srv = s.AddTestingService(c, "mongodb", dummyCharm)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)
	srv = s.AddTestingService(c, "ipv6-svc", dummyCharm)
	s.addUnit(srv, m[3], c)
	// Simulate machine 3 has a public IPv6 address.
	ipv6Addr := instance.Address{
		Value:        "2001:db8::",
		Type:         instance.Ipv4Address, // ..because SelectPublicAddress ignores IPv6 addresses
		NetworkScope: instance.NetworkPublic,
	}
	err = m[3].SetAddresses(ipv6Addr)
	c.Assert(err, gc.IsNil)

	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		ctx := coretesting.Context(c)
		scpcmd := &SCPCommand{}
		scpcmd.proxy = t.proxy

		err := scpcmd.Init(t.args)
		c.Check(err, gc.IsNil)
		err = scpcmd.Run(ctx)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, t.error)
			c.Check(t.result, gc.Equals, "")
		} else {
			c.Check(err, gc.IsNil)
			// we suppress stdout from scp
			c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
			c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
			data, err := ioutil.ReadFile(filepath.Join(s.bin, "scp.args"))
			c.Check(err, gc.IsNil)
			c.Check(string(data), gc.Equals, t.result)
		}
	}
}
