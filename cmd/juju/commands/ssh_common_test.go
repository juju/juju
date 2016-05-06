// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// argsSpec is a test helper which converts a number of options into
// expected ssh/scp command lines.
type argsSpec struct {
	// hostKeyChecking specifies the expected StrictHostKeyChecking
	// option.
	hostKeyChecking bool

	// withProxy specifies if the juju ProxyCommand option is
	// expected.
	withProxy bool

	// enablePty specifies if the forced PTY allocation switches are
	// expected.
	enablePty bool

	// knownHosts may either be:
	// a comma separated list of machine ids - the host keys for these
	//    machines are expected in the UserKnownHostsFile
	// "null" - the UserKnownHostsFile must be "/dev/null"
	// empty - no UserKnownHostsFile option expected
	knownHosts string

	// args specifies any other command line arguments expected. This
	// includes the SSH/SCP targets.
	args string
}

func (s *argsSpec) check(c *gc.C, output string) {
	// The first line in the output from the fake ssh/scp is the
	// command line. The remaining lines should contain the contents
	// of the UserKnownHostsFile file provided (if any).
	parts := strings.SplitN(output, "\n", 2)
	actualCommandLine := parts[0]
	actualKnownHosts := ""
	if len(parts) == 2 {
		actualKnownHosts = parts[1]
	}

	var expected []string
	expect := func(part string) {
		expected = append(expected, part)
	}
	if s.hostKeyChecking {
		expect("-o StrictHostKeyChecking yes")
	} else {
		expect("-o StrictHostKeyChecking no")
	}
	if s.withProxy {
		expect("-o ProxyCommand juju ssh --proxy=false --no-host-key-checks " +
			"--pty=false localhost nc %h %p")
	}
	expect("-o PasswordAuthentication no -o ServerAliveInterval 30")
	if s.enablePty {
		expect("-t -t")
	}
	if s.knownHosts == "null" {
		expect(`-o UserKnownHostsFile /dev/null`)
	} else if s.knownHosts == "" {
		// No UserKnownHostsFile option expected.
	} else {
		expect(`-o UserKnownHostsFile \S+`)

		// Check that the provided known_hosts file contained the
		// expected keys.
		c.Check(actualKnownHosts, gc.Matches, s.expectedKnownHosts())
	}

	// Check the command line matches what is expected.
	pattern := "^" + strings.Join(expected, " ") + " " + regexp.QuoteMeta(s.args) + "$"
	c.Check(actualCommandLine, gc.Matches, pattern)
}

func (s *argsSpec) expectedKnownHosts() string {
	out := ""
	for _, id := range strings.Split(s.knownHosts, ",") {
		out += fmt.Sprintf(".+ dsa-%s\n.+ rsa-%s\n", id, id)
	}
	return out
}

type SSHCommonSuite struct {
	testing.JujuConnSuite
	knownHostsDir string
	binDir        string
}

func (s *SSHCommonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&getJujuExecutable, func() (string, error) { return "juju", nil })

	s.binDir = c.MkDir()
	s.PatchEnvPathPrepend(s.binDir)
	for _, name := range patchedCommands {
		f, err := os.OpenFile(filepath.Join(s.binDir, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		c.Assert(err, jc.ErrorIsNil)
		_, err = f.Write([]byte(fakecommand))
		c.Assert(err, jc.ErrorIsNil)
		err = f.Close()
		c.Assert(err, jc.ErrorIsNil)
	}

	client, _ := ssh.NewOpenSSHClient()
	s.PatchValue(&ssh.DefaultClient, client)
}

func (s *SSHCommonSuite) setupModel(c *gc.C) {
	// Add machine-0 with a mysql service and mysql/0 unit
	u := s.Factory.MakeUnit(c, nil)

	// Set addresses and keys for machine-0
	m := s.getMachineForUnit(c, u)
	s.setAddresses(c, m)
	s.setKeys(c, m)

	// machine-1 has no public host keys available.
	m1 := s.Factory.MakeMachine(c, nil)
	s.setAddresses(c, m1)

	// machine-2 has IPv6 addresses
	m2 := s.Factory.MakeMachine(c, nil)
	s.setAddresses6(c, m2)
	s.setKeys(c, m2)
}

func (s *SSHCommonSuite) getMachineForUnit(c *gc.C, u *state.Unit) *state.Machine {
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	return m
}

func (s *SSHCommonSuite) setAddresses(c *gc.C, m *state.Machine) {
	addrPub := network.NewScopedAddress(
		fmt.Sprintf("%s.public", m.Id()),
		network.ScopePublic,
	)
	addrPriv := network.NewScopedAddress(
		fmt.Sprintf("%s.private", m.Id()),
		network.ScopeCloudLocal,
	)
	err := m.SetProviderAddresses(addrPub, addrPriv)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHCommonSuite) setAddresses6(c *gc.C, m *state.Machine) {
	addrPub := network.NewScopedAddress("2001:db8::1", network.ScopePublic)
	addrPriv := network.NewScopedAddress("fc00:bbb::1", network.ScopeCloudLocal)
	err := m.SetProviderAddresses(addrPub, addrPriv)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHCommonSuite) setKeys(c *gc.C, m *state.Machine) {
	id := m.Id()
	keys := state.SSHHostKeys{"dsa-" + id, "rsa-" + id}
	err := s.State.SetSSHHostKeys(m.MachineTag(), keys)
	c.Assert(err, jc.ErrorIsNil)
}
