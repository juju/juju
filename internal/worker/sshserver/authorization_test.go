// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"errors"

	"github.com/google/uuid"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
)

type authorizationSuite struct {
	facadeClient *MockFacadeClient
	ctx          *MockContext
}

var _ = gc.Suite(&authorizationSuite{})

func (s *authorizationSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facadeClient = NewMockFacadeClient(ctrl)
	s.ctx = NewMockContext(ctrl)

	return ctrl
}

func (s *authorizationSuite) TestAuthorizerViaState(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	modelUUID := uuid.NewString()
	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)

	s.facadeClient.EXPECT().CheckSSHAccess("alice", gomock.Any()).DoAndReturn(
		func(u string, d virtualhostname.Info) (bool, error) {
			if d.ModelUUID() == modelUUID {
				return true, nil
			}
			return false, nil
		},
	).Times(2)

	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(true).Times(2)
	s.ctx.EXPECT().User().Return("alice").Times(2)

	authorizer := authorizer{
		facadeClient: s.facadeClient,
		logger:       loggo.GetLogger("test"),
	}

	c.Check(authorizer.authorize(s.ctx, destination), jc.IsTrue)
	c.Check(authorizer.authorize(s.ctx, virtualhostname.Info{}), jc.IsFalse)
}

func (s *authorizationSuite) TestAuthorizerViaJWT(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	modelUUID := uuid.NewString()
	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)
	modelTag := names.NewModelTag(modelUUID)

	token, err := jwt.NewBuilder().
		Subject("alice").
		Claim(modelTag.String(), permission.AdminAccess).
		Build()
	c.Assert(err, jc.ErrorIsNil)

	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(false).Times(2)
	s.ctx.EXPECT().Value(userJWT{}).Return(token).Times(2)

	authorizer := authorizer{
		facadeClient: s.facadeClient,
		logger:       loggo.GetLogger("test"),
	}

	// Test that alice has access as specified in the JWT.
	c.Check(authorizer.authorize(s.ctx, destination), jc.IsTrue)
	// Test that alice does not have access to arbitrary destinations.
	c.Check(authorizer.authorize(s.ctx, virtualhostname.Info{}), jc.IsFalse)

}

func (s *authorizationSuite) TestMissingContextValues(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	modelUUID := uuid.NewString()
	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)

	authorizer := authorizer{
		facadeClient: s.facadeClient,
		logger:       loggo.GetLogger("test"),
	}

	// Test that the authorizer returns false if the context does not
	// contain the authenticatedViaPublicKey value.
	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(nil)
	c.Check(authorizer.authorize(s.ctx, destination), jc.IsFalse)

	// Test that the authorizer returns false if the context does not
	// contain the userJWT value.
	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(false)
	s.ctx.EXPECT().Value(userJWT{}).Return(nil)
	c.Check(authorizer.authorize(s.ctx, destination), jc.IsFalse)
}

func (s *authorizationSuite) TestFacadeFailure(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	modelUUID := uuid.NewString()
	destination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, jc.ErrorIsNil)

	authorizer := authorizer{
		facadeClient: s.facadeClient,
		logger:       loggo.GetLogger("test"),
	}

	s.facadeClient.EXPECT().CheckSSHAccess(gomock.Any(), gomock.Any()).
		Return(false, errors.New("facade error"))
	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(true)
	s.ctx.EXPECT().User().Return("alice")

	c.Check(authorizer.authorize(s.ctx, destination), jc.IsFalse)
}
