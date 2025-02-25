// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/sshserver"
	"github.com/juju/juju/apiserver/facades/controller/sshserver/mocks"
)

var _ = gc.Suite(&sshserverSuite{})

type sshserverSuite struct {
	testing.IsolationSuite
}

func (s *sshserverSuite) TestAuth(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := mocks.NewMockContext(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)

	gomock.InOrder(
		ctx.EXPECT().Auth().Return(authorizer),
		authorizer.EXPECT().AuthController().Return(false),
	)

	_, err := sshserver.NewFacade(ctx)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}
