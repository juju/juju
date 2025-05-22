// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/secretsdrain"
	"github.com/juju/juju/api/agent/secretsdrain/mocks"
	coretesting "github.com/juju/juju/internal/testing"
)

func TestSecretsDrainSuite(t *testing.T) {
	tc.Run(t, &secretsDrainSuite{})
}

type secretsDrainSuite struct {
	coretesting.BaseSuite
}

func (s *secretsDrainSuite) TestNewClient(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockAPICaller(ctrl)
	apiCaller.EXPECT().BestFacadeVersion("SecretsDrain").Return(1)

	client := secretsdrain.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}
