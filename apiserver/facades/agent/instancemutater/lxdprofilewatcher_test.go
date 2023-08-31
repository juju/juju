// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type lxdProfileWatcherSuite struct {
	state     *mocks.MockInstanceMutaterState
	machine0  *mocks.MockMachine
	unit      *mocks.MockUnit
	principal *mocks.MockUnit
	app       *mocks.MockApplication
	charm     *mocks.MockCharm

	charmsWatcher   *mocks.MockStringsWatcher
	appWatcher      *mocks.MockStringsWatcher
	unitsWatcher    *mocks.MockStringsWatcher
	instanceWatcher *mocks.MockNotifyWatcher

	charmChanges    chan []string
	appChanges      chan []string
	unitChanges     chan []string
	instanceChanges chan struct{}

	wc0 testing.NotifyWatcherC
}

var _ = gc.Suite(&lxdProfileWatcherSuite{})

func (s *lxdProfileWatcherSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = mocks.NewMockInstanceMutaterState(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.principal = mocks.NewMockUnit(ctrl)
	s.app = mocks.NewMockApplication(ctrl)
	s.charm = mocks.NewMockCharm(ctrl)
	s.machine0 = mocks.NewMockMachine(ctrl)

	s.charmsWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.appWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.unitsWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.instanceWatcher = mocks.NewMockNotifyWatcher(ctrl)

	s.charmChanges = make(chan []string, 1)
	s.appChanges = make(chan []string, 1)
	s.unitChanges = make(chan []string, 1)
	s.instanceChanges = make(chan struct{}, 1)

	return ctrl
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherStartStop(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherNoProfile(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioNoProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.setupPrincipalUnit()
	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherProfile(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioNoExistingUnitsWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.setupPrincipalUnit()
	curlStr := "ch:name-me"
	s.unit.EXPECT().CharmURL().Return(&curlStr)
	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherNewCharmRev(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	// Start with a charm having a profile so change the charm's profile
	// from existing to not, should be notified to remove the profile.
	s.updateCharmForMachineLXDProfileWatcher("2", false)
	s.wc0.AssertOneChange()

	// Changing the charm url, and the profile stays empty,
	// should not be notified to remove the profile.
	s.updateCharmForMachineLXDProfileWatcher("3", false)
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherCharmMetadataChange(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	// Start with a charm not having a profile.
	s.updateCharmForMachineLXDProfileWatcher("2", false)
	s.wc0.AssertOneChange()

	// Simulate an asynchronous charm download scenario where the downloaded
	// charm specifies an LXD profile. This should trigger a change.
	s.updateCharmForMachineLXDProfileWatcher("2", true)
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherAddUnit(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioNoExistingUnitsWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	// New unit added to existing machine doesn't have a charm url yet.
	// It may have been added without a machine id either.

	s.state.EXPECT().Unit("bar/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Alive)
	s.unit.EXPECT().PrincipalName().Return("", false)
	s.unit.EXPECT().AssignedMachineId().Return("", errors.NotAssignedf(""))
	s.unitChanges <- []string{"bar/0"}
	s.wc0.AssertNoChange()

	// Add the machine id, this time we should get a notification.
	s.state.EXPECT().Unit("bar/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Alive)
	s.unit.EXPECT().PrincipalName().Return("", false)
	s.unit.EXPECT().AssignedMachineId().Return("0", nil)
	curlStr := "ch:name-me"
	s.unit.EXPECT().CharmURL().Return(&curlStr)
	s.unitChanges <- []string{"bar/0"}
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherAddUnitWrongMachine(c *gc.C) {
	defer s.setup(c).Finish()

	s.unit.EXPECT().ApplicationName().AnyTimes().Return("foo")
	s.unit.EXPECT().Name().AnyTimes().Return("foo/0")
	s.machine0.EXPECT().Units().Return(nil, nil)

	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.state.EXPECT().Unit("foo/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Alive)
	s.unit.EXPECT().AssignedMachineId().Return("1", nil)
	s.unit.EXPECT().PrincipalName().Return("", false)
	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherSubordinateWithProfile(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioNoExistingUnitsWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.assertAddSubordinate()
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) assertAddSubordinate() {
	// Add a new subordinate unit with a profile of a new application.

	s.state.EXPECT().Unit("foo/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Alive)
	s.unit.EXPECT().PrincipalName().Return("principal/0", true)
	s.unit.EXPECT().AssignedMachineId().Return("0", nil)

	curlStr := "ch:name-me"
	s.unit.EXPECT().CharmURL().Return(&curlStr)
	s.unitChanges <- []string{"foo/0"}
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherSubordinateWithProfileUpdateUnit(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.setupScenarioNoExistingUnitsWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.assertAddSubordinate()
	s.wc0.AssertOneChange()

	// Add another subordinate and expect a change.
	another := mocks.NewMockUnit(ctrl)
	another.EXPECT().Name().AnyTimes().Return("foo/1")
	another.EXPECT().ApplicationName().AnyTimes().Return("foo")
	another.EXPECT().Life().Return(state.Alive)
	another.EXPECT().PrincipalName().Return("principal/0", true)
	another.EXPECT().AssignedMachineId().Return("0", nil)
	s.state.EXPECT().Unit("foo/1").Return(another, nil)

	s.unitChanges <- []string{"foo/1"}
	s.wc0.AssertOneChange()

	// A general change for an existing unit should cause no notification.
	another.EXPECT().Life().Return(state.Alive)
	another.EXPECT().PrincipalName().Return("principal/0", true)
	another.EXPECT().AssignedMachineId().Return("0", nil)
	s.state.EXPECT().Unit("foo/1").Return(another, nil)
	s.unitChanges <- []string{"foo/1"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherSubordinateNoProfile(c *gc.C) {
	defer s.setup(c).Finish()

	s.unit.EXPECT().ApplicationName().AnyTimes().Return("foo")
	s.unit.EXPECT().Name().AnyTimes().Return("foo/0")

	curl := charm.MustParseURL("ch:name-me")
	s.state.EXPECT().Charm(curl).Return(s.charm, nil)
	s.machine0.EXPECT().Units().Return(nil, nil)
	s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{})

	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.assertAddSubordinate()
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherRemoveUnitWithProfileTwoUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.setupScenarioWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.state.EXPECT().Unit("foo/0").Return(s.unit, nil)
	s.wc0.AssertNoChange()

	// Add a new unit.
	another := mocks.NewMockUnit(ctrl)
	another.EXPECT().Name().AnyTimes().Return("foo/1")
	another.EXPECT().ApplicationName().AnyTimes().Return("foo")
	another.EXPECT().Life().Return(state.Alive)
	another.EXPECT().PrincipalName().Return("", false)
	another.EXPECT().AssignedMachineId().Return("0", nil)
	s.state.EXPECT().Unit("foo/1").Return(another, nil)

	s.unitChanges <- []string{"foo/1"}
	s.wc0.AssertOneChange()

	// Remove the original unit.
	s.unit.EXPECT().Life().Return(state.Dead)
	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherRemoveOnlyUnit(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.setupScenarioNoExistingUnitsWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.wc0.AssertNoChange()

	s.setupPrincipalUnit()
	curlStr := "ch:name-me"
	s.unit.EXPECT().CharmURL().Return(&curlStr)
	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertOneChange()

	// Remove the original unit.
	s.state.EXPECT().Unit("foo/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Dead)
	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherRemoveUnitWrongMachine(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.unit.EXPECT().ApplicationName().AnyTimes().Return("foo")
	s.unit.EXPECT().Name().AnyTimes().Return("foo/0")
	s.machine0.EXPECT().Units().Return(nil, nil)

	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.wc0.AssertNoChange()

	s.state.EXPECT().Unit("bar/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Dead)
	s.unitChanges <- []string{"bar/0"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherAppChangeCharmURLNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioWithProfile()
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.updateCharmForMachineLXDProfileWatcher("2", false)
	s.wc0.AssertOneChange()

	s.state.EXPECT().Application("foo").Return(s.app, nil)
	curl := "ch:name-me-3"
	s.app.EXPECT().CharmURL().Return(&curl)
	s.state.EXPECT().Charm(charm.MustParseURL(curl)).Return(nil, errors.NotFoundf(""))

	s.appChanges <- []string{"foo"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherUnitChangeAppNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.unit.EXPECT().ApplicationName().AnyTimes().Return("foo")
	s.unit.EXPECT().Name().AnyTimes().Return("foo/0")
	s.machine0.EXPECT().Units().Return(nil, nil)

	s.setupPrincipalUnit()
	s.unit.EXPECT().CharmURL().Return(nil)
	s.unit.EXPECT().Application().Return(nil, errors.NotFoundf(""))

	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherUnitChangeCharmURLNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.unit.EXPECT().ApplicationName().AnyTimes().Return("foo")
	s.unit.EXPECT().Name().AnyTimes().Return("foo/0")
	s.machine0.EXPECT().Units().Return(nil, nil)

	s.setupPrincipalUnit()
	curlStr := "ch:name-me"
	s.unit.EXPECT().CharmURL().Return(&curlStr)
	curl := charm.MustParseURL(curlStr)
	s.state.EXPECT().Charm(curl).Return(nil, errors.NotFoundf(""))

	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.unitChanges <- []string{"foo/0"}
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherMachineProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

	s.setupScenarioWithProfile()
	s.machine0.EXPECT().InstanceId().Return(instance.Id("0"), nil)
	s.state.EXPECT().Machine("0").Return(s.machine0, nil)
	defer workertest.CleanKill(c, s.assertStartLxdProfileWatcher(c))

	s.instanceChanges <- struct{}{}
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) updateCharmForMachineLXDProfileWatcher(rev string, profile bool) {
	curl := "ch:name-me-" + rev
	if profile {
		s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{
			Config: map[string]string{"key1": "value1"},
		})
	} else {
		s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{})
	}
	s.state.EXPECT().Application("foo").Return(s.app, nil)
	s.app.EXPECT().CharmURL().Return(&curl)
	chURL := charm.MustParseURL(curl)
	s.state.EXPECT().Charm(chURL).Return(s.charm, nil)
	s.charmChanges <- []string{curl}
	s.appChanges <- []string{"foo"}
}

func (s *lxdProfileWatcherSuite) setupWatchers(c *gc.C) {
	s.state.EXPECT().WatchCharms().Return(s.charmsWatcher)
	s.state.EXPECT().WatchApplicationCharms().Return(s.appWatcher)
	s.state.EXPECT().WatchUnits().Return(s.unitsWatcher)
	s.machine0.EXPECT().WatchInstanceData().Return(s.instanceWatcher)

	s.charmsWatcher.EXPECT().Changes().AnyTimes().Return(s.charmChanges)
	s.charmsWatcher.EXPECT().Wait().Return(nil)
	s.appWatcher.EXPECT().Changes().AnyTimes().Return(s.appChanges)
	s.appWatcher.EXPECT().Wait().Return(nil)
	s.unitsWatcher.EXPECT().Changes().AnyTimes().Return(s.unitChanges)
	s.unitsWatcher.EXPECT().Wait().Return(nil)
	s.instanceWatcher.EXPECT().Changes().AnyTimes().Return(s.instanceChanges)
	s.instanceWatcher.EXPECT().Wait().Return(nil)
}

func (s *lxdProfileWatcherSuite) assertStartLxdProfileWatcher(c *gc.C) worker.Worker {
	s.setupWatchers(c)

	s.machine0.EXPECT().Id().AnyTimes().Return("0")

	w := instancemutater.NewTestLxdProfileWatcher(c, s.machine0, s.state)
	wc := testing.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	s.wc0 = wc
	return w
}

func (s *lxdProfileWatcherSuite) setupScenarioNoProfile() {
	s.setupScenario(false, false)
}

func (s *lxdProfileWatcherSuite) setupScenarioWithProfile() {
	s.setupScenario(false, true)
}

func (s *lxdProfileWatcherSuite) setupScenarioNoExistingUnitsWithProfile() {
	s.setupScenario(true, true)
}

func (s *lxdProfileWatcherSuite) setupScenario(startEmpty, withProfile bool) {
	s.unit.EXPECT().ApplicationName().AnyTimes().Return("foo")
	s.unit.EXPECT().Name().AnyTimes().Return("foo/0")

	curlStr := "ch:name-me"
	curl := charm.MustParseURL(curlStr)
	s.state.EXPECT().Charm(curl).Return(s.charm, nil)
	if startEmpty {
		s.machine0.EXPECT().Units().Return(nil, nil)
	} else {
		s.machine0.EXPECT().Units().Return([]instancemutater.Unit{s.unit}, nil)
		s.unit.EXPECT().Application().Return(s.app, nil)
		s.app.EXPECT().CharmURL().Return(&curlStr)
	}

	if withProfile {
		s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{
			Config: map[string]string{"key1": "value1"},
		})
	} else {
		s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{})
	}
}

func (s *lxdProfileWatcherSuite) setupPrincipalUnit() {
	s.state.EXPECT().Unit("foo/0").Return(s.unit, nil)
	s.unit.EXPECT().Life().Return(state.Alive)
	s.unit.EXPECT().AssignedMachineId().Return("0", nil)
	s.unit.EXPECT().PrincipalName().Return("", false)
}
