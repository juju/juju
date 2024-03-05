// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain_test

import (
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller/usersecretsdrain"
	"github.com/juju/juju/api/controller/usersecretsdrain/mocks"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&userSecretsdrainSuite{})

type userSecretsdrainSuite struct {
	coretesting.BaseSuite
}

func (s *userSecretsdrainSuite) TestNewClient(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockAPICaller(ctrl)
	apiCaller.EXPECT().BestFacadeVersion("UserSecretsDrain").Return(1)

	client := usersecretsdrain.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}
