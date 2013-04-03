package ec2_test

import (
	"bytes"
	"launchpad.net/goamz/aws"
	amzec2 "launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/goamz/s3"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/environs/jujutest"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"regexp"
)

type ProviderSuite struct{}

var _ = Suite(&ProviderSuite{})

var testImagesContent = []jujutest.FileContent{{
	Name: "/query/precise/server/released.current.txt",
	Content: "" +
		"precise\tserver\trelease\t20121017\tebs\tamd64\ttest\tami-20800c10\taki-98e26fa8\t\tparavirtual\n" +
		"precise\tserver\trelease\t20121017\tebs\ti386\ttest\tami-00000034\tparavirtual\n",
}, {
	Name: "/query/quantal/server/released.current.txt",
	Content: "" +
		"quantal\tserver\trelease\t20121017\tebs\tamd64\ttest\tami-40f97070\taki-98e26fa8\t\tparavirtual\n" +
		"quantal\tserver\trelease\t20121017\tebs\ti386\ttest\tami-01000034\taki-98e26fa8\t\tparavirtual\n",
}, {
	Name: "/query/raring/server/released.current.txt",
	Content: "" +
		"raring\tserver\trelease\t20121017\tebs\tamd64\ttest\tami-40f97070\taki-98e26fa8\t\tparavirtual\n" +
		"raring\tserver\trelease\t20121017\tebs\ti386\ttest\tami-40f97070\taki-98e26fa8\t\tparavirtual\n",
}}

// testInstanceTypeContent holds the cost in USDe-3/hour for each of the
// few available instance types in  the convenient fictional "test" region.
var testInstanceTypeContent = map[string]uint64{
	"m1.small":  60,
	"m1.medium": 120,
	"m1.large":  240,
	"m1.xlarge": 480,
	"t1.micro":  020,
}

func (s *ProviderSuite) TestMetadata(c *C) {
	metadataContent := []jujutest.FileContent{
		{"/2011-01-01/meta-data/instance-id", "dummy.instance.id"},
		{"/2011-01-01/meta-data/public-hostname", "public.dummy.address.invalid"},
		{"/2011-01-01/meta-data/local-hostname", "private.dummy.address.invalid"},
	}
	ec2.UseTestMetadata(metadataContent)
	defer ec2.UseTestMetadata(nil)

	p, err := environs.Provider("ec2")
	c.Assert(err, IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "public.dummy.address.invalid")

	addr, err = p.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "private.dummy.address.invalid")

	id, err := p.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, state.InstanceId("dummy.instance.id"))
}

func registerLocalTests() {
	// N.B. Make sure the region we use here
	// has entries in the images/query txt files.
	aws.Regions["test"] = aws.Region{
		Name: "test",
	}
	attrs := map[string]interface{}{
		"name":                 "sample",
		"type":                 "ec2",
		"region":               "test",
		"control-bucket":       "test-bucket",
		"public-bucket":        "public-tools",
		"public-bucket-region": "test",
		"admin-secret":         "local-secret",
		"access-key":           "x",
		"secret-key":           "x",
		"authorized-keys":      "foo",
		"ca-cert":              testing.CACert,
		"ca-private-key":       testing.CAKey,
	}

	Suite(&localServerSuite{
		Tests: jujutest.Tests{
			TestConfig: jujutest.TestConfig{attrs},
		},
	})
	Suite(&localLiveSuite{
		LiveTests: LiveTests{
			LiveTests: jujutest.LiveTests{
				TestConfig: jujutest.TestConfig{attrs},
			},
		},
	})
	Suite(&localNonUSEastSuite{
		tests: jujutest.Tests{
			TestConfig: jujutest.TestConfig{attrs},
		},
		srv: localServer{
			config: &s3test.Config{
				Send409Conflict: true,
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
	ec2.UseTestImageData(testImagesContent)
	ec2.UseTestInstanceTypeData(testInstanceTypeContent)
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
	ec2.UseTestImageData(nil)
	ec2.UseTestInstanceTypeData(nil)
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
	config *s3test.Config
}

func (srv *localServer) startServer(c *C) {
	var err error
	srv.ec2srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	srv.s3srv, err = s3test.NewServer(srv.config)
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
	envtesting.PutFakeTools(c, ec2.BucketStorage(s3inst.Bucket("public-tools")))
	srv.addSpice(c)
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
	ec2.UseTestImageData(testImagesContent)
	ec2.UseTestInstanceTypeData(testInstanceTypeContent)
	t.Tests.SetUpSuite(c)
	ec2.ShortTimeouts(true)
}

func (t *localServerSuite) TearDownSuite(c *C) {
	t.Tests.TearDownSuite(c)
	ec2.ShortTimeouts(false)
	ec2.UseTestImageData(nil)
	ec2.UseTestInstanceTypeData(nil)
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
	c.Assert(policy, Equals, state.AssignNew)

	err := environs.UploadTools(t.env)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(t.env, constraints.Value{})
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	bootstrapState, err := ec2.LoadState(t.env)
	c.Assert(err, IsNil)
	c.Assert(bootstrapState.StateInstances, HasLen, 1)

	insts, err := t.env.Instances(bootstrapState.StateInstances)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, bootstrapState.StateInstances[0])

	info, apiInfo, err := t.env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)

	// check that the user data is configured to start zookeeper
	// and the machine and provisioning agents.
	inst := t.srv.ec2srv.Instance(string(insts[0].Id()))
	c.Assert(inst, NotNil)
	bootstrapDNS, err := insts[0].DNSName()
	c.Assert(err, IsNil)
	c.Assert(bootstrapDNS, Not(Equals), "")

	userData, err := trivial.Gunzip(inst.UserData)
	c.Assert(err, IsNil)
	c.Logf("first instance: UserData: %q", userData)
	var x map[interface{}]interface{}
	err = goyaml.Unmarshal(userData, &x)
	c.Assert(err, IsNil)
	CheckPackage(c, x, "git", true)
	CheckScripts(c, x, "jujud bootstrap-state", true)
	// TODO check for provisioning agent
	// TODO check for machine agent

	// check that a new instance will be started without
	// zookeeper, with a machine agent, and without a
	// provisioning agent.
	series := version.DefaultSeries()
	info.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	inst1, err := t.env.StartInstance("1", series, constraints.Value{}, info, apiInfo)
	c.Assert(err, IsNil)
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, NotNil)
	userData, err = trivial.Gunzip(inst.UserData)
	c.Assert(err, IsNil)
	c.Logf("second instance: UserData: %q", userData)
	x = nil
	err = goyaml.Unmarshal(userData, &x)
	c.Assert(err, IsNil)
	CheckPackage(c, x, "zookeeperd", false)
	// TODO check for provisioning agent
	// TODO check for machine agent

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

// localNonUSEastSuite is similar to localServerSuite but the S3 mock server
// behaves as if
type localNonUSEastSuite struct {
	testing.LoggingSuite
	tests jujutest.Tests
	srv   localServer
	env   environs.Environ
}

func (t *localNonUSEastSuite) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	ec2.UseTestImageData(testImagesContent)
	ec2.UseTestInstanceTypeData(testInstanceTypeContent)
	t.tests.SetUpSuite(c)
	ec2.ShortTimeouts(true)
}

func (t *localNonUSEastSuite) TearDownSuite(c *C) {
	ec2.ShortTimeouts(false)
	ec2.UseTestImageData(nil)
	ec2.UseTestInstanceTypeData(nil)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *localNonUSEastSuite) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.srv.startServer(c)
	t.tests.SetUpTest(c)
	t.env = t.tests.Env
}

func (t *localNonUSEastSuite) TearDownTest(c *C) {
	t.tests.TearDownTest(c)
	t.srv.stopServer(c)
	t.LoggingSuite.TearDownTest(c)
}

func (t *localNonUSEastSuite) TestPutBucket(c *C) {
	p := ec2.WritablePublicStorage(t.env).(ec2.Storage)
	for i := 0; i < 5; i++ {
		p.ResetMadeBucket()
		var buf bytes.Buffer
		err := p.Put("test-file", &buf, 0)
		c.Assert(err, IsNil)
	}
}
