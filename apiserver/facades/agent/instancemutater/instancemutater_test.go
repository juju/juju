// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	coretesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type instanceMutaterAPISuite struct {
	coretesting.IsolationSuite

	authorizer     *facademocks.MockAuthorizer
	entity         *mocks.MockEntity
	lifer          *mocks.MockLifer
	state          *mocks.MockInstanceMutaterState
	mutatorWatcher *mocks.MockInstanceMutatorWatcher
	resources      *facademocks.MockResources

	machineTag  names.Tag
	notifyDone  chan struct{}
	stringsDone chan []string
}

func (s *instanceMutaterAPISuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.machineTag = names.NewMachineTag("0")
	s.notifyDone = make(chan struct{})
	s.stringsDone = make(chan []string)
}

func (s *instanceMutaterAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.entity = mocks.NewMockEntity(ctrl)
	s.lifer = mocks.NewMockLifer(ctrl)
	s.state = mocks.NewMockInstanceMutaterState(ctrl)
	s.mutatorWatcher = mocks.NewMockInstanceMutatorWatcher(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)

	return ctrl
}

func (s *instanceMutaterAPISuite) facadeAPIForScenario(c *gc.C) *instancemutater.InstanceMutaterAPI {
	facade, err := instancemutater.NewTestAPI(s.state, s.mutatorWatcher, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	return facade
}

func (s *instanceMutaterAPISuite) expectLife(machineTag names.Tag) {
	exp := s.authorizer.EXPECT()
	gomock.InOrder(
		exp.AuthController().Return(true),
		exp.AuthMachineAgent().Return(true),
		exp.GetAuthTag().Return(machineTag),
	)
}

func (s *instanceMutaterAPISuite) expectMachine(machineTag names.Tag, machine *mocks.MockMachine) {
	s.state.EXPECT().Machine(machineTag.Id()).Return(machine, nil)
}

func (s *instanceMutaterAPISuite) expectFindMachineError(machineTag names.Tag, err error) {
	s.state.EXPECT().Machine(machineTag.Id()).Return(nil, err)
}

func (s *instanceMutaterAPISuite) expectAuthMachineAgent() {
	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
}

func (s *instanceMutaterAPISuite) assertNotifyStop(c *gc.C) {
	select {
	case <-s.notifyDone:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
}

func (s *instanceMutaterAPISuite) assertStringsStop(c *gc.C) {
	select {
	case <-s.stringsDone:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
}

type InstanceMutaterAPILifeSuite struct {
	instanceMutaterAPISuite
}

var _ = gc.Suite(&InstanceMutaterAPILifeSuite{})

func (s *InstanceMutaterAPILifeSuite) TestLife(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectFindEntity(s.machineTag, entityShim{
		Entity: s.entity,
		Lifer:  s.lifer,
	})
	facade := s.facadeAPIForScenario(c)

	results, err := facade.Life(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: life.Alive,
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithInvalidType(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	facade := s.facadeAPIForScenario(c)

	results, err := facade.Life(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "user-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Error: &params.Error{
					Message: "permission denied",
					Code:    "unauthorized access",
				},
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithParentId(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0/lxd/0")

	s.expectAuthMachineAgent()
	s.expectLife(machineTag)
	s.expectFindEntity(machineTag, entityShim{
		Entity: s.entity,
		Lifer:  s.lifer,
	})
	facade := s.facadeAPIForScenario(c)

	results, err := facade.Life(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "machine-0-lxd-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: life.Alive,
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithInvalidParentId(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0/lxd/0")

	s.expectAuthMachineAgent()
	s.expectLife(machineTag)
	facade := s.facadeAPIForScenario(c)

	results, err := facade.Life(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "machine-1-lxd-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Error: &params.Error{
					Message: "permission denied",
					Code:    "unauthorized access",
				},
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) expectFindEntity(machineTag names.Tag, entity state.Entity) {
	s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)
}

type entityShim struct {
	state.Entity
	state.Lifer
}

type InstanceMutaterAPICharmProfilingInfoSuite struct {
	instanceMutaterAPISuite

	machine     *mocks.MockMachine
	unit        *mocks.MockUnit
	application *mocks.MockApplication
	charm       *mocks.MockCharm
}

var _ = gc.Suite(&InstanceMutaterAPICharmProfilingInfoSuite{})

func (s *InstanceMutaterAPICharmProfilingInfoSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.application = mocks.NewMockApplication(ctrl)
	s.charm = mocks.NewMockCharm(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmShimNilInnerProfile(c *gc.C) {
	c.Assert(instancemutater.NewEmptyCharmShim().LXDProfile(), gc.DeepEquals, lxdprofile.Profile{})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfo(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectInstanceId("0")
	s.expectUnits(state.Alive)
	s.expectCharmProfiles()
	s.expectProfileExtraction()
	s.expectName()
	facade := s.facadeAPIForScenario(c)

	results, err := facade.CharmProfilingInfo(context.Background(), params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.InstanceId, gc.Equals, instance.Id("0"))
	c.Assert(results.ModelName, gc.Equals, "foo")
	c.Assert(results.ProfileChanges, gc.HasLen, 1)
	c.Assert(results.CurrentProfiles, gc.HasLen, 1)
	c.Assert(results.ProfileChanges, gc.DeepEquals, []params.ProfileInfoResult{
		{
			ApplicationName: "foo",
			Revision:        0,
			Profile: &params.CharmLXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "dummy profile description",
				Devices: map[string]map[string]string{
					"tun": {
						"path": "/dev/net/tun",
					},
				},
			},
		},
	})
	c.Assert(results.CurrentProfiles, gc.DeepEquals, []string{
		"charm-app-0",
	})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithNoProfile(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectInstanceId("0")
	s.expectUnits(state.Alive, state.Alive, state.Dead)
	s.expectCharmProfiles()
	s.expectProfileExtraction()
	s.expectProfileExtractionWithEmpty()
	s.expectName()
	facade := s.facadeAPIForScenario(c)

	results, err := facade.CharmProfilingInfo(context.Background(), params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.InstanceId, gc.Equals, instance.Id("0"))
	c.Assert(results.ModelName, gc.Equals, "foo")
	c.Assert(results.ProfileChanges, gc.HasLen, 2)
	c.Assert(results.CurrentProfiles, gc.HasLen, 1)
	c.Assert(results.ProfileChanges, gc.DeepEquals, []params.ProfileInfoResult{
		{
			ApplicationName: "foo",
			Revision:        0,
			Profile: &params.CharmLXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "dummy profile description",
				Devices: map[string]map[string]string{
					"tun": {
						"path": "/dev/net/tun",
					},
				},
			},
		},
		{
			ApplicationName: "foo",
			Revision:        0,
		},
	})
	c.Assert(results.CurrentProfiles, gc.DeepEquals, []string{
		"charm-app-0",
	})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithInvalidMachine(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectFindMachineError(s.machineTag, errors.New("not found"))
	facade := s.facadeAPIForScenario(c)

	results, err := facade.CharmProfilingInfo(context.Background(), params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.ErrorMatches, "not found")
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithMachineNotProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectInstanceIdNotProvisioned()
	facade := s.facadeAPIForScenario(c)

	results, err := facade.CharmProfilingInfo(context.Background(), params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.ErrorMatches, "machine-0: attempting to get instanceId: ")
	c.Assert(results.InstanceId, gc.Equals, instance.Id(""))
	c.Assert(results.ModelName, gc.Equals, "")
	c.Assert(results.ProfileChanges, gc.HasLen, 0)
	c.Assert(results.CurrentProfiles, gc.HasLen, 0)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectInstanceId(id instance.Id) {
	s.machine.EXPECT().InstanceId().Return(id, nil)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectInstanceIdNotProvisioned() {
	s.machine.EXPECT().InstanceId().Return(instance.Id("0"), params.Error{Code: params.CodeNotProvisioned})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectUnits(lives ...state.Life) {
	machineExp := s.machine.EXPECT()
	units := make([]instancemutater.Unit, len(lives))
	for i := 0; i < len(lives); i++ {
		units[i] = s.unit
		s.unit.EXPECT().Life().Return(lives[i])
		if lives[i] == state.Dead {
			s.unit.EXPECT().Name().Return("foo")
		}
	}
	machineExp.Units().Return(units, nil)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectCharmProfiles() {
	machineExp := s.machine.EXPECT()
	machineExp.CharmProfiles().Return([]string{"charm-app-0"}, nil)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectProfileExtraction() {
	appExp := s.application.EXPECT()
	charmExp := s.charm.EXPECT()
	stateExp := s.state.EXPECT()
	unitExp := s.unit.EXPECT()

	unitExp.ApplicationName().Return("foo")
	stateExp.Application("foo").Return(s.application, nil)
	chURLStr := "ch:app-0"
	appExp.CharmURL().Return(&chURLStr)
	stateExp.Charm(chURLStr).Return(s.charm, nil)
	chURL := charm.MustParseURL(chURLStr)
	charmExp.Revision().Return(chURL.Revision)
	charmExp.LXDProfile().Return(lxdprofile.Profile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Description: "dummy profile description",
		Devices: map[string]map[string]string{
			"tun": {
				"path": "/dev/net/tun",
			},
		},
	})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectProfileExtractionWithEmpty() {
	appExp := s.application.EXPECT()
	charmExp := s.charm.EXPECT()
	stateExp := s.state.EXPECT()
	unitExp := s.unit.EXPECT()

	unitExp.ApplicationName().Return("foo")
	stateExp.Application("foo").Return(s.application, nil)
	chURLStr := "ch:app-0"
	appExp.CharmURL().Return(&chURLStr)
	stateExp.Charm(chURLStr).Return(s.charm, nil)
	chURL := charm.MustParseURL(chURLStr)
	charmExp.Revision().Return(chURL.Revision)
	charmExp.LXDProfile().Return(lxdprofile.Profile{})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectName() {
	modelExp := s.state.EXPECT()
	modelExp.ModelName().Return("foo", nil)
}

type InstanceMutaterAPISetCharmProfilesSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
}

var _ = gc.Suite(&InstanceMutaterAPISetCharmProfilesSuite{})

func (s *InstanceMutaterAPISetCharmProfilesSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPISetCharmProfilesSuite) TestSetCharmProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	profiles := []string{"unit-foo-0"}

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectSetProfiles(profiles, nil)
	facade := s.facadeAPIForScenario(c)

	results, err := facade.SetCharmProfiles(context.Background(), params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: "machine-0"},
				Profiles: profiles,
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{}})
}

func (s *InstanceMutaterAPISetCharmProfilesSuite) TestSetCharmProfilesWithError(c *gc.C) {
	defer s.setup(c).Finish()

	profiles := []string{"unit-foo-0"}

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectSetProfiles(profiles, nil)
	s.expectMachine(s.machineTag, s.machine)
	s.expectSetProfiles(profiles, errors.New("Failure"))
	facade := s.facadeAPIForScenario(c)

	results, err := facade.SetCharmProfiles(context.Background(), params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: "machine-0"},
				Profiles: profiles,
			},
			{
				Entity:   params.Entity{Tag: "machine-0"},
				Profiles: profiles,
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{},
		{
			Error: &params.Error{
				Message: "Failure",
			},
		},
	})
}

func (s *InstanceMutaterAPISetCharmProfilesSuite) expectSetProfiles(profiles []string, err error) {
	s.machine.EXPECT().SetCharmProfiles(profiles).Return(err)
}

type InstanceMutaterAPISetModificationStatusSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
}

var _ = gc.Suite(&InstanceMutaterAPISetModificationStatusSuite{})

func (s *InstanceMutaterAPISetModificationStatusSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPISetModificationStatusSuite) TestSetModificationStatusProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectSetModificationStatus(status.Applied, "applied", nil)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.SetModificationStatus(context.Background(), params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-0", Status: "applied", Info: "applied", Data: nil},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
		},
	})
}

func (s *InstanceMutaterAPISetModificationStatusSuite) TestSetModificationStatusProfilesWithError(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectMachine(s.machineTag, s.machine)
	s.expectSetModificationStatus(status.Applied, "applied", errors.New("failed"))
	facade := s.facadeAPIForScenario(c)

	result, err := facade.SetModificationStatus(context.Background(), params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-0", Status: "applied", Info: "applied", Data: nil},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "failed"}},
		},
	})
}

func (s *InstanceMutaterAPISetModificationStatusSuite) expectSetModificationStatus(st status.Status, message string, err error) {
	now := time.Now()

	sExp := s.state.EXPECT()
	sExp.ControllerTimestamp().Return(&now, nil)

	mExp := s.machine.EXPECT()
	mExp.SetModificationStatus(status.StatusInfo{
		Status:  st,
		Message: message,
		Data:    nil,
		Since:   &now,
	}).Return(err)
}

type InstanceMutaterAPIWatchMachinesSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
	watcher *mocks.MockStringsWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchMachinesSuite{})

func (s *InstanceMutaterAPIWatchMachinesSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)
	s.watcher = mocks.NewMockStringsWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchModelMachines(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectAuthController()
	s.expectWatchModelMachinesWithNotify(1)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchModelMachines(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0"},
	})
	s.assertNotifyStop(c)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchModelMachinesWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectAuthController()
	s.expectWatchModelMachinesWithClosedChannel()
	facade := s.facadeAPIForScenario(c)

	_, err := facade.WatchModelMachines(context.Background())
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial model machines")
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachines(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectAuthController()
	s.expectWatchMachinesWithNotify(1)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchMachines(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0"},
	})
	s.assertNotifyStop(c)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachinesWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectAuthController()
	s.expectWatchMachinesWithClosedChannel()
	facade := s.facadeAPIForScenario(c)

	_, err := facade.WatchMachines(context.Background())
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial model machines")
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesWithNotify(times int) {
	ch := make(chan []string)

	go func() {
		for i := 0; i < times; i++ {
			ch <- []string{fmt.Sprintf("%d", i)}
		}
		close(s.notifyDone)
	}()

	s.state.EXPECT().WatchMachines().Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
	s.resources.EXPECT().Register(s.watcher).Return("1")
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchModelMachinesWithNotify(times int) {
	ch := make(chan []string)

	go func() {
		for i := 0; i < times; i++ {
			ch <- []string{fmt.Sprintf("%d", i)}
		}
		close(s.notifyDone)
	}()

	s.state.EXPECT().WatchModelMachines().Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
	s.resources.EXPECT().Register(s.watcher).Return("1")
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesWithClosedChannel() {
	ch := make(chan []string)
	close(ch)

	s.state.EXPECT().WatchMachines().Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchModelMachinesWithClosedChannel() {
	ch := make(chan []string)
	close(ch)

	s.state.EXPECT().WatchModelMachines().Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
}

type InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
	watcher *mocks.MockNotifyWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite{})

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)
	s.watcher = mocks.NewMockNotifyWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeeded(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectWatchLXDProfileVerificationNeededWithNotify(1)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchLXDProfileVerificationNeeded(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	s.assertNotifyStop(c)
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeededWithInvalidTag(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchLXDProfileVerificationNeeded(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("bob@local").String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeededWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectWatchLXDProfileVerificationNeededWithClosedChannel()
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchLXDProfileVerificationNeeded(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: apiservererrors.ServerError(errors.New("cannot obtain initial machine watch application LXD profiles")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeededWithManualMachine(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectWatchLXDProfileVerificationNeededWithManualMachine()
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchLXDProfileVerificationNeeded(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: apiservererrors.ServerError(errors.NotSupportedf("watching lxd profiles on manual machines")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeededModelCacheError(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectWatchLXDProfileVerificationNeededError()
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchLXDProfileVerificationNeeded(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: apiservererrors.ServerError(errors.New("watcher error")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededWithNotify(times int) {
	ch := make(chan struct{})

	go func() {
		for i := 0; i < times; i++ {
			ch <- struct{}{}
		}
		close(s.notifyDone)
	}()

	s.state.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().IsManual().Return(false, nil)
	s.mutatorWatcher.EXPECT().WatchLXDProfileVerificationForMachine(s.machine, loggo.GetLogger("juju.apiserver.instancemutater")).Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
	s.resources.EXPECT().Register(s.watcher).Return("1")
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededWithClosedChannel() {
	ch := make(chan struct{})
	close(ch)

	s.state.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().IsManual().Return(false, nil)
	s.mutatorWatcher.EXPECT().WatchLXDProfileVerificationForMachine(s.machine, loggo.GetLogger("juju.apiserver.instancemutater")).Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededError() {
	s.state.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().IsManual().Return(false, nil)
	s.mutatorWatcher.EXPECT().WatchLXDProfileVerificationForMachine(s.machine, loggo.GetLogger("juju.apiserver.instancemutater")).Return(s.watcher, errors.New("watcher error"))
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededWithManualMachine() {
	ch := make(chan struct{})
	close(ch)

	s.state.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().IsManual().Return(true, nil)
	s.mutatorWatcher.EXPECT().WatchLXDProfileVerificationForMachine(s.machine, loggo.GetLogger("juju.apiserver.instancemutater")).Times(0)
}

type InstanceMutaterAPIWatchContainersSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
	watcher *mocks.MockStringsWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchContainersSuite{})

func (s *InstanceMutaterAPIWatchContainersSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)
	s.watcher = mocks.NewMockStringsWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainers(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectWatchContainersWithNotify(1)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchContainers(context.Background(), params.Entity{Tag: s.machineTag.String()})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0"},
	})
	s.assertStringsStop(c)
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainersWithInvalidTag(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchContainers(context.Background(), params.Entity{Tag: names.NewUserTag("bob@local").String()})
	c.Logf("%#v", err)
	c.Assert(err, gc.ErrorMatches, "\"user-bob\" is not a valid machine tag")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainersWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthMachineAgent()
	s.expectLife(s.machineTag)
	s.expectWatchContainersWithClosedChannel()
	facade := s.facadeAPIForScenario(c)

	result, err := facade.WatchContainers(context.Background(), params.Entity{Tag: s.machineTag.String()})
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial machine containers")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *InstanceMutaterAPIWatchContainersSuite) expectWatchContainersWithNotify(times int) {
	ch := make(chan []string)

	go func() {
		for i := 0; i < times; i++ {
			ch <- []string{fmt.Sprintf("%d", i)}
		}
		close(s.stringsDone)
	}()

	s.state.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchContainers(instance.LXD).Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
	s.resources.EXPECT().Register(s.watcher).Return("1")
}

func (s *InstanceMutaterAPIWatchContainersSuite) expectWatchContainersWithClosedChannel() {
	ch := make(chan []string)
	close(ch)

	s.state.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchContainers(instance.LXD).Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
}
