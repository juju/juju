// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// baseSuite is the foundation for test suites in this package.
type BaseSuite struct {
	jujutesting.IsolationSuite

	Changes chan interface{}
	Config  ControllerConfig
	Manager *residentManager
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Changes = make(chan interface{})
	s.Config = ControllerConfig{Changes: s.Changes}
	s.Manager = newResidentManager(s.Changes)
}

func (s *BaseSuite) NewController() (*Controller, error) {
	return newController(s.Config, s.Manager)
}

func (s *BaseSuite) NewResident() *Resident {
	return s.Manager.new()
}

func (s *BaseSuite) AssertResident(c *gc.C, id uint64, expectPresent bool) {
	_, present := s.Manager.residents[id]
	c.Assert(present, gc.Equals, expectPresent)
}

func (s *BaseSuite) AssertNoResidents(c *gc.C) {
	c.Assert(s.Manager.residents, gc.HasLen, 0)
}

func (s *BaseSuite) AssertWorkerResource(c *gc.C, resident *Resident, id uint64, expectPresent bool) {
	_, present := resident.workers[id]
	c.Assert(present, gc.Equals, expectPresent)
}

func (s *BaseSuite) NewHub() *pubsub.SimpleHub {
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	return pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{Logger: logger})
}

func (s *BaseSuite) New(c *gc.C) (*Controller, <-chan interface{}) {
	events := s.CaptureEvents(c)
	controller, err := s.NewController()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })
	return controller, events
}

func (s *BaseSuite) CaptureEvents(c *gc.C) <-chan interface{} {
	events := make(chan interface{})
	s.Config.Notify = func(change interface{}) {
		send := false
		switch change.(type) {
		case ControllerConfigChange,
			ModelChange, RemoveModel,
			ApplicationChange, RemoveApplication,
			CharmChange, RemoveCharm,
			MachineChange, RemoveMachine,
			UnitChange, RemoveUnit,
			RelationChange, RemoveRelation,
			BranchChange, RemoveBranch:
			send = true
		default:
			// no-op
		}
		if send {
			c.Logf("sending %#v", change)
			select {
			case events <- change:
			case <-time.After(coretesting.LongWait):
				c.Fatalf("change not processed by test")
			}
		}
	}
	return events
}

func (s *BaseSuite) ProcessChange(c *gc.C, change interface{}, notify <-chan interface{}) {
	s.SendChange(c, change)

	select {
	case obtained := <-notify:
		c.Check(obtained, jc.DeepEquals, change)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("controller did not handle change")
	}
}

// SendChange writes the input change to the suite's changes channel.
// It cares only the the change was read, not about processing.
func (s *BaseSuite) SendChange(c *gc.C, change interface{}) {
	select {
	case s.Changes <- change:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("controller did not read change")
	}
}

func (s *BaseSuite) NextChange(c *gc.C, changes <-chan interface{}) interface{} {
	var obtained interface{}
	select {
	case obtained = <-changes:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no change")
	}
	return obtained
}

// entitySuite is the base suite for testing cached entities
// (models, applications, machines).
type EntitySuite struct {
	BaseSuite

	Gauges *ControllerGauges
	Hub    *pubsub.SimpleHub
}

func (s *EntitySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Gauges = createControllerGauges()
	s.Hub = s.NewHub()
}

func (s *EntitySuite) NewModel(details ModelChange) *Model {
	m := newModel(modelConfig{
		initializing: func() bool { return false },
		metrics:      s.Gauges,
		hub:          s.Hub,
		chub:         s.NewHub(),
		res:          s.Manager.new(),
	})
	m.setDetails(details)
	return m
}

func (s *EntitySuite) NewApplication(details ApplicationChange) *Application {
	a := newApplication(s.Gauges, s.Hub, s.NewResident())
	a.setDetails(details)
	return a
}

func (s *EntitySuite) NewBranch(details BranchChange) *Branch {
	b := newBranch(s.Gauges, s.Hub, s.NewResident())
	b.setDetails(details)
	return b
}

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/cache")

	// This package only brings in other core packages.
	c.Assert(found, jc.SameContents, []string{
		"core/constraints",
		"core/instance",
		"core/life",
		"core/lxdprofile",
		"core/network",
		"core/permission",
		"core/settings",
		"core/status",
	})
}

// NotifyWatcherC wraps a notify watcher, adding testing convenience methods.
type NotifyWatcherC struct {
	*gc.C
	Watcher NotifyWatcher
}

func NewNotifyWatcherC(c *gc.C, watcher NotifyWatcher) NotifyWatcherC {
	return NotifyWatcherC{
		C:       c,
		Watcher: watcher,
	}
}

// AssertOneChange fails if no change is sent before a long time has passed; or
// if, subsequent to that, any further change is sent before a short time has
// passed.
func (c NotifyWatcherC) AssertOneChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c NotifyWatcherC) AssertNoChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		if ok {
			c.Fatalf("watcher sent unexpected change")
		}
		c.Fatalf("watcher changes channel closed")
	case <-time.After(coretesting.ShortWait):
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes channel is closed.
func (c NotifyWatcherC) AssertStops() {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case _, ok := <-c.Watcher.Changes():
		if ok {
			c.Fatalf("watcher sent unexpected change")
		}
	default:
		c.Fatalf("channel not closed")
	}
}

func NewStringsWatcherC(c *gc.C, watcher StringsWatcher) StringsWatcherC {
	return StringsWatcherC{
		C:       c,
		Watcher: watcher,
	}
}

type StringsWatcherC struct {
	*gc.C
	Watcher StringsWatcher
}

// AssertOneChange fails if no change is sent before a long time has passed; or
// if, subsequent to that, any further change is sent before a short time has
// passed.
func (c StringsWatcherC) AssertOneChange(expected []string) {
	select {
	case obtained, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(obtained, jc.SameContents, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertMaybeCombinedChanges fails if no change is sent before a long time
// has passed; if an empty change is found; if the change isn't part of the
// changes expected.
func (c StringsWatcherC) AssertMaybeCombinedChanges(expected []string) {
	var found bool
	expectedSet := set.NewStrings(expected...)
	timeout := time.After(coretesting.LongWait)

	for {
		select {
		case obtained, ok := <-c.Watcher.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Logf("expected %v; obtained %v", expectedSet.Values(), obtained)

			// Maybe the expected changes came through as 1 change.
			if expectedSet.Size() == len(obtained) {
				c.Assert(obtained, jc.SameContents, expectedSet.Values())
				c.Logf("")
				found = true
				break
			}

			// Remove the obtained results from expected, if nothing is removed
			// from expected, fail here, received bad data.
			leftOver := expectedSet.Difference(set.NewStrings(obtained...))
			if expectedSet.Size() == leftOver.Size() {
				c.Fatalf("obtained %v, not contained in expected %v", obtained, expectedSet.Values())
			}
			expectedSet = leftOver
		case <-timeout:
			c.Fatalf("watcher did not send change")
		}
		if found {
			break
		}
	}
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c StringsWatcherC) AssertNoChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		if ok {
			c.Fatalf("watcher sent unexpected change")
		}
		c.Fatalf("watcher changes channel closed")
	case <-time.After(coretesting.ShortWait):
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes channel is closed.
func (c StringsWatcherC) AssertStops() {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case _, ok := <-c.Watcher.Changes():
		if ok {
			c.Fatalf("watcher sent unexpected change")
		}
	default:
		c.Fatalf("channel not closed")
	}
}
