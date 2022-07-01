// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/docker/registry/internal"
	"github.com/juju/juju/v2/docker/registry/internal/mocks"
)

type providerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestInitClient(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	initializer := mocks.NewMockInitializer(ctrl)

	gomock.InOrder(
		initializer.EXPECT().DecideBaseURL().Return(nil),
		initializer.EXPECT().WrapTransport().Return(nil),
	)
	err := internal.InitProvider(initializer)
	c.Assert(err, jc.ErrorIsNil)
}
