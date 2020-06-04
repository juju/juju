// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/uniter/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/testing"
)

type newLxdProfileSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag

	backend *mocks.MockLXDProfileBackendV2
	unit    *mocks.MockLXDProfileUnitV2
	machine *mocks.MockLXDProfileMachineV2
	charm   *mocks.MockLXDProfileCharmV2
}

var _ = gc.Suite(&newLxdProfileSuite{})

func (s *newLxdProfileSuite) SetUpTest(c *gc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
}

func (s *newLxdProfileSuite) TestWatchInstanceData(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectWatchInstanceData()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}

	api := s.newAPI()
	results, err := api.WatchInstanceData(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{NotifyWatcherId: "1", Error: nil},
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *newLxdProfileSuite) TestLXDProfileName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectOneLXDProfileName()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}

	api := s.newAPI()
	results, err := api.LXDProfileName(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Result: "juju-model-mysql-1", Error: nil},
			{Result: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *newLxdProfileSuite) TestLXDProfileRequired(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectOneLXDProfileRequired()

	args := params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "cs:mysql-1"},
			{URL: "cs:testme-3"},
		},
	}

	api := s.newAPI()
	results, err := api.LXDProfileRequired(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: true, Error: nil},
			{Result: false, Error: &params.Error{Message: "cs:testme-3 not found", Code: "not found"}},
		},
	})
}

func (s *newLxdProfileSuite) newAPI() *uniter.LXDProfileAPIv2 {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.unitTag1,
	}
	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}
	api := uniter.NewLXDProfileAPIv2(
		s.backend,
		resources,
		authorizer,
		unitAuthFunc,
		loggo.GetLogger("juju.apiserver.facades.agent.uniter"))
	return api
}

func (s *newLxdProfileSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.backend = mocks.NewMockLXDProfileBackendV2(ctrl)
	s.unit = mocks.NewMockLXDProfileUnitV2(ctrl)
	s.machine = mocks.NewMockLXDProfileMachineV2(ctrl)
	s.charm = mocks.NewMockLXDProfileCharmV2(ctrl)
	return ctrl
}

func (s *newLxdProfileSuite) expectUnitAndMachine() {
	s.backend.EXPECT().Unit(s.unitTag1.Id()).Return(s.unit, nil)
	s.unit.EXPECT().AssignedMachineId().Return(s.machineTag1.Id(), nil)

	s.backend.EXPECT().Machine(s.machineTag1.Id()).Return(s.machine, nil)
}

func (s *newLxdProfileSuite) expectOneLXDProfileName() {
	s.machine.EXPECT().CharmProfiles().Return([]string{"default", "juju-model-mysql-1"}, nil)
	s.unit.EXPECT().ApplicationName().Return("mysql")
}

func (s *newLxdProfileSuite) expectOneLXDProfileRequired() {
	s.backend.EXPECT().Charm(charm.MustParseURL("cs:mysql-1")).Return(s.charm, nil)
	s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{Config: map[string]string{"one": "two"}})

	s.backend.EXPECT().Charm(charm.MustParseURL("cs:testme-3")).Return(nil, errors.NotFoundf("cs:testme-3"))
}

func (s *newLxdProfileSuite) expectWatchInstanceData() {
	watcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	watcher.changes <- struct{}{}

	s.machine.EXPECT().WatchInstanceData().Return(watcher)
}
