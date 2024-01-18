// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager/mocks"
)

var _ = gc.Suite(&caasmodelconfigmanagerSuite{})

type caasmodelconfigmanagerSuite struct {
	testing.IsolationSuite
}

func (s *caasmodelconfigmanagerSuite) TestAuth(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := mocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := caasmodelconfigmanager.NewFacade(facadetest.Context{
		Auth_: authorizer,
	})
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}
