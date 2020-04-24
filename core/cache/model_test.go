// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
)

type ModelSuite struct {
	cache.EntitySuite
}

var _ = gc.Suite(&ModelSuite{})

func (s *ModelSuite) TestReport(c *gc.C) {
	m := s.NewModel(modelChange)
	c.Assert(m.Report(), jc.DeepEquals, map[string]interface{}{
		"name":              "model-owner/test-model",
		"life":              life.Value("alive"),
		"application-count": 0,
		"charm-count":       0,
		"machine-count":     0,
		"unit-count":        0,
		"relation-count":    0,
		"branch-count":      0,
	})
}

func (s *ModelSuite) TestConfig(c *gc.C) {
	m := s.NewModel(modelChange)
	c.Assert(m.Config(), jc.DeepEquals, map[string]interface{}{
		"key":     "value",
		"another": "foo",
	})
}

func (s *ModelSuite) TestNewModelGeneratesHash(c *gc.C) {
	s.NewModel(modelChange)
	c.Check(testutil.ToFloat64(s.Gauges.ModelHashCacheMiss), gc.Equals, float64(1))
}

func (s *ModelSuite) TestModelConfigIncrementsReadCount(c *gc.C) {
	m := s.NewModel(modelChange)
	c.Check(testutil.ToFloat64(s.Gauges.ModelConfigReads), gc.Equals, float64(0))
	m.Config()
	c.Check(testutil.ToFloat64(s.Gauges.ModelConfigReads), gc.Equals, float64(1))
	m.Config()
	c.Check(testutil.ToFloat64(s.Gauges.ModelConfigReads), gc.Equals, float64(2))
}

// Some of the tested behaviour in the following methods is specific to the
// watcher, but using a cached model avoids the need to put scaffolding code in
// export_test.go to create a watcher in isolation.
func (s *ModelSuite) TestConfigWatcherStops(c *gc.C) {
	m := s.NewModel(modelChange)
	w := m.WatchConfig()
	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	wc.AssertStops()
}

func (s *ModelSuite) TestConfigWatcherChange(c *gc.C) {
	m := s.NewModel(modelChange)
	w := m.WatchConfig()
	defer workertest.CleanKill(c, w)
	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := modelChange
	change.Config = map[string]interface{}{
		"key": "changed",
	}

	m.SetDetails(change)
	wc.AssertOneChange()

	// The hash is generated each time we set different details.
	c.Check(testutil.ToFloat64(s.Gauges.ModelHashCacheMiss), gc.Equals, float64(2))

	// The value is retrieved from the cache when the watcher is created and notified.
	c.Check(testutil.ToFloat64(s.Gauges.ModelHashCacheHit), gc.Equals, float64(2))

	// Setting the same values causes no notification and no cache miss.
	m.SetDetails(change)
	wc.AssertNoChange()
	c.Check(testutil.ToFloat64(s.Gauges.ModelHashCacheMiss), gc.Equals, float64(2))
}

func (s *ModelSuite) TestConfigWatcherOneValue(c *gc.C) {
	m := s.NewModel(modelChange)
	w := m.WatchConfig("key")
	defer workertest.CleanKill(c, w)
	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := modelChange
	change.Config = map[string]interface{}{
		"key":     "changed",
		"another": "foo",
	}

	m.SetDetails(change)
	wc.AssertOneChange()
}

func (s *ModelSuite) TestConfigWatcherOneValueOtherChange(c *gc.C) {
	m := s.NewModel(modelChange)
	w := m.WatchConfig("key")

	// The worker is the first and only resource (1).
	resourceId := uint64(1)
	s.AssertWorkerResource(c, m.Resident, resourceId, true)
	defer func() {
		workertest.CleanKill(c, w)
		s.AssertWorkerResource(c, m.Resident, resourceId, false)
	}()

	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	change := modelChange
	change.Config = map[string]interface{}{
		"key":     "value",
		"another": "changed",
	}

	m.SetDetails(change)
	wc.AssertNoChange()
}

func (s *ModelSuite) TestConfigWatcherSameValuesCacheHit(c *gc.C) {
	m := s.NewModel(modelChange)

	w := m.WatchConfig("key", "another")
	defer workertest.CleanKill(c, w)

	w2 := m.WatchConfig("another", "key")
	defer workertest.CleanKill(c, w2)

	// One cache miss for the "all" hash, and one for the specific fields.
	c.Check(testutil.ToFloat64(s.Gauges.ModelHashCacheMiss), gc.Equals, float64(2))

	// Specific field hash should get a hit despite the field ordering.
	c.Check(testutil.ToFloat64(s.Gauges.ModelHashCacheHit), gc.Equals, float64(1))
}

func (s *ModelSuite) TestApplicationNotFoundError(c *gc.C) {
	m := s.NewModel(modelChange)
	_, err := m.Application("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestApplicationReturnsCopy(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateApplication(appChange, s.Manager)

	a1, err := m.Application(appChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	// Make a change to the map returned in the copy.
	ac := a1.Config()
	ac["mister"] = "squiggle"

	// Get another copy from the model and ensure it is unchanged.
	a2, err := m.Application(appChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a2.Config(), gc.DeepEquals, appChange.Config)
}

func (s *ModelSuite) TestApplications(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateApplication(appChange, s.Manager)

	apps := m.Applications()
	c.Assert(apps, gc.HasLen, 1)
	app := apps[appChange.Name]
	c.Check(app.CharmURL(), gc.Equals, appChange.CharmURL)
	c.Check(app.Config(), gc.DeepEquals, appChange.Config)
}

func (s *ModelSuite) TestCharmNotFoundError(c *gc.C) {
	m := s.NewModel(modelChange)
	_, err := m.Charm("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestCharmReturnsCopy(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateCharm(charmChange, s.Manager)

	ch1, err := m.Charm(charmChange.CharmURL)
	c.Assert(err, jc.ErrorIsNil)

	// Make a change to the map returned in the copy.
	cc := ch1.DefaultConfig()
	cc["mister"] = "squiggle"

	// Get another copy from the model and ensure it is unchanged.
	ch2, err := m.Charm(charmChange.CharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch2.DefaultConfig(), gc.DeepEquals, charmChange.DefaultConfig)
}

func (s *ModelSuite) TestMachineNotFoundError(c *gc.C) {
	m := s.NewModel(modelChange)
	_, err := m.Machine("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestMachineReturnsCopy(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateMachine(machineChange, s.Manager)

	m1, err := m.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)

	// Make a change to the map returned in the copy.
	mc := m1.Config()
	mc["mister"] = "squiggle"

	// Get another copy from the model and ensure it is unchanged.
	m2, err := m.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m2.Config(), gc.DeepEquals, machineChange.Config)
}

func (s *ModelSuite) TestUnitNotFoundError(c *gc.C) {
	m := s.NewModel(modelChange)
	_, err := m.Unit("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestUnitReturnsCopy(c *gc.C) {
	m := s.NewModel(modelChange)

	ch := unitChange
	ch.Ports = []network.Port{{Protocol: "tcp", Number: 54321}}

	m.UpdateUnit(ch, s.Manager)

	u1, err := m.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	// Make a change to the slice returned in the copy.
	u1.Ports()[0] = network.Port{Protocol: "tcp", Number: 65432}

	// Get another copy from the model and ensure it is unchanged.
	u2, err := m.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u2.Ports(), gc.DeepEquals, ch.Ports)
}

func (s *ModelSuite) TestBranchNotFoundError(c *gc.C) {
	m := s.NewModel(modelChange)
	_, err := m.Branch("nope")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *ModelSuite) TestBranchReturnsCopy(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateBranch(branchChange, s.Manager)

	b1, err := m.Branch(branchChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	// Make a change to the map returned in the copy.
	au := b1.AssignedUnits()
	au["banana"] = []string{"banana/1", "banana/2"}

	// Get another copy from the model and ensure it is unchanged.
	b2, err := m.Branch(branchChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b2.AssignedUnits(), gc.DeepEquals, branchChange.AssignedUnits)
}

func (s *ModelSuite) TestRemoveBranchPublishesName(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateBranch(branchChange, s.Manager)

	rcv := make(chan interface{}, 1)
	unsub := s.Hub.Subscribe("model-branch-remove", func(_ string, msg interface{}) { rcv <- msg })
	defer unsub()

	err := m.RemoveBranch(cache.RemoveBranch{
		ModelUUID: branchChange.ModelUUID,
		Id:        branchChange.Id,
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case msg := <-rcv:
		name, ok := msg.(string)
		if !ok {
			c.Fatal("wrong type published; expected string.")
		}
		c.Check(name, gc.Equals, branchChange.Name)

	case <-time.After(testing.LongWait):
		c.Fatal("branch removal message not Received")
	}
}

func (s *ModelSuite) TestWaitForUnitNewChange(c *gc.C) {
	m := s.NewModel(modelChange)
	done := m.WaitForUnit("application-name/0", func(u *cache.Unit) bool {
		return u.Life() == life.Alive
	}, nil)

	m.UpdateUnit(unitChange, s.Manager)

	select {
	case <-done:
		// All good.
	case <-time.After(testing.LongWait):
		c.Errorf("change not noticed")
	}
}

func (s *ModelSuite) TestWaitForUnitExistingValue(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateUnit(unitChange, s.Manager)

	done := m.WaitForUnit("application-name/0", func(u *cache.Unit) bool {
		return u.Life() == life.Alive
	}, nil)

	select {
	case <-done:
		// All good.
	case <-time.After(testing.LongWait):
		c.Errorf("change not noticed")
	}
}

func (s *ModelSuite) TestWaitForUnitCancelClosesChannel(c *gc.C) {
	m := s.NewModel(modelChange)
	cancel := make(chan struct{})
	done := m.WaitForUnit("anything", func(*cache.Unit) bool { return false }, cancel)

	select {
	case <-done:
		c.Errorf("change signalled")
	default:
		// All good.
	}

	close(cancel)

	select {
	case <-done:
		// All good.
	case <-time.After(testing.LongWait):
		c.Errorf("done channel not closed")
	}
}

var modelChange = cache.ModelChange{
	ModelUUID:    "model-uuid",
	Name:         "test-model",
	Life:         life.Alive,
	Owner:        "model-owner",
	IsController: false,
	Config: map[string]interface{}{
		"key":     "value",
		"another": "foo",
	},

	Status: status.StatusInfo{
		Status: status.Active,
	},
	UserPermissions: map[string]permission.Access{
		"model-owner": permission.AdminAccess,
		"read-user":   permission.ReadAccess,
	},
}
