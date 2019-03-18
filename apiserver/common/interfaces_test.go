// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/golang/mock/gomock"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
)

type AuthFuncSuite struct {
	coretesting.IsolationSuite

	authorizer common.Authorizer
}

var _ = gc.Suite(&AuthFuncSuite{})

func (s *AuthFuncSuite) setup(c *gc.C, machineTag names.Tag) func() {
	ctrl := gomock.NewController(c)

	authorizer := mocks.NewMockAuthorizer(ctrl)

	gomock.InOrder(
		authorizer.EXPECT().AuthController().Return(true),
		authorizer.EXPECT().AuthMachineAgent().Return(true),
		authorizer.EXPECT().GetAuthTag().Return(machineTag),
	)

	s.authorizer = authorizer

	return ctrl.Finish
}

func (s *AuthFuncSuite) TestAuthFuncForMachineAgent(c *gc.C) {
	machineTag := names.NewMachineTag("machine-test/0")
	finish := s.setup(c, machineTag)
	defer finish()

	authFunc := common.AuthFuncForMachineAgent(s.authorizer)

	fn, err := authFunc()
	c.Assert(err, gc.IsNil)
	c.Assert(fn(machineTag), jc.IsTrue)
}

func (s *AuthFuncSuite) TestAuthFuncForMachineAgentInvalidMachineTag(c *gc.C) {
	machineTag := names.NewMachineTag("machine-test/0")
	finish := s.setup(c, machineTag)
	defer finish()

	authFunc := common.AuthFuncForMachineAgent(s.authorizer)
	invalidTag := names.NewUserTag("user-bob@foo")

	fn, err := authFunc()
	c.Assert(err, gc.IsNil)
	c.Assert(fn(invalidTag), jc.IsFalse)
}

func (s *AuthFuncSuite) TestAuthFuncForMachineAgentInvalidAuthTag(c *gc.C) {
	invalidTag := names.NewUserTag("user-bob@foo")
	finish := s.setup(c, invalidTag)
	defer finish()

	authFunc := common.AuthFuncForMachineAgent(s.authorizer)
	machineTag := names.NewMachineTag("machine-test/0")

	fn, err := authFunc()
	c.Assert(err, gc.IsNil)
	c.Assert(fn(machineTag), jc.IsFalse)
}
