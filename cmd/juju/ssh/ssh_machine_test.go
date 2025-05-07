// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/ssh"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/core/network"
	jujussh "github.com/juju/juju/internal/network/ssh"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
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

func (s *argsSpec) check(c *tc.C, output string) {
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
		c.Check(actualKnownHosts, tc.Matches, s.expectedKnownHosts())
	}

	if s.argsMatch != "" {
		expect(s.argsMatch)
	} else {
		expect(regexp.QuoteMeta(s.args))
	}

	// Check the command line matches what is expected.
	pattern := "^" + strings.Join(expected, " ") + "$"
	c.Check(actualCommandLine, tc.Matches, pattern)
}

func (s *argsSpec) expectedKnownHosts() string {
	out := ""
	for _, id := range strings.Split(s.knownHosts, ",") {
		out += fmt.Sprintf(".+ dsa-%s\n.+ rsa-%s\n", id, id)
	}
	return out
}

type SSHMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	binDir      string
	hostChecker jujussh.ReachableChecker
}

var _ = tc.Suite(&SSHMachineSuite{})

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
	acceptedPort      int
}

var _ jujussh.ReachableChecker = (*fakeHostChecker)(nil)

func (f *fakeHostChecker) FindHost(hostPorts network.HostPorts, publicKeys []string) (network.HostPort, error) {
	// TODO(jam): The real reachable checker won't give deterministic ordering
	// for hostPorts, maybe we should do a random return value?
	for _, hostPort := range hostPorts {
		if f.acceptedAddresses.Contains(hostPort.Host()) && f.acceptedPort == hostPort.Port() {
			return hostPort, nil
		}
	}
	return network.SpaceHostPort{}, errors.Errorf("cannot connect to any address: %v", hostPorts)
}

func validAddresses(acceptedAddresses ...string) *fakeHostChecker {
	return &fakeHostChecker{
		acceptedPort:      22,
		acceptedAddresses: set.NewStrings(acceptedAddresses...),
	}
}

func validAddressesWithPort(port int, acceptedAddresses ...string) *fakeHostChecker {
	return &fakeHostChecker{
		acceptedPort:      port,
		acceptedAddresses: set.NewStrings(acceptedAddresses...),
	}
}

func (s *SSHMachineSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	ssh.ClearClientKeys()
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

func (s *SSHMachineSuite) TestMaybePopulateTargetViaFieldForHostMachineTarget(c *tc.C) {
	target := &resolvedTarget{
		host: "10.0.0.1",
	}

	statusGetter := func(ctx context.Context, _ *client.StatusArgs) (*params.FullStatus, error) {
		return &params.FullStatus{
			Machines: map[string]params.MachineStatus{
				"0": {
					IPAddresses: []string{
						"10.0.0.1",
					},
				},
			},
		}, nil
	}

	err := new(sshMachine).maybePopulateTargetViaField(context.Background(), target, statusGetter)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(target.via, tc.IsNil, tc.Commentf("expected target.via not to be populated for a non-container target"))
}

func (s *SSHMachineSuite) TestMaybePopulateTargetViaFieldForContainerMachineTarget(c *tc.C) {
	target := &resolvedTarget{
		host: "252.66.6.42",
	}

	statusGetter := func(ctx context.Context, _ *client.StatusArgs) (*params.FullStatus, error) {
		return &params.FullStatus{
			Machines: map[string]params.MachineStatus{
				"0": {
					IPAddresses: []string{
						"10.0.0.1",
						"252.66.6.1",
					},
					Containers: map[string]params.MachineStatus{
						"0/lxd/0": {
							IPAddresses: []string{
								"252.66.6.42",
							},
						},
					},
				},
			},
		}, nil
	}

	err := new(sshMachine).maybePopulateTargetViaField(context.Background(), target, statusGetter)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(target.via, tc.Not(tc.IsNil), tc.Commentf("expected target.via to be populated for container target"))
	c.Assert(target.via.user, tc.Equals, "ubuntu")
	c.Assert(target.via.host, tc.Equals, "10.0.0.1", tc.Commentf("expected target.via.host to be set to the container's host machine address"))
}

func (s *SSHMachineSuite) setHostChecker(hostChecker jujussh.ReachableChecker) {
	s.hostChecker = hostChecker
}

func (s *SSHMachineSuite) setupModel(
	ctrl *gomock.Controller, withProxy bool,
	machineAddresses func() []string,
	keysForTarget func(ctx context.Context, target string) ([]string, error),
	targets ...string,
) (SSHClientAPI, *mocks.MockApplicationAPI, StatusClientAPI) {
	applicationClient := mocks.NewMockApplicationAPI(ctrl)
	sshClient := mocks.NewMockSSHClientAPI(ctrl)
	statusClient := mocks.NewMockStatusClientAPI(ctrl)

	if len(targets) == 0 {
		targets = []string{"0"}
	}
	p := strings.Split(targets[0], "@")
	if len(p) > 1 {
		targets[0] = p[1]
	}

	machineTarget := func(t string) string {
		machine := t
		if names.IsValidUnit(machine) {
			machine = "0"
		}
		return machine
	}

	getAddresses := func(target string) ([]string, error) {
		if machineAddresses == nil {
			machine := machineTarget(target)
			addr := []string{
				fmt.Sprintf("%s.public", machine),
				fmt.Sprintf("%s.private", machine),
			}
			if machine != "0" {
				addr = append(addr, "2001:db8::1")
			}
			return addr, nil
		}
		addr := machineAddresses()
		if len(addr) == 0 {
			return nil, network.NoAddressError("machine")
		}
		return addr, nil
	}
	for _, t := range targets {
		sshClient.EXPECT().AllAddresses(gomock.Any(), t).DoAndReturn(func(ctx context.Context, target string) ([]string, error) {
			if target == "5" {
				return nil, errors.NotFoundf("machine 5")
			}
			if target == "nonexistent/123" {
				return nil, errors.NotFoundf(`unit "nonexistent/123"`)
			}
			return getAddresses(target)
		}).MaxTimes(5)
		sshClient.EXPECT().PrivateAddress(gomock.Any(), t).DoAndReturn(func(ctx context.Context, target string) (string, error) {
			addr, err := getAddresses(target)
			if err != nil || len(addr) == 0 {
				return "", err
			}
			for _, a := range addr {
				if strings.HasSuffix(a, ".private") {
					return a, nil
				}
			}
			return addr[0], nil
		}).MaxTimes(5)
	}
	for _, t := range targets {
		f := func(ctx context.Context, target string) ([]string, error) {
			machine := machineTarget(target)
			if machine != "1" {
				return []string{
					fmt.Sprintf("dsa-%s", machine),
					fmt.Sprintf("rsa-%s", machine),
				}, nil
			}
			return nil, errors.NotFoundf("keys")
		}
		if keysForTarget != nil {
			f = keysForTarget
		}
		sshClient.EXPECT().PublicKeys(gomock.Any(), t).DoAndReturn(f).AnyTimes()
	}

	statusClient.EXPECT().Status(gomock.Any(), nil).DoAndReturn(func(ctx context.Context, _ *client.StatusArgs) (*params.FullStatus, error) {
		machine := machineTarget(targets[0])
		addr, err := getAddresses(machine)
		if err != nil {
			return nil, err
		}
		return &params.FullStatus{
			Machines: map[string]params.MachineStatus{
				machine: {
					IPAddresses: addr,
				},
			},
		}, nil
	}).MaxTimes(2)
	sshClient.EXPECT().Proxy(gomock.Any()).Return(withProxy, nil).MaxTimes(1)
	sshClient.EXPECT().Close().Return(nil)
	statusClient.EXPECT().Close().Return(nil)
	// leader api attribute is assigned the application api and both may be closed.
	applicationClient.EXPECT().Close().Return(nil).MinTimes(1)
	return sshClient, applicationClient, statusClient
}
