// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/rpc/params"
)

type k8sSessionSuite struct {
	resolver *MockResolver
	executor *MockExecutor
	session  *MockSession
	context  *MockContext
}

var _ = gc.Suite(&k8sSessionSuite{})

func (s *k8sSessionSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.resolver = NewMockResolver(ctrl)
	s.executor = NewMockExecutor(ctrl)
	s.session = NewMockSession(ctrl)
	s.context = NewMockContext(ctrl)
	return ctrl
}

func (s *k8sSessionSuite) TestSessionHandler(c *gc.C) {
	defer s.setupMocks(c).Finish()
	l := loggo.GetLogger("test")
	s.resolver.EXPECT().ResolveK8sExecInfo(gomock.Any()).Return(params.SSHK8sExecResult{
		PodName:   "test-pod",
		Namespace: "test-namespace",
	}, nil)
	s.executor.EXPECT().Exec(gomock.Any(), gomock.Any()).DoAndReturn(func(params k8sexec.ExecParams, opts <-chan struct{}) error {
		c.Assert(params.PodName, gc.Equals, "test-pod")
		c.Assert(params.ContainerName, gc.Equals, "test-container")
		return nil
	})

	virtualHostame, err := virtualhostname.NewInfoContainerTarget(uuid.New().String(), "test/0", "test-container")
	c.Assert(err, jc.ErrorIsNil)

	k8sHandlers, err := NewHandlers(
		virtualHostame,
		s.resolver,
		l,
		func(string) (k8sexec.Executor, error) {
			return s.executor, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.session.EXPECT().Pty()
	s.session.EXPECT().Environ()
	s.session.EXPECT().Command()
	s.context.EXPECT().Done()
	s.session.EXPECT().Context().Return(s.context)
	s.session.EXPECT().Stderr()

	// test happy path
	k8sHandlers.SessionHandler(s.session)

	// test error path
	readWriter := bytes.Buffer{}

	s.session.EXPECT().Stderr().Return(&readWriter)
	s.session.EXPECT().Exit(1)

	// test error from facade is sent to the session
	s.resolver.EXPECT().ResolveK8sExecInfo(gomock.Any()).Return(params.SSHK8sExecResult{}, errors.New("error")).Times(1)
	k8sHandlers.SessionHandler(s.session)
	c.Assert(readWriter.String(), gc.Equals, "failed to resolve k8s exec info: error\n")
}

func (s *k8sSessionSuite) TestSessionHandlerPty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	l := loggo.GetLogger("test")
	s.resolver.EXPECT().ResolveK8sExecInfo(gomock.Any()).Return(params.SSHK8sExecResult{
		PodName:   "test-pod",
		Namespace: "test-namespace",
	}, nil)

	virtualHostame, err := virtualhostname.NewInfoContainerTarget(uuid.New().String(), "test/0", "test-container")
	c.Assert(err, jc.ErrorIsNil)

	k8sHandlers, err := NewHandlers(
		virtualHostame,
		s.resolver,
		l,
		func(string) (k8sexec.Executor, error) {
			return s.executor, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.session.EXPECT().Environ()
	s.session.EXPECT().Command()
	s.context.EXPECT().Done()
	s.session.EXPECT().Context().Return(s.context)
	s.session.EXPECT().Stderr()
	s.session.EXPECT().Close()

	mockSession := userSession{
		Session: s.session,
		isPty:   true,
		closed:  make(chan struct{}),
	}
	s.executor.EXPECT().Exec(gomock.Any(), gomock.Any()).DoAndReturn(func(params k8sexec.ExecParams, opts <-chan struct{}) error {
		c.Check(params.PodName, gc.Equals, "test-pod")
		c.Check(params.ContainerName, gc.Equals, "test-container")

		_, err = params.Stdout.Write([]byte("test output"))
		c.Check(err, jc.ErrorIsNil)

		return nil
	})
	k8sHandlers.SessionHandler(&mockSession)
	c.Check(mockSession.stdout.String(), gc.Equals, "test output\r\n")
}

func (s *k8sSessionSuite) TestSessionEndsIfClientCloses(c *gc.C) {
	defer s.setupMocks(c).Finish()
	l := loggo.GetLogger("test")
	s.resolver.EXPECT().ResolveK8sExecInfo(gomock.Any()).Return(params.SSHK8sExecResult{
		PodName:   "test-pod",
		Namespace: "test-namespace",
	}, nil)

	virtualHostame, err := virtualhostname.NewInfoContainerTarget(uuid.New().String(), "test/0", "test-container")
	c.Assert(err, jc.ErrorIsNil)

	k8sHandlers, err := NewHandlers(
		virtualHostame,
		s.resolver,
		l,
		func(string) (k8sexec.Executor, error) {
			return s.executor, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.session.EXPECT().Environ()
	s.session.EXPECT().Command()
	s.context.EXPECT().Done()
	s.session.EXPECT().Context().Return(s.context)
	s.session.EXPECT().Stderr()
	s.session.EXPECT().Close().Times(2)

	mockSession := userSession{
		Session: s.session,
		isPty:   true,
		closed:  make(chan struct{}),
	}
	k8sExecStarted := make(chan struct{})
	s.executor.EXPECT().Exec(gomock.Any(), gomock.Any()).DoAndReturn(func(params k8sexec.ExecParams, opts <-chan struct{}) error {
		c.Check(params.PodName, gc.Equals, "test-pod")
		c.Check(params.ContainerName, gc.Equals, "test-container")

		<-k8sExecStarted
		// The reason we don't check the error here is that we may
		// or may not receive an error depending on whether the
		// routine that reads from the session has returned.
		_, _ = params.Stdout.Write([]byte("test output"))
		return nil
	})

	sessionHandlingDone := make(chan struct{})
	go func() {
		k8sHandlers.SessionHandler(&mockSession)
		close(sessionHandlingDone)
	}()

	// We will simulate closing the user's session before the k8s exec
	// has written anything. We expect to see the client has received
	// no data and that the session handler still completes.
	mockSession.Close()
	// Unblock the k8s exec so it can finish.
	close(k8sExecStarted)
	// Wait for the session handler to finish.
	<-sessionHandlingDone
	c.Check(mockSession.stdout.String(), gc.Equals, "")
}

type userSession struct {
	ssh.Session
	stdout    bytes.Buffer
	isPty     bool
	closed    chan struct{}
	closeOnce sync.Once
}

func (u *userSession) Write(p []byte) (n int, err error) {
	select {
	case <-u.closed:
		// if the session is "closed", we won't store writes
		// to simulate a client that has closed their connection.
		return len(p), nil
	default:
	}
	return u.stdout.Write(p)
}

// Read blocks without return anything until the session is closed.
func (u *userSession) Read(p []byte) (n int, err error) {
	<-u.closed
	return 0, io.EOF
}

func (u *userSession) Close() error {
	u.closeOnce.Do(func() {
		close(u.closed)
	})
	return u.Session.Close()
}

func (u *userSession) Pty() (ssh.Pty, <-chan ssh.Window, bool) {
	windowChan := make(chan ssh.Window)
	close(windowChan)
	return ssh.Pty{Window: ssh.Window{Width: 10, Height: 10}}, windowChan, u.isPty
}
