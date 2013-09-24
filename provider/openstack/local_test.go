// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/openstackservice"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/provider/openstack"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type ProviderSuite struct {
	restoreTimeouts func()
}

var _ = gc.Suite(&ProviderSuite{})
var _ = gc.Suite(&localHTTPSServerSuite{})

func (s *ProviderSuite) SetUpTest(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(openstack.ShortAttempt, openstack.StorageAttempt)
}

func (s *ProviderSuite) TearDownTest(c *gc.C) {
	s.restoreTimeouts()
}

func (s *ProviderSuite) TestMetadata(c *gc.C) {
	openstack.UseTestMetadata(openstack.MetadataTesting)
	defer openstack.UseTestMetadata(nil)

	p, err := environs.Provider("openstack")
	c.Assert(err, gc.IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "203.1.1.2")

	addr, err = p.PrivateAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "10.1.1.2")
}

func (s *ProviderSuite) TestPublicFallbackToPrivate(c *gc.C) {
	openstack.UseTestMetadata(map[string]string{
		"/latest/meta-data/public-ipv4": "203.1.1.2",
		"/latest/meta-data/local-ipv4":  "10.1.1.2",
	})
	defer openstack.UseTestMetadata(nil)
	p, err := environs.Provider("openstack")
	c.Assert(err, gc.IsNil)

	addr, err := p.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "203.1.1.2")

	openstack.UseTestMetadata(map[string]string{
		"/latest/meta-data/local-ipv4":  "10.1.1.2",
		"/latest/meta-data/public-ipv4": "",
	})
	addr, err = p.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "10.1.1.2")
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
	config["agent-version"] = version.Current.Number.String()
	config["authorized-keys"] = "fakekey"
	gc.Suite(&localLiveSuite{
		LiveTests: LiveTests{
			cred: cred,
			LiveTests: jujutest.LiveTests{
				TestConfig: config,
			},
		},
	})
	gc.Suite(&localServerSuite{
		cred: cred,
		Tests: jujutest.Tests{
			TestConfig: config,
		},
	})
}

// localServer is used to spin up a local Openstack service double.
type localServer struct {
	Server          *httptest.Server
	Mux             *http.ServeMux
	oldHandler      http.Handler
	Service         *openstackservice.Openstack
	restoreTimeouts func()
	UseTLS          bool
}

func (s *localServer) start(c *gc.C, cred *identity.Credentials) {
	// Set up the HTTP server.
	if s.UseTLS {
		s.Server = httptest.NewTLSServer(nil)
	} else {
		s.Server = httptest.NewServer(nil)
	}
	c.Assert(s.Server, gc.NotNil)
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
	testbase.LoggingSuite
	LiveTests
	srv localServer
}

func (s *localLiveSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	c.Logf("Running live tests using openstack service test double")
	s.srv.start(c, s.cred)
	s.LiveTests.SetUpSuite(c)
	openstack.UseTestImageData(s.Env, s.cred)
}

func (s *localLiveSuite) TearDownSuite(c *gc.C) {
	openstack.RemoveTestImageData(s.Env)
	s.LiveTests.TearDownSuite(c)
	s.srv.stop()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *gc.C) {
	s.LiveTests.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

// localServerSuite contains tests that run against an Openstack service double.
// These tests can test things that would be unreasonably slow or expensive
// to test on a live Openstack server. The service double is started and stopped for
// each test.
type localServerSuite struct {
	testbase.LoggingSuite
	jujutest.Tests
	cred            *identity.Credentials
	srv             localServer
	env             environs.Environ
	metadataStorage storage.Storage
}

func (s *localServerSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.Tests.SetUpSuite(c)
	c.Logf("Running local tests")
}

func (s *localServerSuite) TearDownSuite(c *gc.C) {
	s.Tests.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *localServerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.srv.start(c, s.cred)
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, gc.IsNil)
	containerURL, err := cl.MakeServiceURL("object-store", nil)
	c.Assert(err, gc.IsNil)
	s.TestConfig = s.TestConfig.Merge(coretesting.Attrs{
		"tools-url":          containerURL + "/juju-dist-test/tools",
		"image-metadata-url": containerURL + "/juju-dist-test",
		"auth-url":           s.cred.URL,
	})
	s.Tests.SetUpTest(c)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	s.metadataStorage = openstack.MetadataStorage(s.Env)
	// Put some fake metadata in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running can find the metadata.
	envtesting.GenerateFakeToolsMetadata(c, s.metadataStorage)
	s.env = s.Tests.Env
	openstack.UseTestImageData(s.env, s.cred)
}

func (s *localServerSuite) TearDownTest(c *gc.C) {
	if s.env != nil {
		openstack.RemoveTestImageData(s.env)
	}
	if s.metadataStorage != nil {
		envtesting.RemoveFakeToolsMetadata(c, s.metadataStorage)
	}
	s.Tests.TearDownTest(c)
	s.srv.stop()
	s.LoggingSuite.TearDownTest(c)
}

// If the bootstrap node is configured to require a public IP address,
// bootstrapping fails if an address cannot be allocated.
func (s *localServerSuite) TestBootstrapFailsWhenPublicIPError(c *gc.C) {
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
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "(.|\n)*cannot allocate a public IP as needed(.|\n)*")
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstanceWithoutPublicIP(c *gc.C) {
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
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	inst, _ := testing.StartInstance(c, env, "100")
	err = s.Env.StopInstances([]instance.Instance{inst})
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	err := bootstrap.Bootstrap(s.Env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	_, hc := testing.StartInstanceWithConstraints(c, s.Env, "100", constraints.MustParse("mem=1024"))
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(2048))
	c.Check(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(hc.CpuPower, gc.IsNil)
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

func (s *localServerSuite) TestInstanceStatus(c *gc.C) {
	// goose's test service always returns ACTIVE state.
	inst, _ := testing.StartInstance(c, s.Env, "100")
	c.Assert(inst.Status(), gc.Equals, nova.StatusActive)
	err := s.Env.StopInstances([]instance.Instance{inst})
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestInstancesGathering(c *gc.C) {
	inst0, _ := testing.StartInstance(c, s.Env, "100")
	id0 := inst0.Id()
	inst1, _ := testing.StartInstance(c, s.Env, "101")
	id1 := inst1.Id()
	defer func() {
		err := s.Env.StopInstances([]instance.Instance{inst0, inst1})
		c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.Equals, test.err)
		if err == environs.ErrNoInstances {
			c.Assert(insts, gc.HasLen, 0)
		} else {
			c.Assert(insts, gc.HasLen, len(test.ids))
		}
		for j, inst := range insts {
			if ids[j] != "" {
				c.Assert(inst.Id(), gc.Equals, ids[j])
			} else {
				c.Assert(inst, gc.IsNil)
			}
		}
	}
}

func (s *localServerSuite) TestCollectInstances(c *gc.C) {
	cleanup := s.srv.Service.Nova.RegisterControlPoint(
		"addServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			details := args[0].(*nova.ServerDetail)
			details.Status = "BUILD(networking)"
			return nil
		},
	)
	defer cleanup()
	stateInst, _ := testing.StartInstance(c, s.Env, "100")
	defer func() {
		err := s.Env.StopInstances([]instance.Instance{stateInst})
		c.Assert(err, gc.IsNil)
	}()
	found := make(map[instance.Id]instance.Instance)
	missing := []instance.Id{stateInst.Id()}

	resultMissing := openstack.CollectInstances(s.Env, missing, found)

	c.Assert(resultMissing, gc.DeepEquals, missing)
}

func (s *localServerSuite) TestInstancesBuildSpawning(c *gc.C) {
	// HP servers are available once they are BUILD(spawning).
	cleanup := s.srv.Service.Nova.RegisterControlPoint(
		"addServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			details := args[0].(*nova.ServerDetail)
			details.Status = nova.StatusBuildSpawning
			return nil
		},
	)
	defer cleanup()
	stateInst, _ := testing.StartInstance(c, s.Env, "100")
	defer func() {
		err := s.Env.StopInstances([]instance.Instance{stateInst})
		c.Assert(err, gc.IsNil)
	}()

	instances, err := s.Env.Instances([]instance.Id{stateInst.Id()})

	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Status(), gc.Equals, nova.StatusBuildSpawning)
}

// TODO (wallyworld) - this test was copied from the ec2 provider.
// It should be moved to environs.jujutests.Tests.
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *gc.C) {
	err := bootstrap.Bootstrap(s.env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := provider.LoadState(s.env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(stateData.StateInstances, gc.HasLen, 1)

	expectedHardware := instance.MustParseHardware("arch=amd64 cpu-cores=1 mem=512M")
	insts, err := s.env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, stateData.StateInstances[0])
	c.Check(expectedHardware, gc.DeepEquals, stateData.Characteristics[0])

	info, apiInfo, err := s.env.StateInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.NotNil)

	bootstrapDNS, err := insts[0].DNSName()
	c.Assert(err, gc.IsNil)
	c.Assert(bootstrapDNS, gc.Not(gc.Equals), "")

	// TODO(wallyworld) - 2013-03-01 bug=1137005
	// The nova test double needs to be updated to support retrieving instance userData.
	// Until then, we can't check the cloud init script was generated correctly.

	// check that a new instance will be started with a machine agent,
	// and without a provisioning agent.
	series := s.env.Config().DefaultSeries()
	info.Tag = "machine-1"
	info.Password = "password"
	apiInfo.Tag = "machine-1"
	inst1, _, err := provider.StartInstance(s.env, "1", "fake_nonce", series, constraints.Value{}, info, apiInfo)
	c.Assert(err, gc.IsNil)

	err = s.env.Destroy(append(insts, inst1))
	c.Assert(err, gc.IsNil)

	_, err = provider.LoadState(s.env.Storage())
	c.Assert(err, gc.NotNil)
}

func (s *localServerSuite) TestGetImageMetadataSources(c *gc.C) {
	sources, err := imagemetadata.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	c.Assert(sources, gc.HasLen, 4)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		urls[i] = url
	}
	// The control bucket URL contains the bucket name.
	c.Check(strings.Contains(urls[0], openstack.ControlBucketName(s.env)), jc.IsTrue)
	// The product-streams URL ends with "/imagemetadata".
	c.Check(strings.HasSuffix(urls[1], "/imagemetadata/"), jc.IsTrue)
	// The image-metadata-url ends with "/juju-dist-test/".
	c.Check(strings.HasSuffix(urls[2], "/juju-dist-test/"), jc.IsTrue)
	c.Assert(urls[3], gc.Equals, imagemetadata.DefaultBaseURL+"/")
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	sources, err := tools.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	c.Assert(sources, gc.HasLen, 3)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		urls[i] = url
	}
	// The control bucket URL contains the bucket name.
	c.Check(strings.Contains(urls[0], openstack.ControlBucketName(s.env)+"/tools"), jc.IsTrue)
	c.Assert(err, gc.IsNil)
	// Check that the URL from keystone parses.
	_, err = url.Parse(urls[1])
	c.Assert(err, gc.IsNil)
	// The tools-url ends with "/juju-dist-test/tools/".
	c.Check(strings.HasSuffix(urls[2], "/juju-dist-test/tools/"), jc.IsTrue)
}

func (s *localServerSuite) TestFindImageSpecPublicStorage(c *gc.C) {
	spec, err := openstack.FindInstanceSpec(s.Env, "raring", "amd64", "mem=512M")
	c.Assert(err, gc.IsNil)
	c.Assert(spec.Image.Id, gc.Equals, "id-y")
	c.Assert(spec.InstanceType.Name, gc.Equals, "m1.tiny")
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *gc.C) {
	// An error occurs if no suitable image is found.
	_, err := openstack.FindInstanceSpec(s.Env, "saucy", "amd64", "mem=8G")
	c.Assert(err, gc.ErrorMatches, `no "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	params, err := s.Env.(simplestreams.MetadataValidator).MetadataLookupParams("some-region")
	c.Assert(err, gc.IsNil)
	params.Sources, err = imagemetadata.GetMetadataSources(s.Env)
	c.Assert(err, gc.IsNil)
	params.Series = "raring"
	image_ids, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(image_ids, gc.DeepEquals, []string{"id-y"})
}

func (s *localServerSuite) TestRemoveAll(c *gc.C) {
	stor := s.Env.Storage()
	for _, a := range []byte("abcdefghijklmnopqrstuvwxyz") {
		content := []byte{a}
		name := string(content)
		err := stor.Put(name, bytes.NewBuffer(content),
			int64(len(content)))
		c.Assert(err, gc.IsNil)
	}
	reader, err := storage.Get(stor, "a")
	c.Assert(err, gc.IsNil)
	allContent, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(allContent), gc.Equals, "a")
	err = stor.RemoveAll()
	c.Assert(err, gc.IsNil)
	_, err = storage.Get(stor, "a")
	c.Assert(err, gc.NotNil)
}

func (s *localServerSuite) TestDeleteMoreThan100(c *gc.C) {
	stor := s.Env.Storage()
	// 6*26 = 156 items
	for _, a := range []byte("abcdef") {
		for _, b := range []byte("abcdefghijklmnopqrstuvwxyz") {
			content := []byte{a, b}
			name := string(content)
			err := stor.Put(name, bytes.NewBuffer(content),
				int64(len(content)))
			c.Assert(err, gc.IsNil)
		}
	}
	reader, err := storage.Get(stor, "ab")
	c.Assert(err, gc.IsNil)
	allContent, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(allContent), gc.Equals, "ab")
	err = stor.RemoveAll()
	c.Assert(err, gc.IsNil)
	_, err = storage.Get(stor, "ab")
	c.Assert(err, gc.NotNil)
}

// TestEnsureGroup checks that when creating a duplicate security group, the existing group is
// returned and the existing rules have been left as is.
func (s *localServerSuite) TestEnsureGroup(c *gc.C) {
	rule := []nova.RuleInfo{
		{
			IPProtocol: "tcp",
			FromPort:   22,
			ToPort:     22,
		},
	}

	assertRule := func(group nova.SecurityGroup) {
		c.Check(len(group.Rules), gc.Equals, 1)
		c.Check(*group.Rules[0].IPProtocol, gc.Equals, "tcp")
		c.Check(*group.Rules[0].FromPort, gc.Equals, 22)
		c.Check(*group.Rules[0].ToPort, gc.Equals, 22)
	}

	group, err := openstack.EnsureGroup(s.env, "test group", rule)
	c.Assert(err, gc.IsNil)
	c.Assert(group.Name, gc.Equals, "test group")
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
	c.Assert(err, gc.IsNil)
	c.Check(group.Id, gc.Equals, id)
	c.Assert(group.Name, gc.Equals, "test group")
	assertRule(group)
}

// localHTTPSServerSuite contains tests that run against an Openstack service
// double connected on an HTTPS port with a self-signed certificate. This
// service is set up and torn down for every test.  This should only test
// things that depend on the HTTPS connection, all other functional tests on a
// local connection should be in localServerSuite
type localHTTPSServerSuite struct {
	testbase.LoggingSuite
	attrs                  map[string]interface{}
	cred                   *identity.Credentials
	srv                    localServer
	env                    environs.Environ
	writeablePublicStorage storage.Storage
}

func (s *localHTTPSServerSuite) createConfigAttrs(c *gc.C) map[string]interface{} {
	attrs := makeTestConfig(s.cred)
	attrs["agent-version"] = version.Current.Number.String()
	attrs["authorized-keys"] = "fakekey"
	// In order to set up and tear down the environment properly, we must
	// disable hostname verification
	attrs["ssl-hostname-verification"] = false
	attrs["auth-url"] = s.cred.URL
	// Now connect and set up test-local tools and image-metadata URLs
	cl := client.NewNonValidatingClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, gc.IsNil)
	containerURL, err := cl.MakeServiceURL("object-store", nil)
	c.Assert(err, gc.IsNil)
	c.Check(containerURL[:8], gc.Equals, "https://")
	attrs["tools-url"] = containerURL + "/juju-dist-test/tools"
	c.Logf("Set tools-url=%q", attrs["tools-url"])
	attrs["image-metadata-url"] = containerURL + "/juju-dist-test"
	c.Logf("Set image-metadata-url=%q", attrs["image-metadata-url"])
	return attrs
}

func (s *localHTTPSServerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.srv.UseTLS = true
	cred := &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some-region",
		TenantName: "some tenant",
	}
	// Note: start() will change cred.URL to point to s.srv.Server.URL
	s.srv.start(c, cred)
	s.cred = cred
	attrs := s.createConfigAttrs(c)
	c.Assert(attrs["auth-url"].(string)[:8], gc.Equals, "https://")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	s.env, err = environs.Prepare(cfg)
	c.Assert(err, gc.IsNil)
	s.attrs = s.env.Config().AllAttrs()
}

func (s *localHTTPSServerSuite) TearDownTest(c *gc.C) {
	if s.env != nil {
		err := s.env.Destroy(nil)
		c.Check(err, gc.IsNil)
		s.env = nil
	}
	s.srv.stop()
	s.LoggingSuite.TearDownTest(c)
}

func (s *localHTTPSServerSuite) TestCanUploadTools(c *gc.C) {
	envtesting.UploadFakeTools(c, s.env.Storage())
}

func (s *localHTTPSServerSuite) TestMustDisableSSLVerify(c *gc.C) {
	// If you don't have ssl-hostname-verification set to false, then we
	// fail to connect to the environment. Copy the attrs used by SetUp and
	// force hostname verification.
	newattrs := make(map[string]interface{}, len(s.attrs))
	for k, v := range s.attrs {
		newattrs[k] = v
	}
	newattrs["ssl-hostname-verification"] = true
	env, err := environs.NewFromAttrs(newattrs)
	c.Assert(err, gc.IsNil)
	err = env.Storage().Put("test-name", strings.NewReader("content"), 7)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
	// However, it works just fine if you use the one with the credentials set
	err = s.env.Storage().Put("test-name", strings.NewReader("content"), 7)
	c.Assert(err, gc.IsNil)
	_, err = env.Storage().Get("test-name")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
	reader, err := s.env.Storage().Get("test-name")
	c.Assert(err, gc.IsNil)
	contents, err := ioutil.ReadAll(reader)
	c.Assert(string(contents), gc.Equals, "content")
}

func (s *localHTTPSServerSuite) TestCanBootstrap(c *gc.C) {
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	metadataStorage := openstack.MetadataStorage(s.env)
	url, err := metadataStorage.URL("")
	c.Assert(err, gc.IsNil)
	c.Logf("Generating fake tools for: %v", url)
	envtesting.GenerateFakeToolsMetadata(c, metadataStorage)
	defer envtesting.RemoveFakeTools(c, metadataStorage)
	openstack.UseTestImageData(s.env, s.cred)
	defer openstack.RemoveTestImageData(s.env)

	err = bootstrap.Bootstrap(s.env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (s *localHTTPSServerSuite) TestCanListPublicBucket(c *gc.C) {
	storage := s.env.PublicStorage()
	content, err := storage.List("")
	c.Assert(err, gc.IsNil)
	c.Assert(content, gc.DeepEquals, []string(nil))
}

func (s *localHTTPSServerSuite) TestFetchFromImageMetadataSources(c *gc.C) {
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, gc.IsNil)
	c.Check(customURL[:8], gc.Equals, "https://")

	config, err := s.env.Config().Apply(
		map[string]interface{}{"image-metadata-url": customURL},
	)
	c.Assert(err, gc.IsNil)
	err = s.env.SetConfig(config)
	c.Assert(err, gc.IsNil)
	sources, err := imagemetadata.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	c.Assert(sources, gc.HasLen, 4)

	// Make sure there is something to download from each location
	private := "private-content"
	err = s.env.Storage().Put(private, bytes.NewBufferString(private), int64(len(private)))
	c.Assert(err, gc.IsNil)

	metadata := "metadata-content"
	metadataStorage := openstack.ImageMetadataStorage(s.env)
	err = metadataStorage.Put(metadata, bytes.NewBufferString(metadata), int64(len(metadata)))
	c.Assert(err, gc.IsNil)

	custom := "custom-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, gc.IsNil)

	// Read from the private bucket
	contentReader, url, err := sources[0].Fetch(private)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Check(string(content), gc.Equals, private)
	c.Check(url[:8], gc.Equals, "https://")

	// Check the entry we got from keystone
	contentReader, url, err = sources[1].Fetch(metadata)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, metadata)
	c.Check(url[:8], gc.Equals, "https://")
	// Verify that we are pointing at exactly where metadataStorage thinks we are
	metaURL, err := metadataStorage.URL(metadata)
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, metaURL)

	// Read from the Config entry's image-metadata-url
	contentReader, url, err = sources[2].Fetch(custom)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, custom)
	c.Check(url[:8], gc.Equals, "https://")

}

func (s *localHTTPSServerSuite) TestFetchFromToolsMetadataSources(c *gc.C) {
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-tools-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, gc.IsNil)
	c.Check(customURL[:8], gc.Equals, "https://")

	config, err := s.env.Config().Apply(
		map[string]interface{}{"tools-url": customURL},
	)
	c.Assert(err, gc.IsNil)
	err = s.env.SetConfig(config)
	c.Assert(err, gc.IsNil)
	sources, err := tools.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	c.Assert(sources, gc.HasLen, 4)

	// Make sure there is something to download from each location
	private := "private-tools-content"
	// The Private data storage always tacks on "tools/" to the URL stream,
	// so add it in here
	err = s.env.Storage().Put("tools/"+private, bytes.NewBufferString(private), int64(len(private)))
	c.Assert(err, gc.IsNil)

	keystone := "keystone-tools-content"
	// The keystone entry just points at the root of the Swift storage, and
	// we have to create a container to upload any data. So we just point
	// into a subdirectory for the data we are downloading
	keystoneContainer := "tools-test"
	keystoneStorage := openstack.CreateCustomStorage(s.env, "tools-test")
	err = keystoneStorage.Put(keystone, bytes.NewBufferString(keystone), int64(len(keystone)))
	c.Assert(err, gc.IsNil)

	custom := "custom-tools-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, gc.IsNil)

	// Read from the private bucket
	contentReader, url, err := sources[0].Fetch(private)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Check(string(content), gc.Equals, private)
	//c.Check(url[:8], gc.Equals, "https://")
	c.Check(strings.HasSuffix(url, "tools/"+private), jc.IsTrue)

	// Check the entry we got from keystone
	// Now fetch the data, and verify the contents.
	contentReader, url, err = sources[1].Fetch(keystoneContainer + "/" + keystone)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, keystone)
	c.Check(url[:8], gc.Equals, "https://")
	keystoneURL, err := keystoneStorage.URL(keystone)
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, keystoneURL)

	// Read from the Config entry's tools-url
	contentReader, url, err = sources[2].Fetch(custom)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, custom)
	c.Check(url[:8], gc.Equals, "https://")

	// We *don't* test Fetch for sources[3] because it points to
	// juju.canonical.com
}
