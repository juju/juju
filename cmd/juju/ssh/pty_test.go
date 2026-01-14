// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package ssh

import (
	"context"
	"testing"

	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/internal/cmd"
	jujussh "github.com/juju/juju/internal/network/ssh"
	"github.com/juju/juju/rpc/params"
)

type PTYSuite struct{}

var _ = gc.Suite(&PTYSuite{})

func TestPTY(t *testing.T) {
	gc.TestingT(t)
}

type mockSSHProvider struct {
	args       []string
	ptyEnabled bool
	target     *resolvedTarget
}

// Implement sshProvider interface methods
func (m *mockSSHProvider) initRun(context.Context, ModelCommand) error { return nil }
func (m *mockSSHProvider) cleanupRun()                                 {}
func (m *mockSSHProvider) setLeaderAPI(context.Context, LeaderAPI)     {}
func (m *mockSSHProvider) setHostChecker(jujussh.ReachableChecker)     {}
func (m *mockSSHProvider) resolveTarget(context.Context, string) (*resolvedTarget, error) {
	return &resolvedTarget{host: "0"}, nil
}
func (m *mockSSHProvider) maybePopulateTargetViaField(context.Context, *resolvedTarget, func(context.Context, *client.StatusArgs) (*params.FullStatus, error)) error {
	return nil
}
func (m *mockSSHProvider) maybeResolveLeaderUnit(context.Context, string) (string, error) {
	return "", nil
}

func (m *mockSSHProvider) ssh(ctx Context, enablePty bool, target *resolvedTarget) error {
	m.ptyEnabled = enablePty
	m.target = target
	return nil
}
func (m *mockSSHProvider) copy(Context) error { return nil }

func (m *mockSSHProvider) getTarget() string       { return "" }
func (m *mockSSHProvider) setTarget(target string) {}

func (m *mockSSHProvider) getArgs() []string {
	return m.args
}
func (m *mockSSHProvider) setArgs(args []string) {
	m.args = args
}

func (m *mockSSHProvider) setRetryStrategy(retry.CallArgs)          {}
func (m *mockSSHProvider) setPublicKeyRetryStrategy(retry.CallArgs) {}

func (s *PTYSuite) TestRunPTYLogic(c *gc.C) {
	tests := []struct {
		about       string
		args        []string // args passed to the command (target is args[0])
		flags       []string // flags like --pty=true
		isTerminal  bool
		expectedPTY bool
	}{
		{
			about:       "no command, is terminal -> pty=true",
			args:        []string{"0"},
			isTerminal:  true,
			expectedPTY: true,
		},
		{
			about:       "no command, not terminal -> pty=false",
			args:        []string{"0"},
			isTerminal:  false,
			expectedPTY: false,
		},
		{
			about:       "command provided, is terminal -> pty=false",
			args:        []string{"0", "ls"},
			isTerminal:  true,
			expectedPTY: false,
		},
		{
			about:       "command provided, --pty=true -> pty=true",
			args:        []string{"0", "ls"},
			flags:       []string{"--pty=true"},
			isTerminal:  true,
			expectedPTY: true,
		},
		{
			about:       "command provided, --pty=false -> pty=false",
			args:        []string{"0", "ls"},
			flags:       []string{"--pty=false"},
			isTerminal:  true,
			expectedPTY: false,
		},
		{
			about:       "no command, --pty=true -> pty=true",
			args:        []string{"0"},
			flags:       []string{"--pty=true"},
			isTerminal:  false,
			expectedPTY: true, // forced
		},
	}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)

		mock := &mockSSHProvider{}
		sshCmd := &sshCommand{
			provider:   mock,
			isTerminal: func(interface{}) bool { return t.isTerminal },
			// sshMachine/sshContainer embedded fields are zero-valued
		}

		// 1. Simulate flag parsing
		// Since pty is autoBoolValue, default is nil.
		if len(t.flags) > 0 {
			for _, f := range t.flags {
				if f == "--pty=true" || f == "--pty" {
					b := true
					sshCmd.pty.b = &b
				} else if f == "--pty=false" {
					b := false
					sshCmd.pty.b = &b
				}
			}
		}

		// 2. Set Args
		// args[0] is target. args[1:] are command args.
		if len(t.args) > 1 {
			mock.args = t.args[1:]
		}

		// 3. Run
		ctx := &cmd.Context{Stdin: nil, Stdout: nil, Stderr: nil}
		err := sshCmd.Run(ctx)
		c.Assert(err, jc.ErrorIsNil)

		c.Assert(mock.ptyEnabled, gc.Equals, t.expectedPTY)
	}
}
