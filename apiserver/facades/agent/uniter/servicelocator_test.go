// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/uniter/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/testing"
)

type serviceLocatorSuite struct {
	testing.IsolationSuite

	unitTag1 names.UnitTag
}

var _ = gc.Suite(&serviceLocatorSuite{})

func (s *serviceLocatorSuite) SetUpTest(c *gc.C) {
	s.unitTag1 = names.NewUnitTag("mysql/1")
}

func (s *serviceLocatorSuite) assertBackendAPI(c *gc.C, tag names.Tag) (*uniter.ServiceLocatorAPI, *gomock.Controller, *mocks.MockServiceLocatorBackend) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	ctrl := gomock.NewController(c)
	mockBackend := mocks.NewMockServiceLocatorBackend(ctrl)

	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	api := uniter.NewServiceLocatorAPI(
		mockBackend, authorizer, unitAuthFunc, loggo.GetLogger("juju.apiserver.facades.agent.uniter"))
	return api, ctrl, mockBackend
}

func (s *serviceLocatorSuite) TestAddServiceLocator(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendAPI(c, s.unitTag1)
	defer ctrl.Finish()

	mockBackend.EXPECT().AddServiceLocator("id", "name", "type").Return("id", nil)

	sl, err := api.AddServiceLocator(params.AddServiceLocators{
		ServiceLocators: []params.AddServiceLocatorParams{{
			ServiceLocatorUUID: "id",
			Name:               "name",
			Type:               "type",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sl.Results[0].Result, gc.Equals, "id")
}
