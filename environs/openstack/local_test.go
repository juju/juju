package openstack_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/openstackservice"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/openstack"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/http"
	"net/http/httptest"
)

type ProviderSuite struct{}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpTest(c *C) {
	openstack.ShortTimeouts(true)
}

func (s *ProviderSuite) TearDownTest(c *C) {
	openstack.ShortTimeouts(false)
}

func (s *ProviderSuite) TestMetadata(c *C) {
	openstack.UseTestMetadata(openstack.MetadataTestingBase)
	defer openstack.UseTestMetadata(nil)

	p, err := environs.Provider("openstack")
	c.Assert(err, IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "203.1.1.2")

	addr, err = p.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "10.1.1.2")

	id, err := p.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, state.InstanceId("d8e02d56-2648-49a3-bf97-6be8f1204f38"))
}

func (s *ProviderSuite) TestPublicFallbackToPrivate(c *C) {
	openstack.UseTestMetadata([]jujutest.FileContent{
		{"/latest/meta-data/public-ipv4", "203.1.1.2"},
		{"/latest/meta-data/local-ipv4", "10.1.1.2"},
	})
	defer openstack.UseTestMetadata(nil)
	p, err := environs.Provider("openstack")
	c.Assert(err, IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "203.1.1.2")

	openstack.UseTestMetadata([]jujutest.FileContent{
		{"/latest/meta-data/local-ipv4", "10.1.1.2"},
		{"/latest/meta-data/public-ipv4", ""},
	})
	addr, err = p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "10.1.1.2")
}

func (s *ProviderSuite) TestLegacyInstanceId(c *C) {
	openstack.UseTestMetadata(openstack.MetadataHP)
	defer openstack.UseTestMetadata(nil)

	p, err := environs.Provider("openstack")
	c.Assert(err, IsNil)

	id, err := p.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, state.InstanceId("2748"))
}

// Register tests to run against a test Openstack instance (service doubles).
func registerLocalTests() {
	cred := &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some region",
		TenantName: "some tenant",
	}
	config := makeTestConfig(cred)
	config["agent-version"] = version.CurrentNumber().String()
	config["authorized-keys"] = "fakekey"
	config["default-image-id"] = "1"
	config["default-instance-type"] = "m1.small"
	Suite(&localLiveSuite{
		LiveTests: LiveTests{
			cred:        cred,
			testImageId: "1",
			testFlavor:  "m1.small",
			LiveTests: jujutest.LiveTests{
				TestConfig: jujutest.TestConfig{config},
			},
		},
	})
	Suite(&localServerSuite{
		cred: cred,
		Tests: jujutest.Tests{
			TestConfig: jujutest.TestConfig{config},
		},
	})
}

// localServer is used to spin up a local Openstack service double.
type localServer struct {
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
	Service    *openstackservice.Openstack
}

func (s *localServer) start(c *C, cred *identity.Credentials) {
	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux
	cred.URL = s.Server.URL
	c.Logf("Started service at: %v", s.Server.URL)
	s.Service = openstackservice.New(cred)
	s.Service.SetupHTTP(s.Mux)
	openstack.ShortTimeouts(true)
}

func (s *localServer) stop() {
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
	openstack.ShortTimeouts(false)
}

// localLiveSuite runs tests from LiveTests using an Openstack service double.
type localLiveSuite struct {
	coretesting.LoggingSuite
	LiveTests
	srv localServer
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	c.Logf("Running live tests using openstack service test double")
	s.srv.start(c, s.cred)
	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.srv.stop()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

// localServerSuite contains tests that run against an Openstack service double.
// These tests can test things that would be unreasonably slow or expensive
// to test on a live Openstack server. The service double is started and stopped for
// each test.
type localServerSuite struct {
	coretesting.LoggingSuite
	jujutest.Tests
	cred *identity.Credentials
	srv  localServer
	env  environs.Environ
}

func (s *localServerSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.Tests.SetUpSuite(c)
	c.Logf("Running local tests")
}

func (s *localServerSuite) TearDownSuite(c *C) {
	s.Tests.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *localServerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.srv.start(c, s.cred)
	s.TestConfig.UpdateConfig(map[string]interface{}{
		"auth-url": s.cred.URL,
	})
	s.Tests.SetUpTest(c)
	writeablePublicStorage := openstack.WritablePublicStorage(s.Env)
	envtesting.UploadFakeTools(c, writeablePublicStorage)
	s.env = s.Tests.Env
}

func (s *localServerSuite) TearDownTest(c *C) {
	s.Tests.TearDownTest(c)
	s.srv.stop()
	s.LoggingSuite.TearDownTest(c)
}

// If the bootstrap node is configured to require a public IP address,
// bootstrapping fails if an address cannot be allocated.
func (s *localServerSuite) TestBootstrapFailsWhenPublicIPError(c *C) {
	cleanup := s.srv.Service.Nova.RegisterControlPoint(
		"addFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("failed on purpose")
		},
	)
	defer cleanup()

	// Create a config that matches s.Config but with use-floating-ip set to true
	cfg, err := s.Env.Config().Apply(map[string]interface{}{
		"use-floating-ip": true,
	})
	c.Assert(err, IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, ErrorMatches, "(.|\n)*cannot allocate a public IP as needed(.|\n)*")
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstanceWithoutPublicIP(c *C) {
	cleanup := s.srv.Service.Nova.RegisterControlPoint(
		"addFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("add floating IP should not have been called")
		},
	)
	defer cleanup()
	cleanup = s.srv.Service.Nova.RegisterControlPoint(
		"addServerFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("add server floating IP should not have been called")
		},
	)
	defer cleanup()

	cfg, err := s.Env.Config().Apply(map[string]interface{}{
		"use-floating-ip": false,
	})
	c.Assert(err, IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)
	inst := testing.StartInstance(c, env, "100")
	err = s.Env.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)
}

var instanceGathering = []struct {
	ids []state.InstanceId
	err error
}{
	{ids: []state.InstanceId{"id0"}},
	{ids: []state.InstanceId{"id0", "id0"}},
	{ids: []state.InstanceId{"id0", "id1"}},
	{ids: []state.InstanceId{"id1", "id0"}},
	{ids: []state.InstanceId{"id1", "id0", "id1"}},
	{
		ids: []state.InstanceId{""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []state.InstanceId{"", ""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []state.InstanceId{"", "", ""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []state.InstanceId{"id0", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"", "id1"},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"id0", "id1", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"id0", "", "id0"},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"id0", "id0", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"", "id0", "id1"},
		err: environs.ErrPartialInstances,
	},
}

func (s *localServerSuite) TestInstancesGathering(c *C) {
	inst0 := testing.StartInstance(c, s.Env, "100")
	id0 := inst0.Id()
	inst1 := testing.StartInstance(c, s.Env, "101")
	id1 := inst1.Id()
	defer func() {
		err := s.Env.StopInstances([]environs.Instance{inst0, inst1})
		c.Assert(err, IsNil)
	}()

	for i, test := range instanceGathering {
		c.Logf("test %d: find %v -> expect len %d, err: %v", i, test.ids, len(test.ids), test.err)
		ids := make([]state.InstanceId, len(test.ids))
		for j, id := range test.ids {
			switch id {
			case "id0":
				ids[j] = id0
			case "id1":
				ids[j] = id1
			}
		}
		insts, err := s.Env.Instances(ids)
		c.Assert(err, Equals, test.err)
		if err == environs.ErrNoInstances {
			c.Assert(insts, HasLen, 0)
		} else {
			c.Assert(insts, HasLen, len(test.ids))
		}
		for j, inst := range insts {
			if ids[j] != "" {
				c.Assert(inst.Id(), Equals, ids[j])
			} else {
				c.Assert(inst, IsNil)
			}
		}
	}
}

// TODO (wallyworld) - this test was copied from the ec2 provider.
// It should be moved to environs.jujutests.Tests.
func (t *localServerSuite) TestBootstrapInstanceUserDataAndState(c *C) {
	policy := t.env.AssignmentPolicy()
	c.Assert(policy, Equals, state.AssignNew)

	err := environs.Bootstrap(t.env, constraints.Value{})
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := openstack.LoadState(t.env)
	c.Assert(err, IsNil)
	c.Assert(stateData.StateInstances, HasLen, 1)

	insts, err := t.env.Instances(stateData.StateInstances)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, stateData.StateInstances[0])

	info, apiInfo, err := t.env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)

	bootstrapDNS, err := insts[0].DNSName()
	c.Assert(err, IsNil)
	c.Assert(bootstrapDNS, Not(Equals), "")

	// TODO(wallyworld) - 2013-03-01 bug=1137005
	// The nova test double needs to be updated to support retrieving instance userData.
	// Until then, we can't check the cloud init script was generated correctly.

	// check that a new instance will be started with a machine agent,
	// and without a provisioning agent.
	series := t.env.Config().DefaultSeries()
	info.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	inst1, err := t.env.StartInstance("1", "fake_nonce", series, constraints.Value{}, info, apiInfo)
	c.Assert(err, IsNil)

	err = t.env.Destroy(append(insts, inst1))
	c.Assert(err, IsNil)

	_, err = openstack.LoadState(t.env)
	c.Assert(err, NotNil)
}

func (s *localServerSuite) TestFindInstanceSpec(c *C) {
	list, err := environs.FindAvailableTools(s.Env, version.Current.Major)
	c.Assert(err, IsNil)
	imageId, flavorId, tools, err := openstack.FindInstanceSpec(s.Env, list)
	c.Assert(err, IsNil)
	c.Check(imageId, Equals, "1")
	c.Check(flavorId, Equals, "2")
	c.Check(list.URLs()[tools.Binary], Equals, tools.URL)

	// Extra tests for series/arch forcing while default-image-id is only option.
	c.Check(tools.Arch, Equals, "amd64")
	c.Check(tools.Series, Equals, "precise")
}

func (s *localServerSuite) TestFindInstanceSpecNoTools(c *C) {
	_, _, _, err := openstack.FindInstanceSpec(s.Env, nil)
	c.Check(err, ErrorMatches, "no matching tools available")
	c.Check(err, FitsTypeOf, (*environs.NotFoundError)(nil))
}

func (s *localServerSuite) TestFindInstanceSpecBadFlavor(c *C) {
	cfg, err := s.Env.Config().Apply(map[string]interface{}{
		"default-instance-type": "blobby",
	})
	c.Assert(err, IsNil)
	err = s.Env.SetConfig(cfg)
	c.Assert(err, IsNil)
	_, _, _, err = openstack.FindInstanceSpec(s.Env, nil)
	c.Check(err, ErrorMatches, `flavor "blobby" not found`)
	c.Check(err, FitsTypeOf, (*environs.NotFoundError)(nil))
}

func (s *localServerSuite) TestFindInstanceSpecNoFlavor(c *C) {
	cfg, err := s.Env.Config().Apply(map[string]interface{}{
		"default-instance-type": "",
	})
	c.Assert(err, IsNil)
	err = s.Env.SetConfig(cfg)
	c.Assert(err, IsNil)
	_, _, _, err = openstack.FindInstanceSpec(s.Env, nil)
	c.Check(err, ErrorMatches, "no default-instance-type set")
}

func (s *localServerSuite) TestFindInstanceSpecNoImageId(c *C) {
	cfg, err := s.Env.Config().Apply(map[string]interface{}{
		"default-image-id": "",
	})
	c.Assert(err, IsNil)
	err = s.Env.SetConfig(cfg)
	c.Assert(err, IsNil)
	_, _, _, err = openstack.FindInstanceSpec(s.Env, nil)
	c.Check(err, ErrorMatches, "no default-image-id set")
}
