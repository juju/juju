// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package ssh

import (
	"bytes"

	"github.com/juju/cmd/v3"
	"github.com/juju/retry"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/client"
	jujussh "github.com/juju/juju/network/ssh"
	"github.com/juju/juju/rpc/params"
)

type PTYSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PTYSuite{})

type mockSSHProvider struct {
	args       []string
	ptyEnabled bool
	target     *resolvedTarget
}

// Implement sshProvider interface methods
func (m *mockSSHProvider) initRun(ModelCommand) error { return nil }
func (m *mockSSHProvider) cleanupRun()                {}
func (m *mockSSHProvider) setLeaderAPI(LeaderAPI)     {}
func (m *mockSSHProvider) setHostChecker(jujussh.ReachableChecker) {
}
func (m *mockSSHProvider) resolveTarget(string) (*resolvedTarget, error) {
	return &resolvedTarget{host: "0"}, nil
}
func (m *mockSSHProvider) maybePopulateTargetViaField(*resolvedTarget, func(*client.StatusArgs) (*params.FullStatus, error)) error {
	return nil
}
func (m *mockSSHProvider) maybeResolveLeaderUnit(string) (string, error) {
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
	// We use distinct buffer pointers for stdin and stdout so that
	// the mock isTerminal function can tell which one is being queried.
	stdinBuf := &bytes.Buffer{}
	stdoutBuf := &bytes.Buffer{}

	tests := []struct {
		about            string
		args             []string // args passed to the command (target is args[0])
		flags            []string // flags like --pty=true
		stdinIsTerminal  bool
		stdoutIsTerminal bool
		expectedPTY      bool
	}{
		// ── No command (interactive session) ──────────────────────
		{
			about:            "no command, stdin=tty -> pty=true",
			args:             []string{"0"},
			stdinIsTerminal:  true,
			stdoutIsTerminal: true,
			expectedPTY:      true,
		},
		{
			about:            "no command, stdin=not tty -> pty=false",
			args:             []string{"0"},
			stdinIsTerminal:  false,
			stdoutIsTerminal: true,
			expectedPTY:      false,
		},
		// ── Command provided, both stdin+stdout are terminals ─────
		// This is the #22070 fix: interactive 'sudo -i' at a terminal.
		{
			about:            "command, stdin=tty, stdout=tty -> pty=true (#22070 fix)",
			args:             []string{"0", "sudo", "-i"},
			stdinIsTerminal:  true,
			stdoutIsTerminal: true,
			expectedPTY:      true,
		},
		// ── Command provided, stdout piped/captured ───────────────
		// This is the #19576 fix: output captured by script.
		{
			about:            "command, stdin=tty, stdout=pipe -> pty=false (#19576 fix)",
			args:             []string{"0", "echo", "hello"},
			stdinIsTerminal:  true,
			stdoutIsTerminal: false,
			expectedPTY:      false,
		},
		// ── Command provided, stdin piped ─────────────────────────
		{
			about:            "command, stdin=pipe, stdout=tty -> pty=false",
			args:             []string{"0", "cat"},
			stdinIsTerminal:  false,
			stdoutIsTerminal: true,
			expectedPTY:      false,
		},
		{
			about:            "command, stdin=pipe, stdout=pipe -> pty=false",
			args:             []string{"0", "ls"},
			stdinIsTerminal:  false,
			stdoutIsTerminal: false,
			expectedPTY:      false,
		},
		// ── Explicit --pty flag overrides ─────────────────────────
		{
			about:            "command, --pty=true overrides -> pty=true",
			args:             []string{"0", "ls"},
			flags:            []string{"--pty=true"},
			stdinIsTerminal:  false,
			stdoutIsTerminal: false,
			expectedPTY:      true,
		},
		{
			about:            "command, --pty=false overrides -> pty=false",
			args:             []string{"0", "sudo", "-i"},
			flags:            []string{"--pty=false"},
			stdinIsTerminal:  true,
			stdoutIsTerminal: true,
			expectedPTY:      false,
		},
		{
			about:            "no command, --pty=true, not terminal -> pty=true (forced)",
			args:             []string{"0"},
			flags:            []string{"--pty=true"},
			stdinIsTerminal:  false,
			stdoutIsTerminal: false,
			expectedPTY:      true,
		},
		{
			about:            "no command, --pty=false, is terminal -> pty=false (forced)",
			args:             []string{"0"},
			flags:            []string{"--pty=false"},
			stdinIsTerminal:  true,
			stdoutIsTerminal: true,
			expectedPTY:      false,
		},
	}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)

		mock := &mockSSHProvider{}
		sshCmd := &sshCommand{
			provider: mock,
			isTerminal: func(f interface{}) bool {
				if f == stdinBuf {
					return t.stdinIsTerminal
				}
				if f == stdoutBuf {
					return t.stdoutIsTerminal
				}
				return false
			},
		}

		// Simulate flag parsing.
		sshCmd.pty.b = nil
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

		// Set command args (args[0] is target, args[1:] are command args).
		if len(t.args) > 1 {
			mock.args = t.args[1:]
		} else {
			mock.args = nil
		}

		// Use our identifiable buffers as stdin/stdout so the mock
		// isTerminal function can distinguish between them.
		ctx := &cmd.Context{
			Stdin:  stdinBuf,
			Stdout: stdoutBuf,
			Stderr: &bytes.Buffer{},
		}

		err := sshCmd.Run(ctx)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mock.ptyEnabled, gc.Equals, t.expectedPTY)
	}
}
