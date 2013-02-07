package api_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type suite struct {
	testing.JujuConnSuite
	listener net.Listener
}

var _ = Suite(&suite{})

var operationPermTests = []struct {
	about string
	op    func(c *C, st *api.State) (bool, error)
	allow []string
	deny  []string
}{{
	about: "Unit.Get",
	op:    opGetUnit,
	deny:  []string{"user-admin", "user-other"},
}, {
	about: "Machine.Get",
	op:    opGetMachine,
	deny:  []string{"user-admin", "user-other"},
},
}

// allowed returns the set of allowed entities given an allow list and a
// deny list.  If an allow list is specified, only those entities are
// allowed; otherwise those in deny are disallowed.
func allowed(all, allow, deny []string) map[string]bool {
	p := make(map[string]bool)
	if allow != nil {
		for _, e := range allow {
			p[e] = true
		}
		return p
	}
loop:
	for _, e0 := range all {
		for _, e1 := range deny {
			if e1 == e0 {
				continue loop
			}
		}
		p[e0] = true
	}
	return p
}

func (s *suite) TestOperationPerm(c *C) {
	entities := s.setUpScenario(c)
	for i, t := range operationPermTests {
		allow := allowed(entities, t.allow, t.deny)
		for _, e := range entities {
			c.Logf("test %d; %s; entity %q", i, t.about, e)
			st := s.openAs(c, e)
			reset, err := t.op(c, st)
			if allow[e] {
				c.Check(err, IsNil)
			} else {
				c.Check(err, ErrorMatches, "permission denied")
			}
			st.Close()
			if reset {
				s.Reset(c)
				s.setUpScenario(c)
			}
		}
	}
}

func opGetUnit(c *C, st *api.State) (bool, error) {
	u, err := st.Unit("wordpress/0")
	if err != nil {
		c.Check(u, IsNil)
	} else {
		name, ok := u.DeployerName()
		c.Check(ok, Equals, true)
		c.Check(name, Equals, "machine-1")
	}
	return false, err
}

func opGetMachine(c *C, st *api.State) (bool, error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
	} else {
		name, err := m.InstanceId()
		c.Assert(err, IsNil)
		c.Assert(name, Equals, "i-machine-1")
	}
	return false, err
}

// setUpScenario makes an environment scenario suitable for
// testing most kinds of access scenario. It returns
// a list of all the entities in the scenario.
// 
// When the scenario is initialized, we have:
// user-admin
// user-other
// machine-0
//	instance-id="i-machine-0"
//	jobs=manage-environ
// machine-1
//	instance-id="i-machine-1"
//	jobs=host-units
// machine-2
//	instance-id="i-machine-2"
//	jobs=host-units
// service-wordpress
// service-logging
// unit-wordpress-0
//     deployer-name=machine-1
// unit-logging-0
//	deployer-name=unit-wordpress-0
// unit-wordpress-1
//     deployer-name=machine-2
// unit-logging-1
//	deployer-name=unit-wordpress-1
//
// The passwords for all returned entities are
// set to the entity name with a " password" suffix.
func (s *suite) setUpScenario(c *C) (entities []string) {
	add := func(e state.AuthEntity) {
		entities = append(entities, e.EntityName())
	}
	u, err := s.State.User("admin")
	c.Assert(err, IsNil)
	setDefaultPassword(c, u)
	add(u)

	u, err = s.State.AddUser("other", "")
	c.Assert(err, IsNil)
	setDefaultPassword(c, u)
	add(u)

	m, err := s.State.AddMachine(state.JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.EntityName(), Equals, "machine-0")
	err = m.SetInstanceId(state.InstanceId("i-" + m.EntityName()))
	c.Assert(err, IsNil)
	setDefaultPassword(c, m)
	add(m)

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
		c.Assert(wu.EntityName(), Equals, fmt.Sprintf("unit-wordpress-%d", i))
		setDefaultPassword(c, wu)
		add(wu)

		m, err := s.State.AddMachine(state.JobHostUnits)
		c.Assert(err, IsNil)
		c.Assert(m.EntityName(), Equals, fmt.Sprintf("machine-%d", i+1))
		err = m.SetInstanceId(state.InstanceId("i-" + m.EntityName()))
		c.Assert(err, IsNil)
		setDefaultPassword(c, m)
		add(m)

		err = wu.AssignToMachine(m)
		c.Assert(err, IsNil)

		deployer, ok := wu.DeployerName()
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
		deployer, ok = lu.DeployerName()
		c.Assert(ok, Equals, true)
		c.Assert(deployer, Equals, fmt.Sprintf("unit-wordpress-%d", i))
		setDefaultPassword(c, lu)
		add(lu)
	}
	return
}

func setDefaultPassword(c *C, e state.AuthEntity) {
	err := e.SetPassword(e.EntityName() + " password")
	c.Assert(err, IsNil)
}

var badLoginTests = []struct {
	entityName string
	password   string
	err        string
}{{
	entityName: "user-admin",
	password:   "wrong password",
	err:        "invalid entity name or password",
}, {
	entityName: "user-foo",
	password:   "password",
	err:        "invalid entity name or password",
}, {
	entityName: "bar",
	password:   "password",
	err:        `invalid entity name "bar"`,
}}

func (s *suite) TestBadLogin(c *C) {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	for i, t := range badLoginTests {
		c.Logf("test %d; entity %q; password %q", i, t.entityName, t.password)
		info.EntityName = ""
		info.Password = ""
		func() {
			st, err := api.Open(info)
			c.Assert(err, IsNil)
			defer st.Close()

			_, err = st.Machine("0")
			c.Assert(err, ErrorMatches, "not logged in")

			_, err = st.Unit("foo/0")
			c.Assert(err, ErrorMatches, "not logged in")

			err = st.Login(t.entityName, t.password)
			c.Assert(err, ErrorMatches, t.err)

			_, err = st.Machine("0")
			c.Assert(err, ErrorMatches, "not logged in")
		}()
	}
}

func (s *suite) TestMachineLogin(c *C) {
	stm, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	err = stm.SetPassword("machine-password")
	c.Assert(err, IsNil)
	err = stm.SetInstanceId("i-foo")
	c.Assert(err, IsNil)

	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)

	info.EntityName = stm.EntityName()
	info.Password = "machine-password"

	st, err := api.Open(info)
	c.Assert(err, IsNil)
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, err := m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "i-foo")
}

func (s *suite) TestMachineInstanceId(c *C) {
	stm, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	// Normal users can't access Machines...
	m, err := s.APIState.Machine(stm.Id())
	c.Assert(err, ErrorMatches, "permission denied")
	c.Assert(m, IsNil)

	// ... so login as the machine.
	st := s.openAs(c, stm.EntityName())

	m, err = st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, err := m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(err, ErrorMatches, "instance id for machine 0 not found")

	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)

	instId, err = m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(err, ErrorMatches, "instance id for machine 0 not found")

	err = m.Refresh()
	c.Assert(err, IsNil)

	instId, err = m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "foo")
}

func (s *suite) TestMachineRefresh(c *C) {
	stm, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)

	st := s.openAs(c, stm.EntityName())
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, err := m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "foo")

	err = stm.SetInstanceId("bar")
	c.Assert(err, IsNil)

	instId, err = m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "foo")

	err = m.Refresh()
	c.Assert(err, IsNil)

	instId, err = m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "bar")
}

func (s *suite) TestUnitRefresh(c *C) {
	s.setUpScenario(c)
	st := s.openAs(c, "unit-wordpress-0")

	u, err := st.Unit("wordpress/0")
	c.Assert(err, IsNil)

	deployer, ok := u.DeployerName()
	c.Assert(ok, Equals, true)
	c.Assert(deployer, Equals, "machine-1")

	stu, err := s.State.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = stu.UnassignFromMachine()
	c.Assert(err, IsNil)

	deployer, ok = u.DeployerName()
	c.Assert(ok, Equals, true)
	c.Assert(deployer, Equals, "machine-1")

	err = u.Refresh()
	c.Assert(err, IsNil)

	deployer, ok = u.DeployerName()
	c.Assert(ok, Equals, false)
	c.Assert(deployer, Equals, "")
}

func (s *suite) TestStop(c *C) {
	// Start our own instance of the server so have
	// a handle on it to stop it.
	srv, err := api.NewServer(s.State, "localhost:0", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, IsNil)

	stm, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, IsNil)

	st, err := api.Open(&api.Info{
		EntityName: stm.EntityName(),
		Password:   "password",
		Addrs:      []string{srv.Addr()},
		CACert:     []byte(coretesting.CACert),
	})
	c.Assert(err, IsNil)
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, stm.Id())

	err = srv.Stop()
	c.Assert(err, IsNil)
	c.Logf("srv stopped")

	_, err = st.Machine(stm.Id())
	c.Assert(err, ErrorMatches, "cannot receive response: EOF")

	// Check it can be stopped twice.
	err = srv.Stop()
	c.Assert(err, IsNil)
}

// openAs connects to the API state as the given entity
// with the default password for that entity.
func (s *suite) openAs(c *C, entityName string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.EntityName = entityName
	info.Password = fmt.Sprintf("%s password", entityName)
	c.Logf("opening state; entity %q; password %q", info.EntityName, info.Password)
	st, err := api.Open(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	return st
}
