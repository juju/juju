// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain_test

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain/mocks"
)

type drainSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&drainSuite{})

func (s *drainSuite) TestNewSecretManagerAPII(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := mocks.NewMockAuthorizer(ctrl)
	context := mocks.NewMockContext(ctrl)
	context.EXPECT().Auth().Return(authorizer).AnyTimes()
	authorizer.EXPECT().AuthController().Return(false)

	_, err := usersecretsdrain.NewUserSecretsDrainAPI(context)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
