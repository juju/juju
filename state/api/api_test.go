package api_test

import (
	"errors"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	stdtesting "testing"
	"time"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type suite struct {
	testing.JujuConnSuite
	listener net.Listener
}

var _ = Suite(&suite{})

func init() {
	api.AuthenticationEnabled = true
}

var operationPermTests = []struct {
	about string
	// op performs the operation to be tested using the given state
	// connection.  It returns a function that should be used to
	// undo any changes made by the operation.
	op    func(c *C, st *api.State) (reset func(), err error)
	allow []string
	deny  []string
}{{
	about: "Unit.Get",
	op:    opGetUnitWordpress0,
	deny:  []string{"user-admin", "user-other"},
}, {
	about: "Machine.Get",
	op:    opGetMachine1,
	deny:  []string{"user-admin", "user-other"},
}, {
	about: "Machine.SetPassword",
	op:    opMachine1SetPassword,
	allow: []string{"machine-0", "machine-1"},
}, {
	about: "Unit.SetPassword (on principal unit)",
	op:    opUnitSetPassword("wordpress/0"),
	allow: []string{"unit-wordpress-0", "machine-1"},
}, {
	about: "Unit.SetPassword (on subordinate unit)",
	op:    opUnitSetPassword("logging/0"),
	allow: []string{"unit-logging-0", "unit-wordpress-0"},
}, {
	about: "Client.Status",
	op:    opClientStatus,
	allow: []string{"user-admin", "user-other"},
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
				c.Check(api.ErrCode(err), Equals, api.CodeUnauthorized)
			}
			reset()
			st.Close()
		}
	}
}

func opGetUnitWordpress0(c *C, st *api.State) (func(), error) {
	u, err := st.Unit("wordpress/0")
	if err != nil {
		c.Check(u, IsNil)
	} else {
		name, ok := u.DeployerName()
		c.Check(ok, Equals, true)
		c.Check(name, Equals, "machine-1")
	}
	return func() {}, err
}

func opUnitSetPassword(unitName string) func(c *C, st *api.State) (func(), error) {
	return func(c *C, st *api.State) (func(), error) {
		u, err := st.Unit(unitName)
		if err != nil {
			c.Check(u, IsNil)
			return func() {}, err
		}
		err = u.SetPassword("another password")
		if err != nil {
			return func() {}, err
		}
		return func() {
			setDefaultPassword(c, u)
		}, nil
	}
}

func opGetMachine1(c *C, st *api.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
	} else {
		name, ok := m.InstanceId()
		c.Assert(ok, Equals, true)
		c.Assert(name, Equals, "i-machine-1")
	}
	return func() {}, err
}

func opMachine1SetPassword(c *C, st *api.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	err = m.SetPassword("another password")
	if err != nil {
		return func() {}, err
	}
	return func() {
		setDefaultPassword(c, m)
	}, nil
}

func opClientStatus(c *C, st *api.State) (func(), error) {
	status, err := st.Client().Status()
	if err != nil {
		c.Check(status, IsNil)
		return func() {}, err
	}
	c.Assert(err, IsNil)
	c.Assert(status, DeepEquals, scenarioStatus)
	return func() {}, nil
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
//  jobs=manage-environ
// machine-1
//  instance-id="i-machine-1"
//  jobs=host-units
// machine-2
//  instance-id="i-machine-2"
//  jobs=host-units
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

	m, err := s.State.AddMachine("series", state.JobManageEnviron)
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

		m, err := s.State.AddMachine("series", state.JobHostUnits)
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

// AuthEntity is the same as state.AuthEntity but
// without PasswordValid, which is implemented
// by state entities but not by api entities.
type AuthEntity interface {
	EntityName() string
	SetPassword(pass string) error
	Refresh() error
}

func setDefaultPassword(c *C, e AuthEntity) {
	err := e.SetPassword(e.EntityName() + " password")
	c.Assert(err, IsNil)
}

var badLoginTests = []struct {
	entityName string
	password   string
	err        string
	code       string
}{{
	entityName: "user-admin",
	password:   "wrong password",
	err:        "invalid entity name or password",
	code:       api.CodeUnauthorized,
}, {
	entityName: "user-foo",
	password:   "password",
	err:        "invalid entity name or password",
	code:       api.CodeUnauthorized,
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
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized, Commentf("error %#v", err))

			_, err = st.Unit("foo/0")
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)

			err = st.Login(t.entityName, t.password)
			c.Assert(err, ErrorMatches, t.err)
			c.Assert(api.ErrCode(err), Equals, t.code)

			_, err = st.Machine("0")
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)
		}()
	}
}

func (s *suite) TestClientStatus(c *C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status()
	c.Assert(err, IsNil)
	c.Assert(status, DeepEquals, scenarioStatus)
}

func (s *suite) TestClientEnvironmentInfo(c *C) {
	conf, _ := s.State.EnvironConfig()
	info, err := s.APIState.Client().EnvironmentInfo()
	c.Assert(err, IsNil)
	c.Assert(info.DefaultSeries, Equals, conf.DefaultSeries())
	c.Assert(info.ProviderType, Equals, conf.Type())
}

func (s *suite) TestMachineLogin(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
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

	instId, ok := m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instId, Equals, "i-foo")
}

func (s *suite) TestMachineInstanceId(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	// Normal users can't access Machines...
	m, err := s.APIState.Machine(stm.Id())
	c.Assert(err, ErrorMatches, "permission denied")
	c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)
	c.Assert(m, IsNil)

	// ... so login as the machine.
	st := s.openAs(c, stm.EntityName())
	defer st.Close()

	m, err = st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, ok := m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(ok, Equals, false)

	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)

	instId, ok = m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(ok, Equals, false)

	err = m.Refresh()
	c.Assert(err, IsNil)

	instId, ok = m.InstanceId()
	c.Check(ok, Equals, true)
	c.Assert(instId, Equals, "foo")
}

func (s *suite) TestMachineRefresh(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)

	st := s.openAs(c, stm.EntityName())
	defer st.Close()
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, ok := m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instId, Equals, "foo")

	err = stm.SetInstanceId("bar")
	c.Assert(err, IsNil)

	instId, ok = m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instId, Equals, "foo")

	err = m.Refresh()
	c.Assert(err, IsNil)

	instId, ok = m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instId, Equals, "bar")
}

func (s *suite) TestMachineSetPassword(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.EntityName())
	defer st.Close()
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	err = m.SetPassword("foo")
	c.Assert(err, IsNil)

	err = stm.Refresh()
	c.Assert(err, IsNil)
	c.Assert(stm.PasswordValid("foo"), Equals, true)
}

func (s *suite) TestMachineEntityName(c *C) {
	c.Assert(api.MachineEntityName("2"), Equals, "machine-2")

	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	st := s.openAs(c, "machine-0")
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.EntityName(), Equals, "machine-0")
}

func (s *suite) TestMachineWatch(c *C) {
	stm, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.EntityName())
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)
	w0 := m.Watch()
	w1 := m.Watch()

	// Initial event.
	ok := chanRead(c, w0.Changes(), "watcher 0")
	c.Assert(ok, Equals, true)

	ok = chanRead(c, w1.Changes(), "watcher 1")
	c.Assert(ok, Equals, true)

	// No subsequent event until something changes.
	select {
	case <-w0.Changes():
		c.Fatalf("unexpected value on watcher 0")
	case <-w1.Changes():
		c.Fatalf("unexpected value on watcher 1")
	case <-time.After(20 * time.Millisecond):
	}

	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)
	s.State.StartSync()

	// Next event.
	ok = chanRead(c, w0.Changes(), "watcher 0")
	c.Assert(ok, Equals, true)
	ok = chanRead(c, w1.Changes(), "watcher 1")
	c.Assert(ok, Equals, true)

	err = w0.Stop()
	c.Check(err, IsNil)
	err = w1.Stop()
	c.Check(err, IsNil)

	ok = chanRead(c, w0.Changes(), "watcher 0")
	c.Assert(ok, Equals, false)
	ok = chanRead(c, w1.Changes(), "watcher 1")
	c.Assert(ok, Equals, false)
}

func chanRead(c *C, ch <-chan struct{}, what string) (ok bool) {
	select {
	case _, ok := <-ch:
		return ok
	case <-time.After(10 * time.Second):
		c.Fatalf("timed out reading from %s", what)
	}
	panic("unreachable")
}

func (s *suite) TestUnitRefresh(c *C) {
	s.setUpScenario(c)
	st := s.openAs(c, "unit-wordpress-0")
	defer st.Close()

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

func (s *suite) TestErrors(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	st := s.openAs(c, stm.EntityName())
	defer st.Close()
	// By testing this single call, we test that the
	// error transformation function is correctly called
	// on error returns from the API server. The transformation
	// function itself is tested below.
	_, err = st.Machine("99")
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
}

var errorTransformTests = []struct {
	err  error
	code string
}{{
	err:  state.NotFoundf("hello"),
	code: api.CodeNotFound,
}, {
	err:  state.ErrUnauthorized,
	code: api.CodeUnauthorized,
}, {
	err:  state.ErrCannotEnterScopeYet,
	code: api.CodeCannotEnterScopeYet,
}, {
	err:  state.ErrCannotEnterScope,
	code: api.CodeCannotEnterScope,
}, {
	err:  state.ErrExcessiveContention,
	code: api.CodeExcessiveContention,
}, {
	err:  state.ErrUnitHasSubordinates,
	code: api.CodeUnitHasSubordinates,
}, {
	err:  api.ErrBadId,
	code: api.CodeNotFound,
}, {
	err:  api.ErrBadCreds,
	code: api.CodeUnauthorized,
}, {
	err:  api.ErrPerm,
	code: api.CodeUnauthorized,
}, {
	err:  api.ErrNotLoggedIn,
	code: api.CodeUnauthorized,
}, {
	err:  api.ErrUnknownWatcher,
	code: api.CodeNotFound,
}, {
	err:  &state.NotAssignedError{&state.Unit{}}, // too sleazy?!
	code: api.CodeNotAssigned,
}, {
	err:  api.ErrStoppedWatcher,
	code: api.CodeStopped,
}, {
	err:  errors.New("an error"),
	code: "",
}}

func (s *suite) TestErrorTransform(c *C) {
	for _, t := range errorTransformTests {
		err1 := api.ServerError(t.err)
		c.Assert(err1.Error(), Equals, t.err.Error())
		if t.code != "" {
			c.Assert(api.ErrCode(err1), Equals, t.code)
		} else {
			c.Assert(err1, Equals, t.err)
		}
	}
}

func (s *suite) TestUnitEntityName(c *C) {
	c.Assert(api.UnitEntityName("wordpress/2"), Equals, "unit-wordpress-2")

	s.setUpScenario(c)
	st := s.openAs(c, "unit-wordpress-0")
	defer st.Close()
	u, err := st.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(u.EntityName(), Equals, "unit-wordpress-0")
}

func (s *suite) TestStop(c *C) {
	// Start our own instance of the server so have
	// a handle on it to stop it.
	srv, err := api.NewServer(s.State, "localhost:0", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, IsNil)

	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, IsNil)

	// Note we can't use openAs because we're
	// not connecting to s.APIConn.
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

	_, err = st.Machine(stm.Id())
	// The client has not necessarily seen the server
	// shutdown yet, so there are two possible
	// errors.
	if err != rpc.ErrShutdown && err != io.ErrUnexpectedEOF {
		c.Fatalf("unexpected error from request: %v", err)
	}

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
