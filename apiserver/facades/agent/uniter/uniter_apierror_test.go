// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
)

type uniterAPIErrorSuite struct {
	testing.ApiServerSuite

	controllerConfig *MockControllerConfigGetter
}

var _ = gc.Suite(&uniterAPIErrorSuite{})

func (s *uniterAPIErrorSuite) TestGetStorageStateError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.controllerConfig = NewMockControllerConfigGetter(ctrl)

	uniter.PatchGetStorageStateError(s, errors.New("kaboom"))

	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })

	_, err := uniter.NewUniterAPI(facadetest.Context{
		State_:             s.ControllerModel(c).State(),
		StatePool_:         s.StatePool(),
		Resources_:         resources,
		Auth_:              apiservertesting.FakeAuthorizer{Tag: names.NewUnitTag("nomatter/0")},
		LeadershipChecker_: &fakeLeadershipChecker{false},
	}, s.controllerConfig)

	c.Assert(err, gc.ErrorMatches, "kaboom")
}
