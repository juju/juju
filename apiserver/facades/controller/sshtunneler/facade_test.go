// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"errors"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
	state "github.com/juju/juju/state"
)

var _ = gc.Suite(&sshtunnelerSuite{})

type sshtunnelerSuite struct {
	ctx     *MockContext
	backend *MockBackend
}

func (s *sshtunnelerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctx = NewMockContext(ctrl)
	s.backend = NewMockBackend(ctrl)
	return ctrl
}

func (s *sshtunnelerSuite) TestAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authorizer := NewMockAuthorizer(s.setupMocks(c))
	s.ctx.EXPECT().Auth().Return(authorizer)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := newExternalFacade(s.ctx)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *sshtunnelerSuite) TestInsertSSHConnRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)

	result, err := f.InsertSSHConnRequest(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *sshtunnelerSuite) TestInsertSSHConnRequestError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(errors.New("insert error"))

	result, err := f.InsertSSHConnRequest(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error.Message, gc.Equals, "insert error")
}

func (s *sshtunnelerSuite) TestRemoveSSHConnRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestRemoveArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().RemoveSSHConnRequest(gomock.Any()).Return(nil)

	result, err := f.RemoveSSHConnRequest(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *sshtunnelerSuite) TestRemoveSSHConnRequestError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestRemoveArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().RemoveSSHConnRequest(gomock.Any()).Return(errors.New("remove error"))

	result, err := f.RemoveSSHConnRequest(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error.Message, gc.Equals, "remove error")
}

func (s *sshtunnelerSuite) TestControllerAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.backend.EXPECT().Machine("1").Return(
		&state.Machine{}, nil,
	)

	f := newFacade(s.ctx, s.backend)

	entity := params.Entity{Tag: names.NewMachineTag("1").String()}
	addresses, err := f.ControllerAddresses(entity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.DeepEquals, params.StringsResult{})
}
