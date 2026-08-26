// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"bytes"
	"context"
	"errors"
	"io"
	"syscall"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/environs/cloudspec"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
	"github.com/juju/juju/internal/worker/sshserver/handlers/common"
)

func (s *k8sSuite) TestSessionHandler(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	var received k8sexec.ExecParams
	expectedCloudSpec := cloudspec.CloudSpec{Name: "test-cloud"}
	resolver := newSessionResolver(c, destination, expectedCloudSpec)
	handlers, err := NewHandlers(destination, resolver, loggertesting.WrapCheckLog(c), func(namespace string, actualCloudSpec cloudspec.CloudSpec) (k8sexec.Executor, error) {
		c.Check(namespace, tc.Equals, "test-namespace")
		c.Check(actualCloudSpec, tc.DeepEquals, expectedCloudSpec)
		return executorFunc(func(_ context.Context, params k8sexec.ExecParams, _ <-chan struct{}) error {
			received = params
			_, err := io.WriteString(params.Stdout, "test output\n")
			return err
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout

	err = session.Run("echo hello")

	c.Check(err, tc.ErrorIsNil)
	c.Check(received.PodName, tc.Equals, "test-pod")
	c.Check(received.ContainerName, tc.Equals, "workload")
	c.Check(received.Commands, tc.DeepEquals, []string{"echo hello"})
	c.Check(received.TTY, tc.IsFalse)
	c.Check(stdout.String(), tc.Equals, "test output\n")
}

func (s *k8sSuite) TestSessionHandlerPreservesRawCommand(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	var received k8sexec.ExecParams
	handlers, err := NewHandlers(destination, newSessionResolver(c, destination, cloudspec.CloudSpec{}), loggertesting.WrapCheckLog(c), func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
		return executorFunc(func(_ context.Context, params k8sexec.ExecParams, _ <-chan struct{}) error {
			received = params
			return nil
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})
	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()
	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	command := `printf '<%s>\n' "hello world" "" | cat`
	c.Assert(session.Run(command), tc.ErrorIsNil)
	c.Check(received.Commands, tc.DeepEquals, []string{command})
}

func (s *k8sSuite) TestSessionHandlerStartsDefaultShell(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	var received k8sexec.ExecParams
	handlers, err := NewHandlers(destination, newSessionResolver(c, destination, cloudspec.CloudSpec{}), loggertesting.WrapCheckLog(c), func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
		return executorFunc(func(_ context.Context, params k8sexec.ExecParams, _ <-chan struct{}) error {
			received = params
			return nil
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})
	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()
	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	c.Assert(session.Shell(), tc.ErrorIsNil)
	c.Assert(session.Wait(), tc.ErrorIsNil)
	c.Check(received.Commands, tc.DeepEquals, []string{"/bin/sh"})
}

func (s *k8sSuite) TestSessionHandlerPropagatesExitStatus(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := NewHandlers(destination, newSessionResolver(c, destination, cloudspec.CloudSpec{}), loggertesting.WrapCheckLog(c), func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
		return executorFunc(func(context.Context, k8sexec.ExecParams, <-chan struct{}) error {
			return testExitError{status: 3}
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})
	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()
	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	err = session.Run("exit 3")
	var exitErr *gossh.ExitError
	c.Assert(errors.As(err, &exitErr), tc.IsTrue)
	c.Check(exitErr.ExitStatus(), tc.Equals, 3)
}

func (s *k8sSuite) TestSessionHandlerForwardsSignal(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	received := make(chan syscall.Signal, 1)
	handlers, err := NewHandlers(destination, newSessionResolver(c, destination, cloudspec.CloudSpec{}), loggertesting.WrapCheckLog(c), func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
		return executorFunc(func(ctx context.Context, params k8sexec.ExecParams, _ <-chan struct{}) error {
			select {
			case signal := <-params.Signal:
				received <- signal
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})
	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()
	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	c.Assert(session.Start("sleep 30"), tc.ErrorIsNil)
	c.Assert(session.Signal(gossh.SIGTERM), tc.ErrorIsNil)
	c.Assert(session.Wait(), tc.ErrorIsNil)
	c.Check(<-received, tc.Equals, syscall.SIGTERM)
}

func (s *k8sSuite) TestSessionHandlerReportsResolverFailure(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	resolver := newMockResolver(c)
	resolver.EXPECT().ResolveK8sExecInfo(gomock.Any(), destination).Return("", "", errors.New("resolver failed"))
	handlers, err := NewHandlers(destination, resolver, loggertesting.WrapCheckLog(c), stubExecutor, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr

	err = session.Run("echo hello")

	c.Check(err, tc.ErrorMatches, "Process exited with status 1")
	c.Check(stderr.String(), tc.Equals, "resolving Kubernetes exec information: resolver failed\n")
}

func (s *k8sSuite) TestSessionHandlerWithPTY(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	executed := make(chan k8sexec.ExecParams, 1)
	handlers, err := NewHandlers(destination, newSessionResolver(c, destination, cloudspec.CloudSpec{}), loggertesting.WrapCheckLog(c), func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
		return executorFunc(func(_ context.Context, params k8sexec.ExecParams, _ <-chan struct{}) error {
			executed <- params
			_, err := io.WriteString(params.Stdout, "final output\n")
			return err
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()
	var stdout bytes.Buffer
	session.Stdout = &stdout

	c.Assert(session.RequestPty("xterm", 24, 80, nil), tc.ErrorIsNil)
	c.Assert(session.Run("echo hello"), tc.ErrorIsNil)

	params := <-executed
	c.Check(params.TTY, tc.IsTrue)
	c.Check(params.Stderr, tc.IsNil)
	c.Assert(params.TerminalSizeQueue, tc.NotNil)
	c.Check(params.TerminalSizeQueue.Next(), tc.DeepEquals, &k8sexec.TerminalSize{
		Width:  80,
		Height: 24,
	})
	c.Check(params.Env, tc.DeepEquals, []string{"TERM=xterm"})
	c.Check(stdout.String(), tc.Equals, "final output\r\n")
}

func (s *k8sSuite) TestSessionEnvironmentPreservesExplicitTerm(c *tc.C) {
	env := []string{"LANG=en_US.UTF-8", "TERM=screen"}
	c.Check(sessionEnvironment(env, "xterm", true), tc.DeepEquals, env)
}

func (s *k8sSuite) TestTerminalSizeQueue(c *tc.C) {
	windowChanges := make(chan ssh.Window, 1)
	queue := newTerminalSizeQueue(c.Context(), ssh.Window{
		Width:  80,
		Height: 24,
	}, windowChanges)

	c.Check(queue.Next(), tc.DeepEquals, &k8sexec.TerminalSize{
		Width:  80,
		Height: 24,
	})

	windowChanges <- ssh.Window{Width: 120, Height: 40}
	c.Check(queue.Next(), tc.DeepEquals, &k8sexec.TerminalSize{
		Width:  120,
		Height: 40,
	})
}

func (s *k8sSuite) TestTerminalSizeQueueStopsWithContext(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	queue := newTerminalSizeQueue(ctx, ssh.Window{}, make(chan ssh.Window))
	_ = queue.Next()
	cancel()

	c.Check(queue.Next(), tc.IsNil)
}

func (s *k8sSuite) TestSessionHandlerWithPTYDrainsOutputBeforeErrorExit(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := NewHandlers(destination, newSessionResolver(c, destination, cloudspec.CloudSpec{}), loggertesting.WrapCheckLog(c), func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
		return executorFunc(func(_ context.Context, params k8sexec.ExecParams, _ <-chan struct{}) error {
			_, err := io.WriteString(params.Stdout, "final output\n")
			c.Assert(err, tc.ErrorIsNil)
			return testExitError{status: 3}
		}), nil
	}, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)

	server := startK8sTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})
	client, err := server.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()
	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()
	var stdout bytes.Buffer
	session.Stdout = &stdout

	c.Assert(session.RequestPty("xterm", 80, 24, nil), tc.ErrorIsNil)
	err = session.Run("exit 3")
	var exitErr *gossh.ExitError
	c.Assert(errors.As(err, &exitErr), tc.IsTrue)
	c.Check(exitErr.ExitStatus(), tc.Equals, 3)
	c.Check(stdout.String(), tc.Equals, "final output\r\n")
}

type testExitError struct {
	status int
}

func (e testExitError) Error() string {
	return "command failed"
}

func (e testExitError) String() string {
	return e.Error()
}

func (e testExitError) ExitStatus() int {
	return e.status
}
