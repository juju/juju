// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/juju/charm/v13"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	mgotxn "github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/upgrade"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/mongo/mongotest"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/mocks"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

var goodPassword = "foo-12345678901234567890"
var alternatePassword = "bar-12345678901234567890"

var defaultInstancePrechecker = state.NoopInstancePrechecker{}

// preventUnitDestroyRemove sets a non-allocating status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *gc.C, u *state.Unit) {
	// To have a non-allocating status, a unit needs to
	// be assigned to a machine.
	_, err := u.AssignedMachineId()
	if errors.Is(err, errors.NotAssigned) {
		err = u.AssignToNewMachine(defaultInstancePrechecker)
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

	upgrader *mocks.MockUpgrader
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

func (s *StateSuite) TestParseIDToTag(c *gc.C) {
	model := "42c4f770-86ed-4fcc-8e39-697063d082bc:e"
	machine := "42c4f770-86ed-4fcc-8e39-697063d082bc:m#0"
	application := "c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql"
	unit := "c9741ea1-0c2a-444d-82f5-787583a48557:u#mysql/0"
	moTag := state.TagFromDocID(model)
	maTag := state.TagFromDocID(machine)
	unTag := state.TagFromDocID(unit)
	apTag := state.TagFromDocID(application)

	tag, err := names.ParseTag(moTag.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag.String(), gc.Equals, moTag.String())

	tag, err = names.ParseTag(maTag.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag.String(), gc.Equals, maTag.String())

	tag, err = names.ParseTag(unTag.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag.String(), gc.Equals, unTag.String())

	tag, err = names.ParseTag(apTag.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag.String(), gc.Equals, apTag.String())

	c.Assert(moTag.String(), gc.Equals, "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e")
	c.Assert(maTag.String(), gc.Equals, "machine-0")
	c.Assert(unTag.String(), gc.Equals, "unit-mysql-0")
	c.Assert(apTag.String(), gc.Equals, "application-mysql")
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

func (s *StateSuite) TestOpenControllerRequiresExtantModelTag(c *gc.C) {
	uuid := uuid.MustNewUUID()
	params := s.testOpenParams()
	params.ControllerModelTag = names.NewModelTag(uuid.String())
	controller, err := state.OpenController(params)
	if !c.Check(controller, gc.IsNil) {
		c.Check(controller.Close(), jc.ErrorIsNil)
	}
	expect := fmt.Sprintf("cannot read model %s: model %q not found", uuid, uuid)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *StateSuite) TestOpenControllerSetsModelTag(c *gc.C) {
	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, jc.ErrorIsNil)
	defer controller.Close()

	sysState, err := controller.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	m, err := sysState.Model()
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
				c.Assert(err, jc.ErrorIsNil)
				_, err = st.AddMachineInsideMachine(
					state.MachineTemplate{
						Base: state.UbuntuBase("22.04"),
						Jobs: []state.MachineJob{state.JobHostUnits},
					},
					m.Id(),
					instance.LXD,
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
						Base: state.UbuntuBase("22.04"),
						Jobs: []state.MachineJob{state.JobHostUnits},
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

				unitPortRange, err := u.OpenedPortRanges()
				c.Assert(err, jc.ErrorIsNil)
				unitPortRange.Open(allEndpoints, network.MustParsePortRange("100-200/tcp"))
				c.Assert(st.ApplyOperation(unitPortRange.Changes()), jc.ErrorIsNil)
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
				err = r.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
				c.Assert(err, jc.ErrorIsNil)
				loggo.GetLogger("juju.state").SetLogLevel(loggo.DEBUG)
				return true
			},
			triggerEvent: func(st *state.State) {
				loggo.GetLogger("juju.state").SetLogLevel(loggo.TRACE)
				err := st.Cleanup(context.Background(), state.NewObjectStore(c, st.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
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
			about: "statuses",
			getWatcher: func(st *state.State) interface{} {
				m, err := st.AddMachine(defaultInstancePrechecker, state.UbuntuBase("22.04"), state.JobHostUnits)
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
				err = m.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
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
				return unit.WatchPendingActionNotifications()
			},
			triggerEvent: func(st *state.State) {
				unit, err := st.Unit("dummy/0")
				c.Assert(err, jc.ErrorIsNil)
				m, err := st.Model()
				c.Assert(err, jc.ErrorIsNil)
				operationID, err := m.EnqueueOperation("a test", 1)
				c.Assert(err, jc.ErrorIsNil)
				_, err = m.AddAction(unit, operationID, "snapshot", nil, nil, nil)
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
				_, err := st.AddSubnet(network.SubnetInfo{
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
					wc = statetesting.NewStringsWatcherC(c, w)
					swc := wc.(statetesting.StringsWatcherC)
					// consume initial event
					swc.AssertChange()
					swc.AssertNoChange()
				case statetesting.NotifyWatcher:
					wc = statetesting.NewNotifyWatcherC(c, w)
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
		workertest.CleanKill(tw.c, wc.Watcher)
	case statetesting.NotifyWatcherC:
		workertest.CleanKill(tw.c, wc.Watcher)
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (s *StateSuite) TestAddresses(c *gc.C) {
	var err error
	machines := make([]*state.Machine, 4)
	machines[0], err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machines[1], err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	node, err := s.State.ControllerNode(machines[0].Id())
	c.Assert(err, jc.ErrorIsNil)
	err = node.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)

	changes, _, err := s.State.EnableHA(defaultInstancePrechecker, 3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.DeepEquals, []string{"2", "3"})
	c.Assert(changes.Maintained, gc.DeepEquals, []string{machines[0].Id()})

	machines[2], err = s.State.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	machines[3], err = s.State.Machine("3")
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig := testing.FakeControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	for i, m := range machines {
		err := m.SetProviderAddresses(
			controllerConfig,
			network.NewSpaceAddress(fmt.Sprintf("10.0.0.%d", i), network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("::1", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
			network.NewSpaceAddress("5.4.3.2", network.WithScope(network.ScopePublic)),
		)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *StateSuite) TestPing(c *gc.C) {
	c.Assert(s.State.Ping(), gc.IsNil)
	mgotesting.MgoServer.Restart()
	c.Assert(s.State.Ping(), gc.NotNil)
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
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.Base{})
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no base specified")
	_, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"))
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no jobs specified")
	_, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: duplicate job: .*")
}

func (s *StateSuite) TestAddMachine(c *gc.C) {
	allJobs := []state.MachineJob{
		state.JobHostUnits,
		state.JobManageModel,
	}
	m0, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), allJobs...)
	c.Assert(err, jc.ErrorIsNil)
	check := func(m *state.Machine, id string, base state.Base, jobs []state.MachineJob) {
		c.Assert(m.Id(), gc.Equals, id)
		c.Assert(m.Base().String(), gc.Equals, base.String())
		c.Assert(m.Jobs(), gc.DeepEquals, jobs)
		s.assertMachineContainers(c, m, nil)
	}
	check(m0, "0", state.UbuntuBase("12.10"), allJobs)
	m0, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	check(m0, "0", state.UbuntuBase("12.10"), allJobs)

	oneJob := []state.MachineJob{state.JobHostUnits}
	m1, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("22.04"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	check(m1, "1", state.UbuntuBase("22.04"), oneJob)

	m1, err = s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	check(m1, "1", state.UbuntuBase("22.04"), oneJob)

	m, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.HasLen, 2)
	check(m[0], "0", state.UbuntuBase("12.10"), allJobs)
	check(m[1], "1", state.UbuntuBase("22.04"), oneJob)

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	_, err = st2.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: controller jobs specified but not allowed")
}

func (s *StateSuite) TestAddMachines(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")
	hc := instance.MustParseHardware("mem=2G")
	machineTemplate := state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		DisplayName:             "test-display-name",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
	}
	machines, err := s.State.AddMachines(defaultInstancePrechecker, machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.CheckProvisioned("nonce"), jc.IsTrue)
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, cons)
	mhc, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*mhc, gc.DeepEquals, hc)
	instId, instDN, err := m.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(instId), gc.Equals, "inst-id")
	c.Assert(instDN, gc.Equals, "test-display-name")
}

func (s *StateSuite) TestAddMachinesModelDying(c *gc.C) {
	err := s.Model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testmodel" is dying`)
}

func (s *StateSuite) TestAddMachinesModelDyingAfterInitial(c *gc.C) {
	// Check that machines cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.Model.Life(), gc.Equals, state.Alive)
		c.Assert(s.Model.Destroy(state.DestroyModelParams{}), gc.IsNil)
	}).Check()
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testmodel" is dying`)
}

func (s *StateSuite) TestAddMachinesModelMigrating(c *gc.C) {
	err := s.Model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, `cannot add a new machine: model "testmodel" is being migrated`)
}

func (s *StateSuite) TestAddMachineExtraConstraints(c *gc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cores=4")
	m, err := s.State.AddOneMachine(defaultInstancePrechecker, state.MachineTemplate{
		DisplayName: "test-display-name",
		Base:        state.UbuntuBase("12.10"),
		Constraints: extraCons,
		Jobs:        oneJob,
		Nonce:       "nonce",
		InstanceId:  "inst-id",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("cores=4 mem=4G")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, expectedCons)
	m, err = s.State.Machine(m.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.CheckProvisioned("nonce"), jc.IsTrue)
	_, instDN, err := m.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instDN, gc.Equals, "test-display-name")
}

func (s *StateSuite) TestAddMachinePlacementIgnoresModelConstraints(c *gc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G tags=foo"))
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	m, err := s.State.AddOneMachine(defaultInstancePrechecker, state.MachineTemplate{
		DisplayName: "test-display-name",
		Base:        state.UbuntuBase("12.10"),
		Jobs:        oneJob,
		Placement:   "theplacement",
		Nonce:       "nonce",
		InstanceId:  "inst-id",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.Placement(), gc.Equals, "theplacement")
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, expectedCons)
	m, err = s.State.Machine(m.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.CheckProvisioned("nonce"), jc.IsTrue)
	_, instDN, err := m.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instDN, gc.Equals, "test-display-name")
}

func (s *StateSuite) TestAddMachineWithVolumes(c *gc.C) {
	s.policy.Providers = map[string]domainstorage.StoragePoolDetails{
		"loop-pool": {Name: "loop-pool", Provider: "loop"},
	}

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
		Base:                    state.UbuntuBase("12.10"),
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		DisplayName:             "test-display-name",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
		Volumes: []state.HostVolumeParams{{
			volume0, volumeAttachment0,
		}, {
			volume1, volumeAttachment1,
		}},
	}
	machines, err := s.State.AddMachines(defaultInstancePrechecker, machineTemplate)
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
		c.Assert(err, jc.ErrorIs, errors.NotProvisioned)
		attachmentParams, ok := att.Params()
		c.Assert(ok, jc.IsTrue)
		c.Check(attachmentParams, gc.Equals, machineTemplate.Volumes[i].Attachment)
		volume, err := sb.Volume(att.Volume())
		c.Assert(err, jc.ErrorIsNil)
		_, err = volume.Info()
		c.Assert(err, jc.ErrorIs, errors.NotProvisioned)
		volumeParams, ok := volume.Params()
		c.Assert(ok, jc.IsTrue)
		c.Check(volumeParams, gc.Equals, machineTemplate.Volumes[i].Volume)
	}
	instId, instDN, err := m.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(instId), gc.Equals, "inst-id")
	c.Assert(instDN, gc.Equals, "test-display-name")
}

func (s *StateSuite) assertMachineContainers(c *gc.C, m *state.Machine, containers []string) {
	mc, err := m.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mc, gc.DeepEquals, containers)
}

func (s *StateSuite) TestAddContainerToNewMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: oneJob,
	}
	parentTemplate := state.MachineTemplate{
		Base: state.UbuntuBase("20.04"),
		Jobs: oneJob,
	}
	m, err := s.State.AddMachineInsideNewMachine(defaultInstancePrechecker, template, parentTemplate, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxd/0")
	c.Assert(m.Base().DisplayString(), gc.Equals, "ubuntu@12.10")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)

	m, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, []string{"0/lxd/0"})
	c.Assert(m.Base().DisplayString(), gc.Equals, "ubuntu@20.04")

	m, err = s.State.Machine("0/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineContainers(c, m, nil)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
}

func (s *StateSuite) TestAddContainerToExistingMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	m1, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)

	// Add first container.
	m, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxd/0")
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
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
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxd/1")
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0", "1/lxd/1"})
}

func (s *StateSuite) TestAddContainerToMachineWithKnownSupportedContainers(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxd/0")
	s.assertMachineContainers(c, host, []string{"0/lxd/0"})
}

func (s *StateSuite) TestAddInvalidContainerToMachineWithKnownSupportedContainers(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.ContainerType("abc"))
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host abc containers")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestAddContainerToMachineSupportingNoContainers(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxd containers")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestAddContainerToMachineLockedForSeriesUpgrade(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, jc.ErrorIsNil)
	err = host.CreateUpgradeSeriesLock(nil, state.UbuntuBase("18.04"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 is locked for series upgrade")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestInvalidAddMachineParams(c *gc.C) {
	instIdTemplate := state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "i-foo",
	}
	normalTemplate := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(instIdTemplate, "0", instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(defaultInstancePrechecker, instIdTemplate, normalTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(defaultInstancePrechecker, normalTemplate, instIdTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddOneMachine(defaultInstancePrechecker, instIdTemplate)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")

	_, err = s.State.AddOneMachine(defaultInstancePrechecker, state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobHostUnits},
		InstanceId: "i-foo",
		Nonce:      "nonce",
	})
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("cannot add a new machine: duplicate job: %s", state.JobHostUnits))

	noSeriesTemplate := state.MachineTemplate{
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobHostUnits},
	}
	_, err = s.State.AddOneMachine(defaultInstancePrechecker, noSeriesTemplate)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no base specified")

	_, err = s.State.AddMachineInsideNewMachine(defaultInstancePrechecker, noSeriesTemplate, normalTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no base specified")

	_, err = s.State.AddMachineInsideNewMachine(defaultInstancePrechecker, normalTemplate, noSeriesTemplate, instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no base specified")

	_, err = s.State.AddMachineInsideMachine(noSeriesTemplate, "0", instance.LXD)
	c.Check(err, gc.ErrorMatches, "cannot add a new machine: no base specified")
}

func (s *StateSuite) TestAddContainerErrors(c *gc.C) {
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(template, "10", instance.LXD)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 10 not found")
	_, err = s.State.AddMachineInsideMachine(template, "10", "")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no container type specified")
}

func (s *StateSuite) TestInjectMachineErrors(c *gc.C) {
	injectMachine := func(base state.Base, instanceId instance.Id, nonce string, jobs ...state.MachineJob) error {
		_, err := s.State.AddOneMachine(defaultInstancePrechecker, state.MachineTemplate{
			Base:       base,
			Jobs:       jobs,
			InstanceId: instanceId,
			Nonce:      nonce,
		})
		return err
	}
	err := injectMachine(state.Base{}, "i-minvalid", agent.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no base specified")
	err = injectMachine(state.UbuntuBase("12.10"), "", agent.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: cannot specify a nonce without an instance id")
	err = injectMachine(state.UbuntuBase("12.10"), "i-minvalid", "", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")
	err = injectMachine(state.UbuntuBase("12.10"), agent.BootstrapNonce, "i-mlazy")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no jobs specified")
}

func (s *StateSuite) TestInjectMachine(c *gc.C) {
	cons := constraints.MustParse("mem=4G")
	arch := arch.DefaultArchitecture
	mem := uint64(1024)
	disk := uint64(1024)
	source := "loveshack"
	tags := []string{"foo", "bar"}
	template := state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
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
	m, err := s.State.AddOneMachine(defaultInstancePrechecker, template)
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
		Base:       state.UbuntuBase("12.10"),
		InstanceId: "i-mindustrious",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	}
	m0, err := s.State.AddOneMachine(defaultInstancePrechecker, template)
	c.Assert(err, jc.ErrorIsNil)

	// Add first container.
	template = state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxd/0")
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
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
	c.Assert(m.Base().String(), gc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXD)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxd/0", "0/lxd/1"})
}

func (s *StateSuite) TestAddMachineCanOnlyAddControllerForMachine0(c *gc.C) {
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobManageModel},
	}
	// Check that we can add the bootstrap machine.
	m, err := s.State.AddOneMachine(defaultInstancePrechecker, template)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	node, err := s.State.ControllerNode(m.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.HasVote(), jc.IsFalse)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageModel})

	// Check that the controller information is correct.
	controllerIds, err := s.State.ControllerIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerIds, gc.DeepEquals, []string{"0"})

	const errCannotAdd = "cannot add a new machine: controller jobs specified but not allowed"
	_, err = s.State.AddOneMachine(defaultInstancePrechecker, template)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)

	_, err = s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)

	_, err = s.State.AddMachineInsideNewMachine(defaultInstancePrechecker, template, template, instance.LXD)
	c.Assert(err, gc.ErrorMatches, errCannotAdd)
}

func (s *StateSuite) TestReadMachine(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, expectedId)
}

func (s *StateSuite) TestMachineNotFound(c *gc.C) {
	_, err := s.State.Machine("0")
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
	c.Assert(state.MachineIdLessThan("0/lxd/0", "0/lxd/0"), jc.IsFalse)
}

func (s *StateSuite) TestAllMachines(c *gc.C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%d", i)), "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetAgentVersion(version.MustParseBinary("7.8.9-ubuntu-amd64"))
		c.Assert(err, jc.ErrorIsNil)
		err = m.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
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
		c.Check(tools.Version, gc.DeepEquals, version.MustParseBinary("7.8.9-ubuntu-amd64"))
		c.Assert(m.Life(), gc.Equals, state.Dying)
	}
}

func (s *StateSuite) TestMachineCountForBase(c *gc.C) {
	add_machine := func(base state.Base) {
		m, err := s.State.AddMachine(defaultInstancePrechecker, base, state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%s", base.String())), "", "fake_nonce", nil)
		c.Check(err, jc.ErrorIsNil)
		err = m.SetAgentVersion(version.MustParseBinary("7.8.9-ubuntu-amd64"))
		c.Check(err, jc.ErrorIsNil)
		err = m.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
		c.Check(err, jc.ErrorIsNil)
	}

	var windowsSeries = []string{
		"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2",
		"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
	}
	windowsBases := make([]state.Base, len(windowsSeries))
	for i, s := range windowsSeries {
		windowsBases[i] = state.Base{OS: "windows", Channel: s}
	}
	expectedWinResult := map[string]int{}
	for _, winBase := range windowsBases {
		add_machine(winBase)
		expectedWinResult[winBase.String()] = 1
	}
	add_machine(state.UbuntuBase("12.10"))
	s.AssertMachineCount(c, len(windowsSeries)+1)

	result, err := s.State.MachineCountForBase(windowsBases...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expectedWinResult)

	result, err = s.State.MachineCountForBase(
		state.UbuntuBase("12.10"), // count 1
		state.UbuntuBase("16.04"), // count 0
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, map[string]int{"ubuntu@12.10": 1})
}

func (s *StateSuite) TestInferActiveRelations(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wp := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	_, err = wp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ms := s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	_, err = ms.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("wp", "ms:prod")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	relation, err := s.State.InferActiveRelation("wp", "ms")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relation, gc.Matches, "wp:db ms:prod")

	relation, err = s.State.InferActiveRelation("wp:db", "ms:prod")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relation, gc.Matches, "wp:db ms:prod")

	_, err = s.State.InferActiveRelation("wp", "ms:dev")
	c.Assert(err, gc.ErrorMatches, `relation matching "wp ms:dev" not found`)
}

func (s *StateSuite) TestInferActiveRelationsNoRelations(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wp := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	_, err = wp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ms := s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	_, err = ms.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.InferActiveRelation("wp", "ms")
	c.Assert(err, gc.ErrorMatches, `relation matching "wp ms" not found`)

	_, err = s.State.InferActiveRelation("wp:db", "ms:prod")
	c.Assert(err, gc.ErrorMatches, `relation matching "wp:db ms:prod" not found`)
}

func (s *StateSuite) TestInferActiveRelationsAmbiguous(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wp := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress-nolimit"))
	_, err = wp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ms := s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	_, err = ms.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	eps1, err := s.State.InferEndpoints("wp", "ms:prod")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps1...)
	c.Assert(err, jc.ErrorIsNil)

	eps2, err := s.State.InferEndpoints("wp", "ms:dev")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps2...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.InferActiveRelation("wp", "ms")
	c.Assert(err, gc.ErrorMatches, `ambiguous relation: "wp ms" could refer to "wp:db ms:prod"; "wp:db ms:dev"`)

	relation, err := s.State.InferActiveRelation("wp", "ms:prod")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relation, gc.Matches, "wp:db ms:prod")
}

func (s *StateSuite) TestAllRelations(c *gc.C) {
	const numRelations = 32
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
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

func (s *StateSuite) TestAliveRelationKeys(c *gc.C) {
	const numRelations = 12
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
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
		r, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
		// Destroy half the relations, to check we only get the ones Alive
		if i%2 == 0 {
			_ = r.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
		}
	}

	relationKeys := s.State.AliveRelationKeys()

	c.Assert(len(relationKeys), gc.Equals, numRelations/2)
	num := 1
	for _, relation := range relationKeys {
		c.Assert(relation, gc.Matches, fmt.Sprintf("wordpress%d:.+ mysql:.+", num))
		num += 2
	}
}

func (s *StateSuite) TestSaveCloudService(c *gc.C) {
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id",
			Addresses:  network.NewSpaceAddresses("1.1.1.1"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Refresh(), jc.ErrorIsNil)
	c.Assert(svc.Id(), gc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), gc.Equals, "provider-id")
	c.Assert(svc.Addresses(), gc.DeepEquals, network.NewSpaceAddresses("1.1.1.1"))

	getResult, err := s.State.CloudService("cloud-svc-ID")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(getResult.Id(), gc.Equals, "a#cloud-svc-ID")
	c.Assert(getResult.ProviderId(), gc.Equals, "provider-id")
	c.Assert(getResult.Addresses(), gc.DeepEquals, network.NewSpaceAddresses("1.1.1.1"))
}

func (s *StateSuite) TestSaveCloudServiceChangeAddressesAllGood(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.SaveCloudService(
			state.SaveCloudServiceArgs{
				Id:         "cloud-svc-ID",
				ProviderId: "provider-id",
				Addresses:  network.NewSpaceAddresses("1.1.1.1"),
			},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id",
			Addresses:  network.NewSpaceAddresses("2.2.2.2"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Refresh(), jc.ErrorIsNil)
	c.Assert(svc.Id(), gc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), gc.Equals, "provider-id")
	c.Assert(svc.Addresses(), gc.DeepEquals, network.NewSpaceAddresses("2.2.2.2"))
}

func (s *StateSuite) TestSaveCloudServiceChangeProviderId(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.SaveCloudService(
			state.SaveCloudServiceArgs{
				Id:         "cloud-svc-ID",
				ProviderId: "provider-id-existing",
				Addresses:  network.NewSpaceAddresses("1.1.1.1"),
			},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id-new", // ProviderId is immutable, changing this will get assert error.
			Addresses:  network.NewSpaceAddresses("1.1.1.1"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Refresh(), jc.ErrorIsNil)
	c.Assert(svc.Id(), gc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), gc.Equals, "provider-id-new")
	c.Assert(svc.Addresses(), gc.DeepEquals, network.NewSpaceAddresses("1.1.1.1"))
}

func (s *StateSuite) TestAddApplication(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "haha/borken", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "haha/borken": invalid name`)
	_, err = s.State.Application("haha/borken")
	c.Assert(err, gc.ErrorMatches, `"haha/borken" is not a valid application name`)

	// set that a nil charm is handled correctly
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "umadbro",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "umadbro": charm is nil`)

	// set that a nil charm origin is handled correctly
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name:  "umadbro",
		Charm: ch,
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "umadbro": charm origin is nil`)

	insettings := charm.Settings{"tuning": "optimized"}
	inconfig, err := coreconfig.NewConfig(coreconfig.ConfigAttributes{"outlook": "good"}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)

	wordpress, err := s.State.AddApplication(defaultInstancePrechecker,
		state.AddApplicationArgs{
			Name:              "wordpress",
			Charm:             ch,
			CharmConfig:       insettings,
			ApplicationConfig: inconfig,
			CharmOrigin: &state.CharmOrigin{
				ID:   "charmID",
				Hash: "testing-hash",
				Platform: &state.Platform{
					OS:      "ubuntu",
					Channel: "22.04/stable",
				},
			},
		}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	c.Assert(state.GetApplicationHasResources(wordpress), jc.IsFalse)
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
	cons, err := wordpress.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	a := arch.DefaultArchitecture
	c.Assert(cons, jc.DeepEquals, constraints.Value{
		Arch: &a,
	})

	mysqlArch := arch.ARM64
	mysql, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "mysql", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		Constraints: constraints.Value{Arch: &mysqlArch}},
		state.NewObjectStore(c, s.State.ModelUUID()),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")
	sInfo, err := mysql.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sInfo.Status, gc.Equals, status.Unset)
	c.Assert(sInfo.Message, gc.Equals, "")
	cons, err = mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.Value{
		Arch: &mysqlArch,
	})

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

func (s *StateSuite) TestAddApplicationFailCharmOriginIDOnly(c *gc.C) {
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name:        "testme",
		Charm:       &state.Charm{},
		CharmOrigin: &state.CharmOrigin{ID: "testing", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *StateSuite) TestAddApplicationFailCharmOriginHashOnly(c *gc.C) {
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name:        "testme",
		Charm:       &state.Charm{},
		CharmOrigin: &state.CharmOrigin{Hash: "testing", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *StateSuite) TestAddCAASApplication(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer func() { _ = st.Close() }()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})

	insettings := charm.Settings{"tuning": "optimized"}
	inconfig, err := coreconfig.NewConfig(coreconfig.ConfigAttributes{"outlook": "good"}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)

	gitlab, err := st.AddApplication(defaultInstancePrechecker,
		state.AddApplicationArgs{
			Name: "gitlab", Charm: ch,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			CharmConfig: insettings, ApplicationConfig: inconfig, NumUnits: 1,
		}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gitlab.Name(), gc.Equals, "gitlab")
	c.Assert(gitlab.GetScale(), gc.Equals, 1)
	c.Assert(state.GetApplicationHasResources(gitlab), jc.IsTrue)
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

	cons, err := gitlab.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	a := arch.DefaultArchitecture
	c.Assert(cons, jc.DeepEquals, constraints.Value{
		Arch: &a,
	})

	sInfo, err := gitlab.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sInfo.Status, gc.Equals, status.Unset)
	c.Assert(sInfo.Message, gc.Equals, "")

	// Check that retrieving the newly created application works correctly.
	gitlab, err = st.Application("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gitlab.Name(), gc.Equals, "gitlab")
	ch, _, err = gitlab.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, ch.URL())
	units, err := gitlab.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(unitAssignments), gc.Equals, 0)
}

func (s *StateSuite) TestAddApplicationKubernetesFormatV2(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer func() { _ = st.Close() }()
	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	cockroach, err := st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "mysql", Charm: ch, NumUnits: 1,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	units, err := cockroach.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(unitAssignments), gc.Equals, 0)
}

func (s *StateSuite) TestAddApplicationKubernetesFormatV2SecondDeployUnitNumberStartFrom0(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer func() { _ = st.Close() }()
	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	cockroach, err := st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "cockroach", Charm: ch, NumUnits: 1,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	units, err := cockroach.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(unitAssignments), gc.Equals, 0)

	err = cockroach.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = cockroach.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, st.ModelUUID())
	assertCleanupCount(c, st, 2)

	ch = state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	cockroach, err = st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "cockroach", Charm: ch, NumUnits: 1,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	units, err = cockroach.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	c.Assert(units[0].Name(), gc.Equals, `cockroach/0`)
}

func (s *StateSuite) TestAddCAASApplicationPlacementNotAllowed(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})

	placement := []*instance.Placement{instance.MustParsePlacement("#:2")}
	_, err := st.AddApplication(defaultInstancePrechecker,
		state.AddApplicationArgs{
			Name: "gitlab", Charm: ch,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			Placement: placement,
		}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, ".*"+regexp.QuoteMeta(`cannot add application "gitlab": placement directives on k8s models not valid`))
}

func (s *StateSuite) TestAddApplicationWithNilCharmConfigValues(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	insettings := charm.Settings{"tuning": nil}

	wordpress, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		CharmConfig: insettings},
		state.NewObjectStore(c, s.State.ModelUUID()),
	)
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
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testmodel" is dying`)
}

func (s *StateSuite) TestAddApplicationModelMigrating(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that applications cannot be added if the model is initially Dying.
	err := s.Model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testmodel" is being migrated`)
}

func (s *StateSuite) TestAddApplicationSameRemoteExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": saas application with same name already exists`)
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
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": saas application with same name already exists`)
}

func (s *StateSuite) TestAddApplicationSameLocalExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "s0", charm)
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s0", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
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
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": application already exists`)
}

func (s *StateSuite) TestAddApplicationModelDyingAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "s0", charm)
	// Check that applications cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.Model.Life(), gc.Equals, state.Alive)
		c.Assert(s.Model.Destroy(state.DestroyModelParams{}), gc.IsNil)
	}).Check()
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "s1": model "testmodel" is dying`)
}

func (s *StateSuite) TestApplicationNotFound(c *gc.C) {
	_, err := s.State.Application("bummer")
	c.Assert(err, gc.ErrorMatches, `application "bummer" not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *StateSuite) TestAddApplicationWithDefaultBindings(c *gc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	app, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := app.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings.Map(), jc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
	})

	// Removing the application also removes its bindings.
	err = app.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	state.AssertEndpointBindingsNotFoundForApplication(c, app)
}

func (s *StateSuite) TestAddApplicationWithSpecifiedBindings(c *gc.C) {
	// Add extra spaces to use in bindings.
	dbSpace, err := s.State.AddSpace("db", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	clientSpace, err := s.State.AddSpace("client", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Specify some bindings, but not all when adding the application.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 43)
	app, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		EndpointBindings: map[string]string{
			"client":  clientSpace.Id(),
			"cluster": dbSpace.Id(),
		},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := app.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings.Map(), jc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId, // inherited from defaults.
		"client":  clientSpace.Id(),
		"cluster": dbSpace.Id(),
	})
}

func (s *StateSuite) TestAddApplicationMachinePlacementInvalidSeries(c *gc.C) {
	m, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.04/stable",
		}},
		Placement: []*instance.Placement{
			{instance.MachineScope, m.Id()},
		},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, "cannot add application \"wordpress\": cannot deploy to machine .*: base does not match.*")
}

func (s *StateSuite) TestAddApplicationIncompatibleOSWithSeriesInURL(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "centos",
			Channel: "7/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": OS "centos" not supported by charm "dummy", supported OSes are: ubuntu`)
}

func (s *StateSuite) TestAddApplicationCompatibleOSWithSeriesInURL(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	base, err := corebase.GetBaseFromSeries(charm.MustParseURL(ch.URL()).Series)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      base.OS,
			Channel: base.Channel.String(),
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestAddApplicationCompatibleOSWithNoExplicitSupportedSeries(c *gc.C) {
	// If a charm doesn't declare any series, we can add it with any series we choose.
	charm := s.AddSeriesCharm(c, "dummy", "bionic")
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestAddApplicationOSIncompatibleWithSupportedSeries(c *gc.C) {
	charm := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	_, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "centos",
			Channel: "7/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": OS "centos" not supported by charm "multi-series", supported OSes are: ubuntu`)
}

func (s *StateSuite) TestAllApplications(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(applications), gc.Equals, 0)

	// Check that after adding applications the result is ok.
	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	applications, err = s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(applications), gc.Equals, 1)

	_, err = s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "mysql", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
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
				Scope:     charm.ScopeContainer,
			}}, {
			ApplicationName: "lg2",
			Relation: charm.Relation{
				Name:      "logging-client",
				Role:      "provider",
				Interface: "logging",
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
		err: `ambiguous relation: ".*" could refer to "wp:db ms:dev"; "wp:db ms:prod"; "wp:db ms:test"`,
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
	alive := s.Model

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
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.UUID(), dying.UUID())

	// Progress dying to dead, alive to dying; and see changes reported.
	err = app.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(alive.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(alive.Refresh(), jc.ErrorIsNil)
	c.Assert(alive.Life(), gc.Equals, state.Dying)
	c.Assert(dying.Refresh(), jc.ErrorIs, errors.NotFound)
	wc.AssertChange(alive.UUID())
}

func (s *StateSuite) TestWatchModelsLifecycle(c *gc.C) {
	// Initial event reports the controller model.
	w := s.State.WatchModelLives()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
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
	c.Assert(app.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	c.Assert(st1.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), jc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()
	c.Assert(model.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *StateSuite) TestWatchApplicationsBulkEvents(c *gc.C) {
	// Alive application...
	dummyCharm := s.AddTestingCharm(c, "dummy")
	alive := s.AddTestingApplication(c, "application0", dummyCharm)

	// Dying application...
	dying := s.AddTestingApplication(c, "application1", dummyCharm)
	keepDying, err := dying.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// Dead application (actually, gone, Dead == removed in this case).
	gone := s.AddTestingApplication(c, "application2", dummyCharm)
	err = gone.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// All except gone are reported in initial event.
	w := s.State.WatchApplications()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported.
	err = alive.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = keepDying.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{}), jc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchApplicationsLifecycle(c *gc.C) {
	// Initial event is empty when no applications.
	w := s.State.WatchApplications()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
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
	c.Assert(application.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()

	c.Assert(application.Refresh(), jc.ErrorIsNil)
	c.Check(application.Life(), gc.Equals, state.Dying)

	// Make it Dead(/removed): reported.
	c.Assert(keepDying.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	needs, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(needs, jc.IsTrue)
	c.Assert(s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{}), jc.ErrorIsNil)
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
	alive, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Dying machine...
	dying, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = dying.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// Dead machine...
	dead, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Gone machine.
	gone, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = gone.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = gone.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// All except gone machine are reported in initial event.
	w := s.State.WatchModelMachines()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.Id(), dying.Id(), dead.Id())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = dying.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = dead.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(alive.Id(), dying.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesLifecycle(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchModelMachines()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Change the machine: not reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = machine.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Make it Dead: reported.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Remove it: not reported.
	err = machine.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIncludesOldMachines(c *gc.C) {
	// Older versions of juju do not write the "containertype" field.
	// This has caused machines to not be detected in the initial event.
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$unset", bson.D{{"containertype", 1}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchModelMachines()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(machine.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIgnoresContainers(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchModelMachines()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	machines, err := s.State.AddMachines(defaultInstancePrechecker, template)
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
	err = m.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the container Dead: not reported.
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchContainerLifecycle(c *gc.C) {
	// Add a host machine.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddOneMachine(defaultInstancePrechecker, template)
	c.Assert(err, jc.ErrorIsNil)

	otherMachine, err := s.State.AddOneMachine(defaultInstancePrechecker, template)
	c.Assert(err, jc.ErrorIsNil)

	// Initial event is empty when no containers.
	w := machine.WatchContainers(instance.LXD)
	defer workertest.CleanKill(c, w)
	wAll := machine.WatchAllContainers()
	defer workertest.CleanKill(c, wAll)

	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	wcAll := statetesting.NewStringsWatcherC(c, wAll)
	wcAll.AssertChange()
	wcAll.AssertNoChange()

	// Add a container of the required type: reported.
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Add a nested container of the right type: not reported.
	mchild, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()

	// Add a container of a different machine: not reported.
	m1, err := s.State.AddMachineInsideMachine(template, otherMachine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	workertest.CleanKill(c, w)
	wcAll.AssertNoChange()
	workertest.CleanKill(c, wAll)

	w = machine.WatchContainers(instance.LXD)
	defer workertest.CleanKill(c, w)
	wc = statetesting.NewStringsWatcherC(c, w)
	wAll = machine.WatchAllContainers()
	defer workertest.CleanKill(c, wAll)
	wcAll = statetesting.NewStringsWatcherC(c, wAll)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Make the container Dying: cannot because of nested container.
	err = m.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `machine .* is hosting containers? ".*"`)

	err = mchild.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = mchild.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// Make the container Dying: reported.
	err = m.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Make the other containers Dying: not reported.
	err = m1.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

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
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()
}

func (s *StateSuite) TestWatchMachineHardwareCharacteristics(c *gc.C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := machine.WatchInstanceData()
	defer workertest.CleanKill(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Provision a machine: reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Alter the machine: not reported.
	vers := version.MustParseBinary("1.2.3-ubuntu-ppc")
	err = machine.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
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
			doc := bson.M{
				"model-uuid": modelUUID,
			}
			id := "arbitraryid"
			// We need a "real" application and offer.
			if collName == "applicationOffers" {
				doc["application-name"] = "foo"
			} else if collName == "applications" {
				doc["name"] = "foo"
				id = "foo"
			}
			ops = append(ops, mgotxn.Op{
				C:      collName,
				Id:     state.DocID(st, id),
				Insert: doc,
			})
		}
	}

	state.RunTransaction(c, st, ops)

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
	_, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("amelia@external"),
			CreatedBy: s.Owner,
			Access:    permission.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	return state.UserModelNameIndex(s.Model.Owner().Id(), s.Model.Name())
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

	err = st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(model.Refresh(), jc.ErrorIs, errors.NotFound)
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
	c.Assert(model.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *StateSuite) TestSetDyingModelToDeadRequiresDyingModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetDyingModelToDead()
	c.Assert(err, jc.ErrorIs, state.ErrModelNotDying)

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), jc.DeepEquals, state.Dying)
	c.Assert(st.SetDyingModelToDead(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), jc.DeepEquals, state.Dead)

	err = st.SetDyingModelToDead()
	c.Assert(err, jc.ErrorIs, state.ErrModelNotDying)
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

	err = s.Model.SetMigrationMode(state.MigrationModeImporting)
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

	err = s.Model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveExportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsRemovesOfferPermissions(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	s.createOffer(c)

	coll, closer := state.GetRawCollection(s.State, "permissions")
	defer closer()
	cnt, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cnt, gc.Equals, 8)

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveExportingModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	cnt, err = coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	// 2 model permissions deleted.
	// 2 offer permissions deleted.
	c.Assert(cnt, gc.Equals, 4)
}

func (s *StateSuite) createOffer(c *gc.C) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps := map[string]string{"db": "server", "db-admin": "server-admin"}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	offerArgs := crossmodel.AddApplicationOfferArgs{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		Owner:                  owner.Name(),
		HasRead:                []string{"everyone@external"},
	}
	_, err := sd.AddOffer(offerArgs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestWatchForModelConfigChanges(c *gc.C) {
	cur := jujuversion.Current
	err := statetesting.SetAgentVersion(s.State, cur)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.Model.WatchForModelConfigChanges()
	defer workertest.CleanKill(c, w)

	wc := statetesting.NewNotifyWatcherC(c, w)
	// Initially we get one change notification
	wc.AssertOneChange()

	// Multiple changes will only result in a single change notification
	newVersion := cur
	newVersion.Minor++
	err = statetesting.SetAgentVersion(s.State, newVersion)
	c.Assert(err, jc.ErrorIsNil)

	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()

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
	w := s.Model.WatchForModelConfigChanges()
	defer workertest.CleanKill(c, w)

	wc := statetesting.NewNotifyWatcherC(c, w)
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

	m1, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m2, err := s.State.Machine(m1.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m1, jc.DeepEquals, m2)

	charm1 := s.AddTestingCharm(c, "wordpress")
	charm2, err := s.State.Charm(charm1.URL())
	c.Assert(err, jc.ErrorIsNil)
	// Refresh is required to set the charmURL, so the test will succeed.
	err = charm2.Refresh()
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relation1, jc.DeepEquals, relation2)
	relation3, err := s.State.Relation(relation1.Id())
	c.Assert(err, jc.ErrorIsNil)
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
	c.Check(err, jc.ErrorIs, errors.Unauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = names.NewUserTag("arble"), ""
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(err, jc.ErrorIs, errors.Unauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = nil, ""
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(err, jc.ErrorIsNil)
}

func testSetPassword(c *gc.C, st *state.State, getEntity func() (state.Authenticator, error)) {
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
		testWhenDying(c, state.NewObjectStore(c, st.ModelUUID()), le, noErr, deadErr, func() error {
			return e.SetPassword("arble-farble-dying-yarble")
		})
	}
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
	tag: names.NewControllerAgentTag("0"),
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
	tag: names.NewOperationTag("666"),
	err: `operation "666" not found`,
}, {
	tag: names.NewUserTag("eric"),
}, {
	tag: names.NewUserTag("eric@local"),
}, {
	tag: names.NewUserTag("eric@remote"),
	err: `user "eric@remote" not found`,
}}

var entityTypes = map[string]interface{}{
	names.UserTagKind:            (*state.User)(nil),
	names.ModelTagKind:           (*state.Model)(nil),
	names.ApplicationTagKind:     (*state.Application)(nil),
	names.UnitTagKind:            (*state.Unit)(nil),
	names.MachineTagKind:         (*state.Machine)(nil),
	names.ControllerAgentTagKind: (*state.ControllerNodeInstance)(nil),
	names.RelationTagKind:        (*state.Relation)(nil),
	names.ActionTagKind:          (state.Action)(nil),
	names.OperationTagKind:       (state.Operation)(nil),
}

func (s *StateSuite) TestFindEntity(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "eric"})
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	app := s.AddTestingApplication(c, "ser-vice2", s.AddTestingCharm(c, "mysql"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	operationID, err := s.Model.EnqueueOperation("something", 1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Model.AddAction(unit, operationID, "fakeaction", nil, nil, nil)
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
		tag: names.NewModelTag(s.Model.UUID()),
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
				c.Assert(e.Tag(), gc.Equals, s.Model.Tag())
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
	m, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
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
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	f, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	action, err := s.Model.Action(f.Id())
	c.Assert(err, jc.ErrorIsNil)
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
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, s.Model.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coll, gc.Equals, "models")
	c.Assert(id, gc.Equals, s.Model.UUID())
}

func (s *StateSuite) TestWatchCleanups(c *gc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)
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
	err = relM.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Handle that cleanup doc and create another, check one change.
	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = relV.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Clean up final doc, check change.
	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
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
	c.Skip("Dqlite conversion. Strangled out as intermittent failure of test in line for deletion.")

	// Check initial event.
	w := s.State.WatchCleanups()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)
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
	err = riak.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): reimplement some quiescence on the cleanup watcher
	wc.AssertOneChange()
	err = allHooks.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Clean them both up, check one change.
	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertAtleastOneChange()
}

func (s *StateSuite) TestWatchMinUnits(c *gc.C) {
	// Check initial event.
	w := s.State.WatchMinUnits()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
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
	err = wordpress0.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Two actions: destroy a unit and increase minimum units for a application.
	// A single change should occur, and the application name should appear only
	// one time in the change.
	err = wordpress.SetMinUnits(5)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress1.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Destroy a unit of a application not requiring minimum units; expect no changes.
	err = mysql0.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a application with required minimum units; expect no changes.
	err = wordpress.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a application not requiring minimum units; expect no changes.
	err = mysql.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
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
		return id != "0"
	}
	w := s.State.WatchSubnets(filter)
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	// Check initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	_, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.20.0.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "10.0.0.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("1")
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
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	// Check initial event.
	wc.AssertChange()
	// No change for local relation.
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyRelation(c *gc.C) {
	w := s.State.WatchRemoteRelations()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	_, _, rel := s.setupWatchRemoteRelations(c, wc)

	// Destroy the remote relation.
	// A single change should occur.
	err := rel.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyRemoteApplication(c *gc.C) {
	w := s.State.WatchRemoteRelations()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	remoteApp, _, _ := s.setupWatchRemoteRelations(c, wc)

	// Destroy the remote application.
	// A single change should occur.
	err := remoteApp.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyLocalApplication(c *gc.C) {
	w := s.State.WatchRemoteRelations()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	_, app, _ := s.setupWatchRemoteRelations(c, wc)

	// Destroy the local application.
	// A single change should occur.
	err := app.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		w := st.WatchRemoteRelations()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestSetModelAgentVersionErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	stringVersion := agentVersion.String()

	// Add 4 machines: one with a different version, one with an
	// empty version, one with the current version, and one with
	// the new version.
	machine0, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine0.SetAgentVersion(version.MustParseBinary("9.9.9-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	machine1, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machine2, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine2.SetAgentVersion(version.MustParseBinary(stringVersion + "-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	machine3, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine3.SetAgentVersion(version.MustParseBinary("4.5.6-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// Verify machine0 and machine1 are reported as error.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	expectErr := fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Add a application and 4 units: one with a different version, one
	// with an empty version, one with the current version, and one
	// with the new version.
	application, err := s.State.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress"),
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	unit0, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit0.SetAgentVersion(version.MustParseBinary("6.6.6-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unit2, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit2.SetAgentVersion(version.MustParseBinary(stringVersion + "-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	unit3, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit3.SetAgentVersion(version.MustParseBinary("4.5.6-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// Verify unit0 and unit1 are reported as error, along with the
	// machines from before.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1, unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Now remove the machines.
	for _, machine := range []*state.Machine{machine0, machine1, machine2} {
		err = machine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = machine.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
		c.Assert(err, jc.ErrorIsNil)
	}

	// Verify only the units are reported as error.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)
}

func (s *StateSuite) prepareAgentVersionTests(c *gc.C, st *state.State) (*config.Config, string) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	// Get the agent-version set in the model.
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := m.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine and a unit with the current version.
	machine, err := st.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application, err := st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress"),
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
	}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetAgentVersion(version.MustParseBinary(currentVersion + "-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	return modelConfig, currentVersion
}

func (s *StateSuite) changeEnviron(c *gc.C, modelConfig *config.Config, name string, value interface{}) {
	attrs := modelConfig.AllAttrs()
	attrs[name] = value
	c.Assert(s.Model.UpdateModelConfig(state.NoopConfigSchemaSource, attrs, nil), gc.IsNil)
}

func assertAgentVersion(c *gc.C, st *state.State, vers, stream string) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := m.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion.String(), gc.Equals, vers)
	agentStream := modelConfig.AgentStream()
	c.Assert(agentStream, gc.Equals, stream)

}

func (s *StateSuite) TestSetModelAgentVersionRetriesOnConfigChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	modelConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change something
	// other than the version, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, modelConfig, "default-base", "ubuntu@20.04")
	}).Check()

	// Change the agent-version and ensure it has changed.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionSucceedsWithSameVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	modelConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change the version
	// to the new one, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, modelConfig, "agent-version", "4.5.6")
	}).Check()

	// Change the agent-version and verify.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionUpdateStream(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	proposed := "proposed"
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), &proposed, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", proposed)

	err = s.State.SetModelAgentVersion(version.MustParse("4.5.7"), nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.7", proposed)
}

func (s *StateSuite) TestSetModelAgentVersionUpdateStreamEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	stream := ""
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), &stream, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionOnOtherModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	current := version.MustParseBinary("1.24.7-ubuntu-amd64")
	s.PatchValue(&jujuversion.Current, current.Number)
	s.PatchValue(&arch.HostArch, func() string { return current.Arch })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	higher := version.MustParseBinary("1.25.0-ubuntu-amd64")
	lower := version.MustParseBinary("1.24.6-ubuntu-amd64")

	// Set other model version to < controller model version
	err := otherSt.SetModelAgentVersion(lower.Number, nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, lower.Number.String(), "released")

	// Set other model version == controller version
	err = otherSt.SetModelAgentVersion(jujuversion.Current, nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, otherSt, jujuversion.Current.String(), "released")

	// Set other model version to > server version
	err = otherSt.SetModelAgentVersion(higher.Number, nil, false, s.upgrader)
	expected := fmt.Sprintf("model cannot be upgraded to %s while the controller is %s: upgrade 'controller' model first",
		higher.Number,
		jujuversion.Current,
	)
	c.Assert(err, gc.ErrorMatches, expected)
}

func (s *StateSuite) TestSetModelAgentVersionExcessiveContention(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	modelConfig, currentVersion := s.prepareAgentVersionTests(c, s.State)

	// Set a hook to change the config 3 times
	// to test we return ErrExcessiveContention.
	hooks := []jujutxn.TestHook{
		{Before: func() { s.changeEnviron(c, modelConfig, "default-base", "ubuntu@20.04") }},
		{Before: func() { s.changeEnviron(c, modelConfig, "default-base", "ubuntu@22.04") }},
		{Before: func() { s.changeEnviron(c, modelConfig, "default-base", "ubuntu@20.04") }},
	}

	state.SetMaxTxnAttempts(c, s.State, 3)
	defer state.SetTestHooks(c, s.State, hooks...).Check()
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIs, jujutxn.ErrExcessiveContention)
	// Make sure the version remained the same.
	assertAgentVersion(c, s.State, currentVersion, "released")
}

func (s *StateSuite) TestSetModelAgentVersionMixedVersions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)

	_, currentVersion := s.prepareAgentVersionTests(c, s.State)
	machine, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	// Force this to something old that should not match current versions
	err = machine.SetAgentVersion(version.MustParseBinary("1.0.1-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	// This should be refused because an agent doesn't match "currentVersion"
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false, s.upgrader)
	c.Check(err, gc.ErrorMatches, "some agents have not upgraded to the current model version .*: machine-0")
	// Version hasn't changed
	assertAgentVersion(c, s.State, currentVersion, "released")
	// But we can force it
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, true, s.upgrader)
	c.Assert(err, jc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionFailsIfUpgrading(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	s.expectIsUpgrade(true)

	err = s.State.SetModelAgentVersion(nextVersion, nil, false, s.upgrader)
	c.Assert(err, jc.ErrorIs, upgrade.ErrUpgradeInProgress)
}

func (s *StateSuite) TestSetModelAgentVersionFailsReportsCorrectError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the correct error is reported if an upgrade is
	// progress but that isn't the reason for the
	// SetModelAgentVersion call failing.

	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary("9.9.9-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	s.expectIsUpgrade(true)

	err = s.State.SetModelAgentVersion(nextVersion, nil, false, s.upgrader)
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
	controller, err := state.OpenController(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
	})
	c.Assert(err, jc.ErrorIsNil)
	sysState, err := controller.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	watcher := startWatcher(c, sysState)
	err = controller.Close()
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
	c.Assert(ids.ControllerIds, gc.HasLen, 0)

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

	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, jc.ErrorIsNil)
	defer controller.Close()

	sysState, err := controller.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	info, err = sysState.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *StateSuite) TestStateServingInfo(c *gc.C) {
	_, err := s.State.StateServingInfo()
	c.Assert(err, gc.ErrorMatches, "state serving info not found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	data := controller.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	err = s.State.SetStateServingInfo(data)
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.State.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, data)
}

func (s *StateSuite) TestSetAPIHostPortsNoMgmtSpace(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	addrs, err := s.State.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}, {
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewSpaceAddress("0.6.1.2", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      5,
	}}}
	err = s.State.SetAPIHostPorts(cfg, newHostPorts, newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	newHostPorts = []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      13,
	}}}
	err = s.State.SetAPIHostPorts(cfg, newHostPorts, newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err = ctrlSt.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
}

func (s *StateSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentSame(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	hostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	// API host ports are concurrently changed to the same
	// desired value; second arrival will fail its assertion,
	// refresh finding nothing to do, and then issue a
	// read-only assertion that succeeds.
	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts(cfg, hostPorts, hostPorts)
		c.Assert(err, jc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, jc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts(cfg, hostPorts, hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Equals, prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Equals, prevAgentsRevno)
}

func (s *StateSuite) TestSetAPIHostPortsWithMgmtSpace(c *gc.C) {
	sp, err := s.State.AddSpace("mgmt01", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg := testing.FakeControllerConfig()
	cfg[controller.JujuManagementSpace] = "mgmt01"
	c.Assert(err, jc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	addrs, err := s.State.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	hostPort1 := network.SpaceHostPort{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}
	hostPort2 := network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "0.4.8.16",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			SpaceID: sp.Id(),
		},
		NetPort: 2,
	}
	hostPort3 := network.SpaceHostPort{
		SpaceAddress: network.NewSpaceAddress("0.6.1.2", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      5,
	}
	newHostPorts := []network.SpaceHostPorts{{hostPort1, hostPort2}, {hostPort3}}

	err = s.State.SetAPIHostPorts(cfg, newHostPorts, []network.SpaceHostPorts{{hostPort2}, {hostPort3}})
	c.Assert(err, jc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents(cfg)
	c.Assert(err, jc.ErrorIsNil)
	// First slice filtered down to the address in the management space.
	// Second filtered to zero elements, so retains the supplied slice.
	c.Assert(gotHostPorts, jc.DeepEquals, []network.SpaceHostPorts{{hostPort2}, {hostPort3}})
}

func (s *StateSuite) TestSetAPIHostPortsForAgentsNoDocument(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	addrs, err := s.State.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	// Delete the addresses for agents document before setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), gc.Equals, mgo.ErrNotFound)

	err = s.State.SetAPIHostPorts(cfg, newHostPorts, newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForAgents(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
}

func (s *StateSuite) TestAPIHostPortsForAgentsNoDocument(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	addrs, err := s.State.APIHostPortsForClients(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	err = s.State.SetAPIHostPorts(cfg, newHostPorts, newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	// Delete the addresses for agents document after setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), gc.Equals, mgo.ErrNotFound)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForAgents(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
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
		attempt   int
		duration  time.Duration
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
	params.RunTransactionObserver = func(dbName, modelUUID string, attempt int, duration time.Duration, ops []mgotxn.Op, err error) {
		mu.Lock()
		defer mu.Unlock()
		recordedCalls = append(recordedCalls, args{
			dbName:    dbName,
			modelUUID: modelUUID,
			attempt:   attempt,
			duration:  duration,
			ops:       ops,
			err:       err,
		})
	}
	controller, err := state.OpenController(params)
	c.Assert(err, jc.ErrorIsNil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, jc.ErrorIsNil)

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
		c.Check(call.duration, gc.Not(gc.Equals), 0)
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

func setAdminPassword(c *gc.C, inst *mgotesting.MgoInstance, owner names.UserTag, password string) {
	session, err := inst.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()
	err = mongo.SetAdminMongoPassword(session, owner.String(), password)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SetAdminMongoPasswordSuite) TestSetAdminMongoPassword(c *gc.C) {
	inst := &mgotesting.MgoInstance{
		EnableAuth:       true,
		EnableReplicaSet: true,
	}
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
		CloudName:     "dummy",
		MongoSession:  session,
		AdminPassword: password,
	}, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)
	st, err := ctlr.SystemState()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Check(err, jc.ErrorIs, errors.Unauthorized)
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
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
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
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationIngressNetworksIgnoresEgress(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationIngressNetworks()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationEgressNetworks(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationEgressNetworks()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
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
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationEgressNetworksIgnoresIngress(c *gc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationEgressNetworks()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	relEgress := state.NewRelationIngressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) testOpenParams() state.OpenParams {
	return state.OpenParams{
		Clock:               clock.WallClock,
		ControllerTag:       s.State.ControllerTag(),
		ControllerModelTag:  s.modelTag,
		MongoSession:        s.Session,
		WatcherPollInterval: 10 * time.Millisecond,
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

func (s *StateSuite) TestAddRelationCreatesApplicationSettings(c *gc.C) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	settings := state.NewStateSettings(s.State)

	mysqlKey := fmt.Sprintf("r#%d#mysql", rel.Id())
	_, err = settings.ReadSettings(mysqlKey)
	c.Assert(err, jc.ErrorIsNil)

	wpKey := fmt.Sprintf("r#%d#wordpress", rel.Id())
	_, err = settings.ReadSettings(wpKey)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) TestPeerRelationCreatesApplicationSettings(c *gc.C) {
	app := state.AddTestingApplication(c, s.State, s.objectStore, "riak", state.AddTestingCharm(c, s.State, "riak"))
	ep, err := app.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	settings := state.NewStateSettings(s.State)

	key := fmt.Sprintf("r#%d#riak", rel.Id())
	_, err = settings.ReadSettings(key)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.upgrader = mocks.NewMockUpgrader(ctrl)

	return ctrl
}

func (s *StateSuite) expectIsUpgrade(value bool) {
	s.upgrader.EXPECT().IsUpgrading().Return(value, nil).AnyTimes()
}
