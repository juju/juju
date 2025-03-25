// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler_test

import (
	"errors"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/sshtunneler"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&sshreqconnSuite{})

type sshreqconnSuite struct {
	ctxMock     *MockContext
	backendMock *MockBackend
}

func (s *sshreqconnSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctxMock = NewMockContext(ctrl)
	s.backendMock = NewMockBackend(ctrl)
	return ctrl
}

func (s *sshreqconnSuite) TestAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authorizer := NewMockAuthorizer(s.setupMocks(c))
	s.ctxMock.EXPECT().Auth().Return(authorizer)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := sshtunneler.NewExternalFacade(s.ctxMock)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *sshreqconnSuite) TestInsertSSHConnRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := sshtunneler.NewFacade(s.ctxMock, s.backendMock)

	arg := params.SSHConnRequestArg{
		TunnelID: "tunnel-id",
	}

	s.backendMock.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)

	result, err := f.InsertSSHConnRequest(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *sshreqconnSuite) TestInsertSSHConnRequestError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := sshtunneler.NewFacade(s.ctxMock, s.backendMock)

	arg := params.SSHConnRequestArg{
		TunnelID: "tunnel-id",
	}

	s.backendMock.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(errors.New("insert error"))

	result, err := f.InsertSSHConnRequest(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error.Message, gc.Equals, "insert error")
}

func (s *sshreqconnSuite) TestRemoveSSHConnRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := sshtunneler.NewFacade(s.ctxMock, s.backendMock)

	arg := params.SSHConnRequestRemoveArg{
		TunnelID: "tunnel-id",
	}

	s.backendMock.EXPECT().RemoveSSHConnRequest(gomock.Any()).Return(nil)

	result, err := f.RemoveSSHConnRequest(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *sshreqconnSuite) TestRemoveSSHConnRequestError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := sshtunneler.NewFacade(s.ctxMock, s.backendMock)

	arg := params.SSHConnRequestRemoveArg{
		TunnelID: "tunnel-id",
	}

	s.backendMock.EXPECT().RemoveSSHConnRequest(gomock.Any()).Return(errors.New("remove error"))

	result, err := f.RemoveSSHConnRequest(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error.Message, gc.Equals, "remove error")
}
