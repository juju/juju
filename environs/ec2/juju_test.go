package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/jujutest"
)

var functionalConfig = []byte(`
environments:
  sample:
    type: ec2
    region: test
`)

// localTests wraps jujutest.Tests by adding
// set up and tear down functions that start a new
// ec2test server for each test.
// The server is accessed by using the "test" region,
// which is changed to point to the network address
// of the local server.
type localTests struct {
	*jujutest.Tests
	srv localServer
}

// localLiveTests performs the live test suite, but locally.
type localLiveTests struct {
	*jujutest.LiveTests
	srv localServer
}

type localServer struct {
	srv   *ec2test.Server
	setup func(*ec2test.Server)
}

// Each test is run in each of the following scenarios.
// A scenario is implemented by mutating the ec2test
// server after it starts.
var scenarios = []struct {
	name  string
	setup func(*ec2test.Server)
}{
	{"normal", normalScenario},
	{"initial-state-running", initialStateRunningScenario},
	{"extra-instances", extraInstancesScenario},
}

func normalScenario(*ec2test.Server) {
}

func initialStateRunningScenario(srv *ec2test.Server) {
	srv.SetInitialInstanceState(ec2test.Running)
}

func extraInstancesScenario(srv *ec2test.Server) {
	states := []ec2.InstanceState{
		ec2test.ShuttingDown,
		ec2test.Terminated,
		ec2test.Stopped,
	}
	for _, state := range states {
		srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
	}
}

func registerLocalTests() {
	Regions["test"] = aws.Region{}
	envs, err := environs.ReadEnvironsBytes(functionalConfig)
	if err != nil {
		panic(fmt.Errorf("cannot parse functional tests config data: %v", err))
	}

	for _, name := range envs.Names() {
		for _, scen := range scenarios {
			Suite(&localTests{
				srv: localServer{setup: scen.setup},
				Tests: &jujutest.Tests{
					Environs: envs,
					Name:     name,
				},
			})
			Suite(&localLiveTests{
				srv: localServer{setup: scen.setup},
				LiveTests: &jujutest.LiveTests{
					Environs: envs,
					Name:     name,
				},
			})
		}
	}
}

func (t *localTests) SetUpTest(c *C) {
	t.srv.startServer(c)
	t.Tests.SetUpTest(c)
}

func (t *localTests) TearDownTest(c *C) {
	t.Tests.TearDownTest(c)
	t.srv.stopServer(c)
}

func (t *localLiveTests) SetUpSuite(c *C) {
	t.srv.startServer(c)
	t.LiveTests.SetUpSuite(c)
}

func (t *localLiveTests) TearDownSuite(c *C) {
	t.srv.stopServer(c)
	t.LiveTests.TearDownSuite(c)
}

func (t *localLiveTests) TestStartStop(c *C) {
	c.Assert(Regions["test"].EC2Endpoint, Not(Equals), "")
	t.LiveTests.TestStartStop(c)
}

func (srv *localServer) startServer(c *C) {
	var err error
	srv.srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	Regions["test"] = aws.Region{
		EC2Endpoint: srv.srv.Address(),
	}
	srv.setup(srv.srv)
}

func (srv *localServer) stopServer(c *C) {
	srv.srv.Quit()
	// Clear out the region because the server address is
	// no longer valid.
	Regions["test"] = aws.Region{}
}
