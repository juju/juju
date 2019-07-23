// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	mgotxn "gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

var goodPassword = "foo-12345678901234567890"
var alternatePassword = "bar-12345678901234567890"

// preventUnitDestroyRemove sets a non-allocating status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *gc.C, u *state.Unit) {
	// To have a non-allocating status, a unit needs to
	// be assigned to a machine.
	_, err := u.AssignedMachineId()
	if errors.IsNotAssigned(err) {
		err = u.AssignToNewMachine()
	}
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
}

type StateSuite struct {
	ConnSuite
	model *state.Model
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

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.model = model
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *StateSuite) TestOpenController(c *gc.C) {
	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller.Close(), gc.IsNil)
}

func (s *StateSuite) TestOpenControllerTwice(c *gc.C) {
	for i := 0; i < 2; i++ {
		controller, err := state.OpenController(s.testOpenParams())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(controller.Close(), gc.IsNil)
	}
}

func (s *StateSuite) TestIsController(c *gc.C) {
	c.Assert(s.State.IsController(), jc.IsTrue)
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	c.Assert(st2.IsController(), jc.IsFalse)
}

func (s *StateSuite) TestControllerOwner(c *gc.C) {
	owner, err := s.State.ControllerOwner()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(owner, gc.Equals, s.Owner)

	// Check that other models return the same controller owner.
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	owner2, err := otherSt.ControllerOwner()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(owner2, gc.Equals, s.Owner)
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
		st, err := state.Open(s.testOpenParams())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st.Close(), gc.IsNil)
	}
}

func (s *StateSuite) TestOpenRequiresExtantModelTag(c *gc.C) {
	uuid := utils.MustNewUUID()
	params := s.testOpenParams()
	params.ControllerModelTag = names.NewModelTag(uuid.String())
	st, err := state.Open(params)
	if !c.Check(st, gc.IsNil) {
		c.Check(st.Close(), jc.ErrorIsNil)
	}
	expect := fmt.Sprintf("cannot read model %s: model %q not found", uuid, uuid)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *StateSuite) TestOpenSetsModelTag(c *gc.C) {
	st, err := state.Open(s.testOpenParams())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(m.ModelTag(), gc.Equals, s.modelTag)
}

func (s *StateSuite) TestModelUUID(c *gc.C) {
	c.Assert(s.State.ModelUUID(), gc.Equals, s.modelTag.Id())
}

func (s *StateSuite) TestNoModelDocs(c *gc.C) {
	// For example:
	// found documents for model with uuid 7bfe98b6-7282-48d4-8e37-9b90fb3da4f1: 1 constraints doc, 1 modelusers doc, 1 settings doc, 1 statuses doc
	c.Assert(s.State.EnsureModelRemoved(), gc.ErrorMatches,
		fmt.Sprintf(`found documents for model with uuid %s: (\d+ [a-z]+ doc, )*\d+ [a-z]+ doc`, s.State.ModelUUID()))
}

func (s *StateSuite) TestMongoSession(c *gc.C) {
	session := s.State.MongoSession()
	c.Assert(session.Ping(), gc.IsNil)
}

func (s *StateSuite) TestWatch(c *gc.C) {
	// The allWatcher infrastructure is comprehensively tested
	// elsewhere. This just ensures things are hooked up correctly in
	// State.Watch()

	w := s.State.Watch(state.WatchParams{IncludeOffers: true})
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
	w := s.State.WatchAllModels(s.StatePool)
	defer w.Stop()
	deltasC := makeMultiwatcherOutput(w)

	m := s.Factory.MakeMachine(c, nil)
	s.State.StartSync()
	modelSeen := false
	machineSeen := false
	timeout := time.After(testing.LongWait)
	for !modelSeen || !machineSeen {
		select {
		case deltas := <-deltasC:
			for _, delta := range deltas {
				switch e := delta.Entity.(type) {
				case *multiwatcher.ModelInfo:
					c.Assert(e.ModelUUID, gc.Equals, s.State.ModelUUID())
					modelSeen = true
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
	c.Assert(modelSeen, jc.IsTrue)
	c.Assert(machineSeen, jc.IsTrue)
}

type MultiModelStateSuite struct {
	ConnSuite
	OtherState *state.State
	OtherModel *state.Model
}

func (s *MultiModelStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.OtherState = s.Factory.MakeModel(c, nil)
	m, err := s.OtherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.OtherModel = m
}

func (s *MultiModelStateSuite) TearDownTest(c *gc.C) {
	if s.OtherState != nil {
		s.OtherState.Close()
	}
	s.ConnSuite.TearDownTest(c)
}

func (s *MultiModelStateSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

var _ = gc.Suite(&MultiModelStateSuite{})

func (s *MultiModelStateSuite) TestWatchTwoModels(c *gc.C) {
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
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
			},
		},
		{
			about: "containers",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
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
				f := factory.NewFactory(st, s.StatePool)
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
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), gc.Equals, "0")
				return m.WatchUnits()
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				f.MakeUnit(c, &factory.UnitParams{Machine: m})
			},
		}, {
			about: "applications",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchApplications()
			},
			triggerEvent: func(st *state.State) {
				f := factory.NewFactory(st, s.StatePool)
				f.MakeApplication(c, nil)
			},
		}, {
			about: "remote applications",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchRemoteApplications()
			},
			triggerEvent: func(st *state.State) {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "db2", SourceModel: s.Model.ModelTag()})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "relations",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				wordpress := f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return wordpress.WatchRelations()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
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
			about: "remote relations",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchRemoteRelations()
			},
			setUpState: func(st *state.State) bool {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "mysql", SourceModel: s.OtherModel.ModelTag(),
					Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
				})
				c.Assert(err, jc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				wpCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, jc.ErrorIsNil)
				_, err = st.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "relation ingress networks",
			getWatcher: func(st *state.State) interface{} {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "mysql", SourceModel: s.OtherModel.ModelTag(),
					Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
				})
				c.Assert(err, jc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				wpCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, jc.ErrorIsNil)
				rel, err := st.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)
				return rel.WatchRelationIngressNetworks()
			},
			triggerEvent: func(st *state.State) {
				relIngress := state.NewRelationIngressNetworks(st)
				_, err := relIngress.Save("wordpress:db mysql:database", false, []string{"1.2.3.4/32", "4.3.2.1/16"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "relation egress networks",
			getWatcher: func(st *state.State) interface{} {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "mysql", SourceModel: s.OtherModel.ModelTag(),
					Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
				})
				c.Assert(err, jc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				wpCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, jc.ErrorIsNil)
				rel, err := st.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)
				return rel.WatchRelationEgressNetworks()
			},
			triggerEvent: func(st *state.State) {
				relIngress := state.NewRelationEgressNetworks(st)
				_, err := relIngress.Save("wordpress:db mysql:database", false, []string{"1.2.3.4/32", "4.3.2.1/16"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "open ports",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchOpenedPorts()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
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
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlCharm})

				// add and destroy a relation, so there is something to cleanup.
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, jc.ErrorIsNil)
				r := f.MakeRelation(c, &factory.RelationParams{Endpoints: eps})
				loggo.GetLogger("juju.state").SetLogLevel(loggo.TRACE)
				err = r.Destroy()
				c.Assert(err, jc.ErrorIsNil)
				loggo.GetLogger("juju.state").SetLogLevel(loggo.DEBUG)

				return false
			},
			triggerEvent: func(st *state.State) {
				loggo.GetLogger("juju.state").SetLogLevel(loggo.TRACE)
				err := st.Cleanup()
				c.Assert(err, jc.ErrorIsNil)
				loggo.GetLogger("juju.state").SetLogLevel(loggo.DEBUG)
			},
		}, {
			about: "reboots",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
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
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, &factory.MachineParams{})
				c.Assert(m.Id(), gc.Equals, "0")
				sb, err := state.NewStorageBackend(st)
				c.Assert(err, jc.ErrorIsNil)
				return sb.WatchBlockDevices(m.MachineTag())
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
				// Ensure that all the creation events have flowed through the system.
				s.WaitForModelWatchersIdle(c, st.ModelUUID())
				return m.Watch()
			},
			setUpState: func(st *state.State) bool {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)
				m.SetProvisioned("inst-id", "", "fake_nonce", nil)
				return false
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)

				now := time.Now()
				sInfo := status.StatusInfo{
					Status:  status.Error,
					Message: "some status",
					Since:   &now,
				}
				err = m.SetStatus(sInfo)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "settings",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchApplications()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				app, err := st.Application("wordpress")
				c.Assert(err, jc.ErrorIsNil)

				err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "awesome"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "action status",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				dummyCharm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
				application := f.MakeApplication(c, &factory.ApplicationParams{Name: "dummy", Charm: dummyCharm})

				unit, err := application.AddUnit(state.AddUnitParams{})
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
				f := factory.NewFactory(st, s.StatePool)
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
		}, {
			about: "subnets",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchSubnets(nil)
			},
			triggerEvent: func(st *state.State) {
				_, err := st.AddSubnet(corenetwork.SubnetInfo{
					CIDR: "10.0.0.0/24",
				})
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
					nwc.AssertNoChange()
				default:
					c.Fatalf("unknown watcher type %T", w)
				}
				return TestWatcherC{
					c:       c,
					State:   st,
					Watcher: wc,
				}
			}

			checkIsolationForModel := func(w1, w2 TestWatcherC) {
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
				c.Logf("triggering event")
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
			checkIsolationForModel(wc1, wc2)
			checkIsolationForModel(wc2, wc1)
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

	err = machines[0].SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.DeepEquals, []string{"2", "3"})
	c.Assert(changes.Maintained, gc.DeepEquals, []string{machines[0].Id()})

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

func (s *StateSuite) TestAddMachinesmodelDying(c *gc.C) {
	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testmodel" is no longer alive`)
}

func (s *StateSuite) TestAddMachinesmodelDyingAfterInitial(c *gc.C) {
	// Check that machines cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.model.Life(), gc.Equals, state.Alive)
		c.Assert(s.model.Destroy(state.DestroyModelParams{}), gc.IsNil)
	}).Check()
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testmodel" is no longer alive`)
}

func (s *StateSuite) TestAddMachinesmodelMigrating(c *gc.C) {
	err := s.model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testmodel" is being migrated`)
}

func (s *StateSuite) TestAddMachineExtraConstraints(c *gc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cores=4")
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Constraints: extraCons,
		Jobs:        oneJob,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("cores=4 mem=4G")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *StateSuite) TestAddMachinePlacementIgnoresModelConstraints(c *gc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G tags=foo"))
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:    "quantal",
		Jobs:      oneJob,
		Placement: "theplacement",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.Placement(), gc.Equals, "theplacement")
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("")
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
		Volumes: []state.HostVolumeParams{{
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

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	volumeAttachments, err := sb.MachineVolumeAttachments(m.MachineTag())
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
		volume, err := sb.Volume(att.Volume())
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

func (s *StateSuite) TestAddContainerToMachineLockedForSeriesUpgrade(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine("xenial", oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.CreateUpgradeSeriesLock(nil, "bionic")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "xenial",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 is locked for series upgrade")
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
	source := "loveshack"
	tags := []string{"foo", "bar"}
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits, state.JobManageModel},
		Constraints: cons,
		InstanceId:  "i-mindustrious",
		Nonce:       agent.BootstrapNonce,
		HardwareCharacteristics: instance.HardwareCharacteristics{
			Arch:           &arch,
			Mem:            &mem,
			RootDisk:       &disk,
			RootDiskSource: &source,
			Tags:           &tags,
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
	node, err := s.State.ControllerNode(m.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.HasVote(), jc.IsFalse)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageModel})

	// Check that the controller information is correct.
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.ModelTag, gc.Equals, s.modelTag)
	c.Assert(info.MachineIds, gc.DeepEquals, []string{"0"})

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
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%d", i)), "", "fake_nonce", nil)
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
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	_, err = mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wordpressCharm := s.AddTestingCharm(c, "wordpress")
	for i := 0; i < numRelations; i++ {
		applicationname := fmt.Sprintf("wordpress%d", i)
		wordpress := s.AddTestingApplication(c, applicationname, wordpressCharm)
		_, err = wordpress.AddUnit(state.AddUnitParams{})
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

func (s *StateSuite) TestSaveCloudService(c *gc.C) {
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id",
			Addresses:  network.NewAddresses("1.1.1.1"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Refresh(), jc.ErrorIsNil)
	c.Assert(state.LocalID(s.State, svc.Id()), gc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), gc.Equals, "provider-id")
	c.Assert(svc.Addresses(), gc.DeepEquals, network.NewAddresses("1.1.1.1"))

	getResult, err := s.State.CloudService("cloud-svc-ID")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.LocalID(s.State, getResult.Id()), gc.Equals, "a#cloud-svc-ID")
	c.Assert(getResult.ProviderId(), gc.Equals, "provider-id")
	c.Assert(getResult.Addresses(), gc.DeepEquals, network.NewAddresses("1.1.1.1"))
}

func (s *StateSuite) TestSaveCloudServiceChangeAddressesAllGood(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.SaveCloudService(
			state.SaveCloudServiceArgs{
				Id:         "cloud-svc-ID",
				ProviderId: "provider-id",
				Addresses:  network.NewAddresses("1.1.1.1"),
			},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id",
			Addresses:  network.NewAddresses("2.2.2.2"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Refresh(), jc.ErrorIsNil)
	c.Assert(state.LocalID(s.State, svc.Id()), gc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), gc.Equals, "provider-id")
	c.Assert(svc.Addresses(), gc.DeepEquals, network.NewAddresses("2.2.2.2"))
}

func (s *StateSuite) TestSaveCloudServiceChangeProviderIdFailed(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.SaveCloudService(
			state.SaveCloudServiceArgs{
				Id:         "cloud-svc-ID",
				ProviderId: "provider-id-existing",
				Addresses:  network.NewAddresses("1.1.1.1"),
			},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id-new", // ProviderId is immutable, changing this will get assert error.
			Addresses:  network.NewAddresses("1.1.1.1"),
		},
	)
	c.Assert(err, gc.ErrorMatches,
		`cannot add cloud service "provider-id-new": failed to save cloud service: state changing too quickly; try again soon`,
	)
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
	inconfig, err := application.NewConfig(application.ConfigAttributes{"outlook": "good"}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)

	wordpress, err := s.State.AddApplication(
		state.AddApplicationArgs{Name: "wordpress", Charm: ch, CharmConfig: insettings, ApplicationConfig: inconfig})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	outsettings, err := wordpress.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	expected := ch.Config().DefaultSettings()
	for name, value := range insettings {
		expected[name] = value
	}
	c.Assert(outsettings, gc.DeepEquals, expected)
	outconfig, err := wordpress.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outconfig, gc.DeepEquals, inconfig.Attributes())

	mysql, err := s.State.AddApplication(state.AddApplicationArgs{Name: "mysql", Charm: ch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")
	sInfo, err := mysql.Status()
	c.Assert(sInfo.Status, gc.Equals, status.Waiting)
	c.Assert(sInfo.Message, gc.Equals, "waiting for machine")

	// Check that retrieving the new created applications works correctly.
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

func (s *StateSuite) TestAddCAASApplication(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})

	insettings := charm.Settings{"tuning": "optimized"}
	inconfig, err := application.NewConfig(application.ConfigAttributes{"outlook": "good"}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)

	gitlab, err := st.AddApplication(
		state.AddApplicationArgs{Name: "gitlab", Charm: ch, CharmConfig: insettings, ApplicationConfig: inconfig})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gitlab.Name(), gc.Equals, "gitlab")
	c.Assert(gitlab.GetScale(), gc.Equals, 0)
	outsettings, err := gitlab.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	expected := ch.Config().DefaultSettings()
	for name, value := range insettings {
		expected[name] = value
	}
	c.Assert(outsettings, gc.DeepEquals, expected)
	outconfig, err := gitlab.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outconfig, gc.DeepEquals, inconfig.Attributes())

	sInfo, err := gitlab.Status()
	c.Assert(sInfo.Status, gc.Equals, status.Waiting)
	c.Assert(sInfo.Message, gc.Equals, "waiting for container")

	// Check that retrieving the newly created application works correctly.
	gitlab, err = st.Application("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gitlab.Name(), gc.Equals, "gitlab")
	ch, _, err = gitlab.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, ch.URL())
}

func (s *StateSuite) TestAddCAASApplicationPlacementNotAllowed(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})

	placement := []*instance.Placement{instance.MustParsePlacement("#:2")}
	_, err := st.AddApplication(
		state.AddApplicationArgs{Name: "gitlab", Charm: ch, Placement: placement})
	c.Assert(err, gc.ErrorMatches, ".*"+regexp.QuoteMeta(`cannot add application "gitlab": placement directives on k8s models not valid`))
}

func (s *StateSuite) TestAddApplicationWithNilCharmConfigValues(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	insettings := charm.Settings{"tuning": nil}

	wordpress, err := s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: ch, CharmConfig: insettings})
	c.Assert(err, jc.ErrorIsNil)
	outsettings, err := wordpress.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	expected := ch.Config().DefaultSettings()
	for name, value := range insettings {
		expected[name] = value
	}
	c.Assert(outsettings, gc.DeepEquals, expected)

	// Ensure that during creation, application settings with nil config values
	// were stripped and not written into database.
	dbSettings := state.GetApplicationCharmConfig(s.State, wordpress)
	_, dbFound := dbSettings.Get("tuning")
	c.Assert(dbFound, jc.IsFalse)
}

func (s *StateSuite) TestAddApplicationModelDying(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that applications cannot be added if the model is initially Dying.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testmodel" is no longer alive`)
}

func (s *StateSuite) TestAddApplicationModelMigrating(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that applications cannot be added if the model is initially Dying.
	err := s.model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testmodel" is being migrated`)
}

func (s *StateSuite) TestAddApplicationSameRemoteExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": remote application with same name already exists`)
}

func (s *StateSuite) TestAddApplicationRemoteAddedAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "s1", SourceModel: s.Model.ModelTag()})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": remote application with same name already exists`)
}

func (s *StateSuite) TestAddApplicationSameLocalExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "s0", charm)
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s0", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s0": application already exists`)
}

func (s *StateSuite) TestAddApplicationLocalAddedAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a local application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.AddTestingApplication(c, "s1", charm)
	}).Check()
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": application already exists`)
}

func (s *StateSuite) TestAddApplicationModelDyingAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "s0", charm)
	// Check that applications cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.model.Life(), gc.Equals, state.Alive)
		c.Assert(s.model.Destroy(state.DestroyModelParams{}), gc.IsNil)
	}).Check()
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testmodel" is no longer alive`)
}

func (s *StateSuite) TestApplicationNotFound(c *gc.C) {
	_, err := s.State.Application("bummer")
	c.Assert(err, gc.ErrorMatches, `application "bummer" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestAddApplicationWithDefaultBindings(c *gc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := app.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"server":  "",
		"client":  "",
		"cluster": "",
	})

	// Removing the application also removes its bindings.
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	state.AssertEndpointBindingsNotFoundForApplication(c, app)
}

func (s *StateSuite) TestAddApplicationWithSpecifiedBindings(c *gc.C) {
	// Add extra spaces to use in bindings.
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	// Specify some bindings, but not all when adding the application.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 43)
	app, err := s.State.AddApplication(state.AddApplicationArgs{
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
	bindings, err := app.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"server":  "", // inherited from defaults.
		"client":  "client",
		"cluster": "db",
	})
}

func (s *StateSuite) TestAddApplicationWithInvalidBindings(c *gc.C) {
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
		expectedError: `unknown space "anything" not valid`,
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

func (s *StateSuite) TestAddApplicationMachinePlacementInvalidSeries(c *gc.C) {
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

func (s *StateSuite) TestAddApplicationIncompatibleOSWithSeriesInURL(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: "centos7",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": series "centos7" \(OS \"CentOS"\) not supported by charm, supported series are "quantal"`)
}

func (s *StateSuite) TestAddApplicationCompatibleOSWithSeriesInURL(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: charm.URL().Series,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestAddApplicationCompatibleOSWithNoExplicitSupportedSeries(c *gc.C) {
	// If a charm doesn't declare any series, we can add it with any series we choose.
	charm := s.AddSeriesCharm(c, "dummy", "")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: "quantal",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestAddApplicationOSIncompatibleWithSupportedSeries(c *gc.C) {
	charm := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		Series: "centos7",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": series "centos7" \(OS "CentOS"\) not supported by charm, supported series are "precise, trusty, xenial, yakkety"`)
}

func (s *StateSuite) TestAllApplications(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(applications), gc.Equals, 0)

	// Check that after adding applications the result is ok.
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	applications, err = s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(applications), gc.Equals, 1)

	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "mysql", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	applications, err = s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(applications, gc.HasLen, 2)

	// Check the returned application, order is defined by sorted keys.
	names := make([]string, len(applications))
	for i, app := range applications {
		names[i] = app.Name()
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
		summary: "unknown application",
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
		summary: "implicit relations can be chosen explicitly",
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
	s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	loggingCh := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "lg", loggingCh)
	s.AddTestingApplication(c, "lg2", loggingCh)
	riak := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "rk1", riak)
	s.AddTestingApplication(c, "rk2", riak)
	s.AddTestingApplication(c, "lg-p", s.AddTestingCharm(c, "logging-principal"))

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
	alive := s.model

	// Dying model...
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	// Add a application so Destroy doesn't advance to Dead.
	app := factory.NewFactory(st1, s.StatePool).MakeApplication(c, nil)
	dying, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Add an empty model, destroy and remove it; we should
	// never see it reported.
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model2.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	err = st2.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	// All except the removed model are reported in initial event.
	w := s.State.WatchModels()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent(alive.UUID(), dying.UUID())

	// Progress dying to dead, alive to dying; and see changes reported.
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(alive.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(alive.Refresh(), jc.ErrorIsNil)
	c.Assert(alive.Life(), gc.Equals, state.Dying)
	c.Assert(dying.Refresh(), jc.Satisfies, errors.IsNotFound)
	wc.AssertChangeInSingleEvent(alive.UUID())
}

func (s *StateSuite) TestWatchModelsLifecycle(c *gc.C) {
	// Initial event reports the controller model.
	w := s.State.WatchModelLives()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(s.State.ModelUUID())
	wc.AssertNoChange()

	// Add a non-empty model: reported.
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	app := factory.NewFactory(st1, s.StatePool).MakeApplication(c, nil)
	model, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()

	// Remove the model: reported.
	c.Assert(app.Destroy(), jc.ErrorIsNil)
	c.Assert(st1.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), jc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()
	c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestWatchApplicationsBulkEvents(c *gc.C) {
	// Alive application...
	dummyCharm := s.AddTestingCharm(c, "dummy")
	alive := s.AddTestingApplication(c, "application0", dummyCharm)

	// Dying application...
	dying := s.AddTestingApplication(c, "application1", dummyCharm)
	keepDying, err := dying.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Dead application (actually, gone, Dead == removed in this case).
	gone := s.AddTestingApplication(c, "application2", dummyCharm)
	err = gone.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// All except gone are reported in initial event.
	w := s.State.WatchApplications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported.
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = keepDying.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(), jc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchApplicationsLifecycle(c *gc.C) {
	// Initial event is empty when no applications.
	w := s.State.WatchApplications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a application: reported.
	application := s.AddTestingApplication(c, "application", s.AddTestingCharm(c, "dummy"))
	wc.AssertChange("application")
	wc.AssertNoChange()

	// Change the application: not reported.
	keepDying, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	c.Assert(application.Destroy(), jc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()

	c.Assert(application.Refresh(), jc.ErrorIsNil)
	c.Check(application.Life(), gc.Equals, state.Dying)

	// Make it Dead(/removed): reported.
	c.Assert(keepDying.Destroy(), jc.ErrorIsNil)
	needs, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(needs, jc.IsTrue)
	c.Assert(s.State.Cleanup(), jc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchApplicationsDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.lifecycleWatcher,
	// which is also used by:
	//     State.WatchModels
	//     Application.WatchUnits
	//     Application.WatchRelations
	//     Machine.WatchContainers
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchApplications()
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
	err = dying.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
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

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
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
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
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
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := machine.WatchHardwareCharacteristics()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Provision a machine: reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Alter the machine: not reported.
	vers := version.MustParseBinary("1.2.3-quantal-ppc")
	err = machine.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchControllerConfig(c *gc.C) {
	w := s.State.WatchControllerConfig()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg := testing.FakeControllerConfig()
	c.Assert(cfg, jc.DeepEquals, expectedCfg)

	settings := state.GetControllerSettings(s.State)
	settings.Set("model-logs-size", "5M")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()

	cfg, err = s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg["model-logs-size"] = "5M"
	c.Assert(cfg, jc.DeepEquals, expectedCfg)
}

func (s *StateSuite) insertFakeModelDocs(c *gc.C, st *state.State) string {
	// insert one doc for each multiModelCollection
	var ops []mgotxn.Op
	modelUUID := st.ModelUUID()
	for _, collName := range state.MultiModelCollections() {
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
	for _, collName := range state.MultiModelCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"model-uuid", st.ModelUUID()}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Not(gc.Equals), 0)
	}

	// Add a model user whose permissions should get removed
	// when the model is.
	_, err = s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("amelia@external"),
			CreatedBy: s.Owner,
			Access:    permission.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	return state.UserModelNameIndex(s.model.Owner().Id(), s.model.Name())
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
	c.Assert(err, gc.ErrorMatches, `model "`+st.ModelUUID()+`" not found`)

	// ensure all docs for all MultiModelCollections are removed
	for _, collName := range state.MultiModelCollections() {
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

func (s *StateSuite) TestRemoveModel(c *gc.C) {
	st := s.State

	userModelKey := s.insertFakeModelDocs(c, st)
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: true})

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetDead()
	c.Assert(err, jc.ErrorIsNil)

	cloud, err := s.State.Cloud(model.Cloud())
	c.Assert(err, jc.ErrorIsNil)
	refCount, err := state.CloudModelRefCount(st, cloud.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(refCount, gc.Equals, 1)

	err = st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	cloud, err = s.State.Cloud(model.Cloud())
	c.Assert(err, jc.ErrorIsNil)
	_, err = state.CloudModelRefCount(st, cloud.Name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
}

func (s *StateSuite) TestRemoveDyingModelAliveModelFails(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	err := st.RemoveDyingModel()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "can't remove model: model still alive")
}

func (s *StateSuite) TestRemoveDyingModelForDyingModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(st.SetDyingModelToDead(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), jc.DeepEquals, state.Dead)

	c.Assert(st.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestRemoveDyingModelForDeadModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), jc.DeepEquals, state.Dying)

	c.Assert(st.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestSetDyingModelToDeadRequiresDyingModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetDyingModelToDead()
	c.Assert(errors.Cause(err), gc.Equals, state.ErrModelNotDying)

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), jc.DeepEquals, state.Dying)
	c.Assert(st.SetDyingModelToDead(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), jc.DeepEquals, state.Dead)

	err = st.SetDyingModelToDead()
	c.Assert(errors.Cause(err), gc.Equals, state.ErrModelNotDying)
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
	c.Assert(state.HostedModelCount(c, st), gc.Equals, 1)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = m.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	err = s.model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// remove suite state
	err = s.State.RemoveImportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, st), gc.Equals, 0)
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

	err = s.model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveExportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsRemovesLogs(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	writeLogs(c, st, 5)
	writeLogs(c, s.State, 5)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	assertLogCount(c, s.State, 5)
	assertLogCount(c, st, 0)
}

func (s *StateSuite) TestRemoveImportingModelDocsRemovesLogs(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	writeLogs(c, st, 5)
	writeLogs(c, s.State, 5)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	assertLogCount(c, s.State, 5)
	assertLogCount(c, st, 0)
}

func (s *StateSuite) TestRemoveModelRemovesLogs(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetDead()
	c.Assert(err, jc.ErrorIsNil)

	writeLogs(c, st, 5)
	writeLogs(c, s.State, 5)

	err = st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	assertLogCount(c, s.State, 5)
	assertLogCount(c, st, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsRemovesLogTrackers(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	t1 := state.NewLastSentLogTracker(st, model.UUID(), "go-away")
	defer t1.Close()
	t2 := state.NewLastSentLogTracker(st, s.State.ModelUUID(), "stay")
	defer t2.Close()

	c.Assert(t1.Set(100, 100), jc.ErrorIsNil)
	c.Assert(t2.Set(100, 100), jc.ErrorIsNil)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = t1.Get()
	c.Check(errors.Cause(err), gc.Equals, state.ErrNeverForwarded)

	id, count, err := t2.Get()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(id, gc.Equals, int64(100))
	c.Check(count, gc.Equals, int64(100))
}

func writeLogs(c *gc.C, st *state.State, n int) {
	dbLogger := state.NewDbLogger(st)
	defer dbLogger.Close()
	for i := 0; i < n; i++ {
		err := dbLogger.Log([]state.LogRecord{{
			Time:     time.Now(),
			Entity:   "application-van-occupanther",
			Module:   "chasing after deer",
			Location: "in a log house",
			Level:    loggo.INFO,
			Message:  "why are your fingers like that of a hedge in winter?",
		}})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func assertLogCount(c *gc.C, st *state.State, expected int) {
	logColl := st.MongoSession().DB("logs").C("logs." + st.ModelUUID())
	actual, err := logColl.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, gc.Equals, expected)
}

type attrs map[string]interface{}

func (s *StateSuite) TestWatchForModelConfigChanges(c *gc.C) {
	cur := jujuversion.Current
	err := statetesting.SetAgentVersion(s.State, cur)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.model.WatchForModelConfigChanges()
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
	w := s.model.WatchForModelConfigChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
}

func (s *StateSuite) TestWatchCloudSpecChanges(c *gc.C) {
	w := s.model.WatchCloudSpecChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	// Initially we get one change notification
	wc.AssertOneChange()

	cloud, err := s.State.Cloud(s.Model.Cloud())
	c.Assert(err, jc.ErrorIsNil)

	// Multiple changes will only result in a single change notification
	cloud.StorageEndpoint = "https://storage"
	err = s.State.UpdateCloud(cloud)
	c.Assert(err, jc.ErrorIsNil)
	cloud.StorageEndpoint = "https://storage1"
	err = s.State.UpdateCloud(cloud)
	c.Assert(err, jc.ErrorIsNil)
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

	wordpress1 := s.AddTestingApplication(c, "wordpress", charm1)
	wordpress2, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress1, jc.DeepEquals, wordpress2)

	unit1, err := wordpress1.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unit2, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit1, jc.DeepEquals, unit2)

	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
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
	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	if err != nil {
		return err
	}
	defer session.Close()
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
	})
	if err == nil {
		err = pool.Close()
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
	tag: names.NewRelationTag("app1:rel1 app2:rel2"),
	err: `relation "app1:rel1 app2:rel2" not found`,
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
	app := s.AddTestingApplication(c, "ser-vice2", s.AddTestingCharm(c, "mysql"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeUser(c, &factory.UserParams{Name: "arble"})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "ser-vice2")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db ser-vice2:server")

	findEntityTests = append([]findEntityTest{}, findEntityTests...)
	findEntityTests = append(findEntityTests, findEntityTest{
		tag: names.NewModelTag(s.model.UUID()),
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
				c.Assert(e.Tag(), gc.Equals, s.model.Tag())
			} else if kind == names.UserTagKind {
				// Test the fully qualified username rather than the tag structure itself.
				expected := test.tag.(names.UserTag).Id()
				c.Assert(e.Tag().(names.UserTag).Id(), gc.Equals, expected)
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
	app := s.AddTestingApplication(c, "ser-vice2", s.AddTestingCharm(c, "dummy"))
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, app.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "applications")
	c.Assert(id, gc.Equals, state.DocID(s.State, app.Name()))
}

func (s *StateSuite) TestParseUnitTag(c *gc.C) {
	app := s.AddTestingApplication(c, "application2", s.AddTestingCharm(c, "dummy"))
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, u.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "units")
	c.Assert(id, gc.Equals, state.DocID(s.State, u.Name()))
}

func (s *StateSuite) TestParseActionTag(c *gc.C) {
	app := s.AddTestingApplication(c, "application2", s.AddTestingCharm(c, "dummy"))
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	f, err := u.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)

	action, err := s.model.Action(f.Id())
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
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, s.model.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "models")
	c.Assert(id, gc.Equals, s.model.UUID())
}

func (s *StateSuite) TestWatchCleanups(c *gc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Set up two relations for later use, check no events.
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "varnish", s.AddTestingCharm(c, "varnish"))
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
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
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

	// Create two peer relations by creating their applications.
	riak := s.AddTestingApplication(c, "riak", s.AddTestingCharm(c, "riak"))
	_, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	allHooks := s.AddTestingApplication(c, "all-hooks", s.AddTestingCharm(c, "all-hooks"))
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

	// Set up applications for later use.
	wordpress := s.AddTestingApplication(c,
		"wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpressName := wordpress.Name()

	// Add application units for later use.
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wordpress1, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// No events should occur.
	wc.AssertNoChange()

	// Add minimum units to a application; a single change should occur.
	err = wordpress.SetMinUnits(2)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Decrease minimum units for a application; expect no changes.
	err = wordpress.SetMinUnits(1)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Increase minimum units for two applications; a single change should occur.
	err = mysql.SetMinUnits(1)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.SetMinUnits(3)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(mysql.Name(), wordpressName)
	wc.AssertNoChange()

	// Remove minimum units for a application; expect no changes.
	err = mysql.SetMinUnits(0)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a unit of a application with required minimum units.
	// Also avoid the unit removal. A single change should occur.
	preventUnitDestroyRemove(c, wordpress0)
	err = wordpress0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Two actions: destroy a unit and increase minimum units for a application.
	// A single change should occur, and the application name should appear only
	// one time in the change.
	err = wordpress.SetMinUnits(5)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Destroy a unit of a application not requiring minimum units; expect no changes.
	err = mysql0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a application with required minimum units; expect no changes.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a application not requiring minimum units; expect no changes.
	err = mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchMinUnitsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchMinUnits()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchSubnets(c *gc.C) {
	filter := func(id interface{}) bool {
		return id != "10.20.0.0/24"
	}
	w := s.State.WatchSubnets(filter)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	// Check initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	_, err := s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "10.20.0.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "10.0.0.0/24"})
	wc.AssertChange("10.0.0.0/24")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchSubnetsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchSubnets(nil)
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) setupWatchRemoteRelations(c *gc.C, wc statetesting.StringsWatcherC) (*state.RemoteApplication, *state.Application, *state.Relation) {
	// Check initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: s.Model.ModelTag(),
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Add a remote relation, single change should occur.
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteApp.Refresh(), jc.ErrorIsNil)
	c.Assert(app.Refresh(), jc.ErrorIsNil)

	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()
	return remoteApp, app, rel
}

func (s *StateSuite) TestWatchRemoteRelationsIgnoresLocal(c *gc.C) {
	// Set up a non-remote relation to ensure it is properly filtered out.
	s.AddTestingApplication(c, "wplocal", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysqllocal", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wplocal", "mysqllocal")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	// Check initial event.
	wc.AssertChange()
	// No change for local relation.
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyRelation(c *gc.C) {
	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	_, _, rel := s.setupWatchRemoteRelations(c, wc)

	// Destroy the remote relation.
	// A single change should occur.
	err := rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyRemoteApplication(c *gc.C) {
	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	remoteApp, _, _ := s.setupWatchRemoteRelations(c, wc)

	// Destroy the remote application.
	// A single change should occur.
	err := remoteApp.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyLocalApplication(c *gc.C) {
	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	_, app, _ := s.setupWatchRemoteRelations(c, wc)

	// Destroy the local application.
	// A single change should occur.
	err := app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchRemoteRelations()
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

func (s *StateSuite) TestSetModelAgentVersionErrors(c *gc.C) {
	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
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
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
	expectErr := fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Add a application and 4 units: one with a different version, one
	// with an empty version, one with the current version, and one
	// with the new version.
	application, err := s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	unit0, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit0.SetAgentVersion(version.MustParseBinary("6.6.6-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unit2, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit2.SetAgentVersion(version.MustParseBinary(stringVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	unit3, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit3.SetAgentVersion(version.MustParseBinary("4.5.6-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// Verify unit0 and unit1 are reported as error, along with the
	// machines from before.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
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
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)
}

func (s *StateSuite) prepareAgentVersionTests(c *gc.C, st *state.State) (*config.Config, string) {
	// Get the agent-version set in the model.
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine and a unit with the current version.
	machine, err := st.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application, err := st.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetAgentVersion(version.MustParseBinary(currentVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	return modelConfig, currentVersion
}

func (s *StateSuite) changeEnviron(c *gc.C, modelConfig *config.Config, name string, value interface{}) {
	attrs := modelConfig.AllAttrs()
	attrs[name] = value
	c.Assert(s.Model.UpdateModelConfig(attrs, nil), gc.IsNil)
}

func assertAgentVersion(c *gc.C, st *state.State, vers string) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion.String(), gc.Equals, vers)
}

func (s *StateSuite) TestSetModelAgentVersionRetriesOnConfigChange(c *gc.C) {
	modelConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change something
	// other than the version, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, modelConfig, "default-series", "foo")
	}).Check()

	// Change the agent-version and ensure it has changed.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6")
}

func (s *StateSuite) TestSetModelAgentVersionSucceedsWithSameVersion(c *gc.C) {
	modelConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change the version
	// to the new one, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, modelConfig, "agent-version", "4.5.6")
	}).Check()

	// Change the agent-version and verify.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6")
}

func (s *StateSuite) TestSetModelAgentVersionOnOtherModel(c *gc.C) {
	current := version.MustParseBinary("1.24.7-trusty-amd64")
	s.PatchValue(&jujuversion.Current, current.Number)
	s.PatchValue(&arch.HostArch, func() string { return current.Arch })
	s.PatchValue(&series.MustHostSeries, func() string { return current.Series })

	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	higher := version.MustParseBinary("1.25.0-trusty-amd64")
	lower := version.MustParseBinary("1.24.6-trusty-amd64")

	// Set other model version to < controller model version
	err := otherSt.SetModelAgentVersion(lower.Number, false)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, lower.Number.String())

	// Set other model version == controller version
	err = otherSt.SetModelAgentVersion(jujuversion.Current, false)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, jujuversion.Current.String())

	// Set other model version to > server version
	err = otherSt.SetModelAgentVersion(higher.Number, false)
	expected := fmt.Sprintf("model cannot be upgraded to %s while the controller is %s: upgrade 'controller' model first",
		higher.Number,
		jujuversion.Current,
	)
	c.Assert(err, gc.ErrorMatches, expected)
}

func (s *StateSuite) TestSetModelAgentVersionExcessiveContention(c *gc.C) {
	modelConfig, currentVersion := s.prepareAgentVersionTests(c, s.State)

	// Set a hook to change the config 3 times
	// to test we return ErrExcessiveContention.
	changeFuncs := []func(){
		func() { s.changeEnviron(c, modelConfig, "default-series", "1") },
		func() { s.changeEnviron(c, modelConfig, "default-series", "2") },
		func() { s.changeEnviron(c, modelConfig, "default-series", "3") },
	}
	defer state.SetBeforeHooks(c, s.State, changeFuncs...).Check()
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
	c.Assert(errors.Cause(err), gc.Equals, txn.ErrExcessiveContention)
	// Make sure the version remained the same.
	assertAgentVersion(c, s.State, currentVersion)
}

func (s *StateSuite) TestSetModelAgentVersionMixedVersions(c *gc.C) {
	_, currentVersion := s.prepareAgentVersionTests(c, s.State)
	machine, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	// Force this to something old that should not match current versions
	err = machine.SetAgentVersion(version.MustParseBinary("1.0.1-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	// This should be refused because an agent doesn't match "currentVersion"
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), false)
	c.Check(err, gc.ErrorMatches, "some agents have not upgraded to the current model version .*: machine-0")
	// Version hasn't changed
	assertAgentVersion(c, s.State, currentVersion)
	// But we can force it
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), true)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6")
}

func (s *StateSuite) TestSetModelAgentVersionFailsIfUpgrading(c *gc.C) {
	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	_, err = s.State.EnsureUpgradeInfo(machine.Tag().Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetModelAgentVersion(nextVersion, false)
	c.Assert(err, jc.Satisfies, state.IsUpgradeInProgressError)
}

func (s *StateSuite) TestSetModelAgentVersionFailsReportsCorrectError(c *gc.C) {
	// Ensure that the correct error is reported if an upgrade is
	// progress but that isn't the reason for the
	// SetModelAgentVersion call failing.

	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary("9.9.9-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	_, err = s.State.EnsureUpgradeInfo(machine.Tag().Id(), agentVersion, nextVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetModelAgentVersion(nextVersion, false)
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
// unexpected error (often "Closed explicitly").
func testWatcherDiesWhenStateCloses(
	c *gc.C,
	session *mgo.Session,
	modelTag names.ModelTag,
	controllerTag names.ControllerTag,
	startWatcher func(c *gc.C, st *state.State) waiter,
) {
	st, err := state.Open(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
	})
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

func (s *StateSuite) TestNowToTheSecond(c *gc.C) {
	t := state.NowToTheSecond(s.State)
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

func (s *StateSuite) TestRunTransactionObserver(c *gc.C) {
	type args struct {
		dbName    string
		modelUUID string
		ops       []mgotxn.Op
		err       error
	}
	var mu sync.Mutex
	var recordedCalls []args
	getCalls := func() []args {
		mu.Lock()
		defer mu.Unlock()
		return recordedCalls[:]
	}

	params := s.testOpenParams()
	params.RunTransactionObserver = func(dbName, modelUUID string, ops []mgotxn.Op, err error) {
		mu.Lock()
		defer mu.Unlock()
		recordedCalls = append(recordedCalls, args{
			dbName:    dbName,
			modelUUID: modelUUID,
			ops:       ops,
			err:       err,
		})
	}
	st, err := state.Open(params)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(getCalls(), gc.HasLen, 0)

	err = st.SetModelConstraints(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	calls := getCalls()
	// There may be some leadership txns in the call list.
	// We only care about the constraints call.
	found := false
	for _, call := range calls {
		if call.ops[0].C != "constraints" {
			continue
		}
		c.Check(call.dbName, gc.Equals, "juju")
		c.Check(call.modelUUID, gc.Equals, s.modelTag.Id())
		c.Check(call.err, gc.IsNil)
		c.Check(call.ops, gc.HasLen, 1)
		c.Check(call.ops[0].Update, gc.NotNil)
		found = true
		break
	}
	c.Assert(found, jc.IsTrue)
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
	err := inst.Start(nil)
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
			Addrs:      []string{inst.Addr()},
			CACert:     testing.CACert,
			DisableTLS: true,
		},
	}

	session, err := mongo.DialWithInfo(mongo.MongoInfo{
		Info:     noAuthInfo.Info,
		Tag:      owner,
		Password: password,
	}, mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	cfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  session,
		AdminPassword: password,
	})
	c.Assert(err, jc.ErrorIsNil)
	st := ctlr.SystemState()
	defer ctlr.Close()

	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetAdminMongoPassword("foo")
	c.Assert(err, jc.ErrorIsNil)
	err = st.MongoSession().DB("admin").Login("admin", "foo")
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = tryOpenState(m.ModelTag(), st.ControllerTag(), noAuthInfo)
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
	err = tryOpenState(m.ModelTag(), st.ControllerTag(), &passwordOnlyInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) setUpWatchRelationNetworkScenario(c *gc.C) *state.Relation {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: s.Model.ModelTag(),
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wpCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	return rel
}

func (s *StateSuite) TestWatchRelationIngressNetworks(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationIngressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Initial ingress network creation.
	relIngress := state.NewRelationIngressNetworks(s.State)
	_, err := relIngress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32", "4.3.2.1/16")
	wc.AssertNoChange()

	// Update value.
	_, err = relIngress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32")
	wc.AssertNoChange()

	// Update value, admin override.
	_, err = relIngress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("10.0.0.1/32")
	wc.AssertNoChange()

	// Same value.
	_, err = relIngress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Delete relation.
	state.RemoveRelation(c, rel, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange()
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationIngressNetworksIgnoresEgress(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationIngressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationEgressNetworks(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationEgressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Initial egress network creation.
	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32", "4.3.2.1/16")
	wc.AssertNoChange()

	// Update value.
	_, err = relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32")
	wc.AssertNoChange()

	// Update value, admin override.
	_, err = relEgress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("10.0.0.1/32")
	wc.AssertNoChange()

	// Same value.
	_, err = relEgress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Delete relation.
	state.RemoveRelation(c, rel, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange()
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationEgressNetworksIgnoresIngress(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationEgressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	relEgress := state.NewRelationIngressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) testOpenParams() state.OpenParams {
	return state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.modelTag,
		MongoSession:       s.Session,
	}
}

func (s *StateSuite) TestControllerTimestamp(c *gc.C) {
	now := testing.NonZeroTime()
	clock := testclock.NewClock(now)

	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.State.ControllerTimestamp()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.NotNil)

	c.Assert(*got, jc.DeepEquals, now)
}
