// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/docker/registry/internal"
	"github.com/juju/juju/internal/docker/registry/internal/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type providerSuite struct {
	testhelpers.IsolationSuite
}

func TestProviderSuite(t *stdtesting.T) {
	tc.Run(t, &providerSuite{})
}

func (s *providerSuite) TestInitClient(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	initializer := mocks.NewMockInitializer(ctrl)

	gomock.InOrder(
		initializer.EXPECT().DecideBaseURL().Return(nil),
		initializer.EXPECT().WrapTransport().Return(nil),
	)
	err := internal.InitProvider(initializer)
	c.Assert(err, tc.ErrorIsNil)
}
