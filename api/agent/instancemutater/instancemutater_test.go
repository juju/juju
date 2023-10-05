// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/api/agent/instancemutater/mocks"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
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

func (s *instanceMutaterSuite) TestWatchMachines(c *gc.C) {
	defer s.setup(c).Finish()

	api := s.clientForScenario(c,
		s.expectWatchModelMachines,
		s.expectStringsWatcher,
	)
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

func (s *instanceMutaterSuite) TestWatchMachinesServerError(c *gc.C) {
	defer s.setup(c).Finish()

	api := s.clientForScenario(c,
		s.expectWatchModelMachinesWithError,
	)
	_, err := api.WatchModelMachines()
	c.Assert(err, gc.ErrorMatches, "failed")
}

func (s *instanceMutaterSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)

	return ctrl
}

func (s *instanceMutaterSuite) clientForScenario(c *gc.C, behaviours ...func()) *instancemutater.Client {
	for _, b := range behaviours {
		b()
	}

	return instancemutater.NewClient(s.apiCaller)
}

func (s *instanceMutaterSuite) expectWatchModelMachines() {
	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("InstanceMutater").Return(1)
	aExp.APICall(gomock.Any(), "InstanceMutater", 1, "", "WatchModelMachines", nil, gomock.Any()).Return(nil)
}

func (s *instanceMutaterSuite) expectStringsWatcher() {
	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("StringsWatcher").Return(1)
	aExp.APICall(gomock.Any(), "StringsWatcher", 1, "", "Next", nil, gomock.Any()).Return(nil).MinTimes(1)
}

func (s *instanceMutaterSuite) expectWatchModelMachinesWithError() {
	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("InstanceMutater").Return(1)
	aExp.APICall(gomock.Any(), "InstanceMutater", 1, "", "WatchModelMachines", nil, gomock.Any()).Return(errors.New("failed"))
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
