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

type permTest struct {
	entityName string
	password   string
	err        string
}

// testAsDifferentEntities runs a test as several different entities.
// For each test, it sets up the default scenario, connects
// as the specified entity and runs the given check function.
// If the password is blank, it is derived from the entity name.
// If reset is true, the state will be set up from scratch for each
// test.
func (s *suite) testAsDifferentEntities(c *C, reset bool, check func(st *api.State) error, tests []permTest) {
	for i, t := range tests {
		c.Logf("test %d. entity %q; password %q", i, t.entityName, t.password)
		if i == 0 || reset {
			s.Reset(c)
			s.setupScenario(c)
		}
		password := t.password
		if password == "" {
			password = fmt.Sprintf("%s password", t.entityName)
		}
		st := s.openAs(c, t.entityName, password)
		err := check(st)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
		} else {
			c.Check(err, IsNil)
		}
	}
}

// setupScenario makes an environment scenario suitable for
// testing most kinds of access scenario.
// After the scenario is set up, we have:
// machine-0
//     password="machine-0 password
//	instance-id="i-machine-0"
//	jobs=manage-environ
// machine-1
//	password="machine-1 password"
//	instance-id="i-machine-1"
//	jobs=host-units
// machine-2
//	password="machine-2 password"
//	instance-id="i-machine-2"
//	jobs=host-units
// service-wordpress
// service-logging
// unit-wordpress-0
//	password="unit-wordpress-0 password"
//     deployer-name=machine-1
// unit-logging-0
//	password="unit-logging-0 password"
//	deployer-name=unit-wordpress-0
// unit-wordpress-1
//	password="unit-wordpress-0 password"
//     deployer-name=machine-2
// unit-logging-1
//	password="unit-logging-0 password"
//	deployer-name=unit-wordpress-1
func (s *suite) setupScenario(c *C) {
	m, err := s.State.AddMachine(state.JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.EntityName(), Equals, "machine-0")
	err = m.SetInstanceId(state.InstanceId("i-" + m.EntityName()))
	c.Assert(err, IsNil)
	setDefaultPassword(c, m)

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

		err = wu.SetPassword(fmt.Sprintf("%s password", wu.EntityName()))
		c.Assert(err, IsNil)

		m, err := s.State.AddMachine(state.JobHostUnits)
		c.Assert(err, IsNil)
		c.Assert(m.EntityName(), Equals, fmt.Sprintf("machine-%d", i+1))
		err = m.SetInstanceId(state.InstanceId("i-" + m.EntityName()))
		c.Assert(err, IsNil)
		setDefaultPassword(c, m)

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
	}
}

func setDefaultPassword(c *C, e state.AuthEntity) {
	err := e.SetPassword(e.EntityName() + " password")
	c.Assert(err, IsNil)
}

func (s *suite) TestUnitGet(c *C) {
	s.testAsDifferentEntities(c, false, func(st *api.State) error {
		u, err := st.Unit("wordpress/0")
		if err != nil {
			c.Check(u, IsNil)
		} else {
			name, ok := u.DeployerName()
			c.Check(ok, Equals, true)
			c.Check(name, Equals, "machine-1")
		}
		return err
	}, []permTest{{
		entityName: "user-admin",
		password:   s.Conn.Environ.Config().AdminSecret(),
		err:        "permission denied",
	}, {
		entityName: "machine-0",
	}, {
		entityName: "unit-wordpress-0",
	}, {
		entityName: "unit-logging-0",
	}, {
		entityName: "unit-wordpress-1",
	}, {
		entityName: "unit-logging-1",
	}, {
		entityName: "machine-1",
	}})
}

func (s *suite) TestMachineGet(c *C) {
	s.testAsDifferentEntities(c, false, func(st *api.State) error {
		m, err := st.Machine("1")
		if err != nil {
			c.Check(m, IsNil)
		} else {
			name, err := m.InstanceId()
			c.Assert(err, IsNil)
			c.Assert(name, Equals, "i-machine-1")
		}
		return err
	}, []permTest{{
		entityName: "user-admin",
		password:   s.Conn.Environ.Config().AdminSecret(),
		err:        "permission denied",
	}, {
		entityName: "machine-0",
	}, {
		entityName: "unit-wordpress-0",
	}, {
		entityName: "unit-logging-0",
	}, {
		entityName: "unit-wordpress-1",
	}, {
		entityName: "unit-logging-1",
	}, {
		entityName: "machine-1",
	}})
}

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
	err = stm.SetPassword("machine password")
	c.Assert(err, IsNil)

	// Normal users can't access Machines...
	m, err := s.APIState.Machine(stm.Id())
	c.Assert(err, ErrorMatches, "permission denied")
	c.Assert(m, IsNil)

	// ... so login as the machine.
	st := s.openAs(c, stm.EntityName(), "machine password")

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

//func (s *suite) BenchmarkRequests(c *C) {
//	stm, err := s.State.AddMachine(state.JobHostUnits)
//	c.Assert(err, IsNil)
//	c.ResetTimer()
//	for i := 0; i < c.N; i++ {
//		_, err := s.APIState.Machine(stm.Id())
//		if err != nil {
//			c.Assert(err, IsNil)
//		}
//	}
//}
//
//func (s *suite) BenchmarkTestRequests(c *C) {
//	for i := 0; i < c.N; i++ {
//		err := s.APIState.TestRequest()
//		if err != nil {
//			c.Assert(err, IsNil)
//		}
//	}
//}

func (s *suite) openAs(c *C, entityName, password string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.EntityName = entityName
	info.Password = password
	c.Logf("opening state; entity %q; password %q", entityName, password)
	st, err := api.Open(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	return st
}
