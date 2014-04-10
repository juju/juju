// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/instance"
	coretesting "launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&SCPSuite{})
var _ = gc.Suite(&expandArgsSuite{})

type SCPSuite struct {
	SSHCommonSuite
}

type expandArgsSuite struct{}

var scpTests = []struct {
	about  string
	args   []string
	result string
	error  string
}{
	{
		"scp from machine 0 to current dir",
		[]string{"0:foo", "."},
		commonArgs + "ubuntu@dummyenv-0.dns:foo .\n",
		"",
	}, {
		"scp from machine 0 to current dir with extra args",
		[]string{"0:foo", ".", "-rv", "-o", "SomeOption"},
		commonArgs + "ubuntu@dummyenv-0.dns:foo . -rv -o SomeOption\n",
		"",
	}, {
		"scp from current dir to machine 0",
		[]string{"foo", "0:"},
		commonArgs + "foo ubuntu@dummyenv-0.dns:\n",
		"",
	}, {
		"scp from current dir to machine 0 with extra args",
		[]string{"foo", "0:", "-r", "-v"},
		commonArgs + "foo ubuntu@dummyenv-0.dns: -r -v\n",
		"",
	}, {
		"scp from machine 0 to unit mysql/0",
		[]string{"0:foo", "mysql/0:/foo"},
		commonArgs + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
		"",
	}, {
		"scp from machine 0 to unit mysql/0 and extra args",
		[]string{"0:foo", "mysql/0:/foo", "-q"},
		commonArgs + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo -q\n",
		"",
	}, {
		"scp from machine 0 to unit mysql/0 and extra args before",
		[]string{"-q", "-r", "0:foo", "mysql/0:/foo"},
		commonArgs + "-q -r ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
		"",
	}, {
		"scp two local files to unit mysql/0",
		[]string{"file1", "file2", "mysql/0:/foo/"},
		commonArgs + "file1 file2 ubuntu@dummyenv-0.dns:/foo/\n",
		"",
	}, {
		"scp from unit mongodb/1 to unit mongodb/0 and multiple extra args",
		[]string{"mongodb/1:foo", "mongodb/0:", "-r", "-v", "-q", "-l5"},
		commonArgs + "ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns: -r -v -q -l5\n",
		"",
	}, {
		"scp from unit mongodb/1 to unit mongodb/0 with a --",
		[]string{"--", "-r", "-v", "mongodb/1:foo", "mongodb/0:", "-q", "-l5"},
		commonArgs + "-- -r -v ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns: -q -l5\n",
		"",
	}, {
		"scp works with IPv6 addresses",
		[]string{"ipv6-svc/0:foo", "bar"},
		commonArgs + `ubuntu@\[2001:db8::\]:foo bar` + "\n",
		"",
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
	// Simulate machine 3 has a public IPv6 address.
	ipv6Addr := instance.Address{
		Value:        "2001:db8::",
		Type:         instance.Ipv4Address, // ..because SelectPublicAddress ignores IPv6 addresses
		NetworkScope: instance.NetworkPublic,
	}
	err = m[3].SetAddresses([]instance.Address{ipv6Addr})
	c.Assert(err, gc.IsNil)
	srv = s.AddTestingService(c, "ipv6-svc", dummyCharm)
	s.addUnit(srv, m[3], c)

	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		ctx := coretesting.Context(c)
		scpcmd := &SCPCommand{}

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

var hostsFromTargets = map[string]string{
	"0":          "dummyenv-0.dns",
	"mysql/0":    "dummyenv-0.dns",
	"mongodb/0":  "dummyenv-1.dns",
	"mongodb/1":  "dummyenv-2.dns",
	"ipv6-svc/0": "2001:db8::",
}

func dummyHostsFromTarget(target string) (string, error) {
	if res, ok := hostsFromTargets[target]; ok {
		return res, nil
	}
	return target, nil
}

func (s *expandArgsSuite) TestSCPExpandArgs(c *gc.C) {
	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		// expandArgs doesn't add the commonArgs prefix, so strip it
		// off, along with the trailing '\n'
		c.Check(strings.HasPrefix(t.result, commonArgs), jc.IsTrue)
		argString := t.result[len(commonArgs):]
		c.Check(strings.HasSuffix(argString, "\n"), jc.IsTrue)
		argString = argString[:len(argString)-1]
		args := strings.Split(argString, " ")
		expanded, err := expandArgs(t.args, dummyHostsFromTarget)
		c.Check(err, gc.IsNil)
		c.Check(expanded, gc.DeepEquals, args)
	}
}

var expandTests = []struct {
	about  string
	args   []string
	result []string
}{
	{
		"don't expand params that start with '-'",
		[]string{"-0:stuff", "0:foo", "."},
		[]string{"-0:stuff", "ubuntu@dummyenv-0.dns:foo", "."},
	},
}

func (s *expandArgsSuite) TestExpandArgs(c *gc.C) {
	for i, t := range expandTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		expanded, err := expandArgs(t.args, dummyHostsFromTarget)
		c.Check(err, gc.IsNil)
		c.Check(expanded, gc.DeepEquals, t.result)
	}
}

func (s *expandArgsSuite) TestExpandArgsPropagatesErrors(c *gc.C) {
	erroringHostFromTargets := func(string) (string, error) {
		return "", fmt.Errorf("this is my error")
	}
	expanded, err := expandArgs([]string{"foo:1", "bar"}, erroringHostFromTargets)
	c.Assert(err, gc.ErrorMatches, "this is my error")
	c.Check(expanded, gc.IsNil)
}
