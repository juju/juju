// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
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
	proxy  bool
	error  string
}{
	{
		about:  "scp from machine 0 to current dir",
		args:   []string{"0:foo", "."},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo .\n",
	}, {
		about:  "scp from machine 0 to current dir with extra args",
		args:   []string{"0:foo", ".", "-rv", "-o", "SomeOption"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo . -rv -o SomeOption\n",
	}, {
		about:  "scp from current dir to machine 0",
		args:   []string{"foo", "0:"},
		result: commonArgsNoProxy + "foo ubuntu@dummyenv-0.dns:\n",
	}, {
		about:  "scp from current dir to machine 0 with extra args",
		args:   []string{"foo", "0:", "-r", "-v"},
		result: commonArgsNoProxy + "foo ubuntu@dummyenv-0.dns: -r -v\n",
	}, {
		about:  "scp from machine 0 to unit mysql/0",
		args:   []string{"0:foo", "mysql/0:/foo"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	}, {
		about:  "scp from machine 0 to unit mysql/0 and extra args",
		args:   []string{"0:foo", "mysql/0:/foo", "-q"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo -q\n",
	}, {
		about:  "scp from machine 0 to unit mysql/0 and extra args before",
		args:   []string{"-q", "-r", "0:foo", "mysql/0:/foo"},
		result: commonArgsNoProxy + "-q -r ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	}, {
		about:  "scp two local files to unit mysql/0",
		args:   []string{"file1", "file2", "mysql/0:/foo/"},
		result: commonArgsNoProxy + "file1 file2 ubuntu@dummyenv-0.dns:/foo/\n",
	}, {
		about:  "scp from unit mongodb/1 to unit mongodb/0 and multiple extra args",
		args:   []string{"mongodb/1:foo", "mongodb/0:", "-r", "-v", "-q", "-l5"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns: -r -v -q -l5\n",
	}, {
		about:  "scp works with IPv6 addresses",
		args:   []string{"ipv6-svc/0:foo", "bar"},
		result: commonArgsNoProxy + `ubuntu@[2001:db8::1]:foo bar` + "\n",
	}, {
		about:  "scp from machine 0 to unit mysql/0 with proxy",
		args:   []string{"0:foo", "mysql/0:/foo"},
		result: commonArgs + "ubuntu@dummyenv-0.internal:foo ubuntu@dummyenv-0.internal:/foo\n",
		proxy:  true,
	}, {
		args:   []string{"0:foo", ".", "-rv", "-o", "SomeOption"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-0.dns:foo . -rv -o SomeOption\n",
	}, {
		args:   []string{"foo", "0:", "-r", "-v"},
		result: commonArgsNoProxy + "foo ubuntu@dummyenv-0.dns: -r -v\n",
	}, {
		args:   []string{"mongodb/1:foo", "mongodb/0:", "-r", "-v", "-q", "-l5"},
		result: commonArgsNoProxy + "ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns: -r -v -q -l5\n",
	}, {
		about:  "scp from unit mongodb/1 to unit mongodb/0 with a --",
		args:   []string{"--", "-r", "-v", "mongodb/1:foo", "mongodb/0:", "-q", "-l5"},
		result: commonArgsNoProxy + "-- -r -v ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns: -q -l5\n",
	}, {
		about:  "scp from unit mongodb/1 to current dir as 'mongo' user",
		args:   []string{"mongo@mongodb/1:foo", "."},
		result: commonArgsNoProxy + "mongo@dummyenv-2.dns:foo .\n",
	}, {
		about: "scp with no such machine",
		args:  []string{"5:foo", "bar"},
		error: "machine 5 not found",
	},
}

func (s *SCPSuite) TestSCPCommand(c *gc.C) {
	m := s.makeMachines(4, c, true)
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	dummyCharm, err := s.State.AddCharm(ch, curl, "dummy-path", "dummy-1-sha256")
	c.Assert(err, jc.ErrorIsNil)
	srv := s.AddTestingService(c, "mysql", dummyCharm)
	s.addUnit(srv, m[0], c)

	srv = s.AddTestingService(c, "mongodb", dummyCharm)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)
	srv = s.AddTestingService(c, "ipv6-svc", dummyCharm)
	s.addUnit(srv, m[3], c)
	// Simulate machine 3 has a public IPv6 address.
	ipv6Addr := network.NewAddress("2001:db8::1", network.ScopePublic)
	err = m[3].SetAddresses(ipv6Addr)
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		ctx := coretesting.Context(c)
		scpcmd := &SCPCommand{}
		scpcmd.proxy = t.proxy

		err := envcmd.Wrap(scpcmd).Init(t.args)
		c.Check(err, jc.ErrorIsNil)
		err = scpcmd.Run(ctx)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, t.error)
			c.Check(t.result, gc.Equals, "")
		} else {
			c.Check(err, jc.ErrorIsNil)
			// we suppress stdout from scp
			c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
			c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
			data, err := ioutil.ReadFile(filepath.Join(s.bin, "scp.args"))
			c.Check(err, jc.ErrorIsNil)
			actual := string(data)
			if t.proxy {
				actual = strings.Replace(actual, ".dns", ".internal", 2)
			}
			c.Check(actual, gc.Equals, t.result)
		}
	}
}

type userHost struct {
	user string
	host string
}

var userHostsFromTargets = map[string]userHost{
	"0":               userHost{"ubuntu", "dummyenv-0.dns"},
	"mysql/0":         userHost{"ubuntu", "dummyenv-0.dns"},
	"mongodb/0":       userHost{"ubuntu", "dummyenv-1.dns"},
	"mongodb/1":       userHost{"ubuntu", "dummyenv-2.dns"},
	"mongo@mongodb/1": userHost{"mongo", "dummyenv-2.dns"},
	"ipv6-svc/0":      userHost{"ubuntu", "2001:db8::1"},
}

func dummyHostsFromTarget(target string) (string, string, error) {
	if res, ok := userHostsFromTargets[target]; ok {
		return res.user, res.host, nil
	}
	return "ubuntu", target, nil
}

func (s *expandArgsSuite) TestSCPExpandArgs(c *gc.C) {
	for i, t := range scpTests {
		if t.error != "" {
			// We are just running a focused set of tests on
			// expandArgs, we aren't implementing the full
			// userHostsFromTargets to actually trigger errors
			continue
		}
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		// expandArgs doesn't add the commonArgs prefix, so strip it
		// off, along with the trailing '\n'
		var argString string
		if t.proxy {
			c.Check(strings.HasPrefix(t.result, commonArgs), jc.IsTrue)
			argString = t.result[len(commonArgs):]
		} else {
			c.Check(strings.HasPrefix(t.result, commonArgsNoProxy), jc.IsTrue)
			argString = t.result[len(commonArgsNoProxy):]
		}
		c.Check(strings.HasSuffix(argString, "\n"), jc.IsTrue)
		argString = argString[:len(argString)-1]
		args := strings.Split(argString, " ")
		expanded, err := expandArgs(t.args, func(target string) (string, string, error) {
			if res, ok := userHostsFromTargets[target]; ok {
				if t.proxy {
					res.host = strings.Replace(res.host, ".dns", ".internal", 1)
				}
				return res.user, res.host, nil
			}
			return "ubuntu", target, nil
		})
		c.Check(err, jc.ErrorIsNil)
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
		c.Check(err, jc.ErrorIsNil)
		c.Check(expanded, gc.DeepEquals, t.result)
	}
}

func (s *expandArgsSuite) TestExpandArgsPropagatesErrors(c *gc.C) {
	erroringUserHostFromTargets := func(string) (string, string, error) {
		return "", "", fmt.Errorf("this is my error")
	}
	expanded, err := expandArgs([]string{"foo:1", "bar"}, erroringUserHostFromTargets)
	c.Assert(err, gc.ErrorMatches, "this is my error")
	c.Check(expanded, gc.IsNil)
}
