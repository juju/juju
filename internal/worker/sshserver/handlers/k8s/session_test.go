// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"bytes"
	"errors"

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
}

var _ = gc.Suite(&k8sSessionSuite{})

func (s *k8sSessionSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.resolver = NewMockResolver(ctrl)
	s.executor = NewMockExecutor(ctrl)
	s.session = NewMockSession(ctrl)
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
