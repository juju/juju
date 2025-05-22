// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type UnitAccessorSuite struct {
	testhelpers.IsolationSuite

	applicationService *MockApplicationService
}

func TestUnitAccessorSuite(t *testing.T) {
	tc.Run(t, &UnitAccessorSuite{})
}

func (s *UnitAccessorSuite) TestApplicationAgent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().
		GetApplicationIDByName(gomock.Any(), "gitlab").
		Return(application.ID("1"), nil)

	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}

	getAuthFunc := UnitAccessor(auth, s.applicationService)
	authFunc, err := getAuthFunc(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, tc.IsTrue)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, tc.IsFalse)
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
	_, err := getAuthFunc(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *UnitAccessorSuite) TestUnitAgent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("gitlab/0"),
	}
	getAuthFunc := UnitAccessor(auth, s.applicationService)
	authFunc, err := getAuthFunc(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, tc.IsTrue)
	ok = authFunc(names.NewApplicationTag("gitlab"))
	c.Assert(ok, tc.IsTrue)
	ok = authFunc(names.NewUnitTag("gitlab/1"))
	c.Assert(ok, tc.IsFalse)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, tc.IsFalse)
}

func (s *UnitAccessorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)

	return ctrl

}
