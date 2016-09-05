// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	mgotxn "gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

var goodPassword = "foo-12345678901234567890"
var alternatePassword = "bar-12345678901234567890"

// preventUnitDestroyRemove sets a non-pending status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *gc.C, u *state.Unit) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.StatusIdle,
		Message: "",
		Since:   &now,
	}
	err := u.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
}

type StateSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StateSuite{})

func (s *StateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *StateSuite) TestIsController(c *gc.C) {
	c.Assert(s.State.IsController(), jc.IsTrue)
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	c.Assert(st2.IsController(), jc.IsFalse)
}

func (s *StateSuite) TestUserModelNameIndex(c *gc.C) {
	index := state.UserModelNameIndex("BoB", "testing")
	c.Assert(index, gc.Equals, "bob:testing")
}

func (s *StateSuite) TestDocID(c *gc.C) {
	id := "wordpress"
	docID := state.DocID(s.State, id)
	c.Assert(docID, gc.Equals, s.State.ModelUUID()+":"+id)

	// Ensure that the prefix isn't added if it's already there.
	docID2 := state.DocID(s.State, docID)
	c.Assert(docID2, gc.Equals, docID)
}

func (s *StateSuite) TestLocalID(c *gc.C) {
	id := s.State.ModelUUID() + ":wordpress"
	localID := state.LocalID(s.State, id)
	c.Assert(localID, gc.Equals, "wordpress")
}

func (s *StateSuite) TestIDHelpersAreReversible(c *gc.C) {
	id := "wordpress"
	docID := state.DocID(s.State, id)
	localID := state.LocalID(s.State, docID)
	c.Assert(localID, gc.Equals, id)
}

func (s *StateSuite) TestStrictLocalID(c *gc.C) {
	id := state.DocID(s.State, "wordpress")
	localID, err := state.StrictLocalID(s.State, id)
	c.Assert(localID, gc.Equals, "wordpress")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestStrictLocalIDWithWrongPrefix(c *gc.C) {
	localID, err := state.StrictLocalID(s.State, "foo:wordpress")
	c.Assert(localID, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `unexpected id: "foo:wordpress"`)
}

func (s *StateSuite) TestStrictLocalIDWithNoPrefix(c *gc.C) {
	localID, err := state.StrictLocalID(s.State, "wordpress")
	c.Assert(localID, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `unexpected id: "wordpress"`)
}

func (s *StateSuite) TestDialAgain(c *gc.C) {
	// Ensure idempotent operations on Dial are working fine.
	for i := 0; i < 2; i++ {
		st, err := state.Open(s.modelTag, s.State.ControllerTag(), statetesting.NewMongoInfo(), mongotest.DialOpts(), nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st.Close(), gc.IsNil)
	}
}

func (s *StateSuite) TestOpenRequiresExtantModelTag(c *gc.C) {
	uuid := utils.MustNewUUID()
	tag := names.NewModelTag(uuid.String())
	st, err := state.Open(tag, s.State.ControllerTag(), statetesting.NewMongoInfo(), mongotest.DialOpts(), nil)
	if !c.Check(st, gc.IsNil) {
		c.Check(st.Close(), jc.ErrorIsNil)
	}
	expect := fmt.Sprintf("cannot read model %s: model not found", uuid)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *StateSuite) TestOpenSetsModelTag(c *gc.C) {
	st, err := state.Open(s.modelTag, s.State.ControllerTag(), statetesting.NewMongoInfo(), mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(st.ModelTag(), gc.Equals, s.modelTag)
}

func (s *StateSuite) TestModelUUID(c *gc.C) {
	c.Assert(s.State.ModelUUID(), gc.Equals, s.modelTag.Id())
}

func (s *StateSuite) TestNoModelDocs(c *gc.C) {
	c.Assert(s.State.EnsureModelRemoved(), gc.ErrorMatches,
		fmt.Sprintf("found documents for model with uuid %s: 1 constraints doc, 2 leases doc, 1 modelusers doc, 1 settings doc, 1 statuses doc", s.State.ModelUUID()))
}

func (s *StateSuite) TestMongoSession(c *gc.C) {
	session := s.State.MongoSession()
	c.Assert(session.Ping(), gc.IsNil)
}

func (s *StateSuite) TestWatch(c *gc.C) {
	// The allWatcher infrastructure is comprehensively tested
	// elsewhere. This just ensures things are hooked up correctly in
	// State.Watch()

	w := s.State.Watch()
	defer w.Stop()
	deltasC := makeMultiwatcherOutput(w)
	s.State.StartSync()

	select {
	case deltas := <-deltasC:
		// The Watch() call results in an empty "change" reflecting
		// the initially empty model.
		c.Assert(deltas, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

	m := s.Factory.MakeMachine(c, nil) // Generate event
	s.State.StartSync()

	select {
	case deltas := <-deltasC:
		c.Assert(deltas, gc.HasLen, 1)
		info := deltas[0].Entity.(*multiwatcher.MachineInfo)
		c.Assert(info.ModelUUID, gc.Equals, s.State.ModelUUID())
		c.Assert(info.Id, gc.Equals, m.Id())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func makeMultiwatcherOutput(w *state.Multiwatcher) chan []multiwatcher.Delta {
	deltasC := make(chan []multiwatcher.Delta)
	go func() {
		for {
			deltas, err := w.Next()
			if err != nil {
				return
			}
			deltasC <- deltas
		}
	}()
	return deltasC
}

func (s *StateSuite) TestWatchAllModels(c *gc.C) {
	// The allModelWatcher infrastructure is comprehensively tested
	// elsewhere. This just ensures things are hooked up correctly in
	// State.WatchAllModels()

	w := s.State.WatchAllModels()
	defer w.Stop()
	deltasC := makeMultiwatcherOutput(w)

	m := s.Factory.MakeMachine(c, nil)

	envSeen := false
	machineSeen := false
	timeout := time.After(testing.LongWait)
	for !envSeen || !machineSeen {
		select {
		case deltas := <-deltasC:
			for _, delta := range deltas {
				switch e := delta.Entity.(type) {
				case *multiwatcher.ModelInfo:
					c.Assert(e.ModelUUID, gc.Equals, s.State.ModelUUID())
					envSeen = true
				case *multiwatcher.MachineInfo:
					c.Assert(e.ModelUUID, gc.Equals, s.State.ModelUUID())
					c.Assert(e.Id, gc.Equals, m.Id())
					machineSeen = true
				}
			}
		case <-timeout:
			c.Fatal("timed out")
		}
	}
	c.Assert(envSeen, jc.IsTrue)
	c.Assert(machineSeen, jc.IsTrue)
}

type MultiEnvStateSuite struct {
	ConnSuite
	OtherState *state.State
}

func (s *MultiEnvStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.OtherState = s.Factory.MakeModel(c, nil)
}

func (s *MultiEnvStateSuite) TearDownTest(c *gc.C) {
	if s.OtherState != nil {
		s.OtherState.Close()
	}
	s.ConnSuite.TearDownTest(c)
}

func (s *MultiEnvStateSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

var _ = gc.Suite(&MultiEnvStateSuite{})

func (s *MultiEnvStateSuite) TestWatchTwoEnvironments(c *gc.C) {
	for i, test := range []struct {
		about        string
		getWatcher   func(*state.State) interface{}
		setUpState   func(*state.State) (assertChanges bool)
		triggerEvent func(*state.State)
	}{
		{
			about: "machines",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchModelMachines()
			},
			triggerEvent: func(st *state.State) {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
			},
		},
		{
			about: "containers",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
				return m.WatchAllContainers()
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				_, err = st.AddMachineInsideMachine(
					state.MachineTemplate{
						Series: "trusty",
						Jobs:   []state.MachineJob{state.JobHostUnits},
					},
					m.Id(),
					instance.KVM,
				)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "lxd only containers",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
				return m.WatchContainers(instance.LXD)
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				_, err = st.AddMachineInsideMachine(
					state.MachineTemplate{
						Series: "trusty",
						Jobs:   []state.MachineJob{state.JobHostUnits},
					},
					m.Id(),
					instance.LXD,
				)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "units",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
				return m.WatchUnits()
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				f := factory.NewFactory(st)
				f.MakeUnit(c, &factory.UnitParams{Machine: m})
			},
		}, {
			about: "applications",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchServices()
			},
			triggerEvent: func(st *state.State) {
				f := factory.NewFactory(st)
				f.MakeApplication(c, nil)
			},
		}, {
			about: "relations",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				wordpress := f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return wordpress.WatchRelations()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st)
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, jc.ErrorIsNil)
				_, err = st.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "open ports",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchOpenedPorts()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st)
				mysql := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql"})
				f.MakeUnit(c, &factory.UnitParams{Application: mysql})
				return false
			},
			triggerEvent: func(st *state.State) {
				u, err := st.Unit("mysql/0")
				c.Assert(err, jc.ErrorIsNil)
				err = u.OpenPorts("TCP", 100, 200)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "cleanups",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchCleanups()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlCharm})

				// add and destroy a relation, so there is something to cleanup.
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, jc.ErrorIsNil)
				r := f.MakeRelation(c, &factory.RelationParams{Endpoints: eps})
				err = r.Destroy()
				c.Assert(err, jc.ErrorIsNil)

				return false
			},
			triggerEvent: func(st *state.State) {
				err := st.Cleanup()
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "reboots",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, &factory.MachineParams{})
				c.Assert(m.Id(), gc.Equals, "0")
				w := m.WatchForRebootEvent()
				return w
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				err = m.SetRebootFlag(true)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "block devices",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, &factory.MachineParams{})
				c.Assert(m.Id(), gc.Equals, "0")
				return st.WatchBlockDevices(m.MachineTag())
			},
			setUpState: func(st *state.State) bool {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
				err = m.SetMachineBlockDevices(sdb)
				c.Assert(err, jc.ErrorIsNil)
				return false
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				sdb := state.BlockDeviceInfo{DeviceName: "sdb", Label: "fatty"}
				err = m.SetMachineBlockDevices(sdb)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "statuses",
			getWatcher: func(st *state.State) interface{} {
				m, err := st.AddMachine("trusty", state.JobHostUnits)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(m.Id(), gc.Equals, "0")
				return m.Watch()
			},
			setUpState: func(st *state.State) bool {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				m.SetProvisioned("inst-id", "fake_nonce", nil)
				return false
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)

				now := time.Now()
				sInfo := status.StatusInfo{
					Status:  status.StatusError,
					Message: "some status",
					Since:   &now,
				}
				err = m.SetStatus(sInfo)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "settings",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchServices()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				svc, err := st.Application("wordpress")
				c.Assert(err, jc.ErrorIsNil)

				err = svc.UpdateConfigSettings(charm.Settings{"blog-title": "awesome"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "action status",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				dummyCharm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
				service := f.MakeApplication(c, &factory.ApplicationParams{Name: "dummy", Charm: dummyCharm})

				unit, err := service.AddUnit()
				c.Assert(err, jc.ErrorIsNil)
				return unit.WatchActionNotifications()
			},
			triggerEvent: func(st *state.State) {
				unit, err := st.Unit("dummy/0")
				c.Assert(err, jc.ErrorIsNil)
				_, err = unit.AddAction("snapshot", nil)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "min units",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchMinUnits()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				_ = f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				wordpress, err := st.Application("wordpress")
				c.Assert(err, jc.ErrorIsNil)
				err = wordpress.SetMinUnits(2)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	} {
		c.Logf("Test %d: %s", i, test.about)
		func() {
			getTestWatcher := func(st *state.State) TestWatcherC {
				var wc interface{}
				switch w := test.getWatcher(st).(type) {
				case statetesting.StringsWatcher:
					wc = statetesting.NewStringsWatcherC(c, st, w)
					swc := wc.(statetesting.StringsWatcherC)
					// consume initial event
					swc.AssertChange()
					swc.AssertNoChange()
				case statetesting.NotifyWatcher:
					wc = statetesting.NewNotifyWatcherC(c, st, w)
					nwc := wc.(statetesting.NotifyWatcherC)
					// consume initial event
					nwc.AssertOneChange()
				default:
					c.Fatalf("unknown watcher type %T", w)
				}
				return TestWatcherC{
					c:       c,
					State:   st,
					Watcher: wc,
				}
			}

			checkIsolationForEnv := func(w1, w2 TestWatcherC) {
				c.Logf("Making changes to model %s", w1.State.ModelUUID())
				// switch on type of watcher here
				if test.setUpState != nil {

					assertChanges := test.setUpState(w1.State)
					if assertChanges {
						// Consume events from setup.
						w1.AssertChanges()
						w1.AssertNoChange()
						w2.AssertNoChange()
					}
				}
				test.triggerEvent(w1.State)
				w1.AssertChanges()
				w1.AssertNoChange()
				w2.AssertNoChange()
			}

			wc1 := getTestWatcher(s.State)
			defer wc1.Stop()
			wc2 := getTestWatcher(s.OtherState)
			defer wc2.Stop()
			wc2.AssertNoChange()
			wc1.AssertNoChange()
			checkIsolationForEnv(wc1, wc2)
			checkIsolationForEnv(wc2, wc1)
		}()
		s.Reset(c)
	}
}

type TestWatcherC struct {
	c       *gc.C
	State   *state.State
	Watcher interface{}
}

func (tw *TestWatcherC) AssertChanges() {
	switch wc := tw.Watcher.(type) {
	case statetesting.StringsWatcherC:
		wc.AssertChanges()
	case statetesting.NotifyWatcherC:
		wc.AssertOneChange()
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (tw *TestWatcherC) AssertNoChange() {
	switch wc := tw.Watcher.(type) {
	case statetesting.StringsWatcherC:
		wc.AssertNoChange()
	case statetesting.NotifyWatcherC:
		wc.AssertNoChange()
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (tw *TestWatcherC) Stop() {
	switch wc := tw.Watcher.(type) {
	case statetesting.StringsWatcherC:
		statetesting.AssertStop(tw.c, wc.Watcher)
	case statetesting.NotifyWatcherC:
		statetesting.AssertStop(tw.c, wc.Watcher)
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (s *StateSuite) TestAddresses(c *gc.C) {
	var err error
	machines := make([]*state.Machine, 4)
	machines[0], err = s.State.AddMachine("quantal", state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machines[1], err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)

	machines[2], err = s.State.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	machines[3], err = s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)

	for i, m := range machines {
		err := m.SetProviderAddresses(network.Address{
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
			Value: fmt.Sprintf("10.0.0.%d", i),
		}, network.Address{
			Type:  network.IPv6Address,
			Scope: network.ScopeCloudLocal,
			Value: "::1",
		}, network.Address{
			Type:  network.IPv4Address,
			Scope: network.ScopeMachineLocal,
			Value: "127.0.0.1",
		}, network.Address{
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
			Value: "5.4.3.2",
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 3)
	c.Assert(addrs, jc.SameContents, []string{
		fmt.Sprintf("10.0.0.0:%d", cfg.StatePort()),
		fmt.Sprintf("10.0.0.2:%d", cfg.StatePort()),
		fmt.Sprintf("10.0.0.3:%d", cfg.StatePort()),
	})

	addrs, err = s.State.APIAddressesFromMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 3)
	c.Assert(addrs, jc.SameContents, []string{
		fmt.Sprintf("10.0.0.0:%d", cfg.APIPort()),
		fmt.Sprintf("10.0.0.2:%d", cfg.APIPort()),
		fmt.Sprintf("10.0.0.3:%d", cfg.APIPort()),
	})
}

func (s *StateSuite) TestPing(c *gc.C) {
	c.Assert(s.State.Ping(), gc.IsNil)
	gitjujutesting.MgoServer.Restart()
	c.Assert(s.State.Ping(), gc.NotNil)
}

func (s *StateSuite) TestIsNotFound(c *gc.C) {
	err1 := fmt.Errorf("unrelated error")
	err2 := errors.NotFoundf("foo")
	c.Assert(err1, gc.Not(jc.Satisfies), errors.IsNotFound)
	c.Assert(err2, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) AssertMachineCount(c *gc.C, expect int) {
	ms, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(ms), gc.Equals, expect)
}

var jobStringTests = []struct {
	job state.MachineJob
	s   string
}{
	{state.JobHostUnits, "JobHostUnits"},
	{state.JobManageModel, "JobManageModel"},
	{0, "<unknown job 0>"},
	{5, "<unknown job 5>"},
}

func (s *StateSuite) TestJobString(c *gc.C) {
	for _, t := range jobStringTests {
		c.Check(t.job.String(), gc.Equals, t.s)
	}
}

func (s *StateSuite) TestAddMachineErrors(c *gc.C) {
	_, err := s.State.AddMachine("")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no series specified")
	_, err = s.State.AddMachine("quantal")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no jobs specified")
	_, err = s.State.AddMachine("quantal", state.JobHostUnits, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: duplicate job: .*")
}

func (s *StateSuite) TestAddMachine(c *gc.C) {
	allJobs := []state.MachineJob{
		state.JobHostUnits,
		state.JobManageModel,
	}
	m0, err := s.State.AddMachine("quantal", allJobs...)
	c.Assert(err, jc.ErrorIsNil)
	check := func(m *state.Machine, id, series string, jobs []state.MachineJob) {
		c.Assert(m.Id(), gc.Equals, id)
		c.Assert(m.Series(), gc.Equals, series)
		c.Assert(m.Jobs(), gc.DeepEquals, jobs)
		s.assertMachineContainers(c, m, nil)
	}
	check(m0, "0", "quantal", allJobs)
	m0, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	check(m0, "0", "quantal", allJobs)

	oneJob := []state.MachineJob{state.JobHostUnits}
	m1, err := s.State.AddMachine("blahblah", oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	check(m1, "1", "blahblah", oneJob)

	m1, err = s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	check(m1, "1", "blahblah", oneJob)

	m, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.HasLen, 2)
	check(m[0], "0", "quantal", allJobs)
	check(m[1], "1", "blahblah", oneJob)

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	_, err = st2.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: controller jobs specified but not allowed")
}

func (s *StateSuite) TestAddMachines(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")
	hc := instance.MustParseHardware("mem=2G")
	machineTemplate := state.MachineTemplate{
		Series:                  "precise",
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
	}
	machines, err := s.State.AddMachines(machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, jc.ErrorIsNil)
	instId, err := m.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(instId), gc.Equals, "inst-id")
	c.Assert(m.CheckProvisioned("nonce"), jc.IsTrue)
	c.Assert(m.Series(), gc.Equals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, cons)
	mhc, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*mhc, gc.DeepEquals, hc)
	instId, err = m.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(instId), gc.Equals, "inst-id")
}

func (s *StateSuite) TestAddMachinesEnvironmentDying(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testenv" is no longer alive`)
}

func (s *StateSuite) TestAddMachinesEnvironmentDyingAfterInitial(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(env.Life(), gc.Equals, state.Alive)
		c.Assert(env.Destroy(), gc.IsNil)
	}).Check()
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testenv" is no longer alive`)
}

func (s *StateSuite) TestAddMachinesEnvironmentMigrating(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testenv" is being migrated`)
}

func (s *StateSuite) TestAddMachineExtraConstraints(c *gc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cpu-cores=4")
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Constraints: extraCons,
		Jobs:        oneJob,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("cpu-cores=4 mem=4G")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *StateSuite) TestAddMachineWithVolumes(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")
	hc := instance.MustParseHardware("mem=2G")

	volume0 := state.VolumeParams{
		Pool: "loop-pool",
		Size: 123,
	}
	volume1 := state.VolumeParams{
		Pool: "", // use default
		Size: 456,
	}
	volumeAttachment0 := state.VolumeAttachmentParams{}
	volumeAttachment1 := state.VolumeAttachmentParams{
		ReadOnly: true,
	}

	machineTemplate := state.MachineTemplate{
		Series:                  "precise",
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
		Volumes: []state.MachineVolumeParams{{
			volume0, volumeAttachment0,
		}, {
			volume1, volumeAttachment1,
		}},
	}
	machines, err := s.State.AddMachines(machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, jc.ErrorIsNil)

	// When adding the machine, the default pool should
	// have been set on the volume params.
	machineTemplate.Volumes[1].Volume.Pool = "loop"

	volumeAttachments, err := s.State.MachineVolumeAttachments(m.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 2)
	if volumeAttachments[0].Volume() == names.NewVolumeTag(m.Id()+"/1") {
		va := volumeAttachments
		va[0], va[1] = va[1], va[0]
	}
	for i, att := range volumeAttachments {
		_, err = att.Info()
		c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
		attachmentParams, ok := att.Params()
		c.Assert(ok, jc.IsTrue)
		c.Check(attachmentParams, gc.Equals, machineTemplate.Volumes[i].Attachment)
		volume, err := s.State.Volume(att.Volume())
		c.Assert(err, jc.ErrorIsNil)
		_, err = volume.Info()
		c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
		volumeParams, ok := volume.Params()
		c.Assert(ok, jc.IsTrue)
		c.Check(volumeParams, gc.Equals, machineTemplate.Volumes[i].Volume)
	}
}

func (s *StateSuite) assertMachineContainers(c *gc.C, m *state.Machine, containers []string) {
	mc, err := m.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mc, gc.DeepEquals, containers)
}

func (s *StateSuite) TestAddContainerToNewMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   oneJob,
	}
	parentTemplate := state.MachineTemplate{
		Series: "raring",
		Jobs:   oneJob,
	}
	m, err := s.State.AddMachineInsideNewMachine(template, parentTemplate, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxd/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)

	m, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, []string{"0/lxd/0"})
	c.Assert(m.Series(), gc.Equals, "raring")

	m, err = s.State.Machine("0/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, nil)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
}

func (s *StateSuite) TestAddContainerToExistingMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	m1, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, jc.ErrorIsNil)

	// Add first container.
	m, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxd/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0"})

	s.assertMachineContainers(c, m0, nil)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0"})
	m, err = s.State.Machine("1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, nil)

	// Add second container.
	m, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxd/1")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0", "1/lxd/1"})
}

func (s *StateSuite) TestAddContainerToMachineWithKnownSupportedContainers(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "0", instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/kvm/0")
	s.assertMachineContainers(c, host, []string{"0/kvm/0"})
}

func (s *StateSuite) TestAddInvalidContainerToMachineWithKnownSupportedContainers(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxd containers")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestAddContainerToMachineSupportingNoContainers(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxd containers")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestInvalidAddMachineParams(c *gc.C) {
	instIdTemplate := state.MachineTemplate{
		Series:     "quantal",
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "i-foo",
	}
	normalTemplate := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(instIdTemplate, "0", instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(instIdTemplate, normalTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(normalTemplate, instIdTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddOneMachine(instIdTemplate)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")

	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series:     "quantal",
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobHostUnits},
		InstanceId: "i-foo",
		Nonce:      "nonce",
	})
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("cannot add a new machine: duplicate job: %s", state.JobHostUnits))

	noSeriesTemplate := state.MachineTemplate{
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobHostUnits},
	}
	_, err = s.State.AddOneMachine(noSeriesTemplate)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")

	_, err = s.State.AddMachineInsideNewMachine(noSeriesTemplate, normalTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")

	_, err = s.State.AddMachineInsideNewMachine(normalTemplate, noSeriesTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")

	_, err = s.State.AddMachineInsideMachine(noSeriesTemplate, "0", instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")
}

func (s *StateSuite) TestAddContainerErrors(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(template, "10", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 10 not found")
	_, err = s.State.AddMachineInsideMachine(template, "10", "")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no container type specified")
}

func (s *StateSuite) TestInjectMachineErrors(c *gc.C) {
	injectMachine := func(series string, instanceId instance.Id, nonce string, jobs ...state.MachineJob) error {
		_, err := s.State.AddOneMachine(state.MachineTemplate{
			Series:     series,
			Jobs:       jobs,
			InstanceId: instanceId,
			Nonce:      nonce,
		})
		return err
	}
	err := injectMachine("", "i-minvalid", agent.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no series specified")
	err = injectMachine("quantal", "", agent.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: cannot specify a nonce without an instance id")
	err = injectMachine("quantal", "i-minvalid", "", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")
	err = injectMachine("quantal", agent.BootstrapNonce, "i-mlazy")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no jobs specified")
}

func (s *StateSuite) TestInjectMachine(c *gc.C) {
	cons := constraints.MustParse("mem=4G")
	arch := "amd64"
	mem := uint64(1024)
	disk := uint64(1024)
	tags := []string{"foo", "bar"}
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits, state.JobManageModel},
		Constraints: cons,
		InstanceId:  "i-mindustrious",
		Nonce:       agent.BootstrapNonce,
		HardwareCharacteristics: instance.HardwareCharacteristics{
			Arch:     &arch,
			Mem:      &mem,
			RootDisk: &disk,
			Tags:     &tags,
		},
	}
	m, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, template.Jobs)
	instanceId, err := m.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, template.InstanceId)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, mcons)
	characteristics, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*characteristics, gc.DeepEquals, template.HardwareCharacteristics)

	// Make sure the bootstrap nonce value is set.
	c.Assert(m.CheckProvisioned(template.Nonce), jc.IsTrue)
}

func (s *StateSuite) TestAddContainerToInjectedMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	template := state.MachineTemplate{
		Series:     "quantal",
		InstanceId: "i-mindustrious",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	}
	m0, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	// Add first container.
	template = state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxd/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxd/0"})

	// Add second container.
	m, err = s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxd/1")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxd/0", "0/lxd/1"})
}

func (s *StateSuite) TestAddMachineCanOnlyAddControllerForMachine0(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobManageModel},
	}
	// Check that we can add the bootstrap machine.
	m, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.WantsVote(), jc.IsTrue)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageModel})

	// Check that the controller information is correct.
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.ModelTag, gc.Equals, s.modelTag)
	c.Assert(info.MachineIds, gc.DeepEquals, []string{"0"})
	c.Assert(info.VotingMachineIds, gc.DeepEquals, []string{"0"})

	const errCannotAdd = "cannot add a new machine: controller jobs specified but not allowed"
	m, err = s.State.AddOneMachine(template)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)

	m, err = s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)

	m, err = s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)
}

func (s *StateSuite) TestReadMachine(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, expectedId)
}

func (s *StateSuite) TestMachineNotFound(c *gc.C) {
	_, err := s.State.Machine("0")
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestMachineIdLessThan(c *gc.C) {
	c.Assert(state.MachineIdLessThan("0", "0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("0", "1"), jc.IsTrue)
	c.Assert(state.MachineIdLessThan("1", "0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("10", "2"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("0", "0/lxd/0"), jc.IsTrue)
	c.Assert(state.MachineIdLessThan("0/lxd/0", "0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("1", "0/lxd/0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("0/lxd/0", "1"), jc.IsTrue)
	c.Assert(state.MachineIdLessThan("0/lxd/0/lxd/1", "0/lxd/0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("0/kvm/0", "0/lxd/0"), jc.IsTrue)
}

func (s *StateSuite) TestAllMachines(c *gc.C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%d", i)), "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetAgentVersion(version.MustParseBinary("7.8.9-quantal-amd64"))
		c.Assert(err, jc.ErrorIsNil)
		err = m.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
	s.AssertMachineCount(c, numInserts)
	ms, _ := s.State.AllMachines()
	for i, m := range ms {
		c.Assert(m.Id(), gc.Equals, strconv.Itoa(i))
		instId, err := m.InstanceId()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(instId), gc.Equals, fmt.Sprintf("foo-%d", i))
		tools, err := m.AgentTools()
		c.Check(err, jc.ErrorIsNil)
		c.Check(tools.Version, gc.DeepEquals, version.MustParseBinary("7.8.9-quantal-amd64"))
		c.Assert(m.Life(), gc.Equals, state.Dying)
	}
}

func (s *StateSuite) TestAllRelations(c *gc.C) {
	const numRelations = 32
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	_, err = mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wordpressCharm := s.AddTestingCharm(c, "wordpress")
	for i := 0; i < numRelations; i++ {
		applicationname := fmt.Sprintf("wordpress%d", i)
		wordpress := s.AddTestingService(c, applicationname, wordpressCharm)
		_, err = wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		eps, err := s.State.InferEndpoints(applicationname, "mysql")
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
	}

	relations, _ := s.State.AllRelations()

	c.Assert(len(relations), gc.Equals, numRelations)
	for i, relation := range relations {
		c.Assert(relation.Id(), gc.Equals, i)
		c.Assert(relation, gc.Matches, fmt.Sprintf("wordpress%d:.+ mysql:.+", i))
	}
}

func (s *StateSuite) TestAddApplication(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "haha/borken", Charm: ch})
	c.Assert(err, gc.ErrorMatches, `cannot add application "haha/borken": invalid name`)
	_, err = s.State.Application("haha/borken")
	c.Assert(err, gc.ErrorMatches, `"haha/borken" is not a valid application name`)

	// set that a nil charm is handled correctly
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "umadbro"})
	c.Assert(err, gc.ErrorMatches, `cannot add application "umadbro": charm is nil`)

	insettings := charm.Settings{"tuning": "optimized"}

	wordpress, err := s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: ch, Settings: insettings})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	outsettings, err := wordpress.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outsettings, gc.DeepEquals, insettings)

	mysql, err := s.State.AddApplication(state.AddApplicationArgs{Name: "mysql", Charm: ch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	ch, _, err = wordpress.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, ch.URL())
	mysql, err = s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")
	ch, _, err = mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, ch.URL())
}

func (s *StateSuite) TestAddServiceEnvironmentDying(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that services cannot be added if the model is initially Dying.
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testenv" is no longer alive`)
}

func (s *StateSuite) TestAddServiceEnvironmentMigrating(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that services cannot be added if the model is initially Dying.
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testenv" is being migrated`)
}

func (s *StateSuite) TestAddServiceEnvironmentDyingAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingService(c, "s0", charm)
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	// Check that services cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(env.Life(), gc.Equals, state.Alive)
		c.Assert(env.Destroy(), gc.IsNil)
	}).Check()
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testenv" is no longer alive`)
}

func (s *StateSuite) TestServiceNotFound(c *gc.C) {
	_, err := s.State.Application("bummer")
	c.Assert(err, gc.ErrorMatches, `application "bummer" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestAddServiceWithDefaultBindings(c *gc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	svc, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := svc.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"server":  "",
		"client":  "",
		"cluster": "",
	})

	// Removing the service also removes its bindings.
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = svc.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	state.AssertEndpointBindingsNotFoundForService(c, svc)
}

func (s *StateSuite) TestAddServiceWithSpecifiedBindings(c *gc.C) {
	// Add extra spaces to use in bindings.
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	// Specify some bindings, but not all when adding the service.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 43)
	svc, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
		EndpointBindings: map[string]string{
			"client":  "client",
			"cluster": "db",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := svc.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"server":  "", // inherited from defaults.
		"client":  "client",
		"cluster": "db",
	})
}

func (s *StateSuite) TestAddServiceWithInvalidBindings(c *gc.C) {
	charm := s.AddMetaCharm(c, "mysql", metaBase, 44)
	// Add extra spaces to use in bindings.
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range []struct {
		about         string
		bindings      map[string]string
		expectedError string
	}{{
		about:         "extra endpoint bound to unknown space",
		bindings:      map[string]string{"extra": "missing"},
		expectedError: `unknown endpoint "extra" not valid`,
	}, {
		about:         "extra endpoint not bound to a space",
		bindings:      map[string]string{"extra": ""},
		expectedError: `unknown endpoint "extra" not valid`,
	}, {
		about:         "two extra endpoints, both bound to known spaces",
		bindings:      map[string]string{"ex1": "db", "ex2": "client"},
		expectedError: `unknown endpoint "ex(1|2)" not valid`,
	}, {
		about:         "empty endpoint bound to unknown space",
		bindings:      map[string]string{"": "anything"},
		expectedError: `unknown endpoint "" not valid`,
	}, {
		about:         "empty endpoint not bound to a space",
		bindings:      map[string]string{"": ""},
		expectedError: `unknown endpoint "" not valid`,
	}, {
		about:         "known endpoint bound to unknown space",
		bindings:      map[string]string{"server": "invalid"},
		expectedError: `unknown space "invalid" not valid`,
	}, {
		about:         "known endpoint bound correctly and an extra endpoint",
		bindings:      map[string]string{"server": "db", "foo": "public"},
		expectedError: `unknown endpoint "foo" not valid`,
	}} {
		c.Logf("test #%d: %s", i, test.about)

		_, err := s.State.AddApplication(state.AddApplicationArgs{
			Name:             "yoursql",
			Charm:            charm,
			EndpointBindings: test.bindings,
		})
		c.Check(err, gc.ErrorMatches, `cannot add application "yoursql": `+test.expectedError)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}
}

func (s *StateSuite) TestAddServiceMachinePlacementInvalidSeries(c *gc.C) {
	m, err := s.State.AddMachine("trusty", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Placement: []*instance.Placement{
			{instance.MachineScope, m.Id()},
		},
	})
	c.Assert(err, gc.ErrorMatches, "cannot add application \"wordpress\": cannot deploy to machine .*: series does not match")
}

func (s *StateSuite) TestAddServiceIncompatibleOSWithSeriesInURL(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: "centos7",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": series "centos7" \(OS \"CentOS"\) not supported by charm, supported series are "quantal"`)
}

func (s *StateSuite) TestAddServiceCompatibleOSWithSeriesInURL(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: charm.URL().Series,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestAddServiceCompatibleOSWithNoExplicitSupportedSeries(c *gc.C) {
	// If a charm doesn't declare any series, we can add it with any series we choose.
	charm := s.AddSeriesCharm(c, "dummy", "")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: "quantal",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestAddServiceOSIncompatibleWithSupportedSeries(c *gc.C) {
	charm := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the suppoted series.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: "centos7",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": series "centos7" \(OS "CentOS"\) not supported by charm, supported series are "precise, trusty"`)
}

func (s *StateSuite) TestAllApplications(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	services, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	services, err = s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 1)

	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "mysql", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	services, err = s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(services, gc.HasLen, 2)

	// Check the returned service, order is defined by sorted keys.
	names := make([]string, len(services))
	for i, svc := range services {
		names[i] = svc.Name()
	}
	sort.Strings(names)
	c.Assert(names[0], gc.Equals, "mysql")
	c.Assert(names[1], gc.Equals, "wordpress")
}

var inferEndpointsTests = []struct {
	summary string
	inputs  [][]string
	eps     []state.Endpoint
	err     string
}{
	{
		summary: "insane args",
		inputs:  [][]string{nil},
		err:     `cannot relate 0 endpoints`,
	}, {
		summary: "insane args",
		inputs:  [][]string{{"blah", "blur", "bleurgh"}},
		err:     `cannot relate 3 endpoints`,
	}, {
		summary: "invalid args",
		inputs: [][]string{
			{"ping:"},
			{":pong"},
			{":"},
		},
		err: `invalid endpoint ".*"`,
	}, {
		summary: "unknown service",
		inputs:  [][]string{{"wooble"}},
		err:     `application "wooble" not found`,
	}, {
		summary: "invalid relations",
		inputs: [][]string{
			{"ms", "ms"},
			{"wp", "wp"},
			{"rk1", "rk1"},
			{"rk1", "rk2"},
		},
		err: `no relations found`,
	}, {
		summary: "container scoped relation not possible when there's no subordinate",
		inputs: [][]string{
			{"lg-p", "wp"},
		},
		err: `no relations found`,
	}, {
		summary: "container scoped relations between 2 subordinates is ok",
		inputs:  [][]string{{"lg:logging-directory", "lg2:logging-client"}},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Name:      "logging-directory",
				Role:      "requirer",
				Interface: "logging",
				Limit:     1,
				Scope:     charm.ScopeContainer,
			}}, {
			ApplicationName: "lg2",
			Relation: charm.Relation{
				Name:      "logging-client",
				Role:      "provider",
				Interface: "logging",
				Limit:     0,
				Scope:     charm.ScopeGlobal,
			}},
		},
	},
	{
		summary: "valid peer relation",
		inputs: [][]string{
			{"rk1"},
			{"rk1:ring"},
		},
		eps: []state.Endpoint{{
			ApplicationName: "rk1",
			Relation: charm.Relation{
				Name:      "ring",
				Interface: "riak",
				Limit:     1,
				Role:      charm.RolePeer,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "ambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms", "wp"},
			{"ms", "wp:db"},
		},
		err: `ambiguous relation: ".*" could refer to "wp:db ms:dev"; "wp:db ms:prod"`,
	}, {
		summary: "unambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms:dev", "wp"},
			{"ms:dev", "wp:db"},
		},
		eps: []state.Endpoint{{
			ApplicationName: "ms",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "dev",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
				Limit:     2,
			},
		}, {
			ApplicationName: "wp",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		}},
	}, {
		summary: "explicit logging relation is preferred over implicit juju-info",
		inputs:  [][]string{{"lg", "wp"}},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-directory",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ApplicationName: "wp",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-dir",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeContainer,
			},
		}},
	}, {
		summary: "implict relations can be chosen explicitly",
		inputs: [][]string{
			{"lg:info", "wp"},
			{"lg", "wp:juju-info"},
			{"lg:info", "wp:juju-info"},
		},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ApplicationName: "wp",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "implicit relations will be chosen if there are no other options",
		inputs:  [][]string{{"lg", "ms"}},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ApplicationName: "ms",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	},
}

func (s *StateSuite) TestInferEndpoints(c *gc.C) {
	s.AddTestingService(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	s.AddTestingService(c, "wp", s.AddTestingCharm(c, "wordpress"))
	loggingCh := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "lg", loggingCh)
	s.AddTestingService(c, "lg2", loggingCh)
	riak := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "rk1", riak)
	s.AddTestingService(c, "rk2", riak)
	s.AddTestingService(c, "lg-p", s.AddTestingCharm(c, "logging-principal"))

	for i, t := range inferEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)
		for j, input := range t.inputs {
			c.Logf("  input %d: %+v", j, input)
			eps, err := s.State.InferEndpoints(input...)
			if t.err == "" {
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(eps, gc.DeepEquals, t.eps)
			} else {
				c.Assert(err, gc.ErrorMatches, t.err)
			}
		}
	}
}

func (s *StateSuite) TestModelConstraints(c *gc.C) {
	// Environ constraints start out empty (for now).
	cons, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	// Environ constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(1024)}
	err = s.State.SetModelConstraints(cons2)
	c.Assert(err, jc.ErrorIsNil)
	cons3, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons3, gc.DeepEquals, cons2)

	// Environ constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(250)}
	err = s.State.SetModelConstraints(cons4)
	c.Assert(err, jc.ErrorIsNil)
	cons5, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons5, gc.DeepEquals, cons4)
}

func (s *StateSuite) TestSetInvalidConstraints(c *gc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.State.SetModelConstraints(cons)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *StateSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("constraints-tester", tw), gc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.State.SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`setting model constraints: unsupported constraints: cpu-power`},
	})
	econs, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(econs, gc.DeepEquals, cons)
}

func (s *StateSuite) TestWatchModelsBulkEvents(c *gc.C) {
	// Alive model...
	alive, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Dying model...
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	// Add a service so Destroy doesn't advance to Dead.
	svc := factory.NewFactory(st1).MakeApplication(c, nil)
	dying, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Add an empty model, destroy and remove it; we should
	// never see it reported.
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	env2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env2.Destroy(), jc.ErrorIsNil)
	err = st2.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// All except the removed env are reported in initial event.
	w := s.State.WatchModels()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent(alive.UUID(), dying.UUID())

	// Progress dying to dead, alive to dying; and see changes reported.
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = st1.ProcessDyingModel()
	c.Assert(err, jc.ErrorIsNil)
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent(alive.UUID(), dying.UUID())
}

func (s *StateSuite) TestWatchModelsLifecycle(c *gc.C) {
	// Initial event reports the controller model.
	w := s.State.WatchModels()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(s.State.ModelUUID())
	wc.AssertNoChange()

	// Add a non-empty model: reported.
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	svc := factory.NewFactory(st1).MakeApplication(c, nil)
	env, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(env.UUID())
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(env.UUID())
	wc.AssertNoChange()

	// Remove the model: reported.
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = st1.ProcessDyingModel()
	c.Assert(err, jc.ErrorIsNil)
	err = st1.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(env.UUID())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchServicesBulkEvents(c *gc.C) {
	// Alive service...
	dummyCharm := s.AddTestingCharm(c, "dummy")
	alive := s.AddTestingService(c, "service0", dummyCharm)

	// Dying service...
	dying := s.AddTestingService(c, "service1", dummyCharm)
	keepDying, err := dying.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Dead service (actually, gone, Dead == removed in this case).
	gone := s.AddTestingService(c, "service2", dummyCharm)
	err = gone.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// All except gone are reported in initial event.
	w := s.State.WatchServices()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported.
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = keepDying.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchServicesLifecycle(c *gc.C) {
	// Initial event is empty when no services.
	w := s.State.WatchServices()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a service: reported.
	service := s.AddTestingService(c, "application", s.AddTestingCharm(c, "dummy"))
	wc.AssertChange("application")
	wc.AssertNoChange()

	// Change the service: not reported.
	keepDying, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()

	// Make it Dead(/removed): reported.
	err = keepDying.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchServicesDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.lifecycleWatcher,
	// which is also used by:
	//     State.WatchModels
	//     Service.WatchUnits
	//     Service.WatchRelations
	//     State.WatchEnviron
	//     Machine.WatchContainers
	testWatcherDiesWhenStateCloses(c, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchServices()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchMachinesBulkEvents(c *gc.C) {
	// Alive machine...
	alive, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Dying machine...
	dying, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = dying.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Dead machine...
	dead, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Gone machine.
	gone, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = gone.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = gone.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// All except gone machine are reported in initial event.
	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Id(), dying.Id(), dead.Id())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = dead.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(alive.Id(), dying.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesLifecycle(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Change the machine: not reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Make it Dead: reported.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Remove it: not reported.
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIncludesOldMachines(c *gc.C) {
	// Older versions of juju do not write the "containertype" field.
	// This has caused machines to not be detected in the initial event.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$unset", bson.D{{"containertype", 1}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(machine.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIgnoresContainers(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	machines, err := s.State.AddMachines(template)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	machine := machines[0]
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Add a container: not reported.
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the container Dying: not reported.
	err = m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the container Dead: not reported.
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchContainerLifecycle(c *gc.C) {
	// Add a host machine.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	otherMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	// Initial event is empty when no containers.
	w := machine.WatchContainers(instance.LXD)
	defer statetesting.AssertStop(c, w)
	wAll := machine.WatchAllContainers()
	defer statetesting.AssertStop(c, wAll)

	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	wcAll := statetesting.NewStringsWatcherC(c, s.State, wAll)
	wcAll.AssertChange()
	wcAll.AssertNoChange()

	// Add a container of the required type: reported.
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Add a container of a different type: not reported.
	m1, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Add a nested container of the right type: not reported.
	mchild, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()

	// Add a container of a different machine: not reported.
	m2, err := s.State.AddMachineInsideMachine(template, otherMachine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	statetesting.AssertStop(c, w)
	wcAll.AssertNoChange()
	statetesting.AssertStop(c, wAll)

	w = machine.WatchContainers(instance.LXD)
	defer statetesting.AssertStop(c, w)
	wc = statetesting.NewStringsWatcherC(c, s.State, w)
	wAll = machine.WatchAllContainers()
	defer statetesting.AssertStop(c, wAll)
	wcAll = statetesting.NewStringsWatcherC(c, s.State, wAll)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/kvm/0", "0/lxd/0")
	wcAll.AssertNoChange()

	// Make the container Dying: cannot because of nested container.
	err = m.Destroy()
	c.Assert(err, gc.ErrorMatches, `machine .* is hosting containers ".*"`)

	err = mchild.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = mchild.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Make the container Dying: reported.
	err = m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Make the other containers Dying: not reported.
	err = m1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = m2.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Make the container Dead: reported.
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Make the other containers Dead: not reported.
	err = m1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m2.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()
}

func (s *StateSuite) TestWatchMachineHardwareCharacteristics(c *gc.C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	w := machine.WatchHardwareCharacteristics()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Provision a machine: reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Alter the machine: not reported.
	vers := version.MustParseBinary("1.2.3-quantal-ppc")
	err = machine.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchControllerInfo(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchControllerInfo()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.ControllerInfo{
		CloudName:        "dummy",
		ModelTag:         s.modelTag,
		MachineIds:       []string{"0"},
		VotingMachineIds: []string{"0"},
	})

	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)

	wc.AssertOneChange()

	info, err = s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.ControllerInfo{
		CloudName:        "dummy",
		ModelTag:         s.modelTag,
		MachineIds:       []string{"0", "1", "2"},
		VotingMachineIds: []string{"0", "1", "2"},
	})
}

func (s *StateSuite) insertFakeModelDocs(c *gc.C, st *state.State) string {
	// insert one doc for each multiEnvCollection
	var ops []mgotxn.Op
	modelUUID := st.ModelUUID()
	for _, collName := range state.MultiEnvCollections() {
		// skip adding constraints, modelUser and settings as they were added when the
		// model was created
		if collName == "constraints" || collName == "modelusers" || collName == "settings" {
			continue
		}
		if state.HasRawAccess(collName) {
			coll, closer := state.GetRawCollection(st, collName)
			defer closer()

			err := coll.Insert(bson.M{
				"_id":        state.DocID(st, "arbitraryid"),
				"model-uuid": modelUUID,
			})
			c.Assert(err, jc.ErrorIsNil)
		} else {
			ops = append(ops, mgotxn.Op{
				C:      collName,
				Id:     state.DocID(st, "arbitraryid"),
				Insert: bson.M{"model-uuid": modelUUID},
			})
		}
	}

	err := state.RunTransaction(st, ops)
	c.Assert(err, jc.ErrorIsNil)

	// test that we can find each doc in state
	for _, collName := range state.MultiEnvCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"model-uuid", st.ModelUUID()}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Not(gc.Equals), 0)
	}

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add a model user whose permissions should get removed
	// when the model is.
	_, err = s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      names.NewUserTag("amelia@external"),
			CreatedBy: s.Owner,
			Access:    description.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	return state.UserModelNameIndex(model.Owner().Canonical(), model.Name())
}

type checkUserModelNameArgs struct {
	st     *state.State
	id     string
	exists bool
}

func (s *StateSuite) checkUserModelNameExists(c *gc.C, args checkUserModelNameArgs) {
	indexColl, closer := state.GetCollection(args.st, "usermodelname")
	defer closer()
	n, err := indexColl.FindId(args.id).Count()
	c.Assert(err, jc.ErrorIsNil)
	if args.exists {
		c.Assert(n, gc.Equals, 1)
	} else {
		c.Assert(n, gc.Equals, 0)
	}
}

func (s *StateSuite) AssertModelDeleted(c *gc.C, st *state.State) {
	// check to see if the model itself is gone
	_, err := st.Model()
	c.Assert(err, gc.ErrorMatches, `model not found`)

	// ensure all docs for all multiEnvCollections are removed
	for _, collName := range state.MultiEnvCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"model-uuid", st.ModelUUID()}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0)
	}

	// ensure user permissions for the model are removed
	permPattern := fmt.Sprintf("^%s#%s#", state.ModelGlobalKey, st.ModelUUID())
	permissions, closer := state.GetCollection(st, "permissions")
	defer closer()
	permCount, err := permissions.Find(bson.M{"_id": bson.M{"$regex": permPattern}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(permCount, gc.Equals, 0)
}

func (s *StateSuite) TestRemoveAllModelDocs(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	userModelKey := s.insertFakeModelDocs(c, st)
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: true})

	err := state.SetModelLifeDead(st, st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
}

func (s *StateSuite) TestRemoveAllModelDocsAliveEnvFails(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := st.RemoveAllModelDocs()
	c.Assert(err, gc.ErrorMatches, "can't remove model: model not dead")
}

func (s *StateSuite) TestRemoveImportingModelDocsFailsActive(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := st.RemoveImportingModelDocs()
	c.Assert(err, gc.ErrorMatches, "can't remove model: model not being imported for migration")
}

func (s *StateSuite) TestRemoveImportingModelDocsFailsExporting(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, gc.ErrorMatches, "can't remove model: model not being imported for migration")
}

func (s *StateSuite) TestRemoveImportingModelDocsImporting(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	userModelKey := s.insertFakeModelDocs(c, st)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsFailsActive(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := st.RemoveExportingModelDocs()
	c.Assert(err, gc.ErrorMatches, "can't remove model: model not being exported for migration")
}

func (s *StateSuite) TestRemoveExportingModelDocsFailsImporting(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, gc.ErrorMatches, "can't remove model: model not being exported for migration")
}

func (s *StateSuite) TestRemoveExportingModelDocsExporting(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	userModelKey := s.insertFakeModelDocs(c, st)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

type attrs map[string]interface{}

func (s *StateSuite) TestWatchForModelConfigChanges(c *gc.C) {
	cur := jujuversion.Current
	err := statetesting.SetAgentVersion(s.State, cur)
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchForModelConfigChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	// Initially we get one change notification
	wc.AssertOneChange()

	// Multiple changes will only result in a single change notification
	newVersion := cur
	newVersion.Minor++
	err = statetesting.SetAgentVersion(s.State, newVersion)
	c.Assert(err, jc.ErrorIsNil)

	newerVersion := newVersion
	newerVersion.Minor++
	err = statetesting.SetAgentVersion(s.State, newerVersion)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Setting it to the same value does not trigger a change notification
	err = statetesting.SetAgentVersion(s.State, newerVersion)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchForModelConfigControllerChanges(c *gc.C) {
	w := s.State.WatchForModelConfigChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
}

func (s *StateSuite) TestAddAndGetEquivalence(c *gc.C) {
	// The equivalence tested here isn't necessarily correct, and
	// comparing private details is discouraged in the project.
	// The implementation might choose to cache information, or
	// to have different logic when adding or removing, and the
	// comparison might fail despite it being correct.
	// That said, we've had bugs with txn-revno being incorrect
	// before, so this testing at least ensures we're conscious
	// about such changes.

	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m2, err := s.State.Machine(m1.Id())
	c.Assert(m1, jc.DeepEquals, m2)

	charm1 := s.AddTestingCharm(c, "wordpress")
	charm2, err := s.State.Charm(charm1.URL())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm1, jc.DeepEquals, charm2)

	wordpress1 := s.AddTestingService(c, "wordpress", charm1)
	wordpress2, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress1, jc.DeepEquals, wordpress2)

	unit1, err := wordpress1.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	unit2, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit1, jc.DeepEquals, unit2)

	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	relation1, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	relation2, err := s.State.EndpointsRelation(eps...)
	c.Assert(relation1, jc.DeepEquals, relation2)
	relation3, err := s.State.Relation(relation1.Id())
	c.Assert(relation1, jc.DeepEquals, relation3)
}

func tryOpenState(modelTag names.ModelTag, controllerTag names.ControllerTag, info *mongo.MongoInfo) error {
	st, err := state.Open(modelTag, controllerTag, info, mongotest.DialOpts(), nil)
	if err == nil {
		err = st.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *gc.C) {
	info := statetesting.NewMongoInfo()
	info.Tag, info.Password = names.NewUserTag("arble"), "bar"
	err := tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = names.NewUserTag("arble"), ""
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = nil, ""
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestOpenBadAddress(c *gc.C) {
	info := statetesting.NewMongoInfo()
	info.Addrs = []string{"0.1.2.3:1234"}
	st, err := state.Open(testing.ModelTag, testing.ControllerTag, info, mongo.DialOpts{
		Timeout: 1 * time.Millisecond,
	}, nil)
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "cannot connect to mongodb: no reachable servers")
}

func (s *StateSuite) TestOpenDelaysRetryBadAddress(c *gc.C) {
	// Default mgo retry delay
	retryDelay := 500 * time.Millisecond
	info := statetesting.NewMongoInfo()
	info.Addrs = []string{"0.1.2.3:1234"}

	t0 := time.Now()
	st, err := state.Open(testing.ModelTag, testing.ControllerTag, info, mongo.DialOpts{
		Timeout: 1 * time.Millisecond,
	}, nil)
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "cannot connect to mongodb: no reachable servers")
	// tryOpenState should have delayed for at least retryDelay
	if t1 := time.Since(t0); t1 < retryDelay {
		c.Errorf("mgo.Dial only paused for %v, expected at least %v", t1, retryDelay)
	}
}

func testSetPassword(c *gc.C, getEntity func() (state.Authenticator, error)) {
	e, err := getEntity()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(e.PasswordValid(goodPassword), jc.IsFalse)
	err = e.SetPassword(goodPassword)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(e.PasswordValid(goodPassword), jc.IsTrue)

	// Check a newly-fetched entity has the same password.
	e2, err := getEntity()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(e2.PasswordValid(goodPassword), jc.IsTrue)

	err = e.SetPassword(alternatePassword)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(e.PasswordValid(goodPassword), jc.IsFalse)
	c.Assert(e.PasswordValid(alternatePassword), jc.IsTrue)

	// Check that refreshing fetches the new password
	err = e2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(e2.PasswordValid(alternatePassword), jc.IsTrue)

	if le, ok := e.(lifer); ok {
		testWhenDying(c, le, noErr, deadErr, func() error {
			return e.SetPassword("arble-farble-dying-yarble")
		})
	}
}

type entity interface {
	state.Entity
	state.Lifer
	state.Authenticator
}

type findEntityTest struct {
	tag names.Tag
	err string
}

var findEntityTests = []findEntityTest{{
	tag: names.NewRelationTag("svc1:rel1 svc2:rel2"),
	err: `relation "svc1:rel1 svc2:rel2" not found`,
}, {
	tag: names.NewModelTag("9f484882-2f18-4fd2-967d-db9663db7bea"),
	err: `model "9f484882-2f18-4fd2-967d-db9663db7bea" not found`,
}, {
	tag: names.NewMachineTag("0"),
}, {
	tag: names.NewApplicationTag("ser-vice2"),
}, {
	tag: names.NewRelationTag("wordpress:db ser-vice2:server"),
}, {
	tag: names.NewUnitTag("ser-vice2/0"),
}, {
	tag: names.NewUserTag("arble"),
}, {
	tag: names.NewActionTag("fedcba98-7654-4321-ba98-76543210beef"),
	err: `action "fedcba98-7654-4321-ba98-76543210beef" not found`,
}, {
	tag: names.NewUserTag("eric"),
}, {
	tag: names.NewUserTag("eric@local"),
}, {
	tag: names.NewUserTag("eric@remote"),
	err: `user "eric@remote" not found`,
}}

var entityTypes = map[string]interface{}{
	names.UserTagKind:        (*state.User)(nil),
	names.ModelTagKind:       (*state.Model)(nil),
	names.ApplicationTagKind: (*state.Application)(nil),
	names.UnitTagKind:        (*state.Unit)(nil),
	names.MachineTagKind:     (*state.Machine)(nil),
	names.RelationTagKind:    (*state.Relation)(nil),
	names.ActionTagKind:      (state.Action)(nil),
}

func (s *StateSuite) TestFindEntity(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "eric"})
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	svc := s.AddTestingService(c, "ser-vice2", s.AddTestingCharm(c, "mysql"))
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeUser(c, &factory.UserParams{Name: "arble"})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "ser-vice2")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db ser-vice2:server")

	// model tag is dynamically generated
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	findEntityTests = append([]findEntityTest{}, findEntityTests...)
	findEntityTests = append(findEntityTests, findEntityTest{
		tag: names.NewModelTag(env.UUID()),
	})

	for i, test := range findEntityTests {
		c.Logf("test %d: %q", i, test.tag)
		e, err := s.State.FindEntity(test.tag)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			kind := test.tag.Kind()
			c.Assert(e, gc.FitsTypeOf, entityTypes[kind])
			if kind == names.ModelTagKind {
				// TODO(axw) 2013-12-04 #1257587
				// We *should* only be able to get the entity with its tag, but
				// for backwards-compatibility we accept any non-UUID tag.
				c.Assert(e.Tag(), gc.Equals, env.Tag())
			} else if kind == names.UserTagKind {
				// Test the fully qualified username rather than the tag structure itself.
				expected := test.tag.(names.UserTag).Canonical()
				c.Assert(e.Tag().(names.UserTag).Canonical(), gc.Equals, expected)
			} else {
				c.Assert(e.Tag(), gc.Equals, test.tag)
			}
		}
	}
}

func (s *StateSuite) TestParseNilTagReturnsAnError(c *gc.C) {
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, nil)
	c.Assert(err, gc.ErrorMatches, "tag is nil")
	c.Assert(coll, gc.Equals, "")
	c.Assert(id, gc.IsNil)
}

func (s *StateSuite) TestParseMachineTag(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, m.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "machines")
	c.Assert(id, gc.Equals, state.DocID(s.State, m.Id()))
}

func (s *StateSuite) TestParseApplicationTag(c *gc.C) {
	svc := s.AddTestingService(c, "ser-vice2", s.AddTestingCharm(c, "dummy"))
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, svc.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "applications")
	c.Assert(id, gc.Equals, state.DocID(s.State, svc.Name()))
}

func (s *StateSuite) TestParseUnitTag(c *gc.C) {
	svc := s.AddTestingService(c, "service2", s.AddTestingCharm(c, "dummy"))
	u, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, u.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "units")
	c.Assert(id, gc.Equals, state.DocID(s.State, u.Name()))
}

func (s *StateSuite) TestParseActionTag(c *gc.C) {
	svc := s.AddTestingService(c, "service2", s.AddTestingCharm(c, "dummy"))
	u, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	f, err := u.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.State.Action(f.Id())
	c.Assert(action.Tag(), gc.Equals, names.NewActionTag(action.Id()))
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, action.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "actions")
	c.Assert(id, gc.Equals, action.Id())
}

func (s *StateSuite) TestParseUserTag(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, user.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "users")
	c.Assert(id, gc.Equals, user.Name())
}

func (s *StateSuite) TestParseModelTag(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, env.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "models")
	c.Assert(id, gc.Equals, env.UUID())
}

func (s *StateSuite) TestWatchCleanups(c *gc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Set up two relations for later use, check no events.
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "varnish", s.AddTestingCharm(c, "varnish"))
	c.Assert(err, jc.ErrorIsNil)
	eps, err = s.State.InferEndpoints("wordpress", "varnish")
	c.Assert(err, jc.ErrorIsNil)
	relV, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy one relation, check one change.
	err = relM.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Handle that cleanup doc and create another, check one change.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	err = relV.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Clean up final doc, check change.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchCleanupsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchCleanups()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchCleanupsBulk(c *gc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Create two peer relations by creating their services.
	riak := s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	_, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	allHooks := s.AddTestingService(c, "all-hooks", s.AddTestingCharm(c, "all-hooks"))
	_, err = allHooks.Endpoint("self")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy them both, check one change.
	err = riak.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = allHooks.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Clean them both up, check one change.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *StateSuite) TestWatchMinUnits(c *gc.C) {
	// Check initial event.
	w := s.State.WatchMinUnits()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Set up services for later use.
	wordpress := s.AddTestingService(c,
		"wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpressName := wordpress.Name()

	// Add service units for later use.
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wordpress1, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// No events should occur.
	wc.AssertNoChange()

	// Add minimum units to a service; a single change should occur.
	err = wordpress.SetMinUnits(2)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Decrease minimum units for a service; expect no changes.
	err = wordpress.SetMinUnits(1)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Increase minimum units for two services; a single change should occur.
	err = mysql.SetMinUnits(1)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.SetMinUnits(3)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(mysql.Name(), wordpressName)
	wc.AssertNoChange()

	// Remove minimum units for a service; expect no changes.
	err = mysql.SetMinUnits(0)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a unit of a service with required minimum units.
	// Also avoid the unit removal. A single change should occur.
	preventUnitDestroyRemove(c, wordpress0)
	err = wordpress0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Two actions: destroy a unit and increase minimum units for a service.
	// A single change should occur, and the application name should appear only
	// one time in the change.
	err = wordpress.SetMinUnits(5)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Destroy a unit of a service not requiring minimum units; expect no changes.
	err = mysql0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a service with required minimum units; expect no changes.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a service not requiring minimum units; expect no changes.
	err = mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchMinUnitsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchMinUnits()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestNestingLevel(c *gc.C) {
	c.Assert(state.NestingLevel("0"), gc.Equals, 0)
	c.Assert(state.NestingLevel("0/lxd/1"), gc.Equals, 1)
	c.Assert(state.NestingLevel("0/lxd/1/kvm/0"), gc.Equals, 2)
}

func (s *StateSuite) TestTopParentId(c *gc.C) {
	c.Assert(state.TopParentId("0"), gc.Equals, "0")
	c.Assert(state.TopParentId("0/lxd/1"), gc.Equals, "0")
	c.Assert(state.TopParentId("0/lxd/1/kvm/2"), gc.Equals, "0")
}

func (s *StateSuite) TestParentId(c *gc.C) {
	c.Assert(state.ParentId("0"), gc.Equals, "")
	c.Assert(state.ParentId("0/lxd/1"), gc.Equals, "0")
	c.Assert(state.ParentId("0/lxd/1/kvm/0"), gc.Equals, "0/lxd/1")
}

func (s *StateSuite) TestContainerTypeFromId(c *gc.C) {
	c.Assert(state.ContainerTypeFromId("0"), gc.Equals, instance.ContainerType(""))
	c.Assert(state.ContainerTypeFromId("0/lxd/1"), gc.Equals, instance.LXD)
	c.Assert(state.ContainerTypeFromId("0/lxd/1/kvm/0"), gc.Equals, instance.KVM)
}

func (s *StateSuite) TestIsUpgradeInProgressError(c *gc.C) {
	c.Assert(state.IsUpgradeInProgressError(errors.New("foo")), jc.IsFalse)
	c.Assert(state.IsUpgradeInProgressError(state.UpgradeInProgressError), jc.IsTrue)
	c.Assert(state.IsUpgradeInProgressError(errors.Trace(state.UpgradeInProgressError)), jc.IsTrue)
}

func (s *StateSuite) TestSetEnvironAgentVersionErrors(c *gc.C) {
	// Get the agent-version set in the model.
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	stringVersion := agentVersion.String()

	// Add 4 machines: one with a different version, one with an
	// empty version, one with the current version, and one with
	// the new version.
	machine0, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine0.SetAgentVersion(version.MustParseBinary("9.9.9-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	machine1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machine2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine2.SetAgentVersion(version.MustParseBinary(stringVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	machine3, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine3.SetAgentVersion(version.MustParseBinary("4.5.6-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// Verify machine0 and machine1 are reported as error.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"))
	expectErr := fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Add a service and 4 units: one with a different version, one
	// with an empty version, one with the current version, and one
	// with the new version.
	service, err := s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	unit0, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit0.SetAgentVersion(version.MustParseBinary("6.6.6-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	unit2, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit2.SetAgentVersion(version.MustParseBinary(stringVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	unit3, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit3.SetAgentVersion(version.MustParseBinary("4.5.6-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// Verify unit0 and unit1 are reported as error, along with the
	// machines from before.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"))
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1, unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Now remove the machines.
	for _, machine := range []*state.Machine{machine0, machine1, machine2} {
		err = machine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = machine.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}

	// Verify only the units are reported as error.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"))
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)
}

func (s *StateSuite) prepareAgentVersionTests(c *gc.C, st *state.State) (*config.Config, string) {
	// Get the agent-version set in the model.
	envConfig, err := st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine and a unit with the current version.
	machine, err := st.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	service, err := st.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetAgentVersion(version.MustParseBinary(currentVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	return envConfig, currentVersion
}

func (s *StateSuite) changeEnviron(c *gc.C, envConfig *config.Config, name string, value interface{}) {
	attrs := envConfig.AllAttrs()
	attrs[name] = value
	c.Assert(s.State.UpdateModelConfig(attrs, nil, nil), gc.IsNil)
}

func assertAgentVersion(c *gc.C, st *state.State, vers string) {
	envConfig, err := st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion.String(), gc.Equals, vers)
}

func (s *StateSuite) TestSetEnvironAgentVersionRetriesOnConfigChange(c *gc.C) {
	envConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change something
	// other than the version, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, envConfig, "default-series", "foo")
	}).Check()

	// Change the agent-version and ensure it has changed.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"))
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6")
}

func (s *StateSuite) TestSetEnvironAgentVersionSucceedsWithSameVersion(c *gc.C) {
	envConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change the version
	// to the new one, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, envConfig, "agent-version", "4.5.6")
	}).Check()

	// Change the agent-version and verify.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"))
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6")
}

func (s *StateSuite) TestSetEnvironAgentVersionOnOtherEnviron(c *gc.C) {
	current := version.MustParseBinary("1.24.7-trusty-amd64")
	s.PatchValue(&jujuversion.Current, current.Number)
	s.PatchValue(&arch.HostArch, func() string { return current.Arch })
	s.PatchValue(&series.HostSeries, func() string { return current.Series })

	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	higher := version.MustParseBinary("1.25.0-trusty-amd64")
	lower := version.MustParseBinary("1.24.6-trusty-amd64")

	// Set other environ version to < server environ version
	err := otherSt.SetModelAgentVersion(lower.Number)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, lower.Number.String())

	// Set other environ version == server environ version
	err = otherSt.SetModelAgentVersion(jujuversion.Current)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, jujuversion.Current.String())

	// Set other environ version to > server environ version
	err = otherSt.SetModelAgentVersion(higher.Number)
	expected := fmt.Sprintf("a hosted model cannot have a higher version than the server model: %s > %s",
		higher.Number,
		jujuversion.Current,
	)
	c.Assert(err, gc.ErrorMatches, expected)
}

func (s *StateSuite) TestSetEnvironAgentVersionExcessiveContention(c *gc.C) {
	envConfig, currentVersion := s.prepareAgentVersionTests(c, s.State)

	// Set a hook to change the config 3 times
	// to test we return ErrExcessiveContention.
	changeFuncs := []func(){
		func() { s.changeEnviron(c, envConfig, "default-series", "1") },
		func() { s.changeEnviron(c, envConfig, "default-series", "2") },
		func() { s.changeEnviron(c, envConfig, "default-series", "3") },
	}
	defer state.SetBeforeHooks(c, s.State, changeFuncs...).Check()
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"))
	c.Assert(errors.Cause(err), gc.Equals, txn.ErrExcessiveContention)
	// Make sure the version remained the same.
	assertAgentVersion(c, s.State, currentVersion)
}

func (s *StateSuite) TestSetModelAgentFailsIfUpgrading(c *gc.C) {
	// Get the agent-version set in the model.
	modelConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	_, err = s.State.EnsureUpgradeInfo(machine.Tag().Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetModelAgentVersion(nextVersion)
	c.Assert(err, jc.Satisfies, state.IsUpgradeInProgressError)
}

func (s *StateSuite) TestSetEnvironAgentFailsReportsCorrectError(c *gc.C) {
	// Ensure that the correct error is reported if an upgrade is
	// progress but that isn't the reason for the
	// SetModelAgentVersion call failing.

	// Get the agent-version set in the model.
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary("9.9.9-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	_, err = s.State.EnsureUpgradeInfo(machine.Tag().Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetModelAgentVersion(nextVersion)
	c.Assert(err, gc.ErrorMatches, "some agents have not upgraded to the current model version.+")
}

type waiter interface {
	Wait() error
}

// testWatcherDiesWhenStateCloses calls the given function to start a watcher,
// closes the state and checks that the watcher dies with the expected error.
// The watcher should already have consumed the first
// event, otherwise the watcher's initialisation logic may
// interact with the closed state, causing it to return an
// unexpected error (often "Closed explictly").
func testWatcherDiesWhenStateCloses(c *gc.C, modelTag names.ModelTag, controllerTag names.ControllerTag, startWatcher func(c *gc.C, st *state.State) waiter) {
	st, err := state.Open(modelTag, controllerTag, statetesting.NewMongoInfo(), mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	watcher := startWatcher(c, st)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)
	done := make(chan error)
	go func() {
		done <- watcher.Wait()
	}()
	select {
	case err := <-done:
		c.Assert(err, gc.ErrorMatches, state.ErrStateClosed.Error())
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher %T did not exit when state closed", watcher)
	}
}

func (s *StateSuite) TestControllerInfo(c *gc.C) {
	ids, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids.CloudName, gc.Equals, "dummy")
	c.Assert(ids.ModelTag, gc.Equals, s.modelTag)
	c.Assert(ids.MachineIds, gc.HasLen, 0)
	c.Assert(ids.VotingMachineIds, gc.HasLen, 0)

	// TODO(rog) more testing here when we can actually add
	// controllers.
}

func (s *StateSuite) TestReopenWithNoMachines(c *gc.C) {
	expected := &state.ControllerInfo{
		CloudName: "dummy",
		ModelTag:  s.modelTag,
	}
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	st, err := state.Open(s.modelTag, s.State.ControllerTag(), statetesting.NewMongoInfo(), mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	info, err = s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *StateSuite) TestEnableHAFailsWithBadCount(c *gc.C) {
	for _, n := range []int{-1, 2, 6} {
		changes, err := s.State.EnableHA(n, constraints.Value{}, "", nil)
		c.Assert(err, gc.ErrorMatches, "number of controllers must be odd and non-negative")
		c.Assert(changes.Added, gc.HasLen, 0)
	}
	_, err := s.State.EnableHA(replicaset.MaxPeers+2, constraints.Value{}, "", nil)
	c.Assert(err, gc.ErrorMatches, `controller count is too large \(allowed \d+\)`)
}

func (s *StateSuite) TestEnableHAAddsNewMachines(c *gc.C) {
	// Don't use agent presence to decide on machine availability.
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	ids := make([]string, 3)
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add a non-controller machine just to make sure.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	cons := constraints.Value{
		Mem: newUint64(100),
	}
	changes, err := s.State.EnableHA(3, cons, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i + 1))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotCons, gc.DeepEquals, cons)
		c.Assert(m.WantsVote(), jc.IsTrue)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func (s *StateSuite) TestEnableHATo(c *gc.C) {
	// Don't use agent presence to decide on machine availability.
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	ids := make([]string, 3)
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add two non-controller machines.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", []string{"1", "2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 0)
	c.Assert(changes.Converted, gc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotCons, gc.DeepEquals, constraints.Value{})
		c.Assert(m.WantsVote(), jc.IsTrue)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func newUint64(i uint64) *uint64 {
	return &i
}

func (s *StateSuite) assertControllerInfo(c *gc.C, machineIds []string, votingMachineIds []string, placement []string) {
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.ModelTag, gc.Equals, s.modelTag)
	c.Assert(info.MachineIds, jc.SameContents, machineIds)
	c.Assert(info.VotingMachineIds, jc.SameContents, votingMachineIds)
	for i, id := range machineIds {
		m, err := s.State.Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		if len(placement) == 0 || i >= len(placement) {
			c.Check(m.Placement(), gc.Equals, "")
		} else {
			c.Check(m.Placement(), gc.Equals, placement[i])
		}
	}
}

func (s *StateSuite) TestEnableHASamePlacementAsNewCount(c *gc.C) {
	placement := []string{"p1", "p2", "p3"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *StateSuite) TestEnableHAMorePlacementThanNewCount(c *gc.C) {
	placement := []string{"p1", "p2", "p3", "p4"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *StateSuite) TestEnableHALessPlacementThanNewCount(c *gc.C) {
	placement := []string{"p1", "p2"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2"})
}

func (s *StateSuite) TestEnableHADemotesUnavailableMachines(c *gc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	c.Assert(changes.Maintained, gc.HasLen, 2)

	// New controller machine "3" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *StateSuite) TestEnableHAPromotesAvailableMachines(c *gc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	c.Assert(changes.Demoted, gc.DeepEquals, []string{"0"})
	c.Assert(changes.Maintained, gc.HasLen, 2)

	// New controller machine "3" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)

	// Mark machine 0 as having a vote, so it doesn't get removed, and make it
	// available once more.
	err = m0.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 0)

	// No change; we've got as many voting machines as we need.
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)

	// Make machine 3 unavailable; machine 0 should be promoted, and two new
	// machines created.
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "3", nil
	})
	changes, err = s.State.EnableHA(5, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	c.Assert(changes.Demoted, gc.DeepEquals, []string{"3"})
	s.assertControllerInfo(c, []string{"0", "1", "2", "3", "4", "5"}, []string{"0", "1", "2", "4", "5"}, nil)
	err = m0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsTrue)
}

func (s *StateSuite) TestEnableHARemovesUnavailableMachines(c *gc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)

	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	s.assertControllerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	// machine 0 does not have a vote, so another call to EnableHA
	// will remove machine 0's JobEnvironManager job.
	changes, err = s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(changes.Removed, gc.HasLen, 1)
	c.Assert(changes.Maintained, gc.HasLen, 3)
	c.Assert(err, jc.ErrorIsNil)
	s.assertControllerInfo(c, []string{"1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.IsManager(), jc.IsFalse)
}

func (s *StateSuite) TestEnableHAMaintainsVoteList(c *gc.C) {
	changes, err := s.State.EnableHA(5, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 5)

	s.assertControllerInfo(c,
		[]string{"0", "1", "2", "3", "4"},
		[]string{"0", "1", "2", "3", "4"}, nil)
	// Mark machine-0 as dead, so we'll want to create another one again
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnableHA(0, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)

	// New controller machine "5" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertControllerInfo(c,
		[]string{"0", "1", "2", "3", "4", "5"},
		[]string{"1", "2", "3", "4", "5"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *StateSuite) TestEnableHADefaultsTo3(c *gc.C) {
	changes, err := s.State.EnableHA(0, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	// Mark machine-0 as dead, so we'll want to create it again
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnableHA(0, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)

	// New controller machine "3" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertControllerInfo(c,
		[]string{"0", "1", "2", "3"},
		[]string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *StateSuite) TestEnableHAConcurrentSame(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
		c.Assert(err, jc.ErrorIsNil)
		// The outer EnableHA call will allocate IDs 0..2,
		// and the inner one 3..5.
		c.Assert(changes.Added, gc.HasLen, 3)
		expected := []string{"3", "4", "5"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.DeepEquals, []string{"0", "1", "2"})
	s.assertControllerInfo(c, []string{"3", "4", "5"}, []string{"3", "4", "5"}, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestEnableHAConcurrentLess(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes.Added, gc.HasLen, 3)
		// The outer EnableHA call will initially allocate IDs 0..4,
		// and the inner one 5..7.
		expected := []string{"5", "6", "7"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnableHA will initially attempt to allocate
	// machines 0..4, and fail due to the concurrent change. It will then
	// allocate machines 8..9 to make up the difference from the concurrent
	// EnableHA call.
	changes, err := s.State.EnableHA(5, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	expected := []string{"5", "6", "7", "8", "9"}
	s.assertControllerInfo(c, expected, expected, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestEnableHAConcurrentMore(c *gc.C) {
	s.PatchValue(state.ControllerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(5, constraints.Value{}, "quantal", nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes.Added, gc.HasLen, 5)
		// The outer EnableHA call will allocate IDs 0..2,
		// and the inner one 3..7.
		expected := []string{"3", "4", "5", "6", "7"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnableHA will initially attempt to allocate
	// machines 0..2, and fail due to the concurrent change. It will then
	// find that the number of voting machines in state is greater than
	// what we're attempting to ensure, and fail.
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, gc.ErrorMatches, "failed to create new controller machines: cannot reduce controller count")
	c.Assert(changes.Added, gc.HasLen, 0)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestStateServingInfo(c *gc.C) {
	info, err := s.State.StateServingInfo()
	c.Assert(err, gc.ErrorMatches, "state serving info not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	data := state.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	err = s.State.SetStateServingInfo(data)
	c.Assert(err, jc.ErrorIsNil)

	info, err = s.State.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, data)
}

var setStateServingInfoWithInvalidInfoTests = []func(info *state.StateServingInfo){
	func(info *state.StateServingInfo) { info.APIPort = 0 },
	func(info *state.StateServingInfo) { info.StatePort = 0 },
	func(info *state.StateServingInfo) { info.Cert = "" },
	func(info *state.StateServingInfo) { info.PrivateKey = "" },
}

func (s *StateSuite) TestSetStateServingInfoWithInvalidInfo(c *gc.C) {
	origData := state.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	for i, test := range setStateServingInfoWithInvalidInfoTests {
		c.Logf("test %d", i)
		data := origData
		test(&data)
		err := s.State.SetStateServingInfo(data)
		c.Assert(err, gc.ErrorMatches, "incomplete state serving info set in state")
	}
}

func (s *StateSuite) TestSetAPIHostPorts(c *gc.C) {
	addrs, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
	}, {
		Address: network.Address{
			Value: "0.4.8.16",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		Port: 2,
	}}, {{
		Address: network.Address{
			Value: "0.6.1.2",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 5,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	newHostPorts = [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv6Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 13,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err = s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
}

func (s *StateSuite) TestSetAPIHostPortsConcurrentSame(c *gc.C) {
	hostPorts := [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.4.8.16",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		Port: 2,
	}}, {{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
	}}}

	// API host ports are concurrently changed to the same
	// desired value; second arrival will fail its assertion,
	// refresh finding nothing to do, and then issue a
	// read-only assertion that suceeds.

	var prevRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts(hostPorts)
		c.Assert(err, jc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, "controllers", "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)
	revno, err := state.TxnRevno(s.State, "controllers", "apiHostPorts")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Equals, prevRevno)
}

func (s *StateSuite) TestSetAPIHostPortsConcurrentDifferent(c *gc.C) {
	hostPorts0 := []network.HostPort{{
		Address: network.Address{
			Value: "0.4.8.16",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		Port: 2,
	}}
	hostPorts1 := []network.HostPort{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
	}}

	// API host ports are concurrently changed to different
	// values; second arrival will fail its assertion, refresh
	// finding and reattempt.

	var prevRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts([][]network.HostPort{hostPorts0})
		c.Assert(err, jc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, "controllers", "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts([][]network.HostPort{hostPorts1})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)
	revno, err := state.TxnRevno(s.State, "controllers", "apiHostPorts")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Not(gc.Equals), prevRevno)

	hostPorts, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostPorts, gc.DeepEquals, [][]network.HostPort{hostPorts1})
}

func (s *StateSuite) TestWatchAPIHostPorts(c *gc.C) {
	w := s.State.WatchAPIHostPorts()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	err := s.State.SetAPIHostPorts([][]network.HostPort{
		network.NewHostPorts(99, "0.1.2.3"),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchMachineAddresses(c *gc.C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	w := machine.WatchAddresses()
	defer w.Stop()
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Change the machine: not reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Set machine addresses: reported.
	err = machine.SetMachineAddresses(network.NewAddress("abc"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set provider addresses eclipsing machine addresses: reported.
	err = machine.SetProviderAddresses(network.NewScopedAddress("abc", network.ScopePublic))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set same machine eclipsed by provider addresses: not reported.
	err = machine.SetMachineAddresses(network.NewScopedAddress("abc", network.ScopeCloudLocal))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Set different machine addresses: reported.
	err = machine.SetMachineAddresses(network.NewAddress("def"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set different provider addresses: reported.
	err = machine.SetMachineAddresses(network.NewScopedAddress("def", network.ScopePublic))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Make it Dying: not reported.
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dead: not reported.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove it: watcher eventually closed and Err
	// returns an IsNotFound error.
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher not closed")
	}
	c.Assert(w.Err(), jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestNowToTheSecond(c *gc.C) {
	t := state.NowToTheSecond()
	rounded := t.Round(time.Second)
	c.Assert(t, gc.DeepEquals, rounded)
}

func (s *StateSuite) TestUnitsForInvalidId(c *gc.C) {
	// Check that an error is returned if an invalid machine id is provided.
	// Success cases are tested as part of TestMachinePrincipalUnits in the
	// MachineSuite.
	units, err := s.State.UnitsFor("invalid-id")
	c.Assert(units, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `"invalid-id" is not a valid machine id`)
}

func (s *StateSuite) TestSetOrGetMongoSpaceNameSets(c *gc.C) {
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.MongoSpaceName, gc.Equals, "")
	c.Assert(info.MongoSpaceState, gc.Equals, state.MongoSpaceUnknown)

	spaceName := network.SpaceName("foo")

	name, err := s.State.SetOrGetMongoSpaceName(spaceName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, spaceName)

	info, err = s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.MongoSpaceName, gc.Equals, string(spaceName))
	c.Assert(info.MongoSpaceState, gc.Equals, state.MongoSpaceValid)
}

func (s *StateSuite) TestSetOrGetMongoSpaceNameDoesNotReplaceValidSpace(c *gc.C) {
	spaceName := network.SpaceName("foo")
	name, err := s.State.SetOrGetMongoSpaceName(spaceName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, spaceName)

	name, err = s.State.SetOrGetMongoSpaceName(network.SpaceName("bar"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, spaceName)

	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.MongoSpaceName, gc.Equals, string(spaceName))
	c.Assert(info.MongoSpaceState, gc.Equals, state.MongoSpaceValid)
}

func (s *StateSuite) TestSetMongoSpaceStateSetsValidStates(c *gc.C) {
	mongoStates := []state.MongoSpaceStates{
		state.MongoSpaceUnknown,
		state.MongoSpaceValid,
		state.MongoSpaceInvalid,
		state.MongoSpaceUnsupported,
	}
	for _, st := range mongoStates {
		err := s.State.SetMongoSpaceState(st)
		c.Assert(err, jc.ErrorIsNil)
		info, err := s.State.ControllerInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.MongoSpaceState, gc.Equals, st)
	}
}

func (s *StateSuite) TestSetMongoSpaceStateErrorOnInvalidStates(c *gc.C) {
	err := s.State.SetMongoSpaceState(state.MongoSpaceStates("bad"))
	c.Assert(err, gc.ErrorMatches, "mongoSpaceState: bad not valid")
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.MongoSpaceState, gc.Equals, state.MongoSpaceUnknown)
}

type SetAdminMongoPasswordSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&SetAdminMongoPasswordSuite{})

func setAdminPassword(c *gc.C, inst *gitjujutesting.MgoInstance, owner names.UserTag, password string) {
	session, err := inst.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()
	err = mongo.SetAdminMongoPassword(session, owner.String(), password)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SetAdminMongoPasswordSuite) TestSetAdminMongoPassword(c *gc.C) {
	inst := &gitjujutesting.MgoInstance{EnableAuth: true}
	err := inst.Start(testing.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.DestroyWithLog()

	// We need to make an admin user before we initialize the state
	// because in Mongo3.2 the localhost exception no longer has
	// permission to create indexes.
	// https://docs.mongodb.com/manual/core/security-users/#localhost-exception
	owner := names.NewLocalUserTag("initialize-admin")
	password := "huggies"
	setAdminPassword(c, inst, owner, password)

	noAuthInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{inst.Addr()},
			CACert: testing.CACert,
		},
	}
	authInfo := &mongo.MongoInfo{
		Info:     noAuthInfo.Info,
		Tag:      owner,
		Password: password,
	}
	cfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	st, err := state.Initialize(state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoInfo:     authInfo,
		MongoDialOpts: mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetAdminMongoPassword("foo")
	c.Assert(err, jc.ErrorIsNil)
	err = st.MongoSession().DB("admin").Login("admin", "foo")
	c.Assert(err, jc.ErrorIsNil)

	err = tryOpenState(st.ModelTag(), st.ControllerTag(), noAuthInfo)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	// note: collections are set up in arbitrary order, proximate cause of
	// failure may differ.
	c.Check(err, gc.ErrorMatches, `[^:]+: unauthorized mongo access: .*`)

	passwordOnlyInfo := *noAuthInfo
	passwordOnlyInfo.Password = "foo"

	// Under mongo 3.2 it's not possible to create collections and
	// indexes with no user - the localhost exception only permits
	// creating users. There were some checks for unsetting the
	// password and then creating the state in an older version of
	// this test, but they couldn't be made to work with 3.2.
	err = tryOpenState(st.ModelTag(), st.ControllerTag(), &passwordOnlyInfo)
	c.Assert(err, jc.ErrorIsNil)
}
