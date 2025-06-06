// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"errors"

	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/sshsession"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jujutesting "github.com/juju/juju/testing"
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

func (s *sshreqconnSuite) TestControllerSSHPort(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	s.backendMock.EXPECT().ControllerConfig().Return(controller.Config{
		"ssh-server-port": 17022,
	}, nil)

	result := f.ControllerSSHPort()
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "17022")
}

func (s *sshreqconnSuite) TestControllerPublicKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	// Generate a test private key for use in the test.
	testKey := jujutesting.SSHServerHostKey
	signer, err := ssh.ParsePrivateKey([]byte(testKey))
	c.Assert(err, gc.IsNil)

	s.backendMock.EXPECT().SSHServerHostKey().Return(testKey, nil)

	result := f.ControllerPublicKey()
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.PublicKey, gc.DeepEquals, signer.PublicKey().Marshal())
}

func (s *sshreqconnSuite) TestControllerPublicKeyStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	s.backendMock.EXPECT().SSHServerHostKey().Return("", errors.New("state error"))

	result := f.ControllerPublicKey()
	c.Assert(result.PublicKey, gc.IsNil)
	c.Assert(result.Error, gc.ErrorMatches, `state error`)
}

func (s *sshreqconnSuite) TestControllerPublicKeyParsePrivateKeyError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	// Return an invalid private key string.
	s.backendMock.EXPECT().SSHServerHostKey().Return("not-a-valid-key", nil)

	result := f.ControllerPublicKey()
	c.Assert(result.PublicKey, gc.IsNil)
	c.Assert(result.Error, gc.NotNil)
}
