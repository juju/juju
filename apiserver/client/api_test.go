// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
)

type baseSuite struct {
	testing.JujuConnSuite
	commontesting.BlockHelper
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

var _ = gc.Suite(&baseSuite{})

func chanReadEmpty(c *gc.C, ch <-chan struct{}, what string) bool {
	select {
	case _, ok := <-ch:
		return ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func chanReadStrings(c *gc.C, ch <-chan []string, what string) ([]string, bool) {
	select {
	case changes, ok := <-ch:
		return changes, ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func chanReadConfig(c *gc.C, ch <-chan *config.Config, what string) (*config.Config, bool) {
	select {
	case envConfig, ok := <-ch:
		return envConfig, ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func removeServiceAndUnits(c *gc.C, service *state.Application) {
	// Destroy all units for the application.
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		err = unit.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
	err = service.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// apiAuthenticator represents a simple authenticator object with only the
// SetPassword and Tag methods.  This will fit types from both the state
// and api packages, as those in the api package do not have PasswordValid().
type apiAuthenticator interface {
	state.Entity
	SetPassword(string) error
}

func setDefaultPassword(c *gc.C, e apiAuthenticator) {
	err := e.SetPassword(defaultPassword(e))
	c.Assert(err, jc.ErrorIsNil)
}

func defaultPassword(e apiAuthenticator) string {
	return e.Tag().String() + " password-1234567890"
}

type setStatuser interface {
	SetStatus(status.StatusInfo) error
}

func setDefaultStatus(c *gc.C, entity setStatuser) {
	now := time.Now()
	s := status.StatusInfo{
		Status:  status.StatusStarted,
		Message: "",
		Since:   &now,
	}
	err := entity.SetStatus(s)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseSuite) tryOpenState(c *gc.C, e apiAuthenticator, password string) error {
	stateInfo := s.MongoInfo(c)
	stateInfo.Tag = e.Tag()
	stateInfo.Password = password
	st, err := state.Open(s.State.ModelTag(), s.State.ControllerTag(), stateInfo, mongo.DialOpts{
		Timeout: 25 * time.Millisecond,
	}, nil)
	if err == nil {
		st.Close()
	}
	return err
}

// openAs connects to the API state as the given entity
// with the default password for that entity.
func (s *baseSuite) openAs(c *gc.C, tag names.Tag) api.Connection {
	info := s.APIInfo(c)
	info.Tag = tag
	// Must match defaultPassword()
	info.Password = fmt.Sprintf("%s password-1234567890", tag)
	// Set this always, so that the login attempts as a machine will
	// not fail with ErrNotProvisioned; it's not used otherwise.
	info.Nonce = "fake_nonce"
	c.Logf("opening state; entity %q; password %q", info.Tag, info.Password)
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	return st
}

// scenarioStatus describes the expected state
// of the juju environment set up by setUpScenario.
//
// NOTE: AgentState: "down", AgentStateInfo: "(started)" here is due
// to the scenario not calling SetAgentPresence on the respective entities,
// but this behavior is already tested in cmd/juju/status_test.go and
// also tested live and it works.
var scenarioStatus = &params.FullStatus{
	Model: params.ModelStatusInfo{
		Name:        "controller",
		Cloud:       "dummy",
		CloudRegion: "dummy-region",
		Version:     "1.2.3",
	},
	Machines: map[string]params.MachineStatus{
		"0": {
			Id:         "0",
			InstanceId: instance.Id("i-machine-0"),
			AgentStatus: params.DetailedStatus{
				Status: "started",
				Data:   make(map[string]interface{}),
			},
			InstanceStatus: params.DetailedStatus{
				Status: status.StatusPending.String(),
				Data:   make(map[string]interface{}),
			},
			Series:     "quantal",
			Containers: map[string]params.MachineStatus{},
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobManageModel},
			HasVote:    false,
			WantsVote:  true,
		},
		"1": {
			Id:         "1",
			InstanceId: instance.Id("i-machine-1"),
			AgentStatus: params.DetailedStatus{
				Status: "started",
				Data:   make(map[string]interface{}),
			},
			InstanceStatus: params.DetailedStatus{
				Status: status.StatusPending.String(),
				Data:   make(map[string]interface{}),
			},
			Series:     "quantal",
			Containers: map[string]params.MachineStatus{},
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			HasVote:    false,
			WantsVote:  false,
		},
		"2": {
			Id:         "2",
			InstanceId: instance.Id("i-machine-2"),
			AgentStatus: params.DetailedStatus{
				Status: "started",
				Data:   make(map[string]interface{}),
			},
			InstanceStatus: params.DetailedStatus{
				Status: status.StatusPending.String(),
				Data:   make(map[string]interface{}),
			},
			Series:     "quantal",
			Containers: map[string]params.MachineStatus{},
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			HasVote:    false,
			WantsVote:  false,
		},
	},
	Applications: map[string]params.ApplicationStatus{
		"logging": {
			Charm:  "local:quantal/logging-1",
			Series: "quantal",
			Relations: map[string][]string{
				"logging-directory": {"wordpress"},
			},
			SubordinateTo: []string{"wordpress"},
			// TODO(fwereade): why does the subordinate have no service status?
		},
		"mysql": {
			Charm:         "local:quantal/mysql-1",
			Series:        "quantal",
			Relations:     map[string][]string{},
			SubordinateTo: []string{},
			Units:         map[string]params.UnitStatus{},
			Status: params.DetailedStatus{
				Status: "unknown",
				Info:   "Waiting for agent initialization to finish",
				Data:   map[string]interface{}{},
			},
		},
		"wordpress": {
			Charm:  "local:quantal/wordpress-3",
			Series: "quantal",
			Relations: map[string][]string{
				"logging-dir": {"logging"},
			},
			SubordinateTo: []string{},
			Status: params.DetailedStatus{
				Status: "error",
				Info:   "blam",
				Data:   map[string]interface{}{"remote-unit": "logging/0", "foo": "bar", "relation-id": "0"},
			},
			Units: map[string]params.UnitStatus{
				"wordpress/0": {
					WorkloadStatus: params.DetailedStatus{
						Status: "error",
						Info:   "blam",
						Data:   map[string]interface{}{"relation-id": "0"},
					},
					AgentStatus: params.DetailedStatus{
						Status: "idle",
						Data:   make(map[string]interface{}),
					},
					Machine: "1",
					Subordinates: map[string]params.UnitStatus{
						"logging/0": {
							WorkloadStatus: params.DetailedStatus{
								Status: "unknown",
								Info:   "Waiting for agent initialization to finish",
								Data:   make(map[string]interface{}),
							},
							AgentStatus: params.DetailedStatus{
								Status: "allocating",
								Data:   map[string]interface{}{},
							},
						},
					},
				},
				"wordpress/1": {
					WorkloadStatus: params.DetailedStatus{
						Status: "unknown",
						Info:   "Waiting for agent initialization to finish",
						Data:   make(map[string]interface{}),
					},
					AgentStatus: params.DetailedStatus{
						Status: "allocating",
						Info:   "",
						Data:   make(map[string]interface{}),
					},

					Machine: "2",
					Subordinates: map[string]params.UnitStatus{
						"logging/1": {
							WorkloadStatus: params.DetailedStatus{
								Status: "unknown",
								Info:   "Waiting for agent initialization to finish",
								Data:   make(map[string]interface{}),
							},
							AgentStatus: params.DetailedStatus{
								Status: "allocating",
								Info:   "",
								Data:   make(map[string]interface{}),
							},
						},
					},
				},
			},
		},
	},
	Relations: []params.RelationStatus{
		{
			Id:  0,
			Key: "logging:logging-directory wordpress:logging-dir",
			Endpoints: []params.EndpointStatus{
				{
					ApplicationName: "logging",
					Name:            "logging-directory",
					Role:            "requirer",
					Subordinate:     true,
				},
				{
					ApplicationName: "wordpress",
					Name:            "logging-dir",
					Role:            "provider",
					Subordinate:     false,
				},
			},
			Interface: "logging",
			Scope:     "container",
		},
	},
}

// setUpScenario makes an environment scenario suitable for
// testing most kinds of access scenario. It returns
// a list of all the entities in the scenario.
//
// When the scenario is initialized, we have:
// user-admin
// user-other
// machine-0
//  instance-id="i-machine-0"
//  nonce="fake_nonce"
//  jobs=manage-environ
//  status=started, info=""
// machine-1
//  instance-id="i-machine-1"
//  nonce="fake_nonce"
//  jobs=host-units
//  status=started, info=""
//  constraints=mem=1G
// machine-2
//  instance-id="i-machine-2"
//  nonce="fake_nonce"
//  jobs=host-units
//  status=started, info=""
// application-wordpress
// application-logging
// unit-wordpress-0
//  deployer-name=machine-1
//  status=down with error and status data attached
// unit-logging-0
//  deployer-name=unit-wordpress-0
// unit-wordpress-1
//     deployer-name=machine-2
// unit-logging-1
//  deployer-name=unit-wordpress-1
//
// The passwords for all returned entities are
// set to the entity name with a " password" suffix.
//
// Note that there is nothing special about machine-0
// here - it's the environment manager in this scenario
// just because machine 0 has traditionally been the
// environment manager (bootstrap machine), so is
// hopefully easier to remember as such.
func (s *baseSuite) setUpScenario(c *gc.C) (entities []names.Tag) {
	add := func(e state.Entity) {
		entities = append(entities, e.Tag())
	}
	u, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	setDefaultPassword(c, u)
	add(u)
	err = s.State.UpdateModelConfig(map[string]interface{}{
		config.AgentVersionKey: "1.2.3"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	u = s.Factory.MakeUser(c, &factory.UserParams{Name: "other"})
	setDefaultPassword(c, u)
	add(u)

	m, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, names.NewMachineTag("0"))
	err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	setDefaultPassword(c, m)
	setDefaultStatus(c, m)
	add(m)
	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(wu.Tag(), gc.Equals, names.NewUnitTag(fmt.Sprintf("wordpress/%d", i)))
		setDefaultPassword(c, wu)
		add(wu)

		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Tag(), gc.Equals, names.NewMachineTag(fmt.Sprintf("%d", i+1)))
		if i == 1 {
			err = m.SetConstraints(constraints.MustParse("mem=1G"))
			c.Assert(err, jc.ErrorIsNil)
		}
		err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		setDefaultPassword(c, m)
		setDefaultStatus(c, m)
		add(m)

		err = wu.AssignToMachine(m)
		c.Assert(err, jc.ErrorIsNil)

		deployer, ok := wu.DeployerTag()
		c.Assert(ok, jc.IsTrue)
		c.Assert(deployer, gc.Equals, names.NewMachineTag(fmt.Sprintf("%d", i+1)))

		wru, err := rel.Unit(wu)
		c.Assert(err, jc.ErrorIsNil)

		// Put wordpress/0 in error state (with extra status data set)
		if i == 0 {
			sd := map[string]interface{}{
				"relation-id": "0",
				// these this should get filtered out
				// (not in StatusData whitelist)
				"remote-unit": "logging/0",
				"foo":         "bar",
			}
			now := time.Now()
			sInfo := status.StatusInfo{
				Status:  status.StatusError,
				Message: "blam",
				Data:    sd,
				Since:   &now,
			}
			err := wu.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)
		}

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)

		lu, err := s.State.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(lu.IsPrincipal(), jc.IsFalse)
		deployer, ok = lu.DeployerTag()
		c.Assert(ok, jc.IsTrue)
		c.Assert(deployer, gc.Equals, names.NewUnitTag(fmt.Sprintf("wordpress/%d", i)))
		setDefaultPassword(c, lu)
		s.setAgentPresence(c, wu)
		add(lu)
	}
	return
}

func (s *baseSuite) setAgentPresence(c *gc.C, u *state.Unit) {
	pinger, err := u.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		c.Assert(worker.Stop(pinger), jc.ErrorIsNil)
	})

	s.State.StartSync()
	s.BackingState.StartSync()
	err = u.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)
}
