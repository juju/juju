// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"text/template"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type sshJumpSuite struct {
	SSHMachineSuite

	sshAPIJump *mocks.MockSSHAPIJump
	// hostKey is a valid wire-format public host key used for both the jump
	// server and the target in tests.
	hostKey []byte
}

func TestSSHJumpSuite(t *stdtesting.T) {
	tc.Run(t, &sshJumpSuite{})
}

func (s *sshJumpSuite) TestCheckSSHJumpFacadeVersion(c *tc.C) {
	c.Check(checkSSHJumpFacadeVersion(minSSHJumpFacadeVersion), tc.ErrorIsNil)
	c.Check(checkSSHJumpFacadeVersion(minSSHJumpFacadeVersion+1), tc.ErrorIsNil)
	c.Check(
		checkSSHJumpFacadeVersion(minSSHJumpFacadeVersion-1),
		tc.ErrorMatches,
		`controller does not support SSH proxying; use the --direct flag to connect directly`,
	)
}

func (s *sshJumpSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.sshAPIJump = mocks.NewMockSSHAPIJump(ctrl)
	key, err := pkissh.MarshalPublicKey([]byte(coretesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	s.hostKey = key
	return ctrl
}

// TestResolveTarget exercises the jump target selection flow: the virtual
// hostname is resolved via the API, the target/jump host keys are fetched, and
// a reachable controller jump host is selected. The final destination user is
// fixed to ubuntu.
func (s *sshJumpSuite) TestResolveTarget(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), gomock.Any(), gomock.Any()).Return("resolved-target", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)

	controllerAddress := "1.0.0.1"
	jumpServerHostKey, err := gossh.ParsePublicKey(s.hostKey)
	c.Assert(err, tc.ErrorIsNil)
	hostChecker := mocks.NewMockReachableChecker(ctrl)
	hostChecker.EXPECT().FindHost(
		network.NewMachineHostPorts(17022, "1.0.0.1", "1.0.0.2").HostPorts(),
		[]string{string(gossh.MarshalAuthorizedKey(jumpServerHostKey))},
	).Return(network.NewMachineHostPorts(17022, controllerAddress).HostPorts()[0], nil)
	jump := sshJump{
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1", "1.0.0.2"},
		hostChecker:          hostChecker,
		jumpHostPort:         17022,
	}

	resolved, err := jump.resolveTarget(c.Context(), "test-target")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resolved, tc.DeepEquals, &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			user: "fred",
			host: controllerAddress,
		},
	})
	// A known_hosts file pinning both the jump server and target keys is
	// written.
	c.Assert(jump.knownHostsPath, tc.Not(tc.Equals), "")
	defer func() { _ = os.Remove(jump.knownHostsPath) }()
	contents, err := os.ReadFile(jump.knownHostsPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Contains, "[1.0.0.1]:17022 ")
	c.Check(string(contents), tc.Contains, "resolved-target ")
}

// TestResolveTargetPassesContainerForCAAS ensures that for CAAS models the
// container name is passed through when resolving the virtual hostname.
func (s *sshJumpSuite) TestResolveTargetPassesContainerForCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	container := "redis"
	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "test-target", &container).Return("resolved-target", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)

	jump := sshJump{
		modelType:            model.CAAS,
		container:            container,
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker: &fakeHostChecker{
			acceptedAddresses: set.NewStrings("1.0.0.1"),
			acceptedPort:      17022,
		},
		jumpHostPort: 17022,
	}

	_, err := jump.resolveTarget(c.Context(), "test-target")
	c.Assert(err, tc.ErrorIsNil)
	if jump.knownHostsPath != "" {
		defer func() { _ = os.Remove(jump.knownHostsPath) }()
	}
}

// TestSSHUsesJumpProxyCommand verifies that the SSH invocation proxies through
// the controller jump server using ssh -W on the configured port as the current
// Juju user, and connects to the final destination as the fixed ubuntu user.
func (s *sshJumpSuite) TestSSHUsesJumpProxyCommand(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			// The jump connection authenticates as the Juju user.
			user: "fred",
			host: "1.0.0.1",
		},
	}
	jump := sshJump{jumpHostPort: 17022, knownHostsPath: "/tmp/known_hosts"}

	buffer := bytes.NewBuffer(nil)
	sshCtx := mocks.NewMockContext(ctrl)
	sshCtx.EXPECT().GetStdin().Return(bytes.NewBuffer(nil)).AnyTimes()
	sshCtx.EXPECT().GetStdout().Return(buffer).AnyTimes()
	sshCtx.EXPECT().GetStderr().Return(buffer).AnyTimes()

	err := jump.ssh(sshCtx, false, target)
	c.Assert(err, tc.ErrorIsNil)

	out := buffer.String()
	// The jump box is reached as the current Juju user (fred) with strict host
	// key checking against the pinned known_hosts file.
	c.Check(strings.Contains(out, "-o ProxyCommand ssh -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/tmp/known_hosts -W %h:%p -p 17022 fred@1.0.0.1"), tc.IsTrue)
	// The final destination is reached as the fixed ubuntu user.
	c.Check(strings.Contains(out, "ubuntu@resolved-target"), tc.IsTrue)
}

// TestSSHEnablesPTY ensures that --pty is honoured when the connection is
// proxied through the controller jump server.
func (s *sshJumpSuite) TestSSHEnablesPTY(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			user: "fred",
			host: "1.0.0.1",
		},
	}
	jump := sshJump{jumpHostPort: 17022, knownHostsPath: "/tmp/known_hosts"}

	buffer := bytes.NewBuffer(nil)
	sshCtx := mocks.NewMockContext(ctrl)
	sshCtx.EXPECT().GetStdin().Return(bytes.NewBuffer(nil)).AnyTimes()
	sshCtx.EXPECT().GetStdout().Return(buffer).AnyTimes()
	sshCtx.EXPECT().GetStderr().Return(buffer).AnyTimes()

	err := jump.ssh(sshCtx, true, target)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(strings.Contains(buffer.String(), "-t -t"), tc.IsTrue)
}

func (s *sshJumpSuite) TestSSHSkipsHostKeyChecking(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			user: "fred",
			host: "1.0.0.1",
		},
	}
	jump := sshJump{jumpHostPort: 17022, noHostKeyChecks: true}

	buffer := bytes.NewBuffer(nil)
	sshCtx := mocks.NewMockContext(ctrl)
	sshCtx.EXPECT().GetStdin().Return(bytes.NewBuffer(nil)).AnyTimes()
	sshCtx.EXPECT().GetStdout().Return(buffer).AnyTimes()
	sshCtx.EXPECT().GetStderr().Return(buffer).AnyTimes()

	c.Assert(jump.ssh(sshCtx, false, target), tc.ErrorIsNil)
	out := buffer.String()
	c.Check(strings.Contains(out, "-o ProxyCommand ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -W %h:%p -p 17022 fred@1.0.0.1"), tc.IsTrue)
	c.Check(strings.Contains(out, "-o StrictHostKeyChecking no"), tc.IsTrue)
	c.Check(strings.Contains(out, "-o UserKnownHostsFile /dev/null"), tc.IsTrue)
}

func (s *sshJumpSuite) TestSSHShowsJumpCommand(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via:  &resolvedTarget{user: "fred", host: "1.0.0.1"},
	}
	outputTemplate, err := template.New("output").Parse(openSSHTemplate)
	c.Assert(err, tc.ErrorIsNil)
	jump := sshJump{
		args:              []string{"echo", "hello world"},
		jumpHostPort:      17022,
		showCommand:       true,
		sshOutputTemplate: outputTemplate,
	}

	buffer := bytes.NewBuffer(nil)
	sshCtx := mocks.NewMockContext(ctrl)
	sshCtx.EXPECT().GetStdout().Return(buffer)

	c.Assert(jump.ssh(sshCtx, false, target), tc.ErrorIsNil)
	c.Check(buffer.String(), tc.Equals, "ssh -o \"ProxyCommand=ssh -W %h:%p -p 17022 fred@1.0.0.1\" ubuntu@resolved-target echo \"hello world\"\n")
}

func (s *sshJumpSuite) TestCopyShowsJumpCommand(c *tc.C) {
	outputTemplate, err := template.New("output").Parse(openSCPTemplate)
	c.Assert(err, tc.ErrorIsNil)
	jump := sshJump{jumpHostPort: 17022, scpOutputTemplate: outputTemplate}
	target := &resolvedTarget{via: &resolvedTarget{user: "fred", host: "1.0.0.1"}}

	buffer := bytes.NewBuffer(nil)
	c.Assert(jump.showSCPCommand(buffer, target, []string{"local file", "ubuntu@machine-0:/remote path"}), tc.ErrorIsNil)
	c.Check(buffer.String(), tc.Equals, "scp -o \"ProxyCommand=ssh -W %h:%p -p 17022 fred@1.0.0.1\" \"local file\" \"ubuntu@machine-0:/remote path\"\n")
}

func (s *sshJumpSuite) TestCopyShowsJumpCommandThroughCopy(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "0", nil).Return("machine-0", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), "machine-0").Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)
	s.sshAPIJump.EXPECT().Close().Return(nil)
	outputTemplate, err := template.New("output").Parse(openSCPTemplate)
	c.Assert(err, tc.ErrorIsNil)

	jump := sshJump{
		args:                 []string{"foo.txt", "0:/tmp/foo.txt"},
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		showCommand:          true,
		scpOutputTemplate:    outputTemplate,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker:          validAddressesWithPort(17022, "1.0.0.1"),
		jumpHostPort:         17022,
	}
	defer jump.cleanupRun()

	ctx := cmdtesting.Context(c)
	c.Assert(jump.copy(ctx), tc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "scp -o \"ProxyCommand=ssh -W %h:%p -p 17022 fred@1.0.0.1\" foo.txt ubuntu@machine-0:/tmp/foo.txt\n")
}

func (*sshJumpSuite) TestCopyShowCommandRequiresRemoteTarget(c *tc.C) {
	jump := sshJump{
		args:        []string{"foo.txt", "bar.txt"},
		showCommand: true,
	}

	c.Assert(jump.copy(nil), tc.ErrorMatches, "at least one remote SCP target is required")
}

func (s *sshJumpSuite) TestCopyUsesJumpProxyCommand(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "0", nil).Return("machine-0", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), "machine-0").Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)
	s.sshAPIJump.EXPECT().Close().Return(nil)

	jump := sshJump{
		args:                 []string{"foo.txt", "0:/tmp/foo.txt"},
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker:          validAddressesWithPort(17022, "1.0.0.1"),
		jumpHostPort:         17022,
	}
	defer jump.cleanupRun()

	c.Assert(jump.copy(nil), tc.ErrorIsNil)
	output, err := os.ReadFile(filepath.Join(s.binDir, "scp.args"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(output), tc.Contains, "-o ProxyCommand ssh -o StrictHostKeyChecking=yes")
	c.Check(string(output), tc.Contains, "-W %h:%p -p 17022 fred@1.0.0.1")
	c.Check(string(output), tc.Contains, "foo.txt ubuntu@machine-0:/tmp/foo.txt")
	c.Check(string(output), tc.Contains, "[1.0.0.1]:17022 ")
	c.Check(string(output), tc.Contains, "machine-0 ")
}

func (s *sshJumpSuite) TestCopyPinsAllTargetHostKeys(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "0", nil).Return("machine-0", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), "machine-0").Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)
	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "1", nil).Return("machine-1", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), "machine-1").Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)
	s.sshAPIJump.EXPECT().Close().Return(nil)

	jump := sshJump{
		args:                 []string{"-3", "0:/tmp/source", "bob@1:/tmp/destination"},
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker:          validAddressesWithPort(17022, "1.0.0.1"),
		jumpHostPort:         17022,
	}
	defer jump.cleanupRun()

	c.Assert(jump.copy(nil), tc.ErrorIsNil)
	output, err := os.ReadFile(filepath.Join(s.binDir, "scp.args"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(output), tc.Contains, "-3 ubuntu@machine-0:/tmp/source bob@machine-1:/tmp/destination")
	c.Check(string(output), tc.Contains, "machine-0 ")
	c.Check(string(output), tc.Contains, "machine-1 ")
}

func (s *sshJumpSuite) TestCopyCAASArgValidation(c *tc.C) {
	jump := sshJump{modelType: model.CAAS, args: []string{"source", ":target"}}
	c.Check(jump.copy(nil), tc.ErrorMatches, "target must match format: \\[pod\\[/container\\]:\\]path")
}

func (s *sshJumpSuite) TestCopyCAASShowCommandNotSupported(c *tc.C) {
	jump := sshJump{
		modelType:   model.CAAS,
		showCommand: true,
	}

	c.Check(jump.copy(nil), tc.ErrorMatches, "--show-command is not supported for Kubernetes pod file transfers; use juju ssh with tar instead")
}

func (s *sshJumpSuite) TestCopyToCAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	sshScript := `#!/bin/bash
{
    echo "$@"
    cat >/dev/null
} >> "$0.args"
`
	err := os.WriteFile(filepath.Join(s.binDir, "ssh"), []byte(sshScript), 0777)
	c.Assert(err, tc.ErrorIsNil)

	srcPath := filepath.Join(c.MkDir(), "source")
	err = os.WriteFile(srcPath, []byte("test data"), 0600)
	c.Assert(err, tc.ErrorIsNil)

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "0", gomock.Any()).Return("pod-0", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), "pod-0").Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)

	jump := sshJump{
		modelType:            model.CAAS,
		container:            "charm",
		args:                 []string{srcPath, "0:/tmp/destination"},
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker:          validAddressesWithPort(17022, "1.0.0.1"),
		jumpHostPort:         17022,
	}
	defer jump.cleanupRun()
	s.sshAPIJump.EXPECT().Close().Return(nil)

	c.Assert(jump.copy(nil), tc.ErrorIsNil)
	output, err := os.ReadFile(filepath.Join(s.binDir, "ssh.args"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(output), tc.Contains, "ubuntu@pod-0 test -d /tmp/destination")
	c.Check(string(output), tc.Contains, "ubuntu@pod-0 tar -xmf - -C /tmp")
}

func (s *sshJumpSuite) TestCopyToCAASReportsRemoteStderr(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	sshScript := `#!/bin/bash
if [[ "$*" == *"test -d"* ]]; then
    exit 1
fi
cat >/dev/null
echo "permission denied" >&2
exit 1
`
	err := os.WriteFile(filepath.Join(s.binDir, "ssh"), []byte(sshScript), 0777)
	c.Assert(err, tc.ErrorIsNil)

	srcPath := filepath.Join(c.MkDir(), "source")
	err = os.WriteFile(srcPath, []byte("test data"), 0600)
	c.Assert(err, tc.ErrorIsNil)

	jump := sshJump{
		jumpHostPort:    17022,
		noHostKeyChecks: true,
	}
	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "pod-0",
		via: &resolvedTarget{
			user: "fred",
			host: "1.0.0.1",
		},
	}

	err = jump.copyToCAAS(nil, srcPath, target, "/tmp/destination")
	c.Check(err, tc.ErrorMatches, ".*permission denied.*")
}

func (s *sshJumpSuite) TestCopyFromCAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	sshScript := `#!/bin/bash
echo "$@" >> "$0.args"
cat "$0.archive"
`
	err := os.WriteFile(filepath.Join(s.binDir, "ssh"), []byte(sshScript), 0777)
	c.Assert(err, tc.ErrorIsNil)

	const content = "test data"
	var archive bytes.Buffer
	tarWriter := tar.NewWriter(&archive)
	err = tarWriter.WriteHeader(&tar.Header{
		Name: "remote/source",
		Mode: 0600,
		Size: int64(len(content)),
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = tarWriter.Write([]byte(content))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tarWriter.Close(), tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(s.binDir, "ssh.archive"), archive.Bytes(), 0600)
	c.Assert(err, tc.ErrorIsNil)

	destPath := filepath.Join(c.MkDir(), "destination")
	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "0", gomock.Any()).Return("pod-0", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), "pod-0").Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)

	jump := sshJump{
		modelType:            model.CAAS,
		container:            "charm",
		args:                 []string{"0:/remote/source", destPath},
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker:          validAddressesWithPort(17022, "1.0.0.1"),
		jumpHostPort:         17022,
	}
	defer jump.cleanupRun()
	s.sshAPIJump.EXPECT().Close().Return(nil)

	c.Assert(jump.copy(nil), tc.ErrorIsNil)
	result, err := os.ReadFile(destPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(result), tc.Equals, content)
	output, err := os.ReadFile(filepath.Join(s.binDir, "ssh.args"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(output), tc.Contains, "ubuntu@pod-0 tar cf - /remote/source")
}

func (s *sshJumpSuite) TestGenerateKnownHostsSupportsIPv6(c *tc.C) {
	hostKey, err := pkissh.MarshalPublicKey([]byte(coretesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	jump := sshJump{jumpHostPort: 17022, jumpServerHostKey: hostKey}
	c.Assert(jump.generateKnownHosts("2001:db8::1", "resolved-target", hostKey), tc.ErrorIsNil)
	defer func() { _ = os.Remove(jump.knownHostsPath) }()

	contents, err := os.ReadFile(jump.knownHostsPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Contains, "[2001:db8::1]:17022 ")
}

func (s *sshJumpSuite) TestGetSSHOptionsRequiresTarget(c *tc.C) {
	jump := sshJump{jumpHostPort: 17022, knownHostsPath: "/tmp/known_hosts"}

	_, err := jump.getSSHOptions(false)
	c.Check(err, tc.ErrorMatches, "at least one SSH target is required")
}
