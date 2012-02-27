package ec2_test

import (
	"fmt"
	"launchpad.net/goamz/aws"
	amzec2 "launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
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
    admin-secret: verysecret
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
	ec2.ShortTimeouts(true)
}

func (t *localLiveSuite) TearDownSuite(c *C) {
	t.LiveTests.TearDownSuite(c)
	t.srv.stopServer(c)
	t.env = nil
	ec2.ShortTimeouts(false)
}

func (t *localLiveSuite) TestBootstrap(c *C) {
	c.Skip("cannot test bootstrap on local server")
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

func (t *localServerSuite) SetUpSuite(c *C) {
	t.Tests.SetUpSuite(c)
	ec2.ShortTimeouts(true)
}

func (t *localServerSuite) TearDownSuite(c *C) {
	t.Tests.TearDownSuite(c)
	ec2.ShortTimeouts(false)
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

func (t *localServerSuite) TestBootstrapInstanceUserDataAndState(c *C) {
	err := t.env.Bootstrap()
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	state, err := ec2.LoadState(t.env)
	c.Assert(err, IsNil)
	c.Assert(state.ZookeeperInstances, HasLen, 1)

	insts, err := t.env.Instances(state.ZookeeperInstances)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, state.ZookeeperInstances[0])

	info, err := t.env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)

	// check that the user data is configured to start zookeeper
	// and the machine and provisioning agents.
	inst := t.srv.ec2srv.Instance(insts[0].Id())
	c.Assert(inst, NotNil)
	bootstrapDNS, err := insts[0].DNSName()
	c.Assert(err, IsNil)
	c.Assert(bootstrapDNS, Not(Equals), "")

	c.Logf("first instance: UserData: %q", inst.UserData)
	var x map[interface{}]interface{}
	err = goyaml.Unmarshal(inst.UserData, &x)
	c.Assert(err, IsNil)
	ec2.CheckPackage(c, x, "zookeeper", true)
	ec2.CheckPackage(c, x, "zookeeperd", true)
	ec2.CheckScripts(c, x, "juju-admin initialize", true)
	ec2.CheckScripts(c, x, "python -m juju.agents.provision", true)
	ec2.CheckScripts(c, x, "python -m juju.agents.machine", true)
	ec2.CheckScripts(c, x, fmt.Sprintf("JUJU_ZOOKEEPER='localhost%s'", ec2.ZkPortSuffix), true)
	ec2.CheckScripts(c, x, fmt.Sprintf("JUJU_MACHINE_ID='0'"), true)

	// check that a new instance will be started without
	// zookeeper, with a machine agent, and without a
	// provisioning agent.
	inst1, err := t.env.StartInstance(1, info)
	c.Assert(err, IsNil)
	inst = t.srv.ec2srv.Instance(inst1.Id())
	c.Assert(inst, NotNil)
	c.Logf("second instance: UserData: %q", inst.UserData)
	x = nil
	err = goyaml.Unmarshal(inst.UserData, &x)
	c.Assert(err, IsNil)
	ec2.CheckPackage(c, x, "zookeeperd", false)
	ec2.CheckPackage(c, x, "python-zookeeper", true)
	ec2.CheckScripts(c, x, "python -m juju.agents.machine", true)
	ec2.CheckScripts(c, x, "python -m juju.agents.provision", false)
	ec2.CheckScripts(c, x, fmt.Sprintf("JUJU_ZOOKEEPER='%s%s'", bootstrapDNS, ec2.ZkPortSuffix), true)
	ec2.CheckScripts(c, x, fmt.Sprintf("JUJU_MACHINE_ID='1'"), true)

	err = t.env.Destroy(append(insts, inst1))
	c.Assert(err, IsNil)

	_, err = ec2.LoadState(t.env)
	c.Assert(err, NotNil)
}
