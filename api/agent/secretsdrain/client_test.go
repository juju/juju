// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

import (
	"context"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/secretsdrain"
	"github.com/juju/juju/api/agent/secretsdrain/mocks"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&secretsDrainSuite{})

type secretsDrainSuite struct {
	coretesting.BaseSuite
}

func (s *secretsDrainSuite) TestNewClient(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockAPICaller(ctrl)
	apiCaller.EXPECT().BestFacadeVersion("SecretsDrain").Return(1)
	apiCaller.EXPECT().Context().Return(context.Background()).AnyTimes()

	client := secretsdrain.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}
