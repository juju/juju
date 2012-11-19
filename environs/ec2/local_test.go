package ec2_test

import (
	"launchpad.net/goamz/aws"
	amzec2 "launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/goamz/s3"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"regexp"
	"strings"
)

func registerLocalTests() {
	// N.B. Make sure the region we use here
	// has entries in the images/query txt files.
	aws.Regions["test"] = aws.Region{
		Name: "test",
	}
	attrs := map[string]interface{}{
		"name":           "sample",
		"type":           "ec2",
		"region":         "test",
		"control-bucket": "test-bucket",
		"public-bucket":  "public-tools",
		"admin-secret":   "local-secret",
		"access-key":     "x",
		"secret-key":     "x",
	}

	Suite(&localServerSuite{
		Tests: jujutest.Tests{
			Config: attrs,
		},
	})
	Suite(&localLiveSuite{
		LiveTests: LiveTests{
			LiveTests: jujutest.LiveTests{
				Config: attrs,
			},
		},
	})
}

// localLiveSuite runs tests from LiveTests using a fake
// EC2 server that runs within the test process itself.
type localLiveSuite struct {
	testing.LoggingSuite
	LiveTests
	srv localServer
	env environs.Environ
}

func (t *localLiveSuite) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	ec2.UseTestImageData(true)
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
	ec2.UseTestImageData(false)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *localLiveSuite) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
}

func (t *localLiveSuite) TearDownTest(c *C) {
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}

// localServer represents a fake EC2 server running within
// the test process itself.
type localServer struct {
	ec2srv *ec2test.Server
	s3srv  *s3test.Server
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
	aws.Regions["test"] = aws.Region{
		Name:                 "test",
		EC2Endpoint:          srv.ec2srv.URL(),
		S3Endpoint:           srv.s3srv.URL(),
		S3LocationConstraint: true,
	}
	s3inst := s3.New(aws.Auth{}, aws.Regions["test"])
	putFakeTools(c, ec2.BucketStorage(s3inst.Bucket("public-tools")))
	srv.addSpice(c)
}

// putFakeTools sets up a bucket containing something
// that looks like a tools archive so test methods
// that start an instance can succeed even though they
// do not upload tools.
func putFakeTools(c *C, s environs.StorageWriter) {
	path := environs.ToolsStoragePath(version.Current)
	c.Logf("putting fake tools at %v", path)
	toolsContents := "tools archive, honest guv"
	err := s.Put(path, strings.NewReader(toolsContents), int64(len(toolsContents)))
	c.Assert(err, IsNil)
}

// addSpice adds some "spice" to the local server
// by adding state that may cause tests to fail.
func (srv *localServer) addSpice(c *C) {
	states := []amzec2.InstanceState{
		ec2test.ShuttingDown,
		ec2test.Terminated,
		ec2test.Stopped,
	}
	for _, state := range states {
		srv.ec2srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
	}
}

func (srv *localServer) stopServer(c *C) {
	srv.ec2srv.Quit()
	srv.s3srv.Quit()
	// Clear out the region because the server address is
	// no longer valid.
	delete(aws.Regions, "test")
}

// localServerSuite contains tests that run against a fake EC2 server
// running within the test process itself.  These tests can test things that
// would be unreasonably slow or expensive to test on a live Amazon server.
// It starts a new local ec2test server for each test.  The server is
// accessed by using the "test" region, which is changed to point to the
// network address of the local server.
type localServerSuite struct {
	testing.LoggingSuite
	jujutest.Tests
	srv localServer
	env environs.Environ
}

func (t *localServerSuite) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	ec2.UseTestImageData(true)
	ec2.UseTestMetadata(true)
	t.Tests.SetUpSuite(c)
	ec2.ShortTimeouts(true)
}

func (t *localServerSuite) TearDownSuite(c *C) {
	t.Tests.TearDownSuite(c)
	ec2.ShortTimeouts(false)
	ec2.UseTestMetadata(false)
	ec2.UseTestImageData(false)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *localServerSuite) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.srv.startServer(c)
	t.Tests.SetUpTest(c)
	t.env = t.Tests.Env
}

func (t *localServerSuite) TearDownTest(c *C) {
	t.Tests.TearDownTest(c)
	t.srv.stopServer(c)
	t.LoggingSuite.TearDownTest(c)
}

func (t *localServerSuite) TestBootstrapInstanceUserDataAndState(c *C) {
	policy := t.env.AssignmentPolicy()
	c.Assert(policy, Equals, state.AssignUnused)

	err := juju.Bootstrap(t.env, true, []byte(testing.CACertPEM+testing.CAKeyPEM))
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	state, err := ec2.LoadState(t.env)
	c.Assert(err, IsNil)
	c.Assert(state.StateInstances, HasLen, 1)

	insts, err := t.env.Instances(state.StateInstances)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, state.StateInstances[0])

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
	CheckPackage(c, x, "git", true)
	CheckScripts(c, x, "jujud bootstrap-state", true)
	// TODO check for provisioning agent
	// TODO check for machine agent

	// check that a new instance will be started without
	// zookeeper, with a machine agent, and without a
	// provisioning agent.
	info.EntityName = "machine-1"
	inst1, err := t.env.StartInstance(1, info, nil)
	c.Assert(err, IsNil)
	inst = t.srv.ec2srv.Instance(inst1.Id())
	c.Assert(inst, NotNil)
	c.Logf("second instance: UserData: %q", inst.UserData)
	x = nil
	err = goyaml.Unmarshal(inst.UserData, &x)
	c.Assert(err, IsNil)
	CheckPackage(c, x, "zookeeperd", false)
	// TODO check for provisioning agent
	// TODO check for machine agent

	p := t.env.Provider()
	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "public.dummy.address.example.com")
	addr, err = p.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "private.dummy.address.example.com")

	err = t.env.Destroy(append(insts, inst1))
	c.Assert(err, IsNil)

	_, err = ec2.LoadState(t.env)
	c.Assert(err, NotNil)
}

// If match is true, CheckScripts checks that at least one script started
// by the cloudinit data matches the given regexp pattern, otherwise it
// checks that no script matches.  It's exported so it can be used by tests
// defined in ec2_test.
func CheckScripts(c *C, x map[interface{}]interface{}, pattern string, match bool) {
	scripts0 := x["runcmd"]
	if scripts0 == nil {
		c.Errorf("cloudinit has no entry for runcmd")
		return
	}
	scripts := scripts0.([]interface{})
	re := regexp.MustCompile(pattern)
	found := false
	for _, s0 := range scripts {
		s := s0.(string)
		if re.MatchString(s) {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("script %q not found in %q", pattern, scripts)
	case !match && found:
		c.Errorf("script %q found but not expected in %q", pattern, scripts)
	}
}

// CheckPackage checks that the cloudinit will or won't install the given
// package, depending on the value of match.  It's exported so it can be
// used by tests defined outside the ec2 package.
func CheckPackage(c *C, x map[interface{}]interface{}, pkg string, match bool) {
	pkgs0 := x["packages"]
	if pkgs0 == nil {
		if match {
			c.Errorf("cloudinit has no entry for packages")
		}
		return
	}

	pkgs := pkgs0.([]interface{})

	found := false
	for _, p0 := range pkgs {
		p := p0.(string)
		if p == pkg {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("package %q not found in %v", pkg, pkgs)
	case !match && found:
		c.Errorf("%q found but not expected in %v", pkg, pkgs)
	}
}
