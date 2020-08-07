// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package commands

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujussh "github.com/juju/juju/network/ssh"
)

var _ = gc.Suite(&SCPSuite{})

type SCPSuite struct {
	SSHMachineSuite
}

var scpTests = []struct {
	about       string
	args        []string
	hostChecker jujussh.ReachableChecker
	forceAPIv1  bool
	expected    argsSpec
	error       string
}{
	{
		about:       "scp from machine 0 to current dir (api v1)",
		args:        []string{"0:foo", "."},
		hostChecker: validAddresses("0.private", "0.public"),
		forceAPIv1:  true,
		expected: argsSpec{
			args:            "ubuntu@0.public:foo .",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to current dir (api v2)",
		args:        []string{"0:foo", "."},
		hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
		forceAPIv1:  false,
		expected: argsSpec{
			argsMatch:       `ubuntu@0.(public|private|1\.2\.3):foo \.`, // can be any of the 3
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to current dir with extra args",
		args:        []string{"0:foo", ".", "-rv", "-o", "SomeOption"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo . -rv -o SomeOption",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from current dir to machine 0",
		args:        []string{"foo", "0:"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "foo ubuntu@0.public:",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp when no keys available",
		args:        []string{"foo", "1:"},
		hostChecker: validAddresses("1.public"),
		error:       `retrieving SSH host keys for "1": keys not found`,
	}, {
		about:       "scp when no keys available, with --no-host-key-checks",
		args:        []string{"--no-host-key-checks", "foo", "1:"},
		hostChecker: validAddresses("1.public"),
		expected: argsSpec{
			args:            "foo ubuntu@1.public:",
			hostKeyChecking: "no",
			knownHosts:      "null",
		},
	}, {
		about:       "scp from current dir to machine 0 with extra args",
		args:        []string{"foo", "0:", "-r", "-v"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "foo ubuntu@0.public: -r -v",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0",
		args:        []string{"0:foo", "mysql/0:/foo"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public:/foo",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0 and extra args",
		args:        []string{"0:foo", "mysql/0:/foo", "-q"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public:/foo -q",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about: "scp from machine 0 to unit mysql/0 and extra args before",
		args:  []string{"-q", "-r", "0:foo", "mysql/0:/foo"},
		error: "option provided but not defined: -q",
	}, {
		about:       "scp two local files to unit mysql/0",
		args:        []string{"file1", "file2", "mysql/0:/foo/"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "file1 file2 ubuntu@0.public:/foo/",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0 and multiple extra args",
		args:        []string{"0:foo", "mysql/0:", "-r", "-v", "-q", "-l5"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public: -r -v -q -l5",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp works with IPv6 addresses",
		args:        []string{"2:foo", "bar"},
		hostChecker: validAddresses("2001:db8::1"),
		expected: argsSpec{
			args:            `ubuntu@[2001:db8::1]:foo bar`,
			hostKeyChecking: "yes",
			knownHosts:      "2",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0 with proxy",
		args:        []string{"--proxy=true", "0:foo", "mysql/0:/bar"},
		hostChecker: validAddresses("0.private"),
		expected: argsSpec{
			args:            "ubuntu@0.private:foo ubuntu@0.private:/bar",
			withProxy:       true,
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from unit mysql/0 to machine 2 with a --",
		args:        []string{"--", "-r", "-v", "mysql/0:foo", "2:", "-q", "-l5"},
		hostChecker: validAddresses("0.public", "2001:db8::1"),
		expected: argsSpec{
			args:            "-r -v ubuntu@0.public:foo ubuntu@[2001:db8::1]: -q -l5",
			hostKeyChecking: "yes",
			knownHosts:      "0,2",
		},
	}, {
		about:       "scp from unit mysql/0 to current dir as 'sam' user",
		args:        []string{"sam@mysql/0:foo", "."},
		hostChecker: validAddresses("0.public"),
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
		about:       "scp from arbitrary host name to current dir",
		args:        []string{"some.host:foo", "."},
		hostChecker: validAddresses("some.host"),
		expected: argsSpec{
			args:            "some.host:foo .",
			hostKeyChecking: "",
		},
	}, {
		about:       "scp from arbitrary user & host to current dir",
		args:        []string{"someone@some.host:foo", "."},
		hostChecker: validAddresses("some.host"),
		expected: argsSpec{
			args:            "someone@some.host:foo .",
			hostKeyChecking: "",
		},
	}, {
		about:       "scp with arbitrary host name and an entity",
		args:        []string{"some.host:foo", "0:"},
		hostChecker: validAddresses("0.public"),
		error:       `can't determine host keys for all targets: consider --no-host-key-checks`,
	}, {
		about:       "scp with arbitrary host name and an entity, --no-host-key-checks, --proxy (api v1)",
		args:        []string{"--no-host-key-checks", "--proxy", "some.host:foo", "0:"},
		hostChecker: validAddresses("some.host", "0.private"),
		forceAPIv1:  true,
		expected: argsSpec{
			args:            "some.host:foo ubuntu@0.private:",
			hostKeyChecking: "no",
			withProxy:       true,
			knownHosts:      "null",
		},
	}, {
		about: "scp with no arguments",
		args:  nil,
		error: `at least two arguments required`,
	},
}

func (s *SCPSuite) TestSCPCommand(c *gc.C) {
	s.setupModel(c)

	for i, t := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)

		s.setHostChecker(t.hostChecker)
		s.setForceAPIv1(t.forceAPIv1)

		ctx, err := cmdtesting.RunCommand(c, newSCPCommand(s.hostChecker), t.args...)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, t.error)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			// we suppress stdout from scp, so get the scp args used
			// from the "scp.args" file that the fake scp executable
			// installed by SSHMachineSuite generates.
			c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
			c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
			actual, err := ioutil.ReadFile(filepath.Join(s.binDir, "scp.args"))
			c.Assert(err, jc.ErrorIsNil)
			t.expected.check(c, string(actual))
		}
	}
}
