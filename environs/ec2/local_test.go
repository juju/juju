package ec2_test

import (
	"fmt"
	"launchpad.net/goamz/aws"
	amzec2 "launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/ec2"
	"launchpad.net/juju/go/environs/jujutest"
)

var functionalConfig = []byte(`
environments:
  sample:
    type: ec2
    region: test
    control-bucket: test-bucket
`)

// Each test is run in each of the following scenarios.  A scenario is
// implemented by mutating the ec2test server after it starts.
var scenarios = []struct {
	name  string
	setup func(*localServer)
}{
	{"normal", normalScenario},
	{"initial-state-running", initialStateRunningScenario},
	{"extra-instances", extraInstancesScenario},
}

func normalScenario(*localServer) {
}

func initialStateRunningScenario(srv *localServer) {
	srv.ec2srv.SetInitialInstanceState(ec2test.Running)
}

func extraInstancesScenario(srv *localServer) {
	states := []amzec2.InstanceState{
		ec2test.ShuttingDown,
		ec2test.Terminated,
		ec2test.Stopped,
	}
	for _, state := range states {
		srv.ec2srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
	}
}

func registerLocalTests() {
	ec2.Regions["test"] = aws.Region{}
	envs, err := environs.ReadEnvironsBytes(functionalConfig)
	if err != nil {
		panic(fmt.Errorf("cannot parse functional tests config data: %v", err))
	}

	for _, name := range envs.Names() {
		for _, scen := range scenarios {
			Suite(&localServerSuite{
				srv: localServer{setup: scen.setup},
				Tests: jujutest.Tests{
					Environs: envs,
					Name:     name,
				},
			})
			Suite(&localLiveSuite{
				srv: localServer{setup: scen.setup},
				LiveTests: LiveTests{
					jujutest.LiveTests{
						Environs: envs,
						Name:     name,
					},
				},
			})
		}
	}
}

// localLiveSuite runs tests from LiveTests using a fake
// EC2 server that runs within the test process itself.
type localLiveSuite struct {
	LiveTests
	srv localServer
	env environs.Environ
}

func (t *localLiveSuite) SetUpSuite(c *C) {
	t.srv.startServer(c)
	t.LiveTests.SetUpSuite(c)
	t.env = t.LiveTests.Env
}

func (t *localLiveSuite) TearDownSuite(c *C) {
	t.LiveTests.TearDownSuite(c)
	t.srv.stopServer(c)
	t.env = nil
}

// localServer represents a fake EC2 server running within
// the test process itself.
type localServer struct {
	ec2srv *ec2test.Server
	s3srv  *s3test.Server
	setup  func(*localServer)
}

func (srv *localServer) startServer(c *C) {
	var err error
	srv.ec2srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	srv.s3srv, err = s3test.NewServer()
	if err != nil {
		c.Fatalf("cannot start s3 test server: %v", err)
	}
	ec2.Regions["test"] = aws.Region{
		EC2Endpoint: srv.ec2srv.URL(),
		S3Endpoint:  srv.s3srv.URL(),
	}
	srv.setup(srv)
}

func (srv *localServer) stopServer(c *C) {
	srv.ec2srv.Quit()
	srv.s3srv.Quit()
	// Clear out the region because the server address is
	// no longer valid.
	ec2.Regions["test"] = aws.Region{}
}

// localServerSuite contains tests that run against a fake EC2 server
// running within the test process itself.  These tests can test things that
// would be unreasonably slow or expensive to test on a live Amazon server.
// It starts a new local ec2test server for each test.  The server is
// accessed by using the "test" region, which is changed to point to the
// network address of the local server.
type localServerSuite struct {
	jujutest.Tests
	srv localServer
	env environs.Environ
}

func (t *localServerSuite) SetUpTest(c *C) {
	t.srv.startServer(c)
	t.Tests.SetUpTest(c)
	t.env = t.Tests.Env
}

func (t *localServerSuite) TearDownTest(c *C) {
	t.Tests.TearDownTest(c)
	t.srv.stopServer(c)
}

func (t *localServerSuite) TestBootstrapInstanceAndState(c *C) {
	err := t.env.Bootstrap()
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	state, err := ec2.LoadState(t.env)
	c.Assert(err, IsNil)
	c.Assert(len(state.ZookeeperInstances), Equals, 1)

	insts, err := t.env.Instances(state.ZookeeperInstances)
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)
	c.Check(insts[0].Id(), Equals, state.ZookeeperInstances[0])

	// check that the user data is configured to start zookeeper
	// and the machine and provisioning agents.
	inst := t.srv.ec2srv.Instance(insts[0].Id())
	c.Assert(inst, NotNil)

	err = t.env.Destroy(insts)
	c.Assert(err, IsNil)

	_, err = ec2.LoadState(t.env)
	c.Assert(err, NotNil)
}
