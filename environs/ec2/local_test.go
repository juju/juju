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
	ec2srv *ec2test.Server
	s3srv  *s3test.Server
	setup  func(*localServer)
}

// Each test is run in each of the following scenarios.
// A scenario is implemented by mutating the ec2test
// server after it starts.
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

func (t *localTests) TestBootstrapInstanceAndState(c *C) {
	env, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil)

	info, err := env.Bootstrap()
	// TODO check that bootstrap state corresponds to the actual
	// machine we've started
	c.Assert(info, NotNil)
	c.Assert(err, IsNil)

	insts, err := env.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)

	inst := t.srv.ec2srv.Instance(insts[0].Id())
	c.Assert(inst, NotNil)

	x := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(inst.UserData, &x)
	c.Assert(err, IsNil)

	ec2.CheckPackage(c, x, "zookeeper")
	ec2.CheckPackage(c, x, "zookeeperd")
	ec2.CheckScripts(c, x, "juju-admin initialize")
	ec2.CheckScripts(c, x, "python -m juju.agents.provision")
	ec2.CheckScripts(c, x, "python -m juju.agents.machine")

	state, err := ec2.LoadState(env)
	c.Assert(err, IsNil)
	c.Assert(len(state.ZookeeperInstances), Equals, 1)
	c.Assert(state.ZookeeperInstances[0], Equals, insts[0].Id())

	err = env.Destroy()
	c.Assert(err, IsNil)

	_, err = ec2.LoadState(env)
	c.Assert(err, NotNil)
}

func (t *localTests) TestInstanceGroups(c *C) {
	env, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil)

	ec2conn := amzec2.New(aws.Auth{}, ec2.Regions["test"])

	groups := amzec2.SecurityGroupNames(
		fmt.Sprintf("juju-%s", t.Name),
		fmt.Sprintf("juju-%s-%d", t.Name, 98),
		fmt.Sprintf("juju-%s-%d", t.Name, 99),
	)

	inst0, err := env.StartInstance(98, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer env.StopInstances([]environs.Instance{inst0})

	// create a same-named group for the second instance
	// before starting it, to check that it's deleted and
	// recreated correctly.
	oldGroup := ensureGroupExists(c, ec2conn, groups[2], "old group")

	inst1, err := env.StartInstance(99, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer env.StopInstances([]environs.Instance{inst1})

	// go behind the scenes to check the machines have
	// been put into the correct groups.

	// first check that the old group has been deleted
	groupsResp, err := ec2conn.SecurityGroups([]amzec2.SecurityGroup{oldGroup}, nil)
	c.Assert(err, IsNil)
	c.Check(len(groupsResp.Groups), Equals, 0)

	// then check that the groups have been created.
	groupsResp, err = ec2conn.SecurityGroups(groups, nil)
	c.Assert(err, IsNil)
	c.Assert(len(groupsResp.Groups), Equals, len(groups))

	// for each group, check that it exists and record its id.
	for i, group := range groups {
		found := false
		for _, g := range groupsResp.Groups {
			if g.Name == group.Name {
				groups[i].Id = g.Id
				found = true
				break
			}
		}
		if !found {
			c.Fatalf("group %q not found", group.Name)
		}
	}

	// check that each instance is part of the correct groups.
	resp, err := ec2conn.Instances([]string{inst0.Id(), inst1.Id()}, nil)
	c.Assert(err, IsNil)
	c.Assert(len(resp.Reservations), Equals, 2, Bug("reservations %#v", resp.Reservations))
	for _, r := range resp.Reservations {
		c.Assert(len(r.Instances), Equals, 1)
		// each instance must be part of the general juju group.
		msg := Bug("reservation %#v", r)
		c.Assert(hasSecurityGroup(r, groups[0]), Equals, true, msg)
		inst := r.Instances[0]
		switch inst.InstanceId {
		case inst0.Id():
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, true, msg)
			c.Assert(hasSecurityGroup(r, groups[2]), Equals, false, msg)
		case inst1.Id():
			c.Assert(hasSecurityGroup(r, groups[2]), Equals, true, msg)

			// check that the id of the second machine's group
			// has changed - this implies that StartInstance has
			// correctly deleted and re-created the group.
			c.Assert(groups[2].Id, Not(Equals), oldGroup.Id)
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, false, msg)
		default:
			c.Errorf("unknown instance found: %v", inst)
		}
	}
}

// createGroup creates a new EC2 group if it doesn't already
// exist, and returns full SecurityGroup.
func ensureGroupExists(c *C, ec2conn *amzec2.EC2, group amzec2.SecurityGroup, descr string) amzec2.SecurityGroup {
	groups, err := ec2conn.SecurityGroups([]amzec2.SecurityGroup{group}, nil)
	c.Assert(err, IsNil)
	if len(groups.Groups) > 0 {
		return groups.Groups[0].SecurityGroup
	}

	resp, err := ec2conn.CreateSecurityGroup(group.Name, descr)
	c.Assert(err, IsNil)

	return resp.SecurityGroup
}

func hasSecurityGroup(r amzec2.Reservation, g amzec2.SecurityGroup) bool {
	for _, rg := range r.SecurityGroups {
		if rg.Id == g.Id {
			return true
		}
	}
	return false
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
