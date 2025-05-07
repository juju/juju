// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

type UnitAccessorSuite struct {
	testing.IsolationSuite

	applicationService *MockApplicationService
}

var _ = tc.Suite(&UnitAccessorSuite{})

func (s *UnitAccessorSuite) TestApplicationAgent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().
		GetApplicationIDByName(gomock.Any(), "gitlab").
		Return(application.ID("1"), nil)

	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}

	getAuthFunc := UnitAccessor(auth, s.applicationService)
	authFunc, err := getAuthFunc(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, jc.IsFalse)
}

func (s *UnitAccessorSuite) TestApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().
		GetApplicationIDByName(gomock.Any(), "gitlab").
		Return(application.ID("1"), applicationerrors.ApplicationNotFound)

	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := UnitAccessor(auth, s.applicationService)
	_, err := getAuthFunc(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *UnitAccessorSuite) TestUnitAgent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("gitlab/0"),
	}
	getAuthFunc := UnitAccessor(auth, s.applicationService)
	authFunc, err := getAuthFunc(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewApplicationTag("gitlab"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewUnitTag("gitlab/1"))
	c.Assert(ok, jc.IsFalse)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, jc.IsFalse)
}

func (s *UnitAccessorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)

	return ctrl

}
