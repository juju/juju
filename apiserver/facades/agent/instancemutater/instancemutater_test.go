// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	coretesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type instanceMutaterAPISuite struct {
	coretesting.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	entity     *mocks.MockEntity
	lifer      *mocks.MockLifer
	state      *mocks.MockInstanceMutaterState
	model      *mocks.MockModelCache
	resources  *facademocks.MockResources

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
	s.model = mocks.NewMockModelCache(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)

	return ctrl
}

func (s *instanceMutaterAPISuite) facadeAPIForScenario(c *gc.C, behaviours ...func()) *instancemutater.InstanceMutaterAPI {
	for _, b := range behaviours {
		b()
	}

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.model, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	return facade
}

func (s *instanceMutaterAPISuite) expectLife(machineTag names.Tag) func() {
	return func() {
		exp := s.authorizer.EXPECT()
		gomock.InOrder(
			exp.AuthController().Return(true),
			exp.AuthMachineAgent().Return(true),
			exp.GetAuthTag().Return(machineTag),
		)
	}
}

func (s *instanceMutaterAPISuite) expectFindEntity(machineTag names.Tag, entity state.Entity) func() {
	return func() {
		s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
	}
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, entityShim{
			Entity: s.entity,
			Lifer:  s.lifer,
		}),
	)

	results, err := facade.Life(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: params.Alive,
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithInvalidType(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
	)

	results, err := facade.Life(params.Entities{
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(machineTag),
		s.expectFindEntity(machineTag, entityShim{
			Entity: s.entity,
			Lifer:  s.lifer,
		}),
	)

	results, err := facade.Life(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0-lxd-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: params.Alive,
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithInvalidParentId(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0/lxd/0")

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(machineTag),
	)

	results, err := facade.Life(params.Entities{
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

func (s *InstanceMutaterAPILifeSuite) expectFindEntity(machineTag names.Tag, entity state.Entity) func() {
	return func() {
		s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
		s.lifer.EXPECT().Life().Return(state.Alive)
	}
}

type entityShim struct {
	state.Entity
	state.Lifer
}

type InstanceMutaterAPICharmProfilingInfoSuite struct {
	instanceMutaterAPISuite

	machine     *mocks.MockModelCacheMachine
	unit        *mocks.MockModelCacheUnit
	application *mocks.MockModelCacheApplication
	charm       *mocks.MockModelCacheCharm
	lxdProfile  *mocks.MockLXDProfile
}

var _ = gc.Suite(&InstanceMutaterAPICharmProfilingInfoSuite{})

func (s *InstanceMutaterAPICharmProfilingInfoSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockModelCacheMachine(ctrl)
	s.unit = mocks.NewMockModelCacheUnit(ctrl)
	s.application = mocks.NewMockModelCacheApplication(ctrl)
	s.charm = mocks.NewMockModelCacheCharm(ctrl)
	s.lxdProfile = mocks.NewMockLXDProfile(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfo(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectMachine(instance.Id("0")),
		s.expectInstanceId(instance.Id("0")),
		s.expectUnits(1),
		s.expectCharmProfiles,
		s.expectProfileExtraction,
		s.expectName,
	)

	results, err := facade.CharmProfilingInfo(params.Entity{Tag: "machine-0"})
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectMachine(instance.Id("0")),
		s.expectInstanceId(instance.Id("0")),
		s.expectUnits(2),
		s.expectCharmProfiles,
		s.expectProfileExtraction,
		s.expectProfileExtractionWithEmpty,
		s.expectName,
	)

	results, err := facade.CharmProfilingInfo(params.Entity{Tag: "machine-0"})
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntityWithNotFoundError,
	)

	results, err := facade.CharmProfilingInfo(params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.ErrorMatches, "not found")
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithMachineNotProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectMachine(instance.Id("0")),
		s.expectInstanceIdNotProvisioned,
	)

	results, err := facade.CharmProfilingInfo(params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.ErrorMatches, "machine-0: attempting to get instanceId: ")
	c.Assert(results.InstanceId, gc.Equals, instance.Id(""))
	c.Assert(results.ModelName, gc.Equals, "")
	c.Assert(results.ProfileChanges, gc.HasLen, 0)
	c.Assert(results.CurrentProfiles, gc.HasLen, 0)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectMachine(id instance.Id) func() {
	return func() {
		s.model.EXPECT().Machine(string(id)).Return(s.machine, nil)
	}
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectInstanceId(id instance.Id) func() {
	return func() {
		s.machine.EXPECT().InstanceId().Return(id, nil)
	}
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectFindEntityWithNotFoundError() {
	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, errors.New("not found"))
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectInstanceIdNotProvisioned() {
	s.machine.EXPECT().InstanceId().Return(instance.Id("0"), params.Error{Code: params.CodeNotProvisioned})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectUnits(times int) func() {
	return func() {
		machineExp := s.machine.EXPECT()
		units := make([]instancemutater.ModelCacheUnit, times)
		for i := 0; i < times; i++ {
			units[i] = s.unit
		}
		machineExp.Units().Return(units, nil)
	}
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectCharmProfiles() {
	machineExp := s.machine.EXPECT()
	machineExp.CharmProfiles().Return([]string{"charm-app-0"})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectProfileExtraction() {
	appExp := s.application.EXPECT()
	charmExp := s.charm.EXPECT()
	modelExp := s.model.EXPECT()
	unitExp := s.unit.EXPECT()

	unitExp.Application().Return("foo")
	modelExp.Application("foo").Return(s.application, nil)
	appExp.CharmURL().Return("cs:app-0")
	modelExp.Charm("cs:app-0").Return(s.charm, nil)
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
	modelExp := s.model.EXPECT()
	unitExp := s.unit.EXPECT()

	unitExp.Application().Return("foo")
	modelExp.Application("foo").Return(s.application, nil)
	appExp.CharmURL().Return("cs:app-0")
	modelExp.Charm("cs:app-0").Return(s.charm, nil)
	charmExp.LXDProfile().Return(lxdprofile.Profile{})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectName() {
	modelExp := s.model.EXPECT()
	modelExp.Name().Return("foo")
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfiles(profiles, nil),
	)

	results, err := facade.SetCharmProfiles(params.SetProfileArgs{
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfiles(profiles, nil),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfiles(profiles, errors.New("Failure")),
	)

	results, err := facade.SetCharmProfiles(params.SetProfileArgs{
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

func (s *InstanceMutaterAPISetCharmProfilesSuite) expectSetProfiles(profiles []string, err error) func() {
	return func() {
		s.machine.EXPECT().SetCharmProfiles(profiles).Return(err)
	}
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetModificationStatus(status.Applied, "applied", nil),
	)

	result, err := facade.SetModificationStatus(params.SetStatus{
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetModificationStatus(status.Applied, "applied", errors.New("failed")),
	)

	result, err := facade.SetModificationStatus(params.SetStatus{
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

func (s *InstanceMutaterAPISetModificationStatusSuite) expectSetModificationStatus(st status.Status, message string, err error) func() {
	return func() {
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

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachines(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectAuthController,
		s.expectWatchMachinesWithNotify(1),
	)

	result, err := facade.WatchMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0"},
	})
	s.assertNotifyStop(c)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachinesWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectAuthController,
		s.expectWatchMachinesWithClosedChannel,
	)

	_, err := facade.WatchMachines()
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial model machines")
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachinesModelCacheError(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectAuthController,
		s.expectWatchMachinesError,
	)

	result, err := facade.WatchMachines()
	c.Assert(err, gc.ErrorMatches, "error from model cache")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesWithNotify(times int) func() {
	return func() {
		ch := make(chan []string)

		go func() {
			for i := 0; i < times; i++ {
				ch <- []string{fmt.Sprintf("%d", i)}
			}
			close(s.notifyDone)
		}()

		s.model.EXPECT().WatchMachines().Return(s.watcher, nil)
		s.watcher.EXPECT().Changes().Return(ch)
		s.resources.EXPECT().Register(s.watcher).Return("1")
	}
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesWithClosedChannel() {
	ch := make(chan []string)
	close(ch)

	s.model.EXPECT().WatchMachines().Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesError() {
	s.model.EXPECT().WatchMachines().Return(s.watcher, errors.New("error from model cache"))
}

type InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockModelCacheMachine
	watcher *mocks.MockNotifyWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite{})

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockModelCacheMachine(ctrl)
	s.watcher = mocks.NewMockNotifyWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeeded(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchLXDProfileVerificationNeededWithNotify(1),
	)

	result, err := facade.WatchLXDProfileVerificationNeeded(params.Entities{
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
	)

	result, err := facade.WatchLXDProfileVerificationNeeded(params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("bob@local").String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: common.ServerError(common.ErrPerm),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeededWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchLXDProfileVerificationNeededWithClosedChannel,
	)

	result, err := facade.WatchLXDProfileVerificationNeeded(params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: common.ServerError(errors.New("cannot obtain initial machine watch application LXD profiles")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) TestWatchLXDProfileVerificationNeededModelCacheError(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchLXDProfileVerificationNeededError,
	)

	result, err := facade.WatchLXDProfileVerificationNeeded(params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: common.ServerError(errors.New("error from model cache")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededWithNotify(times int) func() {
	return func() {
		ch := make(chan struct{})

		go func() {
			for i := 0; i < times; i++ {
				ch <- struct{}{}
			}
			close(s.notifyDone)
		}()

		s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
		s.machine.EXPECT().WatchLXDProfileVerificationNeeded().Return(s.watcher, nil)
		s.watcher.EXPECT().Changes().Return(ch)
		s.resources.EXPECT().Register(s.watcher).Return("1")
	}
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededWithClosedChannel() {
	ch := make(chan struct{})
	close(ch)

	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchLXDProfileVerificationNeeded().Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *InstanceMutaterAPIWatchLXDProfileVerificationNeededSuite) expectWatchLXDProfileVerificationNeededError() {
	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchLXDProfileVerificationNeeded().Return(s.watcher, errors.New("error from model cache"))
}

type InstanceMutaterAPIWatchContainersSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockModelCacheMachine
	watcher *mocks.MockStringsWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchContainersSuite{})

func (s *InstanceMutaterAPIWatchContainersSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockModelCacheMachine(ctrl)
	s.watcher = mocks.NewMockStringsWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainers(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchContainersWithNotify(1),
	)

	result, err := facade.WatchContainers(params.Entity{Tag: s.machineTag.String()})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0"},
	})
	s.assertStringsStop(c)
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainersWithInvalidTag(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
	)

	result, err := facade.WatchContainers(params.Entity{Tag: names.NewUserTag("bob@local").String()})
	c.Logf("%#v", err)
	c.Assert(err, gc.ErrorMatches, "\"user-bob\" is not a valid machine tag")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainersWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchContainersWithClosedChannel,
	)

	result, err := facade.WatchContainers(params.Entity{Tag: s.machineTag.String()})
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial machine containers")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *InstanceMutaterAPIWatchContainersSuite) TestWatchContainersModelCacheError(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchContainersError,
	)

	result, err := facade.WatchContainers(params.Entity{Tag: s.machineTag.String()})
	c.Assert(err, gc.ErrorMatches, "error from model cache")
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{})
}

func (s *InstanceMutaterAPIWatchContainersSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *InstanceMutaterAPIWatchContainersSuite) expectWatchContainersWithNotify(times int) func() {
	return func() {
		ch := make(chan []string)

		go func() {
			for i := 0; i < times; i++ {
				ch <- []string{fmt.Sprintf("%d", i)}
			}
			close(s.stringsDone)
		}()

		s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
		s.machine.EXPECT().WatchContainers().Return(s.watcher, nil)
		s.watcher.EXPECT().Changes().Return(ch)
		s.resources.EXPECT().Register(s.watcher).Return("1")
	}
}

func (s *InstanceMutaterAPIWatchContainersSuite) expectWatchContainersWithClosedChannel() {
	ch := make(chan []string)
	close(ch)

	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchContainers().Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *InstanceMutaterAPIWatchContainersSuite) expectWatchContainersError() {
	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchContainers().Return(s.watcher, errors.New("error from model cache"))
}

type machineEntityShim struct {
	instancemutater.Machine
	state.Entity
	state.Lifer
}
