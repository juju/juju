// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/openstackservice"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/openstack"
	"launchpad.net/juju-core/environs/simplestreams"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/http"
	"net/http/httptest"
	"strings"
)

type ProviderSuite struct {
	restoreTimeouts func()
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpTest(c *C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(openstack.ShortAttempt, openstack.StorageAttempt)
}

func (s *ProviderSuite) TearDownTest(c *C) {
	s.restoreTimeouts()
}

func (s *ProviderSuite) TestMetadata(c *C) {
	openstack.UseTestMetadata(openstack.MetadataTesting)
	defer openstack.UseTestMetadata(nil)

	p, err := environs.Provider("openstack")
	c.Assert(err, IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "203.1.1.2")

	addr, err = p.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "10.1.1.2")
}

func (s *ProviderSuite) TestPublicFallbackToPrivate(c *C) {
	openstack.UseTestMetadata(map[string]string{
		"/latest/meta-data/public-ipv4": "203.1.1.2",
		"/latest/meta-data/local-ipv4":  "10.1.1.2",
	})
	defer openstack.UseTestMetadata(nil)
	p, err := environs.Provider("openstack")
	c.Assert(err, IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "203.1.1.2")

	openstack.UseTestMetadata(map[string]string{
		"/latest/meta-data/local-ipv4":  "10.1.1.2",
		"/latest/meta-data/public-ipv4": "",
	})
	addr, err = p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "10.1.1.2")
}

// Register tests to run against a test Openstack instance (service doubles).
func registerLocalTests() {
	cred := &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some-region",
		TenantName: "some tenant",
	}
	config := makeTestConfig(cred)
	config["agent-version"] = version.CurrentNumber().String()
	config["authorized-keys"] = "fakekey"
	Suite(&localLiveSuite{
		LiveTests: LiveTests{
			cred: cred,
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
	Suite(&publicBucketSuite{
		cred: cred,
	})
}

// localServer is used to spin up a local Openstack service double.
type localServer struct {
	Server          *httptest.Server
	Mux             *http.ServeMux
	oldHandler      http.Handler
	Service         *openstackservice.Openstack
	restoreTimeouts func()
}

func (s *localServer) start(c *C, cred *identity.Credentials) {
	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux
	cred.URL = s.Server.URL
	c.Logf("Started service at: %v", s.Server.URL)
	s.Service = openstackservice.New(cred, identity.AuthUserPass)
	s.Service.SetupHTTP(s.Mux)
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(openstack.ShortAttempt, openstack.StorageAttempt)
}

func (s *localServer) stop() {
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
	s.restoreTimeouts()
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
	openstack.UseTestImageData(s.Env, s.cred)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	openstack.RemoveTestImageData(s.Env)
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
	cred                   *identity.Credentials
	srv                    localServer
	env                    environs.Environ
	writeablePublicStorage environs.Storage
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
	s.writeablePublicStorage = openstack.WritablePublicStorage(s.Env)
	envtesting.UploadFakeTools(c, s.writeablePublicStorage)
	s.env = s.Tests.Env
	openstack.UseTestImageData(s.env, s.cred)
}

func (s *localServerSuite) TearDownTest(c *C) {
	if s.env != nil {
		openstack.RemoveTestImageData(s.env)
	}
	if s.writeablePublicStorage != nil {
		envtesting.RemoveFakeTools(c, s.writeablePublicStorage)
	}
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
	inst, _ := testing.StartInstance(c, env, "100")
	err = s.Env.StopInstances([]instance.Instance{inst})
	c.Assert(err, IsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *C) {
	err := environs.Bootstrap(s.Env, constraints.Value{})
	c.Assert(err, IsNil)
	_, hc := testing.StartInstanceWithConstraints(c, s.Env, "100", constraints.MustParse("mem=1024"))
	c.Check(*hc.Arch, Equals, "amd64")
	c.Check(*hc.Mem, Equals, uint64(2048))
	c.Check(*hc.CpuCores, Equals, uint64(1))
	c.Assert(hc.CpuPower, IsNil)
}

var instanceGathering = []struct {
	ids []instance.Id
	err error
}{
	{ids: []instance.Id{"id0"}},
	{ids: []instance.Id{"id0", "id0"}},
	{ids: []instance.Id{"id0", "id1"}},
	{ids: []instance.Id{"id1", "id0"}},
	{ids: []instance.Id{"id1", "id0", "id1"}},
	{
		ids: []instance.Id{""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []instance.Id{"", ""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []instance.Id{"", "", ""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []instance.Id{"id0", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []instance.Id{"", "id1"},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []instance.Id{"id0", "id1", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []instance.Id{"id0", "", "id0"},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []instance.Id{"id0", "id0", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []instance.Id{"", "id0", "id1"},
		err: environs.ErrPartialInstances,
	},
}

func (s *localServerSuite) TestInstanceStatus(c *C) {
	// goose's test service always returns ACTIVE state.
	inst, _ := testing.StartInstance(c, s.Env, "100")
	c.Assert(inst.Status(), Equals, nova.StatusActive)
	err := s.Env.StopInstances([]instance.Instance{inst})
	c.Assert(err, IsNil)
}

func (s *localServerSuite) TestInstancesGathering(c *C) {
	inst0, _ := testing.StartInstance(c, s.Env, "100")
	id0 := inst0.Id()
	inst1, _ := testing.StartInstance(c, s.Env, "101")
	id1 := inst1.Id()
	defer func() {
		err := s.Env.StopInstances([]instance.Instance{inst0, inst1})
		c.Assert(err, IsNil)
	}()

	for i, test := range instanceGathering {
		c.Logf("test %d: find %v -> expect len %d, err: %v", i, test.ids, len(test.ids), test.err)
		ids := make([]instance.Id, len(test.ids))
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
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *C) {
	err := environs.Bootstrap(s.env, constraints.Value{})
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := environs.LoadState(s.env.Storage())
	c.Assert(err, IsNil)
	c.Assert(stateData.StateInstances, HasLen, 1)

	expectedHardware := instance.MustParseHardware("arch=amd64 cpu-cores=1 mem=512M")
	insts, err := s.env.AllInstances()
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, stateData.StateInstances[0])
	c.Check(expectedHardware, DeepEquals, stateData.Characteristics[0])

	info, apiInfo, err := s.env.StateInfo()
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
	series := s.env.Config().DefaultSeries()
	info.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	inst1, _, err := s.env.StartInstance("1", "fake_nonce", series, constraints.Value{}, info, apiInfo)
	c.Assert(err, IsNil)

	err = s.env.Destroy(append(insts, inst1))
	c.Assert(err, IsNil)

	_, err = environs.LoadState(s.env.Storage())
	c.Assert(err, NotNil)
}

func (s *localServerSuite) TestGetMetadataURLs(c *C) {
	urls, err := imagemetadata.GetMetadataURLs(s.env)
	c.Assert(err, IsNil)
	c.Assert(len(urls), Equals, 3)
	// The public bucket URL ends with "/juju-dist/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist/"), Equals, true)
	// The product-streams URL ends with "/imagemetadata".
	c.Check(strings.HasSuffix(urls[1], "/imagemetadata"), Equals, true)
	c.Assert(urls[2], Equals, imagemetadata.DefaultBaseURL)
}

func (s *localServerSuite) TestFindImageSpecPublicStorage(c *C) {
	spec, err := openstack.FindInstanceSpec(s.Env, "raring", "amd64", "mem=512M")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "id-y")
	c.Assert(spec.InstanceType.Name, Equals, "m1.tiny")
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *C) {
	// An error occurs if no suitable image is found.
	_, err := openstack.FindInstanceSpec(s.Env, "saucy", "amd64", "mem=8G")
	c.Assert(err, ErrorMatches, `no "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestValidateImageMetadata(c *C) {
	params, err := s.Env.(simplestreams.MetadataValidator).MetadataLookupParams("some-region")
	c.Assert(err, IsNil)
	params.Series = "raring"
	image_ids, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, IsNil)
	c.Assert(image_ids, DeepEquals, []string{"id-y"})
}

func (s *localServerSuite) TestRemoveAll(c *C) {
	storage := s.Env.Storage()
	for _, a := range []byte("abcdefghijklmnopqrstuvwxyz") {
		content := []byte{a}
		name := string(content)
		err := storage.Put(name, bytes.NewBuffer(content),
			int64(len(content)))
		c.Assert(err, IsNil)
	}
	reader, err := storage.Get("a")
	c.Assert(err, IsNil)
	allContent, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Assert(string(allContent), Equals, "a")
	err = storage.RemoveAll()
	c.Assert(err, IsNil)
	_, err = storage.Get("a")
	c.Assert(err, NotNil)
}

func (s *localServerSuite) TestDeleteMoreThan100(c *C) {
	storage := s.Env.Storage()
	// 6*26 = 156 items
	for _, a := range []byte("abcdef") {
		for _, b := range []byte("abcdefghijklmnopqrstuvwxyz") {
			content := []byte{a, b}
			name := string(content)
			err := storage.Put(name, bytes.NewBuffer(content),
				int64(len(content)))
			c.Assert(err, IsNil)
		}
	}
	reader, err := storage.Get("ab")
	c.Assert(err, IsNil)
	allContent, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Assert(string(allContent), Equals, "ab")
	err = storage.RemoveAll()
	c.Assert(err, IsNil)
	_, err = storage.Get("ab")
	c.Assert(err, NotNil)
}

// TestEnsureGroup checks that when creating a duplicate security group, the existing group is
// returned and the existing rules have been left as is.
func (s *localServerSuite) TestEnsureGroup(c *C) {
	rule := []nova.RuleInfo{
		{
			IPProtocol: "tcp",
			FromPort:   22,
			ToPort:     22,
		},
	}

	assertRule := func(group nova.SecurityGroup) {
		c.Check(len(group.Rules), Equals, 1)
		c.Check(*group.Rules[0].IPProtocol, Equals, "tcp")
		c.Check(*group.Rules[0].FromPort, Equals, 22)
		c.Check(*group.Rules[0].ToPort, Equals, 22)
	}

	group, err := openstack.EnsureGroup(s.env, "test group", rule)
	c.Assert(err, IsNil)
	c.Assert(group.Name, Equals, "test group")
	assertRule(group)
	id := group.Id
	// Do it again and check that the existing group is returned.
	anotherRule := []nova.RuleInfo{
		{
			IPProtocol: "tcp",
			FromPort:   1,
			ToPort:     65535,
		},
	}
	group, err = openstack.EnsureGroup(s.env, "test group", anotherRule)
	c.Assert(err, IsNil)
	c.Check(group.Id, Equals, id)
	c.Assert(group.Name, Equals, "test group")
	assertRule(group)
}

// publicBucketSuite contains tests to ensure the public bucket is correctly set up.
type publicBucketSuite struct {
	cred *identity.Credentials
	srv  localServer
	env  environs.Environ
}

func (s *publicBucketSuite) SetUpTest(c *C) {
	s.srv.start(c, s.cred)
}

func (s *publicBucketSuite) TearDownTest(c *C) {
	err := s.env.Destroy(nil)
	c.Check(err, IsNil)
	s.srv.stop()
}

func (s *publicBucketSuite) TestPublicBucketFromEnv(c *C) {
	config := makeTestConfig(s.cred)
	config["public-bucket-url"] = "http://127.0.0.1/public-bucket"
	var err error
	s.env, err = environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	url, err := s.env.PublicStorage().URL("")
	c.Assert(err, IsNil)
	c.Assert(url, Equals, "http://127.0.0.1/public-bucket/juju-dist/")
}

func (s *publicBucketSuite) TestPublicBucketFromKeystone(c *C) {
	config := makeTestConfig(s.cred)
	config["public-bucket-url"] = ""
	var err error
	s.env, err = environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	url, err := s.env.PublicStorage().URL("")
	c.Assert(err, IsNil)
	swiftURL, err := openstack.GetSwiftURL(s.env)
	c.Assert(err, IsNil)
	c.Assert(url, Equals, fmt.Sprintf("%s/juju-dist/", swiftURL))
}
