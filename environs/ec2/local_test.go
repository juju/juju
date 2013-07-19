// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/jujutest"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"regexp"
)

type ProviderSuite struct{}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) TestMetadata(c *C) {
	metadataContent := []jujutest.FileContent{
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
	srv             localServer
	env             environs.Environ
	restoreTimeouts func()
}

func (t *localLiveSuite) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	ec2.UseTestImageData(ec2.TestImagesData)
	ec2.UseTestInstanceTypeData(ec2.TestInstanceTypeCosts)
	ec2.UseTestRegionData(ec2.TestRegions)
	t.srv.startServer(c)
	t.LiveTests.SetUpSuite(c)
	t.env = t.LiveTests.Env
	t.restoreTimeouts = envtesting.PatchAttemptStrategies(ec2.ShortAttempt)
}

func (t *localLiveSuite) TearDownSuite(c *C) {
	t.LiveTests.TearDownSuite(c)
	t.srv.stopServer(c)
	t.env = nil
	t.restoreTimeouts()
	ec2.UseTestImageData(nil)
	ec2.UseTestInstanceTypeData(nil)
	ec2.UseTestRegionData(nil)
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
	writeablePublicStorage := ec2.BucketStorage(s3inst.Bucket("public-tools"))
	envtesting.UploadFakeTools(c, writeablePublicStorage)
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
	srv             localServer
	env             environs.Environ
	restoreTimeouts func()
}

func (t *localServerSuite) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	ec2.UseTestImageData(ec2.TestImagesData)
	ec2.UseTestInstanceTypeData(ec2.TestInstanceTypeCosts)
	ec2.UseTestRegionData(ec2.TestRegions)
	t.Tests.SetUpSuite(c)
	t.restoreTimeouts = envtesting.PatchAttemptStrategies(ec2.ShortAttempt)
}

func (t *localServerSuite) TearDownSuite(c *C) {
	t.Tests.TearDownSuite(c)
	t.restoreTimeouts()
	ec2.UseTestImageData(nil)
	ec2.UseTestInstanceTypeData(nil)
	ec2.UseTestRegionData(nil)
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
	envtesting.UploadFakeTools(c, t.env.Storage())
	err := environs.Bootstrap(t.env, constraints.Value{})
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	bootstrapState, err := environs.LoadState(t.env.Storage())
	c.Assert(err, IsNil)
	c.Assert(bootstrapState.StateInstances, HasLen, 1)

	expectedHardware := instance.MustParseHardware("arch=amd64 cpu-cores=1 cpu-power=100 mem=1740M")
	insts, err := t.env.AllInstances()
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, bootstrapState.StateInstances[0])
	c.Check(expectedHardware, DeepEquals, bootstrapState.Characteristics[0])

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

	userData, err := utils.Gunzip(inst.UserData)
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
	series := t.env.Config().DefaultSeries()
	info.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	inst1, hc, err := t.env.StartInstance("1", "fake_nonce", series, constraints.Value{}, info, apiInfo)
	c.Assert(err, IsNil)
	c.Check(*hc.Arch, Equals, "amd64")
	c.Check(*hc.Mem, Equals, uint64(1740))
	c.Check(*hc.CpuCores, Equals, uint64(1))
	c.Assert(*hc.CpuPower, Equals, uint64(100))
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, NotNil)
	userData, err = utils.Gunzip(inst.UserData)
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

	_, err = environs.LoadState(t.env.Storage())
	c.Assert(err, NotNil)
}

func (t *localServerSuite) TestStartInstanceHardwareCharacteristics(c *C) {
	err := environs.Bootstrap(t.env, constraints.Value{})
	c.Assert(err, IsNil)
	series := t.env.Config().DefaultSeries()
	info, apiInfo, err := t.env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)
	info.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	_, hc, err := t.env.StartInstance("1", "fake_nonce", series, constraints.MustParse("mem=1024"), info, apiInfo)
	c.Assert(err, IsNil)
	c.Check(*hc.Arch, Equals, "amd64")
	c.Check(*hc.Mem, Equals, uint64(1740))
	c.Check(*hc.CpuCores, Equals, uint64(1))
	c.Assert(*hc.CpuPower, Equals, uint64(100))
}

func (t *localServerSuite) TestValidateImageMetadata(c *C) {
	region, image_ids, err := ec2.ValidateImageMetadata(t.env, "precise", "test")
	c.Assert(err, IsNil)
	c.Assert(region, Equals, "test")
	c.Assert(image_ids, DeepEquals, []string{"ami-00000033", "ami-00000035", "ami-00000034"})
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

func (s *localServerSuite) TestGetImageURLs(c *C) {
	urls, err := ec2.GetImageURLs(s.env)
	c.Assert(err, IsNil)
	c.Assert(len(urls), Equals, 1)
	c.Assert(urls[0], Equals, imagemetadata.DefaultBaseURL)
}

// localNonUSEastSuite is similar to localServerSuite but the S3 mock server
// behaves as if
type localNonUSEastSuite struct {
	testing.LoggingSuite
	tests           jujutest.Tests
	srv             localServer
	env             environs.Environ
	restoreTimeouts func()
}

func (t *localNonUSEastSuite) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	ec2.UseTestImageData(ec2.TestImagesData)
	ec2.UseTestInstanceTypeData(ec2.TestInstanceTypeCosts)
	ec2.UseTestRegionData(ec2.TestRegions)
	t.tests.SetUpSuite(c)
	t.restoreTimeouts = envtesting.PatchAttemptStrategies(ec2.ShortAttempt)
}

func (t *localNonUSEastSuite) TearDownSuite(c *C) {
	t.restoreTimeouts()
	ec2.UseTestImageData(nil)
	ec2.UseTestInstanceTypeData(nil)
	ec2.UseTestRegionData(nil)
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
