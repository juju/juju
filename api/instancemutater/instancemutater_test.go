// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/api/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

type instanceMutaterSuite struct {
	jujutesting.BaseSuite

	tag names.Tag

	fCaller   *mocks.MockFacadeCaller
	apiCaller *mocks.MockAPICaller
}

var _ = gc.Suite(&instanceMutaterSuite{})

func (s *instanceMutaterSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.BaseSuite.SetUpTest(c)
}

func (s *instanceMutaterSuite) TestWatchModelMachines(c *gc.C) {
	defer s.setup(c).Finish()

	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("InstanceMutater").Return(1)
	aExp.APICall("InstanceMutater", 1, "", "WatchModelMachines", nil, gomock.Any()).Return(nil)
	aExp.BestFacadeVersion("StringsWatcher").Return(1)
	aExp.APICall("StringsWatcher", 1, "", "Next", nil, gomock.Any()).Return(nil).MinTimes(1)

	api := instancemutater.NewClient(s.apiCaller)
	ch, err := api.WatchModelMachines()
	c.Assert(err, jc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-ch.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}

func (s *instanceMutaterSuite) TestWatchModelMachinesServerError(c *gc.C) {
	apiCaller := clientErrorAPICaller(c, "WatchModelMachines", nil)
	api := instancemutater.NewClient(apiCaller)
	w, err := api.WatchModelMachines()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(w, gc.IsNil)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}

func (s *instanceMutaterSuite) TestMachineCallsLife(c *gc.C) {
	// We have tested separately the Life method, here we just check
	// it's called internally.
	expectedResults := params.LifeResults{
		Results: []params.LifeResult{{Life: "working"}},
	}
	entitiesArgs := params.Entities{
		Entities: []params.Entity{
			{Tag: s.tag.String()},
		},
	}
	apiCaller := successAPICaller(c, "Life", entitiesArgs, expectedResults)
	api := instancemutater.NewClient(apiCaller)
	m, err := api.Machine(names.NewMachineTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
	c.Assert(m.Tag().String(), gc.Equals, s.tag.String())
}

func (s *instanceMutaterSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)

	return ctrl
}

func clientErrorAPICaller(c *gc.C, method string, expectArgs interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstanceMutater",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Error:         errors.New("client error!"),
	})
}

func successAPICaller(c *gc.C, method string, expectArgs, useResults interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstanceMutater",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	})
}
