// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	stdtesting "testing"
	"time"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type baseSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&baseSuite{})

func chanReadEmpty(c *C, ch <-chan struct{}, what string) bool {
	select {
	case _, ok := <-ch:
		return ok
	case <-time.After(10 * time.Second):
	}
	c.Fatalf("timed out reading from %s", what)
}

func chanReadStrings(c *C, ch <-chan []string, what string) ([]string, bool) {
	select {
	case changes, ok := <-ch:
		return changes, ok
	case <-time.After(10 * time.Second):
	}
	c.Fatalf("timed out reading from %s", what)
}

func chanReadConfig(c *C, ch <-chan *config.Config, what string) (*config.Config, bool) {
	select {
	case envConfig, ok := <-ch:
		return envConfig, ok
	case <-time.After(10 * time.Second):
	}
	c.Fatalf("timed out reading from %s", what)
}

func removeServiceAndUnits(c *C, service *state.Service) {
	// Destroy all units for the service.
	units, err := service.AllUnits()
	c.Assert(err, IsNil)
	for _, unit := range units {
		err = unit.EnsureDead()
		c.Assert(err, IsNil)
		err = unit.Remove()
		c.Assert(err, IsNil)
	}
	err = service.Destroy()
	c.Assert(err, IsNil)

	err = service.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

// apiAuthenticator represents a simple authenticator object with only the
// SetPassword and Tag methods.  This will fit types from both the state
// and api packages, as those in the api package do not have PasswordValid().
type apiAuthenticator interface {
	state.Tagger
	SetPassword(string) error
}

func setDefaultPassword(c *C, e apiAuthenticator) {
	err := e.SetPassword(defaultPassword(e))
	c.Assert(err, IsNil)
}

func defaultPassword(e apiAuthenticator) string {
	return e.Tag() + " password"
}

type setStatuser interface {
	SetStatus(status params.Status, info string) error
}

func setDefaultStatus(c *C, entity setStatuser) {
	err := entity.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
}

func (s *baseSuite) tryOpenState(c *C, e apiAuthenticator, password string) error {
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
func (s *baseSuite) openAs(c *C, tag string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.Tag = tag
	info.Password = fmt.Sprintf("%s password", tag)
	// Set this always, so that the login attempts as a machine will
	// not fail with ErrNotProvisioned; it's not used otherwise.
	info.Nonce = "fake_nonce"
	c.Logf("opening state; entity %q; password %q", info.Tag, info.Password)
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	return st
}

// scenarioStatus describes the expected state
// of the juju environment set up by setUpScenario.
var scenarioStatus = &api.Status{
	Machines: map[string]api.MachineInfo{
		"0": {
			InstanceId: "i-machine-0",
		},
		"1": {
			InstanceId: "i-machine-1",
		},
		"2": {
			InstanceId: "i-machine-2",
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
func (s *baseSuite) setUpScenario(c *C) (entities []string) {
	add := func(e state.Tagger) {
		entities = append(entities, e.Tag())
	}
	u, err := s.State.User("admin")
	c.Assert(err, IsNil)
	setDefaultPassword(c, u)
	add(u)

	u, err = s.State.AddUser("other", "")
	c.Assert(err, IsNil)
	setDefaultPassword(c, u)
	add(u)

	m, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.Tag(), Equals, "machine-0")
	err = m.SetProvisioned(instance.Id("i-"+m.Tag()), "fake_nonce", nil)
	c.Assert(err, IsNil)
	setDefaultPassword(c, m)
	setDefaultStatus(c, m)
	add(m)

	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)

	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)

	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, IsNil)
		c.Assert(wu.Tag(), Equals, fmt.Sprintf("unit-wordpress-%d", i))
		setDefaultPassword(c, wu)
		add(wu)

		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, IsNil)
		c.Assert(m.Tag(), Equals, fmt.Sprintf("machine-%d", i+1))
		if i == 1 {
			err = m.SetConstraints(constraints.MustParse("mem=1G"))
			c.Assert(err, IsNil)
		}
		err = m.SetProvisioned(instance.Id("i-"+m.Tag()), "fake_nonce", nil)
		c.Assert(err, IsNil)
		setDefaultPassword(c, m)
		setDefaultStatus(c, m)
		add(m)

		err = wu.AssignToMachine(m)
		c.Assert(err, IsNil)

		deployer, ok := wu.DeployerTag()
		c.Assert(ok, Equals, true)
		c.Assert(deployer, Equals, fmt.Sprintf("machine-%d", i+1))

		wru, err := rel.Unit(wu)
		c.Assert(err, IsNil)

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, IsNil)

		lu, err := s.State.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, IsNil)
		c.Assert(lu.IsPrincipal(), Equals, false)
		deployer, ok = lu.DeployerTag()
		c.Assert(ok, Equals, true)
		c.Assert(deployer, Equals, fmt.Sprintf("unit-wordpress-%d", i))
		setDefaultPassword(c, lu)
		add(lu)
	}
	return
}
