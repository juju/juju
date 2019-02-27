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
}

var _ = gc.Suite(&AuthFuncSuite{})

func (s *AuthFuncSuite) TestAuthFuncForMachineAgent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := mocks.NewMockAuthorizer(ctrl)
	machineTag := names.NewMachineTag("machine-test/0")

	gomock.InOrder(
		authorizer.EXPECT().AuthController().Return(true),
		authorizer.EXPECT().AuthMachineAgent().Return(true),
		authorizer.EXPECT().GetAuthTag().Return(machineTag),
	)

	authFunc := common.AuthFuncForMachineAgent(authorizer)

	fn, err := authFunc()
	c.Assert(err, gc.IsNil)
	c.Assert(fn(machineTag), jc.IsTrue)
}
