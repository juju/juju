// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/api/agent/instancemutater/mocks"
	apitesting "github.com/juju/juju/api/base/testing"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type instanceMutaterSuite struct {
	jujutesting.BaseSuite

	tag names.Tag

	fCaller   *mocks.MockFacadeCaller
	apiCaller *mocks.MockAPICaller
}

func TestInstanceMutaterSuite(t *testing.T) {
	tc.Run(t, &instanceMutaterSuite{})
}

func (s *instanceMutaterSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewMachineTag("0")
	s.BaseSuite.SetUpTest(c)
}

func (s *instanceMutaterSuite) TestMachineCallsLife(c *tc.C) {
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
	m, err := api.Machine(c.Context(), names.NewMachineTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiCaller.CallCount, tc.Equals, 1)
	c.Assert(m.Tag().String(), tc.Equals, s.tag.String())
}

func (s *instanceMutaterSuite) TestWatchMachines(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.clientForScenario(c,
		s.expectWatchModelMachines,
		s.expectStringsWatcher,
	)
	ch, err := api.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-ch.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}

func (s *instanceMutaterSuite) TestWatchMachinesServerError(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.clientForScenario(c,
		s.expectWatchModelMachinesWithError,
	)
	_, err := api.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, "failed")
}

func (s *instanceMutaterSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)

	return ctrl
}

func (s *instanceMutaterSuite) clientForScenario(c *tc.C, behaviours ...func()) *instancemutater.Client {
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

func successAPICaller(c *tc.C, method string, expectArgs, useResults interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstanceMutater",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	})
}
