// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package commands

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SCPSuite{})

type SCPSuite struct {
	SSHCommonSuite
}

var scpTests = []struct {
	about    string
	args     []string
	expected argsSpec
	error    string
}{
	{
		about: "scp from machine 0 to current dir",
		args:  []string{"0:foo", "."},
		expected: argsSpec{
			args:            "ubuntu@0.public:foo .",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from machine 0 to current dir with extra args",
		args:  []string{"0:foo", ".", "-rv", "-o", "SomeOption"},
		expected: argsSpec{
			args:            "ubuntu@0.public:foo . -rv -o SomeOption",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from current dir to machine 0",
		args:  []string{"foo", "0:"},
		expected: argsSpec{
			args:            "foo ubuntu@0.public:",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp when no keys available",
		args:  []string{"foo", "1:"},
		error: `retrieving SSH host keys for "1": no keys available`,
	}, {
		about: "scp when no keys available, with --no-host-key-checks",
		args:  []string{"--no-host-key-checks", "foo", "1:"},
		expected: argsSpec{
			args:            "foo ubuntu@1.public:",
			hostKeyChecking: "no",
			knownHosts:      "null",
		},
	}, {
		about: "scp from current dir to machine 0 with extra args",
		args:  []string{"foo", "0:", "-r", "-v"},
		expected: argsSpec{
			args:            "foo ubuntu@0.public: -r -v",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from machine 0 to unit mysql/0",
		args:  []string{"0:foo", "mysql/0:/foo"},
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public:/foo",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from machine 0 to unit mysql/0 and extra args",
		args:  []string{"0:foo", "mysql/0:/foo", "-q"},
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public:/foo -q",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from machine 0 to unit mysql/0 and extra args before",
		args:  []string{"-q", "-r", "0:foo", "mysql/0:/foo"},
		error: "flag provided but not defined: -q",
	}, {
		about: "scp two local files to unit mysql/0",
		args:  []string{"file1", "file2", "mysql/0:/foo/"},
		expected: argsSpec{
			args:            "file1 file2 ubuntu@0.public:/foo/",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from machine 0 to unit mysql/0 and multiple extra args",
		args:  []string{"0:foo", "mysql/0:", "-r", "-v", "-q", "-l5"},
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public: -r -v -q -l5",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp works with IPv6 addresses",
		args:  []string{"2:foo", "bar"},
		expected: argsSpec{
			args:            `ubuntu@[2001:db8::1]:foo bar`,
			hostKeyChecking: "yes",
			knownHosts:      "2",
		},
	}, {
		about: "scp from machine 0 to unit mysql/0 with proxy",
		args:  []string{"--proxy=true", "0:foo", "mysql/0:/bar"},
		expected: argsSpec{
			args:            "ubuntu@0.private:foo ubuntu@0.private:/bar",
			withProxy:       true,
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from unit mysql/0 to machine 2 with a --",
		args:  []string{"--", "-r", "-v", "mysql/0:foo", "2:", "-q", "-l5"},
		expected: argsSpec{
			args:            "-r -v ubuntu@0.public:foo ubuntu@[2001:db8::1]: -q -l5",
			hostKeyChecking: "yes",
			knownHosts:      "0,2",
		},
	}, {
		about: "scp from unit mysql/0 to current dir as 'sam' user",
		args:  []string{"sam@mysql/0:foo", "."},
		expected: argsSpec{
			args:            "sam@0.public:foo .",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp with no such machine",
		args:  []string{"5:foo", "bar"},
		error: `machine 5 not found`,
	}, {
		about: "scp from arbitrary host name to current dir",
		args:  []string{"some.host:foo", "."},
		expected: argsSpec{
			args:            "ubuntu@some.host:foo .",
			hostKeyChecking: "",
		},
	}, {
		about: "scp with arbitrary host name and an entity",
		args:  []string{"some.host:foo", "0:"},
		error: `can't determine host keys for all targets: consider --no-host-key-checks`,
	}, {
		about: "scp with arbitrary host name and an entity, --no-host-key-checks",
		args:  []string{"--no-host-key-checks", "some.host:foo", "0:"},
		expected: argsSpec{
			args:            "ubuntu@some.host:foo ubuntu@0.public:",
			hostKeyChecking: "no",
			knownHosts:      "null",
		},
	},
}

func (s *SCPSuite) TestSCPCommand(c *gc.C) {
	s.setupModel(c)

	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)

		ctx, err := coretesting.RunCommand(c, newSCPCommand(), t.args...)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, t.error)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			// we suppress stdout from scp, so get the scp args used
			// from the "scp.args" file that the fake scp executable
			// installed by SSHCommonSuite generates.
			c.Check(coretesting.Stderr(ctx), gc.Equals, "")
			c.Check(coretesting.Stdout(ctx), gc.Equals, "")
			actual, err := ioutil.ReadFile(filepath.Join(s.binDir, "scp.args"))
			c.Assert(err, jc.ErrorIsNil)
			t.expected.check(c, string(actual))
		}
	}
}
