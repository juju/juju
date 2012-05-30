package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
	"launchpad.net/juju/go/state"
	"launchpad.net/juju/go/testing"

	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/log"

	"time"
)

func init() {
	log.Debug = true
}

type ProvisioningSuite struct {
	zkConn *zookeeper.Conn
	st     *state.State
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) SetUpTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)

	s.zkConn = zk
	info := &state.Info{
		Addrs: []string{zkAddr},
	}
	s.st, err = state.Initialize(info)
	c.Assert(err, IsNil)

	dummy.Reset()

	// seed /environment to point to dummy
	env, err := s.st.Environment()
	c.Assert(err, IsNil)
	env.Set("type", "dummy")
	env.Set("zookeeper", false)
	env.Set("name", "testing")
	_, err = env.Write()
	c.Assert(err, IsNil)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	testing.ZkRemoveTree(s.zkConn, "/")
	s.zkConn.Close()
}

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *main.AgentConf) {
		a := &main.ProvisioningAgent{}
		return a, &a.Conf
	}
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	a := &main.ProvisioningAgent{}
	err := ParseAgentCommand(a, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["nincompoops"\]`)
}

func initProvisioningAgent() (*main.ProvisioningAgent, error) {
	args := []string{"--zookeeper-servers", zkAddr}
	c := main.NewProvisioningAgent()
	return c, initCmd(c, args)
}

func runPA(c *C, a *main.ProvisioningAgent) chan bool {
	done := make(chan bool)
	go func() {
		defer close(done)
		err := a.Run(nil) // context is unused
		c.Assert(err, ErrorMatches, ".*ZooKeeper connection closed; ")
	}()
	return done
}

// Start and stop one machine, watch the PA.
func (s *ProvisioningSuite) TestSimple(c *C) {
	a, err := initProvisioningAgent()
	c.Assert(err, IsNil)

	done := runPA(c, a)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// place a new machine into the state
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Fatalf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// now remove it
	c.Assert(s.st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStopInstances)
	case <-time.After(3 * time.Second):
		c.Fatalf("ProvisioningAgent did not action RemoveMachine after 3 second")
	}

	// close the PA's underlying state, shutting it down.
	// this races with the PA writing data to the state,
	// so pause a little.
	<-time.After(2 * time.Second)
	a.State.Close()

	<-done // blocks until PA exits
}

// Start and stop lots machines, watch the PA.
func (s *ProvisioningSuite) TestHard(c *C) {
	var N = 200

	a, err := initProvisioningAgent()
	c.Assert(err, IsNil)

	paDone := runPA(c, a)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	go func() {
		for i := 0; i < N; i++ {
			m, err := s.st.AddMachine()
			<-time.After(20 * time.Millisecond)
			c.Assert(err, IsNil)
			c.Logf("added machine: %d", m.Id())
			go func() {
				err := s.st.RemoveMachine(m.Id())
				c.Assert(err, IsNil)
				c.Logf("removed machine: %d", m.Id())
			}()
		}
	}()

	var started, stopped int

	for stopped < N {
		select {
		case o := <-op:
			switch o.Kind {
			case dummy.OpStartInstance:
				started++
			case dummy.OpStopInstances:
				stopped++
			default:
				c.Fatalf("dummy reported unknown operation: %v", o)
			}
		case <-time.After(10 * time.Second):
			c.Fatalf("PA stalled")
		}
	}

	c.Assert(started, Equals, N)
	c.Assert(stopped, Equals, N)

	// close the PA's underlying state, shutting it down.
	// this races with the PA writing data to the state,
	// so pause a little.
	<-time.After(2 * time.Second)
	a.State.Close()

	<-paDone // will block until the PA exits
}
