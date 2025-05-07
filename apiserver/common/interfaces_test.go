// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	coretesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
)

type AuthFuncSuite struct {
	coretesting.IsolationSuite

	authorizer common.Authorizer
}

var _ = tc.Suite(&AuthFuncSuite{})

func (s *AuthFuncSuite) setup(c *tc.C, machineTag names.Tag) func() {
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

func (s *AuthFuncSuite) TestAuthFuncForMachineAgent(c *tc.C) {
	machineTag := names.NewMachineTag("machine-test/0")
	finish := s.setup(c, machineTag)
	defer finish()

	authFunc := common.AuthFuncForMachineAgent(s.authorizer)

	fn, err := authFunc(context.Background())
	c.Assert(err, tc.IsNil)
	c.Assert(fn(machineTag), tc.IsTrue)
}

func (s *AuthFuncSuite) TestAuthFuncForMachineAgentInvalidMachineTag(c *tc.C) {
	machineTag := names.NewMachineTag("machine-test/0")
	finish := s.setup(c, machineTag)
	defer finish()

	authFunc := common.AuthFuncForMachineAgent(s.authorizer)
	invalidTag := names.NewUserTag("user-bob@foo")

	fn, err := authFunc(context.Background())
	c.Assert(err, tc.IsNil)
	c.Assert(fn(invalidTag), tc.IsFalse)
}

func (s *AuthFuncSuite) TestAuthFuncForMachineAgentInvalidAuthTag(c *tc.C) {
	invalidTag := names.NewUserTag("user-bob@foo")
	finish := s.setup(c, invalidTag)
	defer finish()

	authFunc := common.AuthFuncForMachineAgent(s.authorizer)
	machineTag := names.NewMachineTag("machine-test/0")

	fn, err := authFunc(context.Background())
	c.Assert(err, tc.IsNil)
	c.Assert(fn(machineTag), tc.IsFalse)
}
