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

	jujuerrors "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testservices/hook"
	"launchpad.net/goose/testservices/openstackservice"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/openstack"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
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
	s.Service.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{Name: "test-unavailable"},
		nova.AvailabilityZone{
			Name: "test-available",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)
}

func (s *localServer) stop() {
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
	s.restoreTimeouts()
}

// localLiveSuite runs tests from LiveTests using an Openstack service double.
type localLiveSuite struct {
	coretesting.BaseSuite
	LiveTests
	srv localServer
}

func (s *localLiveSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	c.Logf("Running live tests using openstack service test double")
	s.srv.start(c, s.cred)
	s.LiveTests.SetUpSuite(c)
	openstack.UseTestImageData(openstack.ImageMetadataStorage(s.Env), s.cred)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) { restoreFinishBootstrap() })
}

func (s *localLiveSuite) TearDownSuite(c *gc.C) {
	openstack.RemoveTestImageData(openstack.ImageMetadataStorage(s.Env))
	s.LiveTests.TearDownSuite(c)
	s.srv.stop()
	s.BaseSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
}

func (s *localLiveSuite) TearDownTest(c *gc.C) {
	s.LiveTests.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

// localServerSuite contains tests that run against an Openstack service double.
// These tests can test things that would be unreasonably slow or expensive
// to test on a live Openstack server. The service double is started and stopped for
// each test.
type localServerSuite struct {
	coretesting.BaseSuite
	jujutest.Tests
	cred                 *identity.Credentials
	srv                  localServer
	toolsMetadataStorage storage.Storage
	imageMetadataStorage storage.Storage
}

func (s *localServerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) { restoreFinishBootstrap() })
	c.Logf("Running local tests")
}

func (s *localServerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.srv.start(c, s.cred)
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, gc.IsNil)
	containerURL, err := cl.MakeServiceURL("object-store", nil)
	c.Assert(err, gc.IsNil)
	s.TestConfig = s.TestConfig.Merge(coretesting.Attrs{
		"tools-metadata-url": containerURL + "/juju-dist-test/tools",
		"image-metadata-url": containerURL + "/juju-dist-test",
		"auth-url":           s.cred.URL,
	})
	s.Tests.SetUpTest(c)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	env := s.Prepare(c)
	s.toolsMetadataStorage = openstack.MetadataStorage(env)
	// Put some fake metadata in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running can find the metadata.
	envtesting.UploadFakeTools(c, s.toolsMetadataStorage)
	s.imageMetadataStorage = openstack.ImageMetadataStorage(env)
	openstack.UseTestImageData(s.imageMetadataStorage, s.cred)
}

func (s *localServerSuite) TearDownTest(c *gc.C) {
	if s.imageMetadataStorage != nil {
		openstack.RemoveTestImageData(s.imageMetadataStorage)
	}
	if s.toolsMetadataStorage != nil {
		envtesting.RemoveFakeToolsMetadata(c, s.toolsMetadataStorage)
	}
	s.Tests.TearDownTest(c)
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
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

	// Create a config that matches s.TestConfig but with use-floating-ip set to true
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		"use-floating-ip": true,
	}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	_, err = bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
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

	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		"use-floating-ip": false,
	}))
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, coretesting.Context(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)
	_, err = bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)
	inst, _ := testing.AssertStartInstance(c, env, "100")
	err = env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	// Ensure amd64 tools are available, to ensure an amd64 image.
	amd64Version := version.Current
	amd64Version.Arch = arch.AMD64
	for _, series := range bootstrap.ToolsLtsSeries {
		amd64Version.Series = series
		envtesting.AssertUploadFakeToolsVersions(c, s.toolsMetadataStorage, amd64Version)
	}

	env := s.Prepare(c)
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, env, "100", constraints.MustParse("mem=1024"))
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(2048))
	c.Check(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(hc.CpuPower, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceNetwork(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		// A label that corresponds to a nova test service network
		"network": "net",
	}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	inst, _ := testing.AssertStartInstance(c, env, "100")
	err = env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkUnknownLabel(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		// A label that has no related network in the nova test service
		"network": "no-network-with-this-label",
	}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	inst, _, _, err := testing.StartInstance(env, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "No networks exist with label .*")
}

func (s *localServerSuite) TestStartInstanceNetworkUnknownId(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		// A valid UUID but no related network in the nova test service
		"network": "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
	}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	inst, _, _, err := testing.StartInstance(env, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot run instance: (\\n|.)*"+
		"caused by: "+
		"request \\(.*/servers\\) returned unexpected status: "+
		"404; error info: .*itemNotFound.*")
}

func assertSecurityGroups(c *gc.C, env environs.Environ, expected []string) {
	novaClient := openstack.GetNovaClient(env)
	groups, err := novaClient.ListSecurityGroups()
	c.Assert(err, gc.IsNil)
	for _, name := range expected {
		found := false
		for _, group := range groups {
			if group.Name == name {
				found = true
				break
			}
		}
		if !found {
			c.Errorf("expected security group %q not found", name)
		}
	}
	for _, group := range groups {
		found := false
		for _, name := range expected {
			if group.Name == name {
				found = true
				break
			}
		}
		if !found {
			c.Errorf("existing security group %q is not expected", group.Name)
		}
	}
}

func (s *localServerSuite) TestStopInstance(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		"firewall-mode": "instance"}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	instanceName := "100"
	inst, _ := testing.AssertStartInstance(c, env, instanceName)
	// Openstack now has three security groups for the server, the default
	// group, one group for the entire environment, and another for the
	// new instance.
	name := env.Config().Name()
	assertSecurityGroups(c, env, []string{"default", fmt.Sprintf("juju-%v", name), fmt.Sprintf("juju-%v-%v", name, instanceName)})
	err = env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
	// The security group for this instance is now removed.
	assertSecurityGroups(c, env, []string{"default", fmt.Sprintf("juju-%v", name)})
}

// Due to bug #1300755 it can happen that the security group intended for
// an instance is also used as the common security group of another
// environment. If this is the case, the attempt to delete the instance's
// security group fails but StopInstance succeeds.
func (s *localServerSuite) TestStopInstanceSecurityGroupNotDeleted(c *gc.C) {
	// Force an error when a security group is deleted.
	cleanup := s.srv.Service.Nova.RegisterControlPoint(
		"removeSecurityGroup",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("failed on purpose")
		},
	)
	defer cleanup()
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		"firewall-mode": "instance"}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	instanceName := "100"
	inst, _ := testing.AssertStartInstance(c, env, instanceName)
	name := env.Config().Name()
	allSecurityGroups := []string{"default", fmt.Sprintf("juju-%v", name), fmt.Sprintf("juju-%v-%v", name, instanceName)}
	assertSecurityGroups(c, env, allSecurityGroups)
	err = env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
	assertSecurityGroups(c, env, allSecurityGroups)
}

func (s *localServerSuite) TestDestroyEnvironmentDeletesSecurityGroupsFWModeInstance(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		"firewall-mode": "instance"}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	instanceName := "100"
	testing.AssertStartInstance(c, env, instanceName)
	name := env.Config().Name()
	allSecurityGroups := []string{"default", fmt.Sprintf("juju-%v", name), fmt.Sprintf("juju-%v-%v", name, instanceName)}
	assertSecurityGroups(c, env, allSecurityGroups)
	err = env.Destroy()
	c.Check(err, gc.IsNil)
	assertSecurityGroups(c, env, []string{"default"})
}

func (s *localServerSuite) TestDestroyEnvironmentDeletesSecurityGroupsFWModeGlobal(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(coretesting.Attrs{
		"firewall-mode": "global"}))
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	instanceName := "100"
	testing.AssertStartInstance(c, env, instanceName)
	name := env.Config().Name()
	allSecurityGroups := []string{"default", fmt.Sprintf("juju-%v", name), fmt.Sprintf("juju-%v-global", name)}
	assertSecurityGroups(c, env, allSecurityGroups)
	err = env.Destroy()
	c.Check(err, gc.IsNil)
	assertSecurityGroups(c, env, []string{"default"})
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
	env := s.Prepare(c)
	// goose's test service always returns ACTIVE state.
	inst, _ := testing.AssertStartInstance(c, env, "100")
	c.Assert(inst.Status(), gc.Equals, nova.StatusActive)
	err := env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestInstancesGathering(c *gc.C) {
	env := s.Prepare(c)
	inst0, _ := testing.AssertStartInstance(c, env, "100")
	id0 := inst0.Id()
	inst1, _ := testing.AssertStartInstance(c, env, "101")
	id1 := inst1.Id()
	defer func() {
		err := env.StopInstances(inst0.Id(), inst1.Id())
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
		insts, err := env.Instances(ids)
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
	env := s.Prepare(c)
	cleanup := s.srv.Service.Nova.RegisterControlPoint(
		"addServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			details := args[0].(*nova.ServerDetail)
			details.Status = "BUILD(networking)"
			return nil
		},
	)
	defer cleanup()
	stateInst, _ := testing.AssertStartInstance(c, env, "100")
	defer func() {
		err := env.StopInstances(stateInst.Id())
		c.Assert(err, gc.IsNil)
	}()
	found := make(map[instance.Id]instance.Instance)
	missing := []instance.Id{stateInst.Id()}

	resultMissing := openstack.CollectInstances(env, missing, found)

	c.Assert(resultMissing, gc.DeepEquals, missing)
}

func (s *localServerSuite) TestInstancesBuildSpawning(c *gc.C) {
	env := s.Prepare(c)
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
	stateInst, _ := testing.AssertStartInstance(c, env, "100")
	defer func() {
		err := env.StopInstances(stateInst.Id())
		c.Assert(err, gc.IsNil)
	}()

	instances, err := env.Instances([]instance.Id{stateInst.Id()})

	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Status(), gc.Equals, nova.StatusBuildSpawning)
}

// TODO (wallyworld) - this test was copied from the ec2 provider.
// It should be moved to environs.jujutests.Tests.
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *gc.C) {
	env := s.Prepare(c)
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := common.LoadState(env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(stateData.StateInstances, gc.HasLen, 1)

	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, stateData.StateInstances[0])

	addresses, err := insts[0].Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	// TODO(wallyworld) - 2013-03-01 bug=1137005
	// The nova test double needs to be updated to support retrieving instance userData.
	// Until then, we can't check the cloud init script was generated correctly.
	// When we can, we should also check cloudinit for a non-manager node (as in the
	// ec2 tests).
}

func (s *localServerSuite) assertGetImageMetadataSources(c *gc.C, stream, officialSourcePath string) {
	// Create a config that matches s.TestConfig but with the specified stream.
	envAttrs := s.TestConfig
	if stream != "" {
		envAttrs = envAttrs.Merge(coretesting.Attrs{"image-stream": stream})
	}
	cfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(sources, gc.HasLen, 4)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		urls[i] = url
	}
	// The image-metadata-url ends with "/juju-dist-test/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist-test/"), jc.IsTrue)
	// The control bucket URL contains the bucket name.
	c.Check(strings.Contains(urls[1], openstack.ControlBucketName(env)+"/images"), jc.IsTrue)
	// The product-streams URL ends with "/imagemetadata".
	c.Check(strings.HasSuffix(urls[2], "/imagemetadata/"), jc.IsTrue)
	c.Assert(urls[3], gc.Equals, fmt.Sprintf("http://cloud-images.ubuntu.com/%s/", officialSourcePath))
}

func (s *localServerSuite) TestGetImageMetadataSources(c *gc.C) {
	s.assertGetImageMetadataSources(c, "", "releases")
	s.assertGetImageMetadataSources(c, "released", "releases")
	s.assertGetImageMetadataSources(c, "daily", "daily")
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	env := s.Open(c)
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(sources, gc.HasLen, 3)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		urls[i] = url
	}
	// The tools-metadata-url ends with "/juju-dist-test/tools/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist-test/tools/"), jc.IsTrue)
	// The control bucket URL contains the bucket name.
	c.Check(strings.Contains(urls[1], openstack.ControlBucketName(env)+"/tools"), jc.IsTrue)
	// Check that the URL from keystone parses.
	_, err = url.Parse(urls[2])
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestSupportedArchitectures(c *gc.C) {
	env := s.Open(c)
	a, err := env.SupportedArchitectures()
	c.Assert(err, gc.IsNil)
	c.Assert(a, jc.SameContents, []string{"amd64", "i386", "ppc64"})
}

func (s *localServerSuite) TestSupportNetworks(c *gc.C) {
	env := s.Open(c)
	c.Assert(env.SupportNetworks(), jc.IsFalse)
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *gc.C) {
	// Prevent falling over to the public datasource.
	s.BaseSuite.PatchValue(&imagemetadata.DefaultBaseURL, "")

	env := s.Open(c)

	// An error occurs if no suitable image is found.
	_, err := openstack.FindInstanceSpec(env, "saucy", "amd64", "mem=1G")
	c.Assert(err, gc.ErrorMatches, `no "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestConstraintsValidator(c *gc.C) {
	env := s.Open(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, gc.IsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power"})
}

func (s *localServerSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.Open(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=arm64")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=arm64\nvalid values are:.*")
	cons = constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (s *localServerSuite) TestConstraintsMerge(c *gc.C) {
	env := s.Open(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	consA := constraints.MustParse("arch=amd64 mem=1G root-disk=10G")
	consB := constraints.MustParse("instance-type=m1.small")
	cons, err := validator.Merge(consA, consB)
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, constraints.MustParse("instance-type=m1.small"))
}

func (s *localServerSuite) TestFindImageInstanceConstraint(c *gc.C) {
	// Prevent falling over to the public datasource.
	s.BaseSuite.PatchValue(&imagemetadata.DefaultBaseURL, "")

	env := s.Open(c)
	spec, err := openstack.FindInstanceSpec(env, "precise", "amd64", "instance-type=m1.tiny")
	c.Assert(err, gc.IsNil)
	c.Assert(spec.InstanceType.Name, gc.Equals, "m1.tiny")
}

func (s *localServerSuite) TestFindImageInvalidInstanceConstraint(c *gc.C) {
	// Prevent falling over to the public datasource.
	s.BaseSuite.PatchValue(&imagemetadata.DefaultBaseURL, "")

	env := s.Open(c)
	_, err := openstack.FindInstanceSpec(env, "precise", "amd64", "instance-type=m1.large")
	c.Assert(err, gc.ErrorMatches, `no instance types in some-region matching constraints "instance-type=m1.large"`)
}

func (s *localServerSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	env := s.Open(c)
	cons := constraints.MustParse("instance-type=m1.small")
	placement := ""
	err := env.PrecheckInstance("precise", cons, placement)
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := s.Open(c)
	cons := constraints.MustParse("instance-type=m1.large")
	placement := ""
	err := env.PrecheckInstance("precise", cons, placement)
	c.Assert(err, gc.ErrorMatches, `invalid Openstack flavour "m1.large" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-available"
	err := env.PrecheckInstance("precise", constraints.Value{}, placement)
	c.Assert(err, gc.IsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unavailable"
	err := env.PrecheckInstance("precise", constraints.Value{}, placement)
	c.Assert(err, gc.IsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unknown"
	err := env.PrecheckInstance("precise", constraints.Value{}, placement)
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZonesUnsupported(c *gc.C) {
	t.srv.Service.Nova.SetAvailabilityZones() // no availability zone support
	env := t.Prepare(c)
	placement := "zone=test-unknown"
	err := env.PrecheckInstance("precise", constraints.Value{}, placement)
	c.Assert(err, jc.Satisfies, jujuerrors.IsNotImplemented)
}

func (s *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	env := s.Open(c)
	params, err := env.(simplestreams.MetadataValidator).MetadataLookupParams("some-region")
	c.Assert(err, gc.IsNil)
	params.Sources, err = imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	params.Series = "raring"
	image_ids, _, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(image_ids, jc.SameContents, []string{"id-y"})
}

func (s *localServerSuite) TestRemoveAll(c *gc.C) {
	env := s.Prepare(c)
	stor := env.Storage()
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
	env := s.Prepare(c)
	stor := env.Storage()
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
	env := s.Prepare(c)
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

	group, err := openstack.EnsureGroup(env, "test group", rule)
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
	group, err = openstack.EnsureGroup(env, "test group", anotherRule)
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
	coretesting.BaseSuite
	attrs map[string]interface{}
	cred  *identity.Credentials
	srv   localServer
	env   environs.Environ
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
	attrs["tools-metadata-url"] = containerURL + "/juju-dist-test/tools"
	c.Logf("Set tools-metadata-url=%q", attrs["tools-metadata-url"])
	attrs["image-metadata-url"] = containerURL + "/juju-dist-test"
	c.Logf("Set image-metadata-url=%q", attrs["image-metadata-url"])
	return attrs
}

func (s *localHTTPSServerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
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
	s.env, err = environs.Prepare(cfg, coretesting.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	s.attrs = s.env.Config().AllAttrs()
}

func (s *localHTTPSServerSuite) TearDownTest(c *gc.C) {
	if s.env != nil {
		err := s.env.Destroy()
		c.Check(err, gc.IsNil)
		s.env = nil
	}
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
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
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	defer restoreFinishBootstrap()

	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	metadataStorage := openstack.MetadataStorage(s.env)
	url, err := metadataStorage.URL("")
	c.Assert(err, gc.IsNil)
	c.Logf("Generating fake tools for: %v", url)
	envtesting.UploadFakeTools(c, metadataStorage)
	defer envtesting.RemoveFakeTools(c, metadataStorage)
	openstack.UseTestImageData(metadataStorage, s.cred)
	defer openstack.RemoveTestImageData(metadataStorage)

	_, err = bootstrap.Bootstrap(coretesting.Context(c), s.env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)
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
	err = s.env.Storage().Put("images/"+private, bytes.NewBufferString(private), int64(len(private)))
	c.Assert(err, gc.IsNil)

	metadata := "metadata-content"
	metadataStorage := openstack.ImageMetadataStorage(s.env)
	err = metadataStorage.Put(metadata, bytes.NewBufferString(metadata), int64(len(metadata)))
	c.Assert(err, gc.IsNil)

	custom := "custom-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, gc.IsNil)

	// Read from the Config entry's image-metadata-url
	contentReader, url, err := sources[0].Fetch(custom)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, custom)
	c.Check(url[:8], gc.Equals, "https://")

	// Read from the private bucket
	contentReader, url, err = sources[1].Fetch(private)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Check(string(content), gc.Equals, private)
	c.Check(url[:8], gc.Equals, "https://")

	// Check the entry we got from keystone
	contentReader, url, err = sources[2].Fetch(metadata)
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

}

func (s *localHTTPSServerSuite) TestFetchFromToolsMetadataSources(c *gc.C) {
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-tools-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, gc.IsNil)
	c.Check(customURL[:8], gc.Equals, "https://")

	config, err := s.env.Config().Apply(
		map[string]interface{}{"tools-metadata-url": customURL},
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

	// Read from the Config entry's tools-metadata-url
	contentReader, url, err := sources[0].Fetch(custom)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, custom)
	c.Check(url[:8], gc.Equals, "https://")

	// Read from the private bucket
	contentReader, url, err = sources[1].Fetch(private)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Check(string(content), gc.Equals, private)
	//c.Check(url[:8], gc.Equals, "https://")
	c.Check(strings.HasSuffix(url, "tools/"+private), jc.IsTrue)

	// Check the entry we got from keystone
	// Now fetch the data, and verify the contents.
	contentReader, url, err = sources[2].Fetch(keystoneContainer + "/" + keystone)
	c.Assert(err, gc.IsNil)
	defer contentReader.Close()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, keystone)
	c.Check(url[:8], gc.Equals, "https://")
	keystoneURL, err := keystoneStorage.URL(keystone)
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, keystoneURL)

	// We *don't* test Fetch for sources[3] because it points to
	// streams.canonical.com
}

func (s *localServerSuite) TestAllInstancesIgnoresOtherMachines(c *gc.C) {
	env := s.Prepare(c)
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	// Check that we see 1 instance in the environment
	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Check(insts, gc.HasLen, 1)

	// Now start a machine 'manually' in the same account, with a similar
	// but not matching name, and ensure it isn't seen by AllInstances
	// See bug #1257481, for how similar names were causing them to get
	// listed (and thus destroyed) at the wrong time
	existingEnvName := s.TestConfig["name"]
	newMachineName := fmt.Sprintf("juju-%s-2-machine-0", existingEnvName)

	// We grab the Nova client directly from the env, just to save time
	// looking all the stuff up
	novaClient := openstack.GetNovaClient(env)
	entity, err := novaClient.RunServer(nova.RunServerOpts{
		Name:     newMachineName,
		FlavorId: "1", // test service has 1,2,3 for flavor ids
		ImageId:  "1", // UseTestImageData sets up images 1 and 2
	})
	c.Assert(err, gc.IsNil)
	c.Assert(entity, gc.NotNil)

	// List all servers with no filter, we should see both instances
	servers, err := novaClient.ListServersDetail(nova.NewFilter())
	c.Assert(err, gc.IsNil)
	c.Assert(servers, gc.HasLen, 2)

	insts, err = env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Check(insts, gc.HasLen, 1)
}

func (s *localServerSuite) TestResolveNetworkUUID(c *gc.C) {
	env := s.Prepare(c)
	var sampleUUID = "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"
	networkId, err := openstack.ResolveNetwork(env, sampleUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(networkId, gc.Equals, sampleUUID)
}

func (s *localServerSuite) TestResolveNetworkLabel(c *gc.C) {
	env := s.Prepare(c)
	// For now this test has to cheat and use knowledge of goose internals
	var networkLabel = "net"
	var expectNetworkId = "1"
	networkId, err := openstack.ResolveNetwork(env, networkLabel)
	c.Assert(err, gc.IsNil)
	c.Assert(networkId, gc.Equals, expectNetworkId)
}

func (s *localServerSuite) TestResolveNetworkNotPresent(c *gc.C) {
	env := s.Prepare(c)
	var notPresentNetwork = "no-network-with-this-label"
	networkId, err := openstack.ResolveNetwork(env, notPresentNetwork)
	c.Check(networkId, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `No networks exist with label "no-network-with-this-label"`)
}

// TODO(gz): TestResolveNetworkMultipleMatching when can inject new networks

func (t *localServerSuite) TestStartInstanceAvailZone(c *gc.C) {
	inst, err := t.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, gc.IsNil)
	c.Assert(openstack.InstanceServerDetail(inst).AvailabilityZone, gc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceAvailZoneUnavailable(c *gc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-unavailable")
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unavailable" is unavailable`)
}

func (t *localServerSuite) TestStartInstanceAvailZoneUnknown(c *gc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (t *localServerSuite) testStartInstanceAvailZone(c *gc.C, zone string) (instance.Instance, error) {
	env := t.Prepare(c)
	envtesting.UploadFakeTools(c, env.Storage())
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	params := environs.StartInstanceParams{Placement: "zone=" + zone}
	inst, _, _, err := testing.StartInstanceWithParams(env, "1", params, nil)
	return inst, err
}

func (t *localServerSuite) TestGetAvailabilityZones(c *gc.C) {
	var resultZones []nova.AvailabilityZone
	var resultErr error
	t.PatchValue(openstack.NovaListAvailabilityZones, func(c *nova.Client) ([]nova.AvailabilityZone, error) {
		return append([]nova.AvailabilityZone{}, resultZones...), resultErr
	})
	env := t.Prepare(c).(common.ZonedEnviron)

	resultErr = fmt.Errorf("failed to get availability zones")
	zones, err := env.AvailabilityZones()
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zones, gc.IsNil)

	resultErr = nil
	resultZones = make([]nova.AvailabilityZone, 1)
	resultZones[0].Name = "whatever"
	zones, err = env.AvailabilityZones()
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	resultErr = fmt.Errorf("failed to get availability zones")
	resultZones[0].Name = "andever"
	zones, err = env.AvailabilityZones()
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
}

func (t *localServerSuite) TestGetAvailabilityZonesCommon(c *gc.C) {
	var resultZones []nova.AvailabilityZone
	t.PatchValue(openstack.NovaListAvailabilityZones, func(c *nova.Client) ([]nova.AvailabilityZone, error) {
		return append([]nova.AvailabilityZone{}, resultZones...), nil
	})
	env := t.Prepare(c).(common.ZonedEnviron)
	resultZones = make([]nova.AvailabilityZone, 2)
	resultZones[0].Name = "az1"
	resultZones[1].Name = "az2"
	resultZones[0].State.Available = true
	resultZones[1].State.Available = false
	zones, err := env.AvailabilityZones()
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.HasLen, 2)
	c.Assert(zones[0].Name(), gc.Equals, resultZones[0].Name)
	c.Assert(zones[1].Name(), gc.Equals, resultZones[1].Name)
	c.Assert(zones[0].Available(), jc.IsTrue)
	c.Assert(zones[1].Available(), jc.IsFalse)
}

type mockAvailabilityZoneAllocations struct {
	group  []instance.Id // input param
	result []common.AvailabilityZoneInstances
	err    error
}

func (t *mockAvailabilityZoneAllocations) AvailabilityZoneAllocations(
	e common.ZonedEnviron, group []instance.Id,
) ([]common.AvailabilityZoneInstances, error) {
	t.group = group
	return t.result, t.err
}

func (t *localServerSuite) TestStartInstanceDistributionParams(c *gc.C) {
	env := t.Prepare(c)
	envtesting.UploadFakeTools(c, env.Storage())
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	var mock mockAvailabilityZoneAllocations
	t.PatchValue(openstack.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	// no distribution group specified
	testing.AssertStartInstance(c, env, "1")
	c.Assert(mock.group, gc.HasLen, 0)

	// distribution group specified: ensure it's passed through to AvailabilityZone.
	expectedInstances := []instance.Id{"i-0", "i-1"}
	params := environs.StartInstanceParams{
		DistributionGroup: func() ([]instance.Id, error) {
			return expectedInstances, nil
		},
	}
	_, _, _, err = testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(mock.group, gc.DeepEquals, expectedInstances)
}

func (t *localServerSuite) TestStartInstanceDistributionErrors(c *gc.C) {
	env := t.Prepare(c)
	envtesting.UploadFakeTools(c, env.Storage())
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	mock := mockAvailabilityZoneAllocations{
		err: fmt.Errorf("AvailabilityZoneAllocations failed"),
	}
	t.PatchValue(openstack.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)
	_, _, _, err = testing.StartInstance(env, "1")
	c.Assert(err, gc.Equals, mock.err)

	mock.err = nil
	dgErr := fmt.Errorf("DistributionGroup failed")
	params := environs.StartInstanceParams{
		DistributionGroup: func() ([]instance.Id, error) {
			return nil, dgErr
		},
	}
	_, _, _, err = testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, gc.Equals, dgErr)
}

func (t *localServerSuite) TestStartInstanceDistribution(c *gc.C) {
	env := t.Prepare(c)
	envtesting.UploadFakeTools(c, env.Storage())
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	// test-available is the only available AZ, so AvailabilityZoneAllocations
	// is guaranteed to return that.
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(openstack.InstanceServerDetail(inst).AvailabilityZone, gc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceDistributionAZNotImplemented(c *gc.C) {
	env := t.Prepare(c)
	envtesting.UploadFakeTools(c, env.Storage())
	_, err := bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	mock := mockAvailabilityZoneAllocations{
		err: jujuerrors.NotImplementedf("availability zones"),
	}
	t.PatchValue(openstack.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	// Instance will be created without an availability zone specified.
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(openstack.InstanceServerDetail(inst).AvailabilityZone, gc.Equals, "")
}
