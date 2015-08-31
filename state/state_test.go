// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	mgotxn "gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

var goodPassword = "foo-12345678901234567890"
var alternatePassword = "bar-12345678901234567890"

// preventUnitDestroyRemove sets a non-pending status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *gc.C, u *state.Unit) {
	err := u.SetAgentStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

type StateSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StateSuite{})

func (s *StateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *StateSuite) TestIsStateServer(c *gc.C) {
	c.Assert(s.State.IsStateServer(), jc.IsTrue)
	st2 := s.Factory.MakeEnvironment(c, nil)
	defer st2.Close()
	c.Assert(st2.IsStateServer(), jc.IsFalse)
}

func (s *StateSuite) TestUserEnvNameIndex(c *gc.C) {
	index := state.UserEnvNameIndex("BoB", "testing")
	c.Assert(index, gc.Equals, "bob:testing")
}

func (s *StateSuite) TestDocID(c *gc.C) {
	id := "wordpress"
	docID := state.DocID(s.State, id)
	c.Assert(docID, gc.Equals, s.State.EnvironUUID()+":"+id)

	// Ensure that the prefix isn't added if it's already there.
	docID2 := state.DocID(s.State, docID)
	c.Assert(docID2, gc.Equals, docID)
}

func (s *StateSuite) TestLocalID(c *gc.C) {
	id := s.State.EnvironUUID() + ":wordpress"
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
		st, err := state.Open(s.envTag, statetesting.NewMongoInfo(), statetesting.NewDialOpts(), state.Policy(nil))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st.Close(), gc.IsNil)
	}
}

func (s *StateSuite) TestOpenAcceptsMissingEnvironmentTag(c *gc.C) {
	st, err := state.Open(names.EnvironTag{}, statetesting.NewMongoInfo(), statetesting.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(st.EnvironTag(), gc.Equals, s.envTag)
	c.Check(st.Close(), jc.ErrorIsNil)
}

func (s *StateSuite) TestOpenRequiresExtantEnvironmentTag(c *gc.C) {
	uuid := utils.MustNewUUID()
	tag := names.NewEnvironTag(uuid.String())
	st, err := state.Open(tag, statetesting.NewMongoInfo(), statetesting.NewDialOpts(), state.Policy(nil))
	if !c.Check(st, gc.IsNil) {
		c.Check(st.Close(), jc.ErrorIsNil)
	}
	expect := fmt.Sprintf("cannot read environment %s: environment not found", uuid)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *StateSuite) TestOpenSetsEnvironmentTag(c *gc.C) {
	st, err := state.Open(s.envTag, statetesting.NewMongoInfo(), statetesting.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(st.EnvironTag(), gc.Equals, s.envTag)
}

func (s *StateSuite) TestEnvironUUID(c *gc.C) {
	c.Assert(s.State.EnvironUUID(), gc.Equals, s.envTag.Id())
}

func (s *StateSuite) TestNoEnvDocs(c *gc.C) {
	c.Assert(s.State.EnsureEnvironmentRemoved(), gc.ErrorMatches,
		fmt.Sprintf("found documents for environment with uuid %s: 1 constraints doc, 1 envusers doc, 1 leases doc, 1 settings doc", s.State.EnvironUUID()))
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

	m := s.Factory.MakeMachine(c, nil) // Generate event
	s.State.StartSync()

	select {
	case deltas := <-deltasC:
		c.Assert(deltas, gc.HasLen, 1)
		info := deltas[0].Entity.(*multiwatcher.MachineInfo)
		c.Assert(info.EnvUUID, gc.Equals, s.State.EnvironUUID())
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

func (s *StateSuite) TestWatchAllEnvs(c *gc.C) {
	// The allEnvWatcher infrastructure is comprehensively tested
	// elsewhere. This just ensures things are hooked up correctly in
	// State.WatchAllEnvs()

	w := s.State.WatchAllEnvs()
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
				case *multiwatcher.EnvironmentInfo:
					c.Assert(e.EnvUUID, gc.Equals, s.State.EnvironUUID())
					envSeen = true
				case *multiwatcher.MachineInfo:
					c.Assert(e.EnvUUID, gc.Equals, s.State.EnvironUUID())
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
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.OtherState = s.Factory.MakeEnvironment(c, nil)
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
				return st.WatchEnvironMachines()
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
			about: "LXC only containers",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
				return m.WatchContainers(instance.LXC)
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
					instance.LXC,
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
			about: "services",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchServices()
			},
			triggerEvent: func(st *state.State) {
				f := factory.NewFactory(st)
				f.MakeService(c, nil)
			},
		}, {
			about: "relations",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				wordpress := f.MakeService(c, &factory.ServiceParams{Name: "wordpress", Charm: wordpressCharm})
				return wordpress.WatchRelations()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st)
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeService(c, &factory.ServiceParams{Name: "mysql", Charm: mysqlCharm})
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
				mysql := f.MakeService(c, &factory.ServiceParams{Name: "mysql"})
				f.MakeUnit(c, &factory.UnitParams{Service: mysql})
				return false
			},
			triggerEvent: func(st *state.State) {
				u, err := st.Unit("mysql/0")
				c.Assert(err, jc.ErrorIsNil)
				err = u.OpenPorts("TCP", 100, 200)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "network interfaces",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				m := f.MakeMachine(c, &factory.MachineParams{})
				c.Assert(m.Id(), gc.Equals, "0")

				return m.WatchInterfaces()
			},
			setUpState: func(st *state.State) bool {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)

				_, err = st.AddNetwork(state.NetworkInfo{"net1", "net1", "0.1.2.3/24", 0})
				c.Assert(err, jc.ErrorIsNil)

				_, err = m.AddNetworkInterface(state.NetworkInterfaceInfo{
					MACAddress:    "aa:bb:cc:dd:ee:ff",
					InterfaceName: "eth0",
					NetworkName:   "net1",
					IsVirtual:     false,
				})
				c.Assert(err, jc.ErrorIsNil)
				return true
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				_, err = m.NetworkInterfaces()
				c.Assert(err, jc.ErrorIsNil)

				ifaces, err := m.NetworkInterfaces()
				c.Assert(err, jc.ErrorIsNil)
				err = ifaces[0].Disable()
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
				f.MakeService(c, &factory.ServiceParams{Name: "wordpress", Charm: wordpressCharm})
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeService(c, &factory.ServiceParams{Name: "mysql", Charm: mysqlCharm})

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
				w, err := m.WatchForRebootEvent()
				c.Assert(err, jc.ErrorIsNil)
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

				err = m.SetStatus("error", "some status", nil)
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
				f.MakeService(c, &factory.ServiceParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				svc, err := st.Service("wordpress")
				c.Assert(err, jc.ErrorIsNil)

				err = svc.UpdateConfigSettings(charm.Settings{"blog-title": "awesome"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "action status",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st)
				dummyCharm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
				service := f.MakeService(c, &factory.ServiceParams{Name: "dummy", Charm: dummyCharm})

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
				_ = f.MakeService(c, &factory.ServiceParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				wordpress, err := st.Service("wordpress")
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
				c.Logf("Making changes to environment %s", w1.State.EnvironUUID())
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
	machines[0], err = s.State.AddMachine("quantal", state.JobManageEnviron, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machines[1], err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
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
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 3)
	c.Assert(addrs, jc.SameContents, []string{
		fmt.Sprintf("10.0.0.0:%d", envConfig.StatePort()),
		fmt.Sprintf("10.0.0.2:%d", envConfig.StatePort()),
		fmt.Sprintf("10.0.0.3:%d", envConfig.StatePort()),
	})

	addrs, err = s.State.APIAddressesFromMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 3)
	c.Assert(addrs, jc.SameContents, []string{
		fmt.Sprintf("10.0.0.0:%d", envConfig.APIPort()),
		fmt.Sprintf("10.0.0.2:%d", envConfig.APIPort()),
		fmt.Sprintf("10.0.0.3:%d", envConfig.APIPort()),
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

func (s *StateSuite) dummyCharm(c *gc.C, curlOverride string) (ch charm.Charm, curl *charm.URL, storagePath, bundleSHA256 string) {
	ch = testcharms.Repo.CharmDir("dummy")
	if curlOverride != "" {
		curl = charm.MustParseURL(curlOverride)
	} else {
		curl = charm.MustParseURL(
			fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
		)
	}
	storagePath = "dummy-1"
	bundleSHA256 = "dummy-1-sha256"
	return ch, curl, storagePath, bundleSHA256
}

func (s *StateSuite) TestAddCharm(c *gc.C) {
	// Check that adding charms from scratch works correctly.
	ch, curl, storagePath, bundleSHA256 := s.dummyCharm(c, "")
	dummy, err := s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL().String(), gc.Equals, curl.String())

	doc := state.CharmDoc{}
	err = s.charms.FindId(state.DocID(s.State, curl.String())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, gc.DeepEquals, curl)
}

func (s *StateSuite) TestAddCharmUpdatesPlaceholder(c *gc.C) {
	// Check that adding charms updates any existing placeholder charm
	// with the same URL.
	ch := testcharms.Repo.CharmDir("dummy")

	// Add a placeholder charm.
	curl := charm.MustParseURL("cs:quantal/dummy-1")
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Add a deployed charm.
	storagePath := "dummy-1"
	bundleSHA256 := "dummy-1-sha256"
	dummy, err := s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL().String(), gc.Equals, curl.String())

	// Charm doc has been updated.
	var docs []state.CharmDoc
	err = s.charms.FindId(state.DocID(s.State, curl.String())).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0].URL, gc.DeepEquals, curl)
	c.Assert(docs[0].StoragePath, gc.DeepEquals, storagePath)

	// No more placeholder charm.
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) assertPendingCharmExists(c *gc.C, curl *charm.URL) {
	// Find charm directly and verify only the charm URL and
	// PendingUpload are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl.String())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, gc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, jc.IsTrue)
	c.Assert(doc.Placeholder, jc.IsFalse)
	c.Assert(doc.Meta, gc.IsNil)
	c.Assert(doc.Config, gc.IsNil)
	c.Assert(doc.StoragePath, gc.Equals, "")
	c.Assert(doc.BundleSha256, gc.Equals, "")

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestPrepareLocalCharmUpload(c *gc.C) {
	// First test the sanity checks.
	curl, err := s.State.PrepareLocalCharmUpload(charm.MustParseURL("local:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(curl, gc.IsNil)
	curl, err = s.State.PrepareLocalCharmUpload(charm.MustParseURL("cs:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with local schema, got .*")
	c.Assert(curl, gc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := charm.MustParseURL("local:quantal/missing-123")
	curl, err = s.State.PrepareLocalCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, testCurl)
	s.assertPendingCharmExists(c, curl)

	// Try adding it again with the same revision and ensure it gets bumped.
	curl, err = s.State.PrepareLocalCharmUpload(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 124)

	// Also ensure the revision cannot decrease.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(42))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 125)

	// Check the given revision is respected.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(1234))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 1234)
}

func (s *StateSuite) TestPrepareStoreCharmUpload(c *gc.C) {
	// First test the sanity checks.
	sch, err := s.State.PrepareStoreCharmUpload(charm.MustParseURL("cs:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(sch, gc.IsNil)
	sch, err = s.State.PrepareStoreCharmUpload(charm.MustParseURL("local:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with cs schema, got .*")
	c.Assert(sch, gc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := charm.MustParseURL("cs:quantal/missing-123")
	sch, err = s.State.PrepareStoreCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, testCurl)
	c.Assert(sch.IsUploaded(), jc.IsFalse)

	s.assertPendingCharmExists(c, sch.URL())

	// Try adding it again with the same revision and ensure we get the same document.
	schCopy, err := s.State.PrepareStoreCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch, jc.DeepEquals, schCopy)

	// Now add a charm and try again - we should get the same result
	// as with AddCharm.
	ch, curl, storagePath, bundleSHA256 := s.dummyCharm(c, "cs:precise/dummy-2")
	sch, err = s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)
	schCopy, err = s.State.PrepareStoreCharmUpload(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch, jc.DeepEquals, schCopy)

	// Finally, try poking around the state with a placeholder and
	// bundlesha256 to make sure we do the right thing.
	curl = curl.WithRevision(999)
	first := txn.TestHook{
		Before: func() {
			err := s.State.AddStoreCharmPlaceholder(curl)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: func() {
			err := s.charms.RemoveId(state.DocID(s.State, curl.String()))
			c.Assert(err, jc.ErrorIsNil)
		},
	}
	second := txn.TestHook{
		Before: func() {
			err := s.State.AddStoreCharmPlaceholder(curl)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: func() {
			err := s.charms.UpdateId(state.DocID(s.State, curl.String()), bson.D{{"$set", bson.D{
				{"bundlesha256", "fake"}},
			}})
			c.Assert(err, jc.ErrorIsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, first, second, first).Check()

	_, err = s.State.PrepareStoreCharmUpload(curl)
	cause := errors.Cause(err)
	c.Assert(cause, gc.Equals, txn.ErrExcessiveContention)
}

func (s *StateSuite) TestUpdateUploadedCharm(c *gc.C) {
	ch, curl, storagePath, bundleSHA256 := s.dummyCharm(c, "")
	_, err := s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)

	// Test with already uploaded and a missing charms.
	sch, err := s.State.UpdateUploadedCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("charm %q already uploaded", curl))
	c.Assert(sch, gc.IsNil)
	missingCurl := charm.MustParseURL("local:quantal/missing-1")
	sch, err = s.State.UpdateUploadedCharm(ch, missingCurl, storagePath, "missing")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(sch, gc.IsNil)

	// Test with with an uploaded local charm.
	_, err = s.State.PrepareLocalCharmUpload(missingCurl)
	c.Assert(err, jc.ErrorIsNil)
	sch, err = s.State.UpdateUploadedCharm(ch, missingCurl, storagePath, "missing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, missingCurl)
	c.Assert(sch.Revision(), gc.Equals, missingCurl.Revision)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(sch.Config(), gc.DeepEquals, ch.Config())
	c.Assert(sch.StoragePath(), gc.DeepEquals, storagePath)
	c.Assert(sch.BundleSha256(), gc.Equals, "missing")
}

func (s *StateSuite) TestUpdateUploadedCharmEscapesSpecialCharsInConfig(c *gc.C) {
	// Make sure when we have mongodb special characters like "$" and
	// "." in the name of any charm config option, we do proper
	// escaping before storing them and unescaping after loading. See
	// also http://pad.lv/1308146.

	// Clone the dummy charm and change the config.
	configWithProblematicKeys := []byte(`
options:
  $bad.key: {default: bad, description: bad, type: string}
  not.ok.key: {description: not ok, type: int}
  valid-key: {description: all good, type: boolean}
  still$bad.: {description: not good, type: float}
  $.$: {description: awful, type: string}
  ...: {description: oh boy, type: int}
  just$: {description: no no, type: float}
`[1:])
	chDir := testcharms.Repo.ClonedDirPath(c.MkDir(), "dummy")
	err := utils.AtomicWriteFile(
		filepath.Join(chDir, "config.yaml"),
		configWithProblematicKeys,
		0666,
	)
	c.Assert(err, jc.ErrorIsNil)
	ch, err := charm.ReadCharmDir(chDir)
	c.Assert(err, jc.ErrorIsNil)
	missingCurl := charm.MustParseURL("local:quantal/missing-1")
	storagePath := "dummy-1"

	preparedCurl, err := s.State.PrepareLocalCharmUpload(missingCurl)
	c.Assert(err, jc.ErrorIsNil)
	sch, err := s.State.UpdateUploadedCharm(ch, preparedCurl, storagePath, "missing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, missingCurl)
	c.Assert(sch.Revision(), gc.Equals, missingCurl.Revision)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(sch.Config(), gc.DeepEquals, ch.Config())
	c.Assert(sch.StoragePath(), gc.DeepEquals, storagePath)
	c.Assert(sch.BundleSha256(), gc.Equals, "missing")
}

func (s *StateSuite) assertPlaceholderCharmExists(c *gc.C, curl *charm.URL) {
	// Find charm directly and verify only the charm URL and
	// Placeholder are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl.String())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doc.URL, gc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, jc.IsFalse)
	c.Assert(doc.Placeholder, jc.IsTrue)
	c.Assert(doc.Meta, gc.IsNil)
	c.Assert(doc.Config, gc.IsNil)
	c.Assert(doc.StoragePath, gc.Equals, "")
	c.Assert(doc.BundleSha256, gc.Equals, "")

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestLatestPlaceholderCharm(c *gc.C) {
	// Add a deployed charm
	ch, curl, storagePath, bundleSHA256 := s.dummyCharm(c, "cs:quantal/dummy-1")
	_, err := s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)

	// Deployed charm not found.
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Add a charm reference
	curl2 := charm.MustParseURL("cs:quantal/dummy-2")
	err = s.State.AddStoreCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2)

	// Use a URL with an arbitrary rev to search.
	curl = charm.MustParseURL("cs:quantal/dummy-23")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pending.URL(), gc.DeepEquals, curl2)
	c.Assert(pending.IsPlaceholder(), jc.IsTrue)
	c.Assert(pending.Meta(), gc.IsNil)
	c.Assert(pending.Config(), gc.IsNil)
	c.Assert(pending.StoragePath(), gc.Equals, "")
	c.Assert(pending.BundleSha256(), gc.Equals, "")
}

func (s *StateSuite) TestAddStoreCharmPlaceholderErrors(c *gc.C) {
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, gc.ErrorMatches, "expected charm URL with cs schema, got .*")

	curl = charm.MustParseURL("cs:quantal/dummy")
	err = s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
}

func (s *StateSuite) TestAddStoreCharmPlaceholder(c *gc.C) {
	curl := charm.MustParseURL("cs:quantal/dummy-1")
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl)

	// Add the same one again, should be a no-op
	err = s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl)
}

func (s *StateSuite) assertAddStoreCharmPlaceholder(c *gc.C) (*charm.URL, *charm.URL, *state.Charm) {
	// Add a deployed charm
	ch, curl, storagePath, bundleSHA256 := s.dummyCharm(c, "cs:quantal/dummy-1")
	dummy, err := s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)

	// Add a charm placeholder
	curl2 := charm.MustParseURL("cs:quantal/dummy-2")
	err = s.State.AddStoreCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2)

	// Deployed charm is still there.
	existing, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existing, jc.DeepEquals, dummy)

	return curl, curl2, dummy
}

func (s *StateSuite) TestAddStoreCharmPlaceholderLeavesDeployedCharmsAlone(c *gc.C) {
	s.assertAddStoreCharmPlaceholder(c)
}

func (s *StateSuite) TestAddStoreCharmPlaceholderDeletesOlder(c *gc.C) {
	curl, curlOldRef, dummy := s.assertAddStoreCharmPlaceholder(c)

	// Add a new charm placeholder
	curl3 := charm.MustParseURL("cs:quantal/dummy-3")
	err := s.State.AddStoreCharmPlaceholder(curl3)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl3)

	// Deployed charm is still there.
	existing, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existing, jc.DeepEquals, dummy)

	// Older charm placeholder is gone.
	doc := state.CharmDoc{}
	err = s.charms.FindId(curlOldRef).One(&doc)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *StateSuite) TestAllCharms(c *gc.C) {
	// Add a deployed charm
	ch, curl, storagePath, bundleSHA256 := s.dummyCharm(c, "cs:quantal/dummy-1")
	sch, err := s.State.AddCharm(ch, curl, storagePath, bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)

	// Add a charm reference
	curl2 := charm.MustParseURL("cs:quantal/dummy-2")
	err = s.State.AddStoreCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)

	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charms, gc.HasLen, 2)

	c.Assert(charms[0], gc.DeepEquals, sch)
	c.Assert(charms[1].URL(), gc.DeepEquals, curl2)
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
	{state.JobManageEnviron, "JobManageEnviron"},
	{state.JobManageStateDeprecated, "JobManageState"},
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
		state.JobManageEnviron,
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

	st2 := s.Factory.MakeEnvironment(c, nil)
	defer st2.Close()
	_, err = st2.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: state server jobs specified but not allowed")
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
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the environment is initially Dying.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: environment is no longer alive")
}

func (s *StateSuite) TestAddMachinesEnvironmentDyingAfterInitial(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the environment is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(env.Life(), gc.Equals, state.Alive)
		c.Assert(env.Destroy(), gc.IsNil)
	}).Check()
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: environment is no longer alive")
}

func (s *StateSuite) TestAddMachineExtraConstraints(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=4G"))
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
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)

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
	m, err := s.State.AddMachineInsideNewMachine(template, parentTemplate, instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)

	m, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, []string{"0/lxc/0"})
	c.Assert(m.Series(), gc.Equals, "raring")

	m, err = s.State.Machine("0/lxc/0")
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
	}, "1", instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0"})

	s.assertMachineContainers(c, m0, nil)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0"})
	m, err = s.State.Machine("1/lxc/0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, nil)

	// Add second container.
	m, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxc/1")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0", "1/lxc/1"})
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
	}, "0", instance.LXC)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxc containers")
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
	}, "0", instance.LXC)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxc containers")
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
	_, err := s.State.AddMachineInsideMachine(instIdTemplate, "0", instance.LXC)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(instIdTemplate, normalTemplate, instance.LXC)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(normalTemplate, instIdTemplate, instance.LXC)
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

	_, err = s.State.AddMachineInsideNewMachine(noSeriesTemplate, normalTemplate, instance.LXC)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")

	_, err = s.State.AddMachineInsideNewMachine(normalTemplate, noSeriesTemplate, instance.LXC)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")

	_, err = s.State.AddMachineInsideMachine(noSeriesTemplate, "0", instance.LXC)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no series specified")
}

func (s *StateSuite) TestAddContainerErrors(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(template, "10", instance.LXC)
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
		Jobs:        []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
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
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
	}
	m0, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	// Add first container.
	template = state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineInsideMachine(template, "0", instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxc/0"})

	// Add second container.
	m, err = s.State.AddMachineInsideMachine(template, "0", instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/1")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxc/0", "0/lxc/1"})
}

func (s *StateSuite) TestAddMachineCanOnlyAddStateServerForMachine0(c *gc.C) {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobManageEnviron},
	}
	// Check that we can add the bootstrap machine.
	m, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.WantsVote(), jc.IsTrue)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageEnviron})

	// Check that the state server information is correct.
	info, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.EnvironmentTag, gc.Equals, s.envTag)
	c.Assert(info.MachineIds, gc.DeepEquals, []string{"0"})
	c.Assert(info.VotingMachineIds, gc.DeepEquals, []string{"0"})

	const errCannotAdd = "cannot add a new machine: state server jobs specified but not allowed"
	m, err = s.State.AddOneMachine(template)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)

	m, err = s.State.AddMachineInsideMachine(template, "0", instance.LXC)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)

	m, err = s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
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

func (s *StateSuite) TestReadPreEnvUUIDMachine(c *gc.C) {
	type oldMachineDoc struct {
		Id     string `bson:"_id"`
		Series string
	}

	s.machines.Insert(&oldMachineDoc{"99", "quantal"})

	machine, err := s.State.Machine("99")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, "99")
	c.Assert(machine.Tag(), gc.Equals, names.NewMachineTag("99"))
	c.Assert(machine.Series(), gc.Equals, "quantal") // Sanity check.
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
	c.Assert(state.MachineIdLessThan("0", "0/lxc/0"), jc.IsTrue)
	c.Assert(state.MachineIdLessThan("0/lxc/0", "0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("1", "0/lxc/0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("0/lxc/0", "1"), jc.IsTrue)
	c.Assert(state.MachineIdLessThan("0/lxc/0/lxc/1", "0/lxc/0"), jc.IsFalse)
	c.Assert(state.MachineIdLessThan("0/kvm/0", "0/lxc/0"), jc.IsTrue)
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
		serviceName := fmt.Sprintf("wordpress%d", i)
		wordpress := s.AddTestingService(c, serviceName, wordpressCharm)
		_, err = wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		eps, err := s.State.InferEndpoints(serviceName, "mysql")
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

var addNetworkErrorsTests = []struct {
	args      state.NetworkInfo
	expectErr string
}{{
	state.NetworkInfo{"", "provider-id", "0.3.1.0/24", 0},
	`cannot add network "": name must be not empty`,
}, {
	state.NetworkInfo{"$-invalid-", "provider-id", "0.3.1.0/24", 0},
	`cannot add network "\$-invalid-": invalid name`,
}, {
	state.NetworkInfo{"net2", "", "0.3.1.0/24", 0},
	`cannot add network "net2": provider id must be not empty`,
}, {
	state.NetworkInfo{"net2", "provider-id", "invalid", 0},
	`cannot add network "net2": invalid CIDR address: invalid`,
}, {
	state.NetworkInfo{"net2", "provider-id", "0.3.1.0/24", -1},
	`cannot add network "net2": invalid VLAN tag -1: must be between 0 and 4094`,
}, {
	state.NetworkInfo{"net2", "provider-id", "0.3.1.0/24", 9999},
	`cannot add network "net2": invalid VLAN tag 9999: must be between 0 and 4094`,
}, {
	state.NetworkInfo{"net1", "provider-id", "0.3.1.0/24", 0},
	`cannot add network "net1": network "net1" already exists`,
}, {
	state.NetworkInfo{"net42", "provider-net1", "0.3.1.0/24", 0},
	`cannot add network "net42": network with provider id "provider-net1" already exists`,
}}

func (s *StateSuite) TestAddNetworkErrors(c *gc.C) {
	includeNetworks := []string{"net1", "net2", "net3", "net4"}
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Constraints:       constraints.MustParse("networks=net3,net4,^net5,^net6"),
		RequestedNetworks: includeNetworks[:2], // net1, net2
	})
	c.Assert(err, jc.ErrorIsNil)

	for i, netName := range includeNetworks {
		stateNet, _ := addNetworkAndInterface(
			c, s.State, machine,
			netName, "provider-"+netName, fmt.Sprintf("0.%02d.2.0/24", i), 0, false,
			fmt.Sprintf("aa:%02x:cc:dd:ee:f0", i), fmt.Sprintf("eth%d", i))

		net, err := s.State.Network(netName)
		c.Check(err, jc.ErrorIsNil)
		c.Check(net, gc.DeepEquals, stateNet)
		c.Check(net.Name(), gc.Equals, netName)
		c.Check(string(net.ProviderId()), gc.Equals, "provider-"+netName)
	}
	_, err = s.State.Network("missing")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `network "missing" not found`)

	for i, test := range addNetworkErrorsTests {
		c.Logf("test %d: %#v", i, test.args)
		_, err := s.State.AddNetwork(test.args)
		c.Check(err, gc.ErrorMatches, test.expectErr)
		if strings.Contains(test.expectErr, "already exists") {
			c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
		}
	}
}

func (s *StateSuite) TestAllNetworks(c *gc.C) {
	machine1, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Constraints:       constraints.MustParse("networks=^net3,^net4"),
		RequestedNetworks: []string{"net1", "net2"},
	})
	c.Assert(err, jc.ErrorIsNil)
	machine2, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Constraints:       constraints.MustParse("networks=^net1,^net2"),
		RequestedNetworks: []string{"net3", "net4"},
	})
	c.Assert(err, jc.ErrorIsNil)

	networks := []*state.Network{}
	for i := 0; i < 4; i++ {
		netName := fmt.Sprintf("net%d", i+1)
		cidr := fmt.Sprintf("0.1.%d.0/24", i)
		ifaceName := fmt.Sprintf("eth%d", i%2)
		macAddress := fmt.Sprintf("aa:bb:cc:dd:ee:f%d", i)
		machine := machine1
		if i >= 2 {
			machine = machine2
		}
		network, _ := addNetworkAndInterface(
			c, s.State, machine,
			netName, "provider-"+netName, cidr, i, false,
			macAddress, ifaceName)
		networks = append(networks, network)

		allNetworks, err := s.State.AllNetworks()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(allNetworks, gc.HasLen, len(networks))
		c.Assert(allNetworks, jc.DeepEquals, networks)
	}
}

func (s *StateSuite) TestAddService(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("haha/borken", s.Owner.String(), charm, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add service "haha/borken": invalid name`)
	_, err = s.State.Service("haha/borken")
	c.Assert(err, gc.ErrorMatches, `"haha/borken" is not a valid service name`)

	// set that a nil charm is handled correctly
	_, err = s.State.AddService("umadbro", s.Owner.String(), nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add service "umadbro": charm is nil`)

	wordpress, err := s.State.AddService("wordpress", s.Owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	mysql, err := s.State.AddService("mysql", s.Owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.State.Service("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	ch, _, err := wordpress.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
	mysql, err = s.State.Service("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")
	ch, _, err = mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
}

func (s *StateSuite) TestAddServiceEnvironmentDying(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingService(c, "s0", charm)
	// Check that services cannot be added if the environment is initially Dying.
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddService("s1", s.Owner.String(), charm, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add service "s1": environment is no longer alive`)
}

func (s *StateSuite) TestAddServiceEnvironmentDyingAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingService(c, "s0", charm)
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	// Check that services cannot be added if the environment is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(env.Life(), gc.Equals, state.Alive)
		c.Assert(env.Destroy(), gc.IsNil)
	}).Check()
	_, err = s.State.AddService("s1", s.Owner.String(), charm, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add service "s1": environment is no longer alive`)
}

func (s *StateSuite) TestServiceNotFound(c *gc.C) {
	_, err := s.State.Service("bummer")
	c.Assert(err, gc.ErrorMatches, `service "bummer" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestAddServiceNoTag(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("wordpress", "admin", charm, nil, nil)
	c.Assert(err, gc.ErrorMatches, "cannot add service \"wordpress\": Invalid ownertag admin: \"admin\" is not a valid tag")
}

func (s *StateSuite) TestAddServiceNotUserTag(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("wordpress", "machine-3", charm, nil, nil)
	c.Assert(err, gc.ErrorMatches, "cannot add service \"wordpress\": Invalid ownertag machine-3: \"machine-3\" is not a valid user tag")
}

func (s *StateSuite) TestAddServiceNonExistentUser(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("wordpress", "user-notAuser", charm, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add service "wordpress": environment user "notAuser@local" not found`)
}

func (s *StateSuite) TestAllServices(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	services, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.State.AddService("wordpress", s.Owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	services, err = s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 1)

	_, err = s.State.AddService("mysql", s.Owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	services, err = s.State.AllServices()
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
		err:     `service "wooble" not found`,
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
			ServiceName: "lg",
			Relation: charm.Relation{
				Name:      "logging-directory",
				Role:      "requirer",
				Interface: "logging",
				Limit:     1,
				Scope:     charm.ScopeContainer,
			}}, {
			ServiceName: "lg2",
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
			ServiceName: "rk1",
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
			ServiceName: "ms",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "dev",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
				Limit:     2,
			},
		}, {
			ServiceName: "wp",
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
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-directory",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "wp",
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
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "wp",
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
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "ms",
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

func (s *StateSuite) TestEnvironConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	}
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	oldCfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(oldCfg, gc.DeepEquals, cfg)
}

func (s *StateSuite) TestEnvironConstraints(c *gc.C) {
	// Environ constraints start out empty (for now).
	cons, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	// Environ constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(1024)}
	err = s.State.SetEnvironConstraints(cons2)
	c.Assert(err, jc.ErrorIsNil)
	cons3, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons3, gc.DeepEquals, cons2)

	// Environ constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(250)}
	err = s.State.SetEnvironConstraints(cons4)
	c.Assert(err, jc.ErrorIsNil)
	cons5, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons5, gc.DeepEquals, cons4)
}

func (s *StateSuite) TestSetInvalidConstraints(c *gc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.State.SetEnvironConstraints(cons)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *StateSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("constraints-tester", tw, loggo.DEBUG), gc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.State.SetEnvironConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`setting environment constraints: unsupported constraints: cpu-power`},
	})
	econs, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(econs, gc.DeepEquals, cons)
}

func (s *StateSuite) TestWatchIPAddresses(c *gc.C) {
	w := s.State.WatchIPAddresses()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent()

	// add an IP address
	addr, err := s.State.AddIPAddress(network.NewAddress("0.1.2.3"), "foo")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent(addr.Value())

	// Make it Dead: reported.
	err = addr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent(addr.Value())
}

func (s *StateSuite) TestWatchEnvironmentsBulkEvents(c *gc.C) {
	// Alive environment...
	alive, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Dying environment...
	st1 := s.Factory.MakeEnvironment(c, nil)
	defer st1.Close()
	dying, err := st1.Environment()
	c.Assert(err, jc.ErrorIsNil)
	dying.Destroy()

	st2 := s.Factory.MakeEnvironment(c, nil)
	defer st2.Close()
	env2, err := st2.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env2.Destroy(), jc.ErrorIsNil)
	err = state.RemoveEnvironment(s.State, st2.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	// All except the dead env are reported in initial event.
	w := s.State.WatchEnvironments()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent(alive.UUID(), dying.UUID())

	// Remove alive and dying and see changes reported.
	err = state.RemoveEnvironment(s.State, dying.UUID())
	c.Assert(err, jc.ErrorIsNil)
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent(alive.UUID(), dying.UUID())
}

func (s *StateSuite) TestWatchEnvironmentsLifecycle(c *gc.C) {
	// Initial event reports the state server environment.
	w := s.State.WatchEnvironments()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(s.State.EnvironUUID())
	wc.AssertNoChange()

	// Add an environment: reported.
	st1 := s.Factory.MakeEnvironment(c, nil)
	defer st1.Close()
	env, err := st1.Environment()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(env.UUID())
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(env.UUID())
	wc.AssertNoChange()

	// Remove the environment: reported.
	err = state.RemoveEnvironment(s.State, env.UUID())
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
	service := s.AddTestingService(c, "service", s.AddTestingCharm(c, "dummy"))
	wc.AssertChange("service")
	wc.AssertNoChange()

	// Change the service: not reported.
	keepDying, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("service")
	wc.AssertNoChange()

	// Make it Dead(/removed): reported.
	err = keepDying.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("service")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchServicesDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.lifecycleWatcher,
	// which is also used by:
	//     State.WatchEnvironments
	//     Service.WatchUnits
	//     Service.WatchRelations
	//     State.WatchEnviron
	//     Machine.WatchContainers
	testWatcherDiesWhenStateCloses(c, s.envTag, func(c *gc.C, st *state.State) waiter {
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
	w := s.State.WatchEnvironMachines()
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
	w := s.State.WatchEnvironMachines()
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

	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(machine.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIgnoresContainers(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchEnvironMachines()
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
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
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
	w := machine.WatchContainers(instance.LXC)
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
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxc/0")
	wcAll.AssertNoChange()

	// Add a container of a different type: not reported.
	m1, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Add a nested container of the right type: not reported.
	mchild, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()

	// Add a container of a different machine: not reported.
	m2, err := s.State.AddMachineInsideMachine(template, otherMachine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	statetesting.AssertStop(c, w)
	wcAll.AssertNoChange()
	statetesting.AssertStop(c, wAll)

	w = machine.WatchContainers(instance.LXC)
	defer statetesting.AssertStop(c, w)
	wc = statetesting.NewStringsWatcherC(c, s.State, w)
	wAll = machine.WatchAllContainers()
	defer statetesting.AssertStop(c, wAll)
	wcAll = statetesting.NewStringsWatcherC(c, s.State, wAll)
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/kvm/0", "0/lxc/0")
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
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxc/0")
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
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxc/0")
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

func (s *StateSuite) TestWatchStateServerInfo(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchStateServerInfo()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	info, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.StateServerInfo{
		EnvironmentTag:   s.envTag,
		MachineIds:       []string{"0"},
		VotingMachineIds: []string{"0"},
	})

	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)

	wc.AssertOneChange()

	info, err = s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.StateServerInfo{
		EnvironmentTag:   s.envTag,
		MachineIds:       []string{"0", "1", "2"},
		VotingMachineIds: []string{"0", "1", "2"},
	})
}

func (s *StateSuite) TestAdditionalValidation(c *gc.C) {
	updateAttrs := map[string]interface{}{"logging-config": "juju=ERROR"}
	configValidator1 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		c.Assert(updateAttrs, gc.DeepEquals, map[string]interface{}{"logging-config": "juju=ERROR"})
		if _, found := updateAttrs["logging-config"]; found {
			return fmt.Errorf("cannot change logging-config")
		}
		return nil
	}
	removeAttrs := []string{"logging-config"}
	configValidator2 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		c.Assert(removeAttrs, gc.DeepEquals, []string{"logging-config"})
		for _, i := range removeAttrs {
			if i == "logging-config" {
				return fmt.Errorf("cannot remove logging-config")
			}
		}
		return nil
	}
	configValidator3 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		return nil
	}

	err := s.State.UpdateEnvironConfig(updateAttrs, nil, configValidator1)
	c.Assert(err, gc.ErrorMatches, "cannot change logging-config")
	err = s.State.UpdateEnvironConfig(nil, removeAttrs, configValidator2)
	c.Assert(err, gc.ErrorMatches, "cannot remove logging-config")
	err = s.State.UpdateEnvironConfig(updateAttrs, nil, configValidator3)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestRemoveAllEnvironDocs(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, nil)
	defer st.Close()

	// insert one doc for each multiEnvCollection
	var ops []mgotxn.Op
	for _, collName := range state.MultiEnvCollections() {
		// skip adding constraints, envuser and settings as they were added when the
		// environment was created
		if collName == "constraints" || collName == "envusers" || collName == "settings" {
			continue
		}
		if state.HasRawAccess(collName) {
			coll, closer := state.GetRawCollection(st, collName)
			defer closer()

			err := coll.Insert(bson.M{
				"_id":      state.DocID(st, "arbitraryid"),
				"env-uuid": st.EnvironUUID(),
			})
			c.Assert(err, jc.ErrorIsNil)
		} else {
			ops = append(ops, mgotxn.Op{
				C:      collName,
				Id:     state.DocID(st, "arbitraryid"),
				Insert: bson.M{"env-uuid": st.EnvironUUID()}})
		}
	}
	err := state.RunTransaction(st, ops)
	c.Assert(err, jc.ErrorIsNil)

	// test that we can find each doc in state
	for _, collName := range state.MultiEnvCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"env-uuid", st.EnvironUUID()}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Not(gc.Equals), 0)
	}

	// test that we can find the user:envName unique index
	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	indexColl, closer := state.GetCollection(st, "userenvname")
	defer closer()
	id := state.UserEnvNameIndex(env.Owner().Username(), env.Name())
	n, err := indexColl.FindId(id).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)

	err = state.SetEnvLifeDying(st, st.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveAllEnvironDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	n, err = indexColl.FindId(id).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)

	// ensure all docs for all multiEnvCollections are removed
	for _, collName := range state.MultiEnvCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"env-uuid", st.EnvironUUID()}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0)
	}
}

func (s *StateSuite) TestRemoveAllEnvironDocsAliveEnvFails(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, nil)
	defer st.Close()

	err := st.RemoveAllEnvironDocs()
	c.Assert(err, gc.ErrorMatches, "transaction aborted")
}

type attrs map[string]interface{}

func (s *StateSuite) TestWatchEnvironConfig(c *gc.C) {
	w := s.State.WatchEnvironConfig()
	defer statetesting.AssertStop(c, w)

	// TODO(fwereade) just use a NotifyWatcher and NotifyWatcherC to test it.
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case got := <-w.Changes():
			c.Fatalf("got unexpected change: %#v", got)
		case <-time.After(testing.ShortWait):
		}
	}
	assertChange := func(change attrs) {
		cfg, err := s.State.EnvironConfig()
		c.Assert(err, jc.ErrorIsNil)
		cfg, err = cfg.Apply(change)
		c.Assert(err, jc.ErrorIsNil)
		if change != nil {
			err = s.State.UpdateEnvironConfig(change, nil, nil)
			c.Assert(err, jc.ErrorIsNil)
		}
		s.State.StartSync()
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Assert(got.AllAttrs(), gc.DeepEquals, cfg.AllAttrs())
		case <-time.After(testing.LongWait):
			c.Fatalf("did not get change: %#v", change)
		}
		assertNoChange()
	}
	assertChange(nil)
	assertChange(attrs{"default-series": "another-series"})
	assertChange(attrs{"fancy-new-key": "arbitrary-value"})
}

func (s *StateSuite) TestWatchEnvironConfigDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.envTag, func(c *gc.C, st *state.State) waiter {
		w := st.WatchEnvironConfig()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchForEnvironConfigChanges(c *gc.C) {
	cur := version.Current.Number
	err := statetesting.SetAgentVersion(s.State, cur)
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchForEnvironConfigChanges()
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

func (s *StateSuite) TestWatchEnvironConfigCorruptConfig(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	// Corrupt the environment configuration.
	settings := s.Session.DB("juju").C("settings")
	err = settings.UpdateId(state.DocID(s.State, "e"), bson.D{{"$unset", bson.D{{"name", 1}}}})
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()

	// Start watching the configuration.
	watcher := s.State.WatchEnvironConfig()
	defer watcher.Stop()
	done := make(chan *config.Config)
	go func() {
		select {
		case cfg, ok := <-watcher.Changes():
			if !ok {
				c.Errorf("watcher channel closed")
			} else {
				done <- cfg
			}
		case <-time.After(5 * time.Second):
			c.Fatalf("no environment configuration observed")
		}
	}()

	s.State.StartSync()

	// The invalid configuration must not have been generated.
	select {
	case <-done:
		c.Fatalf("configuration returned too soon")
	case <-time.After(testing.ShortWait):
	}

	// Fix the configuration.
	err = settings.UpdateId(state.DocID(s.State, "e"), bson.D{{"$set", bson.D{{"name", "foo"}}}})
	c.Assert(err, jc.ErrorIsNil)
	fixed := cfg.AllAttrs()
	err = s.State.UpdateEnvironConfig(fixed, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	select {
	case got := <-done:
		c.Assert(got.AllAttrs(), gc.DeepEquals, fixed)
	case <-time.After(5 * time.Second):
		c.Fatalf("no environment configuration observed")
	}
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
	wordpress2, err := s.State.Service("wordpress")
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

func tryOpenState(envTag names.EnvironTag, info *mongo.MongoInfo) error {
	st, err := state.Open(envTag, info, statetesting.NewDialOpts(), state.Policy(nil))
	if err == nil {
		err = st.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *gc.C) {
	info := statetesting.NewMongoInfo()
	info.Tag, info.Password = names.NewUserTag("arble"), "bar"
	err := tryOpenState(s.envTag, info)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = names.NewUserTag("arble"), ""
	err = tryOpenState(s.envTag, info)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = nil, ""
	err = tryOpenState(s.envTag, info)
	c.Check(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestOpenBadAddress(c *gc.C) {
	info := statetesting.NewMongoInfo()
	info.Addrs = []string{"0.1.2.3:1234"}
	st, err := state.Open(testing.EnvironmentTag, info, mongo.DialOpts{
		Timeout: 1 * time.Millisecond,
	}, state.Policy(nil))
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
	st, err := state.Open(testing.EnvironmentTag, info, mongo.DialOpts{
		Timeout: 1 * time.Millisecond,
	}, state.Policy(nil))
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

func testSetAgentCompatPassword(c *gc.C, entity state.Authenticator) {
	// In Juju versions 1.16 and older we used UserPasswordHash(password,CompatSalt)
	// for Machine and Unit agents. This was determined to be overkill
	// (since we know that Unit agents will actually use
	// utils.RandomPassword() and get 18 bytes of entropy, and thus won't
	// be brute-forced.)
	c.Assert(entity.PasswordValid(goodPassword), jc.IsFalse)
	agentHash := utils.AgentPasswordHash(goodPassword)
	err := state.SetPasswordHash(entity, agentHash)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.PasswordValid(goodPassword), jc.IsTrue)
	c.Assert(entity.PasswordValid(alternatePassword), jc.IsFalse)
	c.Assert(state.GetPasswordHash(entity), gc.Equals, agentHash)

	backwardsCompatibleHash := utils.UserPasswordHash(goodPassword, utils.CompatSalt)
	c.Assert(backwardsCompatibleHash, gc.Not(gc.Equals), agentHash)
	err = state.SetPasswordHash(entity, backwardsCompatibleHash)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.PasswordValid(alternatePassword), jc.IsFalse)
	c.Assert(state.GetPasswordHash(entity), gc.Equals, backwardsCompatibleHash)
	// After succeeding to log in with the old compatible hash, the db
	// should be updated with the new hash
	c.Assert(entity.PasswordValid(goodPassword), jc.IsTrue)
	c.Assert(state.GetPasswordHash(entity), gc.Equals, agentHash)
	c.Assert(entity.PasswordValid(goodPassword), jc.IsTrue)

	// Agents are unable to set short passwords
	err = entity.SetPassword("short")
	c.Check(err, gc.ErrorMatches, "password is only 5 bytes long, and is not a valid Agent password")
	// Grandfather clause. Agents that have short passwords are allowed if
	// it was done in the compatHash form
	agentHash = utils.AgentPasswordHash("short")
	backwardsCompatibleHash = utils.UserPasswordHash("short", utils.CompatSalt)
	err = state.SetPasswordHash(entity, backwardsCompatibleHash)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.PasswordValid("short"), jc.IsTrue)
	// We'll still update the hash, but now it points to the hash of the
	// shorter password. Agents still can't set the password to it
	c.Assert(state.GetPasswordHash(entity), gc.Equals, agentHash)
	// Still valid with the shorter password
	c.Assert(entity.PasswordValid("short"), jc.IsTrue)
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
	tag: names.NewEnvironTag("9f484882-2f18-4fd2-967d-db9663db7bea"),
	err: `environment "9f484882-2f18-4fd2-967d-db9663db7bea" not found`,
}, {
	tag: names.NewMachineTag("0"),
}, {
	tag: names.NewServiceTag("ser-vice2"),
}, {
	tag: names.NewRelationTag("wordpress:db ser-vice2:server"),
}, {
	tag: names.NewUnitTag("ser-vice2/0"),
}, {
	tag: names.NewUserTag("arble"),
}, {
	tag: names.NewNetworkTag("missing"),
	err: `network "missing" not found`,
}, {
	tag: names.NewNetworkTag("net1"),
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
	names.UserTagKind:     (*state.User)(nil),
	names.EnvironTagKind:  (*state.Environment)(nil),
	names.ServiceTagKind:  (*state.Service)(nil),
	names.UnitTagKind:     (*state.Unit)(nil),
	names.MachineTagKind:  (*state.Machine)(nil),
	names.RelationTagKind: (*state.Relation)(nil),
	names.NetworkTagKind:  (*state.Network)(nil),
	names.ActionTagKind:   (*state.Action)(nil),
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
	net1, err := s.State.AddNetwork(state.NetworkInfo{
		Name:       "net1",
		ProviderId: "provider-id",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(net1.Tag().String(), gc.Equals, "network-net1")
	c.Assert(string(net1.ProviderId()), gc.Equals, "provider-id")

	// environment tag is dynamically generated
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	findEntityTests = append([]findEntityTest{}, findEntityTests...)
	findEntityTests = append(findEntityTests, findEntityTest{
		tag: names.NewEnvironTag(env.UUID()),
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
			if kind == names.EnvironTagKind {
				// TODO(axw) 2013-12-04 #1257587
				// We *should* only be able to get the entity with its tag, but
				// for backwards-compatibility we accept any non-UUID tag.
				c.Assert(e.Tag(), gc.Equals, env.Tag())
			} else if kind == names.UserTagKind {
				// Test the fully qualified username rather than the tag structure itself.
				expected := test.tag.(names.UserTag).Username()
				c.Assert(e.Tag().(names.UserTag).Username(), gc.Equals, expected)
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

func (s *StateSuite) TestParseServiceTag(c *gc.C) {
	svc := s.AddTestingService(c, "ser-vice2", s.AddTestingCharm(c, "dummy"))
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, svc.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "services")
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

func (s *StateSuite) TestParseEnvironmentTag(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, env.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "environments")
	c.Assert(id, gc.Equals, env.UUID())
}

func (s *StateSuite) TestParseNetworkTag(c *gc.C) {
	net1, err := s.State.AddNetwork(state.NetworkInfo{
		Name:       "net1",
		ProviderId: "provider-id",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	})
	c.Assert(err, jc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, net1.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "networks")
	c.Assert(id, gc.Equals, state.DocID(s.State, net1.Name()))
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
	testWatcherDiesWhenStateCloses(c, s.envTag, func(c *gc.C, st *state.State) waiter {
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
	// A single change should occur, and the service name should appear only
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
	testWatcherDiesWhenStateCloses(c, s.envTag, func(c *gc.C, st *state.State) waiter {
		w := st.WatchMinUnits()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestNestingLevel(c *gc.C) {
	c.Assert(state.NestingLevel("0"), gc.Equals, 0)
	c.Assert(state.NestingLevel("0/lxc/1"), gc.Equals, 1)
	c.Assert(state.NestingLevel("0/lxc/1/kvm/0"), gc.Equals, 2)
}

func (s *StateSuite) TestTopParentId(c *gc.C) {
	c.Assert(state.TopParentId("0"), gc.Equals, "0")
	c.Assert(state.TopParentId("0/lxc/1"), gc.Equals, "0")
	c.Assert(state.TopParentId("0/lxc/1/kvm/2"), gc.Equals, "0")
}

func (s *StateSuite) TestParentId(c *gc.C) {
	c.Assert(state.ParentId("0"), gc.Equals, "")
	c.Assert(state.ParentId("0/lxc/1"), gc.Equals, "0")
	c.Assert(state.ParentId("0/lxc/1/kvm/0"), gc.Equals, "0/lxc/1")
}

func (s *StateSuite) TestContainerTypeFromId(c *gc.C) {
	c.Assert(state.ContainerTypeFromId("0"), gc.Equals, instance.ContainerType(""))
	c.Assert(state.ContainerTypeFromId("0/lxc/1"), gc.Equals, instance.LXC)
	c.Assert(state.ContainerTypeFromId("0/lxc/1/kvm/0"), gc.Equals, instance.KVM)
}

func (s *StateSuite) TestIsUpgradeInProgressError(c *gc.C) {
	c.Assert(state.IsUpgradeInProgressError(errors.New("foo")), jc.IsFalse)
	c.Assert(state.IsUpgradeInProgressError(state.UpgradeInProgressError), jc.IsTrue)
	c.Assert(state.IsUpgradeInProgressError(errors.Trace(state.UpgradeInProgressError)), jc.IsTrue)
}

func (s *StateSuite) TestSetEnvironAgentVersionErrors(c *gc.C) {
	// Get the agent-version set in the environment.
	envConfig, err := s.State.EnvironConfig()
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
	err = s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	expectErr := fmt.Sprintf("some agents have not upgraded to the current environment version %s: machine-0, machine-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Add a service and 4 units: one with a different version, one
	// with an empty version, one with the current version, and one
	// with the new version.
	service, err := s.State.AddService("wordpress", s.Owner.String(), s.AddTestingCharm(c, "wordpress"), nil, nil)
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
	err = s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	expectErr = fmt.Sprintf("some agents have not upgraded to the current environment version %s: machine-0, machine-1, unit-wordpress-0, unit-wordpress-1", stringVersion)
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
	err = s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	expectErr = fmt.Sprintf("some agents have not upgraded to the current environment version %s: unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)
}

func (s *StateSuite) prepareAgentVersionTests(c *gc.C, st *state.State) (*config.Config, string) {
	// Get the agent-version set in the environment.
	envConfig, err := st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine and a unit with the current version.
	machine, err := st.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	service, err := st.AddService("wordpress", s.Owner.String(), s.AddTestingCharm(c, "wordpress"), nil, nil)
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
	c.Assert(s.State.UpdateEnvironConfig(attrs, nil, nil), gc.IsNil)
}

func assertAgentVersion(c *gc.C, st *state.State, vers string) {
	envConfig, err := st.EnvironConfig()
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
	err := s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
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
	err := s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6")
}

func (s *StateSuite) TestSetEnvironAgentVersionOnOtherEnviron(c *gc.C) {
	otherSt := s.Factory.MakeEnvironment(c, nil)
	defer otherSt.Close()

	higher := version.Current
	higher.Patch++
	lower := version.Current
	lower.Patch--

	// Set other environ version to < server envrion version
	err := otherSt.SetEnvironAgentVersion(lower.Number)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, lower.Number.String())

	// Set other environ version == server envrion version
	err = otherSt.SetEnvironAgentVersion(version.Current.Number)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, version.Current.Number.String())

	// Set other environ version to > server envrion version
	err = otherSt.SetEnvironAgentVersion(higher.Number)
	expected := fmt.Sprintf("a hosted environment cannot have a higher version than the server environment: %s > %s",
		higher.Number.String(),
		version.Current.Number.String(),
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
	err := s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	c.Assert(errors.Cause(err), gc.Equals, txn.ErrExcessiveContention)
	// Make sure the version remained the same.
	assertAgentVersion(c, s.State, currentVersion)
}

func (s *StateSuite) TestSetEnvironAgentFailsIfUpgrading(c *gc.C) {
	// Get the agent-version set in the environment.
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine("series", state.JobManageEnviron)
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

	err = s.State.SetEnvironAgentVersion(nextVersion)
	c.Assert(errors.Cause(err), gc.Equals, state.UpgradeInProgressError)
	c.Assert(err, gc.ErrorMatches,
		"an upgrade is already in progress or the last upgrade did not complete")
}

func (s *StateSuite) TestSetEnvironAgentFailsReportsCorrectError(c *gc.C) {
	// Ensure that the correct error is reported if an upgrade is
	// progress but that isn't the reason for the
	// SetEnvironAgentVersion call failing.

	// Get the agent-version set in the environment.
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine("series", state.JobManageEnviron)
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

	err = s.State.SetEnvironAgentVersion(nextVersion)
	c.Assert(err, gc.ErrorMatches, "some agents have not upgraded to the current environment version.+")
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
func testWatcherDiesWhenStateCloses(c *gc.C, envTag names.EnvironTag, startWatcher func(c *gc.C, st *state.State) waiter) {
	st, err := state.Open(envTag, statetesting.NewMongoInfo(), statetesting.NewDialOpts(), state.Policy(nil))
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

func (s *StateSuite) TestStateServerInfo(c *gc.C) {
	ids, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids.EnvironmentTag, gc.Equals, s.envTag)
	c.Assert(ids.MachineIds, gc.HasLen, 0)
	c.Assert(ids.VotingMachineIds, gc.HasLen, 0)

	// TODO(rog) more testing here when we can actually add
	// state servers.
}

func (s *StateSuite) TestStateServerInfoWithPreMigrationDoc(c *gc.C) {
	err := s.stateServers.Update(
		nil,
		bson.D{{"$unset", bson.D{{"env-uuid", 1}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	ids, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids.EnvironmentTag, gc.Equals, s.envTag)
}

func (s *StateSuite) TestReopenWithNoMachines(c *gc.C) {
	expected := &state.StateServerInfo{
		EnvironmentTag: s.envTag,
	}
	info, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	st, err := state.Open(s.envTag, statetesting.NewMongoInfo(), statetesting.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	info, err = s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *StateSuite) TestEnsureAvailabilityFailsWithBadCount(c *gc.C) {
	for _, n := range []int{-1, 2, 6} {
		changes, err := s.State.EnsureAvailability(n, constraints.Value{}, "", nil)
		c.Assert(err, gc.ErrorMatches, "number of state servers must be odd and non-negative")
		c.Assert(changes.Added, gc.HasLen, 0)
	}
	_, err := s.State.EnsureAvailability(replicaset.MaxPeers+2, constraints.Value{}, "", nil)
	c.Assert(err, gc.ErrorMatches, `state server count is too large \(allowed \d+\)`)
}

func (s *StateSuite) TestEnsureAvailabilityAddsNewMachines(c *gc.C) {
	// Don't use agent presence to decide on machine availability.
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	ids := make([]string, 3)
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add a non-state-server machine just to make sure.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStateServerInfo(c, []string{"0"}, []string{"0"}, nil)

	cons := constraints.Value{
		Mem: newUint64(100),
	}
	changes, err := s.State.EnsureAvailability(3, cons, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i + 1))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageEnviron,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotCons, gc.DeepEquals, cons)
		c.Assert(m.WantsVote(), jc.IsTrue)
		ids[i] = m.Id()
	}
	s.assertStateServerInfo(c, ids, ids, nil)
}

func (s *StateSuite) TestEnsureAvailabilityTo(c *gc.C) {
	// Don't use agent presence to decide on machine availability.
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	ids := make([]string, 3)
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add two non-state-server machines.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStateServerInfo(c, []string{"0"}, []string{"0"}, nil)

	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", []string{"1", "2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 0)
	c.Assert(changes.Converted, gc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageEnviron,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotCons, gc.DeepEquals, constraints.Value{})
		c.Assert(m.WantsVote(), jc.IsTrue)
		ids[i] = m.Id()
	}
	s.assertStateServerInfo(c, ids, ids, nil)
}

func newUint64(i uint64) *uint64 {
	return &i
}

func (s *StateSuite) assertStateServerInfo(c *gc.C, machineIds []string, votingMachineIds []string, placement []string) {
	info, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.EnvironmentTag, gc.Equals, s.envTag)
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

func (s *StateSuite) TestEnsureAvailabilitySamePlacementAsNewCount(c *gc.C) {
	placement := []string{"p1", "p2", "p3"}
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *StateSuite) TestEnsureAvailabilityMorePlacementThanNewCount(c *gc.C) {
	placement := []string{"p1", "p2", "p3", "p4"}
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *StateSuite) TestEnsureAvailabilityLessPlacementThanNewCount(c *gc.C) {
	placement := []string{"p1", "p2"}
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2"})
}

func (s *StateSuite) TestEnsureAvailabilityDemotesUnavailableMachines(c *gc.C) {
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	c.Assert(changes.Maintained, gc.HasLen, 2)

	// New state server machine "3" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertStateServerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)
	c.Assert(m0.IsManager(), jc.IsTrue) // job still intact for now
	m3, err := s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m3.WantsVote(), jc.IsTrue)
	c.Assert(m3.IsManager(), jc.IsTrue)
}

func (s *StateSuite) TestEnsureAvailabilityPromotesAvailableMachines(c *gc.C) {
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	c.Assert(changes.Demoted, gc.DeepEquals, []string{"0"})
	c.Assert(changes.Maintained, gc.HasLen, 2)

	// New state server machine "3" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertStateServerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsFalse)

	// Mark machine 0 as having a vote, so it doesn't get removed, and make it
	// available once more.
	err = m0.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})
	changes, err = s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 0)

	// No change; we've got as many voting machines as we need.
	s.assertStateServerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)

	// Make machine 3 unavailable; machine 0 should be promoted, and two new
	// machines created.
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "3", nil
	})
	changes, err = s.State.EnsureAvailability(5, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	c.Assert(changes.Demoted, gc.DeepEquals, []string{"3"})
	s.assertStateServerInfo(c, []string{"0", "1", "2", "3", "4", "5"}, []string{"0", "1", "2", "4", "5"}, nil)
	err = m0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.WantsVote(), jc.IsTrue)
}

func (s *StateSuite) TestEnsureAvailabilityRemovesUnavailableMachines(c *gc.C) {
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)

	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)
	s.assertStateServerInfo(c, []string{"0", "1", "2", "3"}, []string{"1", "2", "3"}, nil)
	// machine 0 does not have a vote, so another call to EnsureAvailability
	// will remove machine 0's JobEnvironManager job.
	changes, err = s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(changes.Removed, gc.HasLen, 1)
	c.Assert(changes.Maintained, gc.HasLen, 3)
	c.Assert(err, jc.ErrorIsNil)
	s.assertStateServerInfo(c, []string{"1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.IsManager(), jc.IsFalse)
}

func (s *StateSuite) TestEnsureAvailabilityMaintainsVoteList(c *gc.C) {
	changes, err := s.State.EnsureAvailability(5, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 5)

	s.assertStateServerInfo(c,
		[]string{"0", "1", "2", "3", "4"},
		[]string{"0", "1", "2", "3", "4"}, nil)
	// Mark machine-0 as dead, so we'll want to create another one again
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnsureAvailability(0, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)

	// New state server machine "5" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertStateServerInfo(c,
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

func (s *StateSuite) TestEnsureAvailabilityDefaultsTo3(c *gc.C) {
	changes, err := s.State.EnsureAvailability(0, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)
	s.assertStateServerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	// Mark machine-0 as dead, so we'll want to create it again
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return m.Id() != "0", nil
	})
	changes, err = s.State.EnsureAvailability(0, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 1)

	// New state server machine "3" is created; "0" still exists in MachineIds,
	// but no longer in VotingMachineIds.
	s.assertStateServerInfo(c,
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

func (s *StateSuite) TestEnsureAvailabilityConcurrentSame(c *gc.C) {
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
		c.Assert(err, jc.ErrorIsNil)
		// The outer EnsureAvailability call will allocate IDs 0..2,
		// and the inner one 3..5.
		c.Assert(changes.Added, gc.HasLen, 3)
		expected := []string{"3", "4", "5"}
		s.assertStateServerInfo(c, expected, expected, nil)
	}).Check()

	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.DeepEquals, []string{"0", "1", "2"})
	s.assertStateServerInfo(c, []string{"3", "4", "5"}, []string{"3", "4", "5"}, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestEnsureAvailabilityConcurrentLess(c *gc.C) {
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes.Added, gc.HasLen, 3)
		// The outer EnsureAvailability call will initially allocate IDs 0..4,
		// and the inner one 5..7.
		expected := []string{"5", "6", "7"}
		s.assertStateServerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnsureAvailability will initially attempt to allocate
	// machines 0..4, and fail due to the concurrent change. It will then
	// allocate machines 8..9 to make up the difference from the concurrent
	// EnsureAvailability call.
	changes, err := s.State.EnsureAvailability(5, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 2)
	expected := []string{"5", "6", "7", "8", "9"}
	s.assertStateServerInfo(c, expected, expected, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestEnsureAvailabilityConcurrentMore(c *gc.C) {
	s.PatchValue(state.StateServerAvailable, func(m *state.Machine) (bool, error) {
		return true, nil
	})

	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnsureAvailability(5, constraints.Value{}, "quantal", nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes.Added, gc.HasLen, 5)
		// The outer EnsureAvailability call will allocate IDs 0..2,
		// and the inner one 3..7.
		expected := []string{"3", "4", "5", "6", "7"}
		s.assertStateServerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnsureAvailability will initially attempt to allocate
	// machines 0..2, and fail due to the concurrent change. It will then
	// find that the number of voting machines in state is greater than
	// what we're attempting to ensure, and fail.
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, gc.ErrorMatches, "failed to create new state server machines: cannot reduce state server count")
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
			Value:       "0.2.4.6",
			Type:        network.IPv4Address,
			NetworkName: "net",
			Scope:       network.ScopeCloudLocal,
		},
		Port: 1,
	}, {
		Address: network.Address{
			Value:       "0.4.8.16",
			Type:        network.IPv4Address,
			NetworkName: "foo",
			Scope:       network.ScopePublic,
		},
		Port: 2,
	}}, {{
		Address: network.Address{
			Value:       "0.6.1.2",
			Type:        network.IPv4Address,
			NetworkName: "net",
			Scope:       network.ScopeCloudLocal,
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
			Value:       "0.2.4.6",
			Type:        network.IPv6Address,
			NetworkName: "net",
			Scope:       network.ScopeCloudLocal,
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
			Value:       "0.4.8.16",
			Type:        network.IPv4Address,
			NetworkName: "foo",
			Scope:       network.ScopePublic,
		},
		Port: 2,
	}}, {{
		Address: network.Address{
			Value:       "0.2.4.6",
			Type:        network.IPv4Address,
			NetworkName: "net",
			Scope:       network.ScopeCloudLocal,
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
		revno, err := state.TxnRevno(s.State, "stateServers", "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)
	revno, err := state.TxnRevno(s.State, "stateServers", "apiHostPorts")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Equals, prevRevno)
}

func (s *StateSuite) TestSetAPIHostPortsConcurrentDifferent(c *gc.C) {
	hostPorts0 := []network.HostPort{{
		Address: network.Address{
			Value:       "0.4.8.16",
			Type:        network.IPv4Address,
			NetworkName: "foo",
			Scope:       network.ScopePublic,
		},
		Port: 2,
	}}
	hostPorts1 := []network.HostPort{{
		Address: network.Address{
			Value:       "0.2.4.6",
			Type:        network.IPv4Address,
			NetworkName: "net",
			Scope:       network.ScopeCloudLocal,
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
		revno, err := state.TxnRevno(s.State, "stateServers", "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts([][]network.HostPort{hostPorts1})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)
	revno, err := state.TxnRevno(s.State, "stateServers", "apiHostPorts")
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

type SetAdminMongoPasswordSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&SetAdminMongoPasswordSuite{})

func (s *SetAdminMongoPasswordSuite) TestSetAdminMongoPassword(c *gc.C) {
	inst := &gitjujutesting.MgoInstance{EnableAuth: true}
	err := inst.Start(testing.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.DestroyWithLog()

	owner := names.NewLocalUserTag("initialize-admin")
	mongoInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{inst.Addr()},
			CACert: testing.CACert,
		},
	}
	cfg := testing.EnvironConfig(c)
	st, err := state.Initialize(owner, mongoInfo, cfg, statetesting.NewDialOpts(), nil)
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

	err = tryOpenState(st.EnvironTag(), mongoInfo)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	// note: collections are set up in arbitrary order, proximate cause of
	// failure may differ.
	c.Check(err, gc.ErrorMatches, `[^:]+: unauthorized mongo access: .*`)

	mongoInfo.Password = "foo"
	err = tryOpenState(st.EnvironTag(), mongoInfo)
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetAdminMongoPassword("")
	c.Assert(err, jc.ErrorIsNil)

	mongoInfo.Password = ""
	err = tryOpenState(st.EnvironTag(), mongoInfo)
	c.Assert(err, jc.ErrorIsNil)
}
