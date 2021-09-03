// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/docker/registry/mocks"
)

type registrySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&registrySuite{})

func (s *registrySuite) TestNew(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	initializer := mocks.NewMockInitializer(ctrl)

	gomock.InOrder(
		initializer.EXPECT().DecideBaseURL().Return(nil),
		initializer.EXPECT().WrapTransport().Return(nil),
		initializer.EXPECT().Ping().Return(nil),
	)
	err := registry.InitClient(initializer)
	c.Assert(err, jc.ErrorIsNil)
}
