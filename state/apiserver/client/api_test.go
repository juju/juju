// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type baseSuite struct {
	testing.JujuConnSuite
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

func removeServiceAndUnits(c *gc.C, service *state.Service) {
	// Destroy all units for the service.
	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	for _, unit := range units {
		err = unit.EnsureDead()
		c.Assert(err, gc.IsNil)
		err = unit.Remove()
		c.Assert(err, gc.IsNil)
	}
	err = service.Destroy()
	c.Assert(err, gc.IsNil)

	err = service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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
	c.Assert(err, gc.IsNil)
}

func defaultPassword(e apiAuthenticator) string {
	return e.Tag() + " password-1234567890"
}

type setStatuser interface {
	SetStatus(status params.Status, info string, data params.StatusData) error
}

func setDefaultStatus(c *gc.C, entity setStatuser) {
	err := entity.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
}

func (s *baseSuite) tryOpenState(c *gc.C, e apiAuthenticator, password string) error {
	stateInfo := s.StateInfo(c)
	stateInfo.Tag = e.Tag()
	stateInfo.Password = password
	st, err := state.Open(stateInfo, state.DialOpts{
		Timeout: 25 * time.Millisecond,
	})
	if err == nil {
		st.Close()
	}
	return err
}

// openAs connects to the API state as the given entity
// with the default password for that entity.
func (s *baseSuite) openAs(c *gc.C, tag string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	info.Tag = tag
	// Must match defaultPassword()
	info.Password = fmt.Sprintf("%s password-1234567890", tag)
	// Set this always, so that the login attempts as a machine will
	// not fail with ErrNotProvisioned; it's not used otherwise.
	info.Nonce = "fake_nonce"
	c.Logf("opening state; entity %q; password %q", info.Tag, info.Password)
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	return st
}

// scenarioStatus describes the expected state
// of the juju environment set up by setUpScenario.
//
// NOTE: AgentState: "down", AgentStateInfo: "(started)" here is due
// to the scenario not calling SetAgentAlive on the respective entities,
// but this behavior is already tested in cmd/juju/status_test.go and
// also tested live and it works.
var scenarioStatus = &api.Status{
	EnvironmentName: "dummyenv",
	Machines: map[string]api.MachineStatus{
		"0": {
			Id:             "0",
			InstanceId:     instance.Id("i-machine-0"),
			AgentState:     "down",
			AgentStateInfo: "(started)",
			Series:         "quantal",
			Containers:     map[string]api.MachineStatus{},
		},
		"1": {
			Id:             "1",
			InstanceId:     instance.Id("i-machine-1"),
			AgentState:     "down",
			AgentStateInfo: "(started)",
			Series:         "quantal",
			Containers:     map[string]api.MachineStatus{},
		},
		"2": {
			Id:             "2",
			InstanceId:     instance.Id("i-machine-2"),
			AgentState:     "down",
			AgentStateInfo: "(started)",
			Series:         "quantal",
			Containers:     map[string]api.MachineStatus{},
		},
	},
	Services: map[string]api.ServiceStatus{
		"logging": api.ServiceStatus{
			Charm: "local:quantal/logging-1",
			Relations: map[string][]string{
				"logging-directory": []string{"wordpress"},
			},
			SubordinateTo: []string{"wordpress"},
		},
		"mysql": api.ServiceStatus{
			Charm:         "local:quantal/mysql-1",
			Relations:     map[string][]string{},
			SubordinateTo: []string{},
			Units:         map[string]api.UnitStatus{},
		},
		"wordpress": api.ServiceStatus{
			Charm: "local:quantal/wordpress-3",
			Relations: map[string][]string{
				"logging-dir": []string{"logging"},
			},
			SubordinateTo: []string{},
			Units: map[string]api.UnitStatus{
				"wordpress/0": api.UnitStatus{
					AgentState: "pending",
					Machine:    "1",
					Subordinates: map[string]api.UnitStatus{
						"logging/0": api.UnitStatus{
							AgentState: "pending",
						},
					},
				},
				"wordpress/1": api.UnitStatus{
					AgentState: "pending",
					Machine:    "2",
					Subordinates: map[string]api.UnitStatus{
						"logging/1": api.UnitStatus{
							AgentState: "pending",
						},
					},
				},
			},
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
// service-wordpress
// service-logging
// unit-wordpress-0
//     deployer-name=machine-1
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
func (s *baseSuite) setUpScenario(c *gc.C) (entities []string) {
	add := func(e state.Entity) {
		entities = append(entities, e.Tag())
	}
	u, err := s.State.User("admin")
	c.Assert(err, gc.IsNil)
	setDefaultPassword(c, u)
	add(u)

	u, err = s.State.AddUser("other", "")
	c.Assert(err, gc.IsNil)
	setDefaultPassword(c, u)
	add(u)

	m, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Tag(), gc.Equals, "machine-0")
	err = m.SetProvisioned(instance.Id("i-"+m.Tag()), "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	setDefaultPassword(c, m)
	setDefaultStatus(c, m)
	add(m)
	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, gc.IsNil)
		c.Assert(wu.Tag(), gc.Equals, fmt.Sprintf("unit-wordpress-%d", i))
		setDefaultPassword(c, wu)
		add(wu)

		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, gc.IsNil)
		c.Assert(m.Tag(), gc.Equals, fmt.Sprintf("machine-%d", i+1))
		if i == 1 {
			err = m.SetConstraints(constraints.MustParse("mem=1G"))
			c.Assert(err, gc.IsNil)
		}
		err = m.SetProvisioned(instance.Id("i-"+m.Tag()), "fake_nonce", nil)
		c.Assert(err, gc.IsNil)
		setDefaultPassword(c, m)
		setDefaultStatus(c, m)
		add(m)

		err = wu.AssignToMachine(m)
		c.Assert(err, gc.IsNil)

		deployer, ok := wu.DeployerTag()
		c.Assert(ok, gc.Equals, true)
		c.Assert(deployer, gc.Equals, fmt.Sprintf("machine-%d", i+1))

		wru, err := rel.Unit(wu)
		c.Assert(err, gc.IsNil)

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, gc.IsNil)

		lu, err := s.State.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, gc.IsNil)
		c.Assert(lu.IsPrincipal(), gc.Equals, false)
		deployer, ok = lu.DeployerTag()
		c.Assert(ok, gc.Equals, true)
		c.Assert(deployer, gc.Equals, fmt.Sprintf("unit-wordpress-%d", i))
		setDefaultPassword(c, lu)
		add(lu)
	}
	return
}
