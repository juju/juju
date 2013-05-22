// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/openstackservice"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/openstack"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/http"
	"net/http/httptest"
	"strings"
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
	s.Service = openstackservice.New(cred, identity.AuthUserPass)
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
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *C) {
	policy := s.env.AssignmentPolicy()
	c.Assert(policy, Equals, state.AssignNew)

	err := environs.Bootstrap(s.env, constraints.Value{})
	c.Assert(err, IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := openstack.LoadState(s.env)
	c.Assert(err, IsNil)
	c.Assert(stateData.StateInstances, HasLen, 1)

	insts, err := s.env.Instances(stateData.StateInstances)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 1)
	c.Check(insts[0].Id(), Equals, stateData.StateInstances[0])

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
	inst1, err := s.env.StartInstance("1", "fake_nonce", series, constraints.Value{}, info, apiInfo)
	c.Assert(err, IsNil)

	err = s.env.Destroy(append(insts, inst1))
	c.Assert(err, IsNil)

	_, err = openstack.LoadState(s.env)
	c.Assert(err, NotNil)
}

func (s *localServerSuite) TestGetImageURLs(c *C) {
	urls, err := openstack.GetImageURLs(s.env)
	c.Assert(err, IsNil)
	c.Assert(len(urls), Equals, 3)
	// The public bucket URL ends with "/juju-dist/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist/"), Equals, true)
	// The product-streams URL ends with "/imagemetadata".
	c.Check(strings.HasSuffix(urls[1], "/imagemetadata"), Equals, true)
	c.Assert(urls[2], Equals, imagemetadata.DefaultBaseURL)
}

func (s *localServerSuite) TestFindImageSpecPublicStorage(c *C) {
	openstack.SetDefaultInstanceType(s.Env, "")
	openstack.SetDefaultImageId(s.Env, "")
	spec, err := openstack.FindInstanceSpec(s.Env, "raring", "amd64", "mem=512M")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "id-y")
	c.Assert(spec.InstanceTypeName, Equals, "m1.tiny")
}

// If no suitable image is found, use the default if specified.
func (s *localServerSuite) TestFindImageSpecDefaultWhenNoImage(c *C) {
	openstack.SetDefaultImageId(s.Env, "1234")
	spec, err := openstack.FindInstanceSpec(s.Env, "saucy", "amd64", "")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "1234")
	c.Assert(spec.InstanceTypeName, Not(Equals), "")
}

// If multiple images are found, use the default if specified.
func (s *localServerSuite) TestFindImageSpecDefaultImage(c *C) {
	openstack.SetDefaultImageId(s.Env, "1")
	spec, err := openstack.FindInstanceSpec(s.Env, "precise", "amd64", "")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "1")
	c.Assert(spec.InstanceTypeName, Not(Equals), "")
}

// If multiple instance types found, use the default flavor if specified.
func (s *localServerSuite) TestFindImageSpecDefaultFlavor(c *C) {
	openstack.SetDefaultImageId(s.Env, "1")
	openstack.SetDefaultInstanceType(s.Env, "m1.small")
	spec, err := openstack.FindInstanceSpec(s.Env, "precise", "amd64", "")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "1")
	c.Assert(spec.InstanceTypeName, Equals, "m1.small")
}

// An error occurs if multiple matching instance types found and the default flavor is invalid.
func (s *localServerSuite) TestFindImageBadDefaultFlavor(c *C) {
	openstack.SetDefaultInstanceType(s.Env, "bad.flavor")
	_, err := openstack.FindInstanceSpec(s.Env, "precise", "amd64", "")
	c.Assert(err, ErrorMatches, `invalid default instance type name "bad.flavor"`)
}

// An error occurs if no suitable image is found and the default not specified.
func (s *localServerSuite) TestFindImageBadDefaultImage(c *C) {
	openstack.SetDefaultImageId(s.Env, "")
	_, err := openstack.FindInstanceSpec(s.Env, "saucy", "amd64", "mem=8G")
	c.Assert(err, ErrorMatches, `no "saucy" images in some-region with arches \[amd64\], and no default specified`)
}

func (s *localServerSuite) TestDeleteAll(c *C) {
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
	err = openstack.DeleteStorageContent(storage)
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
	err = openstack.DeleteStorageContent(storage)
	c.Assert(err, IsNil)
	_, err = storage.Get("ab")
	c.Assert(err, NotNil)
}
