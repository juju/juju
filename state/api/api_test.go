package api_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state"
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

var badLoginTests = []struct{
	entityName string
	password string
	err string
}{{
	entityName: "user-admin",
	password: "wrong password",
	err: "invalid entity name or password",
}, {
	entityName: "user-foo",
	password: "password",
	err: "invalid entity name or password",
}, {
	entityName: "bar",
	password: "password",
	err: `invalid entity name "bar"`,
}}

func (s *suite) TestBadLogin(c *C) {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	for i, t := range badLoginTests {
		c.Logf("test %d; entity %q; password %q", i, t.entityName, t.password)
		info.EntityName = ""
		info.Password = ""
		func(){
			st, err := api.Open(info)
			c.Assert(err, IsNil)
			defer st.Close()

			_, err = st.Machine("0")
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
	st, err := s.openAs(stm.EntityName(), "machine password")
	c.Assert(err, IsNil)

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
		Password: "password",
		Addrs: []string{srv.Addr()},
		CACert: []byte(coretesting.CACert),
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

func (s *suite) openAs(entityName, password string) (*api.State, error) {
	_, info, err := s.APIConn.Environ.StateInfo()
	if err != nil {
		return nil, err
	}
	info.EntityName = entityName
	info.Password = password
	return api.Open(info)
}
