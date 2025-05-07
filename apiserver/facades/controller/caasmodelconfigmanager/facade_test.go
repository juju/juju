// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager/mocks"
)

var _ = tc.Suite(&caasmodelconfigmanagerSuite{})

type caasmodelconfigmanagerSuite struct {
	testing.IsolationSuite
}

func (s *caasmodelconfigmanagerSuite) TestAuth(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := mocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := caasmodelconfigmanager.NewFacade(facadetest.ModelContext{
		Auth_: authorizer,
	})
	c.Assert(err, tc.ErrorMatches, `permission denied`)
}
