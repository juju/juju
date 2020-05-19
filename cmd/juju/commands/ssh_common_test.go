// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/juju/testing"
	jujussh "github.com/juju/juju/network/ssh"
	"github.com/juju/juju/state"
)

// argsSpec is a test helper which converts a number of options into
// expected ssh/scp command lines.
type argsSpec struct {
	// hostKeyChecking specifies the expected StrictHostKeyChecking
	// option.
	hostKeyChecking string

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
	// includes the SSH/SCP targets. Ignored if argsMatch is set as well.
	args string

	// argsMatch is like args, but instead of a literal string it's interpreted
	// as a regular expression. When argsMatch is set, args is ignored.
	argsMatch string
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
	if s.hostKeyChecking != "" {
		expect("-o StrictHostKeyChecking " + s.hostKeyChecking)
	}

	if s.withProxy {
		expect("-o ProxyCommand juju ssh " +
			"--model=controller " +
			"--proxy=false " +
			"--no-host-key-checks " +
			"--pty=false ubuntu@localhost -q \"nc %h %p\"")
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

	if s.argsMatch != "" {
		expect(s.argsMatch)
	} else {
		expect(regexp.QuoteMeta(s.args))
	}

	// Check the command line matches what is expected.
	pattern := "^" + strings.Join(expected, " ") + "$"
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
	hostChecker   jujussh.ReachableChecker
}

// Commands to patch
var patchedCommands = []string{"ssh", "scp"}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

{
    echo "$@"

    # If a custom known_hosts file was passed, emit the contents of
    # that too.
    while (( "$#" )); do
        if [[ $1 = UserKnownHostsFile* ]]; then
            IFS=" " read -ra parts <<< $1
            cat "${parts[1]}"
            break
        fi
        shift
    done
}| tee $0.args
`

type fakeHostChecker struct {
	acceptedAddresses set.Strings
}

var _ jujussh.ReachableChecker = (*fakeHostChecker)(nil)

func (f *fakeHostChecker) FindHost(hostPorts network.HostPorts, publicKeys []string) (network.HostPort, error) {
	// TODO(jam): The real reachable checker won't give deterministic ordering
	// for hostPorts, maybe we should do a random return value?
	for _, hostPort := range hostPorts {
		if f.acceptedAddresses.Contains(hostPort.Host()) {
			return hostPort, nil
		}
	}
	return network.SpaceHostPort{}, errors.Errorf("cannot connect to any address: %v", hostPorts)
}

func validAddresses(acceptedAddresses ...string) *fakeHostChecker {
	return &fakeHostChecker{
		acceptedAddresses: set.NewStrings(acceptedAddresses...),
	}
}

func (s *SSHCommonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	ssh.ClearClientKeys()
	s.PatchValue(&getJujuExecutable, func() (string, error) { return "juju", nil })
	s.setForceAPIv1(false)

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

func (s *SSHCommonSuite) setForceAPIv1(enabled bool) {
	if enabled {
		os.Setenv(jujuSSHClientForceAPIv1, "1")
	} else {
		os.Unsetenv(jujuSSHClientForceAPIv1)
	}
}

func (s *SSHCommonSuite) setHostChecker(hostChecker jujussh.ReachableChecker) {
	s.hostChecker = hostChecker
}

func (s *SSHCommonSuite) setupModel(c *gc.C) {
	// Add machine-0 with a mysql application and mysql/0 unit
	u := s.Factory.MakeUnit(c, nil)

	// Set both the preferred public and private addresses for machine-0, add a
	// couple of link-layer devices (loopback and ethernet) with addresses, and
	// the ssh keys.
	m := s.getMachineForUnit(c, u)
	s.setAddresses(c, m)
	s.setKeys(c, m)
	s.setLinkLayerDevicesAddresses(c, m)

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
	addrPub := network.NewScopedSpaceAddress(
		fmt.Sprintf("%s.public", m.Id()),
		network.ScopePublic,
	)
	addrPriv := network.NewScopedSpaceAddress(
		fmt.Sprintf("%s.private", m.Id()),
		network.ScopeCloudLocal,
	)
	err := m.SetProviderAddresses(addrPub, addrPriv)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHCommonSuite) setLinkLayerDevicesAddresses(c *gc.C, m *state.Machine) {
	devicesArgs := []state.LinkLayerDeviceArgs{{
		Name: "lo",
		Type: network.LoopbackDevice,
	}, {
		Name: "eth0",
		Type: network.EthernetDevice,
	}}
	err := m.SetLinkLayerDevices(devicesArgs...)
	c.Assert(err, jc.ErrorIsNil)

	addressesArgs := []state.LinkLayerDeviceAddress{{
		DeviceName:   "lo",
		CIDRAddress:  "127.0.0.1/8", // will be filtered
		ConfigMethod: network.LoopbackAddress,
	}, {
		DeviceName:   "eth0",
		CIDRAddress:  "0.1.2.3/24", // needs to be a valid CIDR
		ConfigMethod: network.StaticAddress,
	}}
	err = m.SetDevicesAddresses(addressesArgs...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHCommonSuite) setAddresses6(c *gc.C, m *state.Machine) {
	addrPub := network.NewScopedSpaceAddress("2001:db8::1", network.ScopePublic)
	addrPriv := network.NewScopedSpaceAddress("fc00:bbb::1", network.ScopeCloudLocal)
	err := m.SetProviderAddresses(addrPub, addrPriv)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHCommonSuite) setKeys(c *gc.C, m *state.Machine) {
	id := m.Id()
	keys := state.SSHHostKeys{"dsa-" + id, "rsa-" + id}
	err := s.State.SetSSHHostKeys(m.MachineTag(), keys)
	c.Assert(err, jc.ErrorIsNil)
}
