package ec2_test

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	eec2 "launchpad.net/juju/go/environs/ec2"
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
	eec2.Regions["test"] = aws.Region{}
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

func (t *localTests) testInstanceGroups(c *C, conn *ec2.EC2) {
	env, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil)

	ec2conn := env.(*eec2.Environ).EC2

	gnames := []string{
		fmt.Sprintf("juju-%s", t.Name),
		fmt.Sprintf("juju-%s-%d", t.Name, 98),
		fmt.Sprintf("juju-%s-%d", t.Name, 99),
	}

	inst0, err := env.StartInstance(98)
	c.Assert(err, IsNil)
	defer env.StopInstances([]environs.Instance{inst0})

	// create a same-named group for the second instance
	// before starting it, to check that it's deleted and
	// recreated correctly.
	oldGroupId := ensureGroupExists(c, ec2conn, gnames[2], "old group")

	inst1, err := env.StartInstance(99)
	c.Assert(err, IsNil)
	defer env.StopInstances([]environs.Instance{inst1})

	// go behind the scenes the check the machines have
	// been put into the correct groups.

	// first check that the groups have been created.
	groupsResp, err := ec2conn.SecurityGroups(gnames, nil)
	c.Assert(err, IsNil)
	c.Assert(len(groupsResp.SecurityGroups), Equals, len(gnames))

	// find the SecurityGroup for each group name
	groups := make([]ec2.SecurityGroup, len(gnames))
	for i, name := range gnames {
		found := false
		for j, g := range groupsResp.SecurityGroups {
			if g.GroupName == name {
				groups[i] = g
				found = true
				break
			}
		}
		if !found {
			c.Fatalf("group %q not found", name)
		}
	}

	// check that each instance is part of the correct groups.
	resp, err := ec2conn.Instances([]string{inst0.Id(), inst1.Id()}, nil)
	c.Assert(err, IsNil)
	c.Assert(len(resp.Reservations), Equals, 2)
	for _, r := range resp.Reservations {
		c.Assert(len(r.Instances), Equals, 1)
		c.Assert(hasSecurityGroup(r, groups[0]), Equals, true)
		inst := r.Instances[0]
		switch inst.InstanceId {
		case inst0.Id():
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, true)
			c.Assert(hasSecurityGroup(r, groups[2]), Equals, false)
		case inst1.Id():
			c.Assert(hasSecurityGroup(r, groups[2]), Equals, true)
			c.Assert(groups[2].GroupId, Not(Equals), oldGroupId)
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, false)
		default:
			c.Errorf("unknown instance found: %v", inst)
		}
	}
}

// createGroup creates a new EC2 group if it doesn't already
// exist, and returns the id of the group.
func ensureGroupExists(c *C, ec2conn *ec2.EC2, name, descr string) string {
	groups, err := ec2conn.SecurityGroups([]string{name}, nil)
	c.Assert(err, IsNil)

	if len(groups) > 0 {
		return groups[0].GroupId
	}

	_, err = ec2conn.CreateSecurityGroup(jujuGroupName, "juju group for "+e.name)
	c.Assert(err, IsNil)

	groups, err = ec2conn.SecurityGroups([]string{name}, nil)
	c.Assert(err, IsNil)

	// current version of API means we have to query the security
	// groups to get the group id.
	return groups[0].GroupId
}

func hasSecurityGroup(r ec2.Reservation, g ec2.SecurityGroup) bool {
	for _, id := range r.SecurityGroups {
		if id == g.GroupId {
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
	srv.srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	eec2.Regions["test"] = aws.Region{
		EC2Endpoint: srv.srv.Address(),
	}
	srv.setup(srv.srv)
}

func (srv *localServer) stopServer(c *C) {
	srv.srv.Quit()
	// Clear out the region because the server address is
	// no longer valid.
	eec2.Regions["test"] = aws.Region{}
}
