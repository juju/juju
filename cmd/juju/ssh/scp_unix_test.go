// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	jujussh "github.com/juju/juju/internal/network/ssh"
)

func TestSCPSuiteLegacy(t *stdtesting.T) { tc.Run(t, &SCPSuiteLegacy{}) }

type SCPSuiteLegacy struct {
	SSHMachineSuite
}

var scpTests = []struct {
	about       string
	args        []string
	targets     []string
	hostChecker jujussh.ReachableChecker
	expected    argsSpec
	error       string
	noClose     bool
}{
	{
		about:       "scp from machine 0 to current dir",
		args:        []string{"0:foo", "."},
		hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
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
		targets:     []string{"1"},
		hostChecker: validAddresses("1.public"),
		error:       `attempt count exceeded: retrieving SSH host keys for "1": keys not found`,
	}, {
		about:       "scp when no keys available, with --no-host-key-checks",
		args:        []string{"--no-host-key-checks", "foo", "1:"},
		targets:     []string{"1"},
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
		targets:     []string{"0", "mysql/0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public:/foo",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0 and extra args",
		args:        []string{"0:foo", "mysql/0:/foo", "-q"},
		targets:     []string{"mysql/0", "0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public:/foo -q",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:   "scp from machine 0 to unit mysql/0 and extra args before",
		args:    []string{"-q", "-r", "0:foo", "mysql/0:/foo"},
		targets: []string{"mysql/0", "0"},
		error:   "option provided but not defined: -q",
		noClose: true,
	}, {
		about:       "scp two local files to unit mysql/0",
		args:        []string{"file1", "file2", "mysql/0:/foo/"},
		targets:     []string{"mysql/0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "file1 file2 ubuntu@0.public:/foo/",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0 and multiple extra args",
		args:        []string{"0:foo", "mysql/0:", "-r", "-v", "-q", "-l5"},
		targets:     []string{"mysql/0", "0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "ubuntu@0.public:foo ubuntu@0.public: -r -v -q -l5",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:       "scp works with IPv6 addresses",
		args:        []string{"2:foo", "bar"},
		targets:     []string{"2"},
		hostChecker: validAddresses("2001:db8::1"),
		expected: argsSpec{
			args:            `ubuntu@[2001:db8::1]:foo bar`,
			hostKeyChecking: "yes",
			knownHosts:      "2",
		},
	}, {
		about:       "scp from machine 0 to unit mysql/0 with proxy",
		args:        []string{"--proxy=true", "0:foo", "mysql/0:/bar"},
		targets:     []string{"0", "mysql/0"},
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
		targets:     []string{"mysql/0", "2"},
		hostChecker: validAddresses("0.public", "2001:db8::1"),
		expected: argsSpec{
			args:            "-r -v ubuntu@0.public:foo ubuntu@[2001:db8::1]: -q -l5",
			hostKeyChecking: "yes",
			knownHosts:      "0,2",
		},
	}, {
		about:       "scp from unit mysql/0 to current dir as 'sam' user",
		args:        []string{"sam@mysql/0:foo", "."},
		targets:     []string{"mysql/0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			args:            "sam@0.public:foo .",
			hostKeyChecking: "yes",
			knownHosts:      "0",
		},
	}, {
		about:   "scp with no such machine",
		args:    []string{"5:foo", "bar"},
		targets: []string{"5"},
		error:   `machine 5 not found`,
	}, {
		about:       "scp from arbitrary host name to current dir",
		args:        []string{"some.host:foo", "."},
		targets:     []string{"some.host"},
		hostChecker: validAddresses("some.host"),
		expected: argsSpec{
			args:            "some.host:foo .",
			hostKeyChecking: "",
		},
	}, {
		about:       "scp from arbitrary user & host to current dir",
		args:        []string{"someone@some.host:foo", "."},
		targets:     []string{"some.host"},
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
		about:   "scp with no arguments",
		args:    nil,
		error:   `at least two arguments required`,
		noClose: true,
	},
}

func (s *SCPSuiteLegacy) TestSCPCommand(c *tc.C) {
	for i, test := range scpTests {
		c.Logf("test %d: %s -> %s\n", i, test.about, test.args)
		c.Run(fmt.Sprintf("Test%d", i), func(t *stdtesting.T) {
			c := &tc.TBC{t}
			s.setHostChecker(test.hostChecker)

			ctrl := gomock.NewController(c)
			defer ctrl.Finish()

			ssh, app, status := s.setupModel(ctrl, test.expected.withProxy, test.noClose, nil, nil, test.targets...)
			scpCmd := NewSCPCommandForTest(app, ssh, status, test.hostChecker, baseTestingRetryStrategy, baseTestingRetryStrategy)

			ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(scpCmd), test.args...)
			if test.error != "" {
				c.Assert(err, tc.ErrorMatches, test.error, tc.Commentf("test %d", i))
			} else {
				c.Assert(err, tc.ErrorIsNil)
				// we suppress stdout from scp, so get the scp args used
				// from the "scp.args" file that the fake scp executable
				// installed by SSHMachineSuite generates.
				c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "", tc.Commentf("test %d", i))
				c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "", tc.Commentf("test %d", i))
				actual, err := os.ReadFile(filepath.Join(s.binDir, "scp.args"))
				c.Assert(err, tc.ErrorIsNil, tc.Commentf("test %d", i))
				test.expected.check(c, string(actual))
			}
		})
	}
}
