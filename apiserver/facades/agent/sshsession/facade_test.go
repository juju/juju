// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/sshsession"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&sshreqconnSuite{})

type sshreqconnSuite struct {
	ctxMock        *MockContext
	backendMock    *MockBackend
	resourceMock   *MockResources
	authorizerMock *MockAuthorizer
}

func (s *sshreqconnSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctxMock = NewMockContext(ctrl)
	s.backendMock = NewMockBackend(ctrl)
	s.resourceMock = NewMockResources(ctrl)
	s.authorizerMock = NewMockAuthorizer(ctrl)
	return ctrl
}

func (s *sshreqconnSuite) TestAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Auth().Return(s.authorizerMock)
	s.authorizerMock.EXPECT().AuthMachineAgent().Return(false)

	_, err := sshsession.NewExternalFacade(s.ctxMock)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *sshreqconnSuite) TestGetSSHConnRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	s.backendMock.EXPECT().GetSSHConnRequest("doc-id").Return(state.SSHConnRequest{
		Username: "username",
		Password: "password",
	}, nil)

	result, err := f.GetSSHConnRequest(params.SSHConnRequestGetArg{RequestId: "doc-id"})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.SSHConnRequest.Username, gc.Equals, "username")
	c.Assert(result.SSHConnRequest.Password, gc.Equals, "password")
}

func (s *sshreqconnSuite) TestWatchSSHConnReq(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sshConnChanges := make(chan []string, 1)
	watcher := statetesting.NewMockStringsWatcher(sshConnChanges)
	defer workertest.DirtyKill(c, watcher)

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)
	s.backendMock.EXPECT().WatchSSHConnRequest("").Return(watcher).AnyTimes()
	s.resourceMock.EXPECT().Register(watcher).Return("id").AnyTimes()

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	sshConnChanges <- []string{"doc-id"}
	result, err := f.WatchSSHConnRequest(params.SSHConnRequestWatchArg{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "id")
	c.Assert(result.Changes, gc.DeepEquals, []string{"doc-id"})

	sshConnChanges <- []string{"doc-id2"}
	result, err = f.WatchSSHConnRequest(params.SSHConnRequestWatchArg{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "id")
	c.Assert(result.Changes, gc.DeepEquals, []string{"doc-id2"})
}
