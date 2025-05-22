// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/controller/usersecretsdrain"
	"github.com/juju/juju/api/controller/usersecretsdrain/mocks"
	coretesting "github.com/juju/juju/internal/testing"
)

func TestUserSecretsdrainSuite(t *stdtesting.T) {
	tc.Run(t, &userSecretsdrainSuite{})
}

type userSecretsdrainSuite struct {
	coretesting.BaseSuite
}

func (s *userSecretsdrainSuite) TestNewClient(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockAPICaller(ctrl)
	apiCaller.EXPECT().BestFacadeVersion("UserSecretsDrain").Return(1)

	client := usersecretsdrain.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}
