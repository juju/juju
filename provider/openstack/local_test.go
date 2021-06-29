// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"bytes"
	stdcontext "context"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-goose/goose/v4/cinder"
	"github.com/go-goose/goose/v4/client"
	"github.com/go-goose/goose/v4/identity"
	"github.com/go-goose/goose/v4/neutron"
	"github.com/go-goose/goose/v4/nova"
	"github.com/go-goose/goose/v4/testservices/hook"
	"github.com/go-goose/goose/v4/testservices/identityservice"
	"github.com/go-goose/goose/v4/testservices/neutronmodel"
	"github.com/go-goose/goose/v4/testservices/neutronservice"
	"github.com/go-goose/goose/v4/testservices/novaservice"
	"github.com/go-goose/goose/v4/testservices/openstackservice"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/utils/v2/arch"
	"github.com/juju/utils/v2/ssh"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envstorage "github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type ProviderSuite struct {
	restoreTimeouts func()
}

var _ = gc.Suite(&ProviderSuite{})

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
	testConfig := makeTestConfig(cred)
	testConfig["agent-version"] = coretesting.FakeVersionNumber.String()
	testConfig["authorized-keys"] = "fakekey"
	testConfig["network"] = "private_999"
	gc.Suite(&localLiveSuite{
		LiveTests: LiveTests{
			cred: cred,
			LiveTests: jujutest.LiveTests{
				TestConfig: testConfig,
			},
		},
	})
	gc.Suite(&localServerSuite{
		cred: cred,
		Tests: jujutest.Tests{
			TestConfig: testConfig,
		},
	})
	gc.Suite(&noNeutronSuite{
		cred: cred,
	})
}

// localServer is used to spin up a local Openstack service double.
type localServer struct {
	OpenstackSvc    *openstackservice.Openstack
	Nova            *novaservice.Nova
	Neutron         *neutronservice.Neutron
	restoreTimeouts func()
	UseTLS          bool
}

type newOpenstackFunc func(*identity.Credentials, identity.AuthMode, bool) (*openstackservice.Openstack, []string)

func (s *localServer) start(
	c *gc.C, cred *identity.Credentials, newOpenstackFunc newOpenstackFunc,
) {
	var logMsg []string
	s.OpenstackSvc, logMsg = newOpenstackFunc(cred, identity.AuthUserPass, s.UseTLS)
	s.Nova = s.OpenstackSvc.Nova
	s.Neutron = s.OpenstackSvc.Neutron
	for _, msg := range logMsg {
		c.Logf("%v", msg)
	}
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(openstack.ShortAttempt, openstack.StorageAttempt)
	s.Nova.SetAvailabilityZones(
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
	if s.OpenstackSvc != nil {
		s.OpenstackSvc.Stop()
	} else if s.Nova != nil {
		s.Nova.Stop()
	}
	s.restoreTimeouts()
}

func (s *localServer) openstackCertificate(c *gc.C) ([]string, error) {
	certificate, err := s.OpenstackSvc.Certificate(openstackservice.Identity)
	if err != nil {
		return []string{}, err
	}
	if certificate == nil {
		return []string{}, errors.New("No certificate returned from openstack test double")
	}
	buf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	return []string{string(buf)}, nil
}

func (s *localHTTPSServerSuite) envUsingCertificate(c *gc.C) environs.Environ {
	newattrs := make(map[string]interface{}, len(s.attrs))
	for k, v := range s.attrs {
		newattrs[k] = v
	}
	newattrs["ssl-hostname-verification"] = true
	cfg, err := config.New(config.NoDefaults, newattrs)
	c.Assert(err, jc.ErrorIsNil)

	cloudSpec := makeCloudSpec(s.cred)
	cloudSpec.CACertificates, err = s.srv.openstackCertificate(c)
	c.Assert(err, jc.ErrorIsNil)

	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

// localLiveSuite runs tests from LiveTests using an Openstack service double.
type localLiveSuite struct {
	coretesting.BaseSuite
	LiveTests
	srv localServer
}

func makeMockAdapter() *mockAdapter {
	volumes := make(map[string]*cinder.Volume)
	return &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			metadata := args.Metadata.(map[string]string)
			volume := cinder.Volume{
				ID:               args.Name,
				Metadata:         metadata,
				Status:           "cool",
				AvailabilityZone: args.AvailabilityZone,
			}
			volumes[volume.ID] = &volume
			return &volume, nil
		},
		getVolumesDetail: func() ([]cinder.Volume, error) {
			var result []cinder.Volume
			for _, volume := range volumes {
				result = append(result, *volume)
			}
			return result, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			if volume, ok := volumes[volumeId]; ok {
				return volume, nil
			}
			return nil, errors.New("not found")
		},
		setVolumeMetadata: func(volumeId string, metadata map[string]string) (map[string]string, error) {
			if volume, ok := volumes[volumeId]; ok {
				for k, v := range metadata {
					volume.Metadata[k] = v
				}
				return volume.Metadata, nil
			}
			return nil, errors.New("not found")
		},
	}
}

func overrideCinderProvider(s *gitjujutesting.CleanupSuite, adapter *mockAdapter) {
	s.PatchValue(openstack.NewOpenstackStorage, func(*openstack.Environ) (openstack.OpenstackStorage, error) {
		return adapter, nil
	})
}

func (s *localLiveSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	c.Logf("Running live tests using openstack service test double")
	s.srv.start(c, s.cred, newFullOpenstackService)

	// Set credentials to use when bootstrapping. Must be done after
	// starting server to get the auth URL.
	s.Credential = makeCredential(s.cred)
	s.CloudEndpoint = s.cred.URL
	s.CloudRegion = s.cred.Region

	s.LiveTests.SetUpSuite(c)
	openstack.UseTestImageData(openstack.ImageMetadataStorage(s.Env), s.cred)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) { restoreFinishBootstrap() })
	overrideCinderProvider(&s.CleanupSuite, &mockAdapter{})
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
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "")
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
	env                  environs.Environ
	toolsMetadataStorage envstorage.Storage
	imageMetadataStorage envstorage.Storage
	storageAdapter       *mockAdapter
	callCtx              context.ProviderCallContext
}

func (s *localServerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) { restoreFinishBootstrap() })
	c.Logf("Running local tests")
}

func (s *localServerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.srv.start(c, s.cred, newFullOpenstackService)

	// Set credentials to use when bootstrapping. Must be done after
	// starting server to get the auth URL.
	s.Credential = makeCredential(s.cred)
	s.CloudEndpoint = s.cred.URL
	s.CloudRegion = s.cred.Region
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, jc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.TestConfig = s.TestConfig.Merge(coretesting.Attrs{
		"agent-metadata-url": containerURL + "/juju-dist-test/tools",
		"image-metadata-url": containerURL + "/juju-dist-test",
		"auth-url":           s.cred.URL,
	})
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.Tests.SetUpTest(c)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	s.env = s.Prepare(c)
	s.toolsMetadataStorage = openstack.MetadataStorage(s.env)
	// Put some fake metadata in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running can find the metadata.
	envtesting.UploadFakeTools(c, s.toolsMetadataStorage, s.env.Config().AgentStream(), s.env.Config().AgentStream())
	s.imageMetadataStorage = openstack.ImageMetadataStorage(s.env)
	openstack.UseTestImageData(s.imageMetadataStorage, s.cred)
	s.storageAdapter = makeMockAdapter()
	overrideCinderProvider(&s.CleanupSuite, s.storageAdapter)
	s.callCtx = context.NewEmptyCloudCallContext()
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

func (s *localServerSuite) openEnviron(c *gc.C, attrs coretesting.Attrs) environs.Environ {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(attrs))
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  s.CloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func (s *localServerSuite) TestBootstrap(c *gc.C) {
	// Tests uses Prepare, so destroy first.
	err := environs.Destroy(s.env.Config().Name(), s.env, s.callCtx, s.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
	s.Tests.TestBootstrap(c)
}

func (s *localServerSuite) TestStartStop(c *gc.C) {
	// Tests uses Prepare, so destroy first.
	err := environs.Destroy(s.env.Config().Name(), s.env, s.callCtx, s.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
	s.Tests.TestStartStop(c)
}

// If the bootstrap node is configured to require a public IP address,
// bootstrapping fails if an address cannot be allocated.
func (s *localServerSuite) TestBootstrapFailsWhenPublicIPError(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Neutron.RegisterControlPoint(
		"addFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("failed on purpose")
		},
	)
	defer cleanup()

	err := environs.Destroy(s.env.Config().Name(), s.env, s.callCtx, s.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)

	env := s.openEnviron(c, coretesting.Attrs{"use-floating-ip": true})
	err = bootstrapEnv(c, env)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*cannot allocate a public IP as needed(.|\n)*")
}

func (s *localServerSuite) TestAddressesWithPublicIP(c *gc.C) {
	s.assertAddressesWithPublicIP(c, constraints.Value{}, true)
}

func (s *localServerSuite) TestAddressesWithPublicIPConstraintsOverride(c *gc.C) {
	s.assertAddressesWithPublicIP(c, constraints.MustParse("allocate-public-ip=true"), false)
}

func (s *localServerSuite) assertAddressesWithPublicIP(c *gc.C, cons constraints.Value, useFloatingIP bool) {
	// Floating IP address is 10.0.0.1
	bootstrapFinished := false
	s.PatchValue(&common.FinishBootstrap, func(
		ctx environs.BootstrapContext,
		client ssh.Client,
		env environs.Environ,
		callCtx context.ProviderCallContext,
		inst instances.Instance,
		instanceConfig *instancecfg.InstanceConfig,
		_ environs.BootstrapDialOpts,
	) error {
		addr, err := inst.Addresses(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(addr, jc.SameContents, network.ProviderAddresses{
			network.NewProviderAddress("10.0.0.1", corenetwork.WithScope(corenetwork.ScopePublic)),
			network.NewProviderAddress("127.0.0.1", corenetwork.WithScope(corenetwork.ScopeMachineLocal)),
			network.NewProviderAddress("::face::000f"),
			network.NewProviderAddress("127.10.0.1", corenetwork.WithScope(corenetwork.ScopePublic)),
			network.NewProviderAddress("::dead:beef:f00d", corenetwork.WithScope(corenetwork.ScopePublic)),
		})
		bootstrapFinished = true
		return nil
	})

	env := s.openEnviron(c, coretesting.Attrs{
		"network":         "private_999",
		"use-floating-ip": useFloatingIP,
	})
	err := bootstrapEnvWithConstraints(c, env, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapFinished, jc.IsTrue)
}

func (s *localServerSuite) TestAddressesWithoutPublicIP(c *gc.C) {
	s.assertAddressesWithoutPublicIP(c, constraints.Value{}, false)
}

func (s *localServerSuite) TestAddressesWithoutPublicIPConstraintsOverride(c *gc.C) {
	s.assertAddressesWithoutPublicIP(c, constraints.MustParse("allocate-public-ip=false"), true)
}

func (s *localServerSuite) assertAddressesWithoutPublicIP(c *gc.C, cons constraints.Value, useFloatingIP bool) {
	bootstrapFinished := false
	s.PatchValue(&common.FinishBootstrap, func(
		ctx environs.BootstrapContext,
		client ssh.Client,
		env environs.Environ,
		callCtx context.ProviderCallContext,
		inst instances.Instance,
		instanceConfig *instancecfg.InstanceConfig,
		_ environs.BootstrapDialOpts,
	) error {
		addr, err := inst.Addresses(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(addr, jc.SameContents, network.ProviderAddresses{
			network.NewProviderAddress("127.0.0.1", corenetwork.WithScope(corenetwork.ScopeMachineLocal)),
			network.NewProviderAddress("::face::000f"),
			network.NewProviderAddress("127.10.0.1", corenetwork.WithScope(corenetwork.ScopePublic)),
			network.NewProviderAddress("::dead:beef:f00d", corenetwork.WithScope(corenetwork.ScopePublic)),
		})
		bootstrapFinished = true
		return nil
	})

	env := s.openEnviron(c, coretesting.Attrs{"use-floating-ip": useFloatingIP})
	err := bootstrapEnvWithConstraints(c, env, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapFinished, jc.IsTrue)
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstanceWithoutPublicIP(c *gc.C) {
	cleanup := s.srv.Neutron.RegisterControlPoint(
		"addFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("add floating IP should not have been called")
		},
	)
	defer cleanup()
	cleanup = s.srv.Nova.RegisterControlPoint(
		"addServerFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("add server floating IP should not have been called")
		},
	)
	defer cleanup()

	err := environs.Destroy(s.env.Config().Name(), s.env, s.callCtx, s.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)

	s.TestConfig["use-floating-ip"] = false
	env := s.Prepare(c)
	err = bootstrapEnv(c, env)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, "100")
	err = env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	// Ensure amd64 tools are available, to ensure an amd64 image.
	env := s.ensureAMDImages(c)
	err := bootstrapEnv(c, env)
	c.Assert(err, jc.ErrorIsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, env, s.callCtx, s.ControllerUUID, "100", constraints.MustParse("mem=1024 arch=amd64"))
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(2048))
	c.Check(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(hc.CpuPower, gc.IsNil)
}

func (s *localServerSuite) TestInstanceName(c *gc.C) {
	inst, _ := testing.AssertStartInstance(c, s.env, s.callCtx, s.ControllerUUID, "100")
	serverDetail := openstack.InstanceServerDetail(inst)
	envName := s.env.Config().Name()
	c.Assert(serverDetail.Name, gc.Matches, "juju-06f00d-"+envName+"-100")
}

func (s *localServerSuite) TestStartInstanceNetwork(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that corresponds to a neutron test service network
		"network": "net",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _ := testing.AssertStartInstance(c, s.env, s.callCtx, s.ControllerUUID, "100")
	err = s.env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceMultiNetworkFound(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `multiple networks with label .*
	To resolve this error, set a value for "network" in model-config or model-defaults;
	or supply it via --config when creating a new model`)
}

func (s *localServerSuite) TestStartInstanceExternalNetwork(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that corresponds to a neutron test service external network
		"external-network": "ext-net",
		"use-floating-ip":  true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _ := testing.AssertStartInstance(c, s.env, s.callCtx, s.ControllerUUID, "100")
	err = s.env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkUnknownLabel(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that has no related network in the neutron test service
		"network": "no-network-with-this-label",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "no networks exist with label .*")
}

func (s *localServerSuite) TestStartInstanceExternalNetworkUnknownLabel(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that has no related network in the neutron test service
		"external-network": "no-network-with-this-label",
		"use-floating-ip":  true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkUnknownId(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A valid UUID but no related network in the nova test service
		"network": "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "failed to get network detail\n"+
		"caused by: "+
		"Resource at http://.*/networks/.* not found\n"+
		"caused by: "+
		"request \\(http://.*/networks/.*\\) returned unexpected status: "+
		"404; error info: .*itemNotFound.*")
}

func (s *localServerSuite) TestStartInstanceNoNetworksNetworkNotSetNoError(c *gc.C) {
	// Modify the Openstack service that is created by default,
	// to clear the networks.
	model := neutronmodel.New()
	for _, net := range model.AllNetworks() {
		_ = model.RemoveNetwork(net.Id)
	}
	s.srv.OpenstackSvc.Neutron.AddNeutronModel(model)
	s.srv.OpenstackSvc.Nova.AddNeutronModel(model)

	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceOneNetworkNetworkNotSetNoError(c *gc.C) {
	// Modify the Openstack service that is created by default,
	// to remove all but 1 internal network.
	model := neutronmodel.New()
	var foundOne bool
	for _, net := range model.AllNetworks() {
		if !net.External {
			if !foundOne {
				foundOne = true
				continue
			}
			err := model.RemoveNetwork(net.Id)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
	s.srv.OpenstackSvc.Neutron.AddNeutronModel(model)
	s.srv.OpenstackSvc.Nova.AddNeutronModel(model)

	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworksDifferentAZ(c *gc.C) {
	// If both the network and external-network config values are
	// specified, there is not check for them being on different
	// network availability zones when use-floating-ips specified.
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network":          "net",     // az = nova
		"external-network": "ext-net", // az = test-available
		"use-floating-ip":  true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworksEmptyAZ(c *gc.C) {
	// Modify the Openstack service that is created by default,
	// to clear the networks.
	model := neutronmodel.New()
	for _, net := range model.AllNetworks() {
		_ = model.RemoveNetwork(net.Id)
	}

	// Add 2 networks to the Openstack service, one private,
	// one external without availability zones.  LP: 1891227.
	err := model.AddNetwork(neutron.NetworkV2{
		Id:        "1",
		Name:      "no-az-net",
		SubnetIds: []string{"sub-net"},
		External:  false,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = model.AddNetwork(neutron.NetworkV2{
		Id:        "2",
		Name:      "ext-no-az-net",
		SubnetIds: []string{"ext-sub-net"},
		External:  true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.srv.OpenstackSvc.Neutron.AddNeutronModel(model)
	s.srv.OpenstackSvc.Nova.AddNeutronModel(model)

	// Set floating ip to ensure we try to find the external
	// network.
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network":         "no-az-net", // az = nova
		"use-floating-ip": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkNoExternalNetInAZ(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network":         "net", // az = nova
		"use-floating-ip": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(err, gc.ErrorMatches, "cannot allocate a public IP as needed: could not find an external network in availability zone.*")
}

func (s *localServerSuite) TestStartInstancePortSecurityEnabled(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "net",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(err, jc.ErrorIsNil)
	novaClient := openstack.GetNovaClient(s.env)
	detail, err := novaClient.GetServer(string(inst.Id()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(detail.Groups, gc.NotNil)
}

func (s *localServerSuite) TestStartInstancePortSecurityDisabled(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "net-disabled",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(err, jc.ErrorIsNil)
	novaClient := openstack.GetNovaClient(s.env)
	detail, err := novaClient.GetServer(string(inst.Id()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(detail.Groups, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceGetServerFail(c *gc.C) {
	// Force an error in waitForActiveServerDetails
	cleanup := s.srv.Nova.RegisterControlPoint(
		"server",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("GetServer failed on purpose")
		},
	)
	defer cleanup()
	inst, _, _, err := testing.StartInstance(s.env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot run instance: "+
		"request \\(.*/servers\\) returned unexpected status: "+
		"500; error info: .*GetServer failed on purpose")
	c.Assert(err, jc.Satisfies, environs.IsAvailabilityZoneIndependent)
}

func (s *localServerSuite) TestStartInstanceWaitForActiveDetails(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})

	s.srv.Nova.SetServerStatus(nova.StatusBuild)
	defer s.srv.Nova.SetServerStatus("")

	// Make time advance in zero time
	clk := testclock.NewClock(time.Time{})
	clock := testclock.AutoAdvancingClock{Clock: clk, Advance: clk.Advance}
	env.(*openstack.Environ).SetClock(&clock)

	inst, _, _, err := testing.StartInstance(env, s.callCtx, s.ControllerUUID, "100")
	c.Check(inst, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot run instance: max duration exceeded: instance .* has status BUILD")

	// Ensure that the started instance got terminated.
	insts, err := env.AllInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 0, gc.Commentf("expected launched instance to be terminated if stuck in BUILD state"))
}

func assertSecurityGroups(c *gc.C, env environs.Environ, expected []string) {
	neutronClient := openstack.GetNeutronClient(env)
	groups, err := neutronClient.ListSecurityGroupsV2()
	c.Assert(err, jc.ErrorIsNil)
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

type portAssertion struct {
	SubnetIDs  []string
	NamePrefix string
}

func assertPorts(c *gc.C, env environs.Environ, expected []portAssertion) {
	neutronClient := openstack.GetNeutronClient(env)
	ports, err := neutronClient.ListPortsV2()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, len(expected))
	for k, port := range ports {
		c.Assert(port.Name, jc.HasPrefix, expected[k].NamePrefix)
		c.Assert(port.FixedIPs, gc.HasLen, len(expected[k].SubnetIDs))
		for i, ip := range port.FixedIPs {
			c.Assert(ip.SubnetID, gc.Equals, expected[k].SubnetIDs[i])
		}
	}
}

func assertInstanceIds(c *gc.C, env environs.Environ, callCtx context.ProviderCallContext, expected ...instance.Id) {
	allInstances, err := env.AllRunningInstances(callCtx)
	c.Assert(err, jc.ErrorIsNil)
	instIds := make([]instance.Id, len(allInstances))
	for i, inst := range allInstances {
		instIds[i] = inst.Id()
	}
	c.Assert(instIds, jc.SameContents, expected)
}

func (s *localServerSuite) TestStopInstance(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})
	instanceName := "100"
	inst, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, instanceName)
	// Openstack now has three security groups for the server, the default
	// group, one group for the entire environment, and another for the
	// new instance.
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, instanceName),
	}
	assertSecurityGroups(c, env, allSecurityGroups)
	err := env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
	// The security group for this instance is now removed.
	assertSecurityGroups(c, env, []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
	})
}

// Due to bug #1300755 it can happen that the security group intended for
// an instance is also used as the common security group of another
// environment. If this is the case, the attempt to delete the instance's
// security group fails but StopInstance succeeds.
func (s *localServerSuite) TestStopInstanceSecurityGroupNotDeleted(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	// Force an error when a security group is deleted.
	cleanup := s.srv.Neutron.RegisterControlPoint(
		"removeSecurityGroup",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("failed on purpose")
		},
	)
	defer cleanup()
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})
	instanceName := "100"
	inst, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, instanceName)
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, instanceName),
	}
	assertSecurityGroups(c, env, allSecurityGroups)

	// Make time advance in zero time
	clk := testclock.NewClock(time.Time{})
	clock := testclock.AutoAdvancingClock{Clock: clk, Advance: clk.Advance}
	env.(*openstack.Environ).SetClock(&clock)

	err := env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
	assertSecurityGroups(c, env, allSecurityGroups)
}

func (s *localServerSuite) TestDestroyEnvironmentDeletesSecurityGroupsFWModeInstance(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})
	instanceName := "100"
	testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, instanceName)
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, instanceName),
	}
	assertSecurityGroups(c, env, allSecurityGroups)
	err := env.Destroy(s.callCtx)
	c.Check(err, jc.ErrorIsNil)
	assertSecurityGroups(c, env, []string{"default"})
}

func (s *localServerSuite) TestDestroyEnvironmentDeletesSecurityGroupsFWModeGlobal(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwGlobal})
	instanceName := "100"
	testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, instanceName)
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-global", s.ControllerUUID, modelUUID),
	}
	assertSecurityGroups(c, env, allSecurityGroups)
	err := env.Destroy(s.callCtx)
	c.Check(err, jc.ErrorIsNil)
	assertSecurityGroups(c, env, []string{"default"})
}

func (s *localServerSuite) TestDestroyController(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"uuid": utils.MustNewUUID().String()})
	controllerEnv := s.env

	controllerInstanceName := "100"
	testing.AssertStartInstance(c, controllerEnv, s.callCtx, s.ControllerUUID, controllerInstanceName)
	hostedModelInstanceName := "200"
	testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, hostedModelInstanceName)
	modelUUID := env.Config().UUID()
	allControllerSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, controllerEnv.Config().UUID()),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, controllerEnv.Config().UUID(), controllerInstanceName),
	}
	allHostedModelSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, hostedModelInstanceName),
	}
	assertSecurityGroups(c, controllerEnv, append(
		allControllerSecurityGroups, allHostedModelSecurityGroups...,
	))

	err := controllerEnv.DestroyController(s.callCtx, s.ControllerUUID)
	c.Check(err, jc.ErrorIsNil)
	assertSecurityGroups(c, controllerEnv, []string{"default"})
	assertInstanceIds(c, env, s.callCtx)
	assertInstanceIds(c, controllerEnv, s.callCtx)
}

func (s *localServerSuite) TestDestroyHostedModel(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"uuid": utils.MustNewUUID().String()})
	controllerEnv := s.env

	controllerInstanceName := "100"
	controllerInstance, _ := testing.AssertStartInstance(c, controllerEnv, s.callCtx, s.ControllerUUID, controllerInstanceName)
	hostedModelInstanceName := "200"
	testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, hostedModelInstanceName)
	modelUUID := env.Config().UUID()
	allControllerSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, controllerEnv.Config().UUID()),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, controllerEnv.Config().UUID(), controllerInstanceName),
	}
	allHostedModelSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, hostedModelInstanceName),
	}
	assertSecurityGroups(c, controllerEnv, append(
		allControllerSecurityGroups, allHostedModelSecurityGroups...,
	))

	err := env.Destroy(s.callCtx)
	c.Check(err, jc.ErrorIsNil)
	assertSecurityGroups(c, controllerEnv, allControllerSecurityGroups)
	assertInstanceIds(c, env, s.callCtx)
	assertInstanceIds(c, controllerEnv, s.callCtx, controllerInstance.Id())
}

func (s *localServerSuite) TestDestroyControllerSpaceConstraints(c *gc.C) {
	uuid := utils.MustNewUUID().String()
	env := s.openEnviron(c, coretesting.Attrs{"uuid": uuid})
	controllerEnv := s.env

	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "zone-0",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	controllerInstanceName := "100"
	params := environs.StartInstanceParams{
		ControllerUUID:   s.ControllerUUID,
		AvailabilityZone: "zone-0",
		Constraints:      constraints.MustParse("spaces=space-1 zones=zone-0"),
		SubnetsToZones: []map[corenetwork.Id][]string{
			{
				"xxx-yyy-zzz": {"zone-0"},
			},
		},
	}
	_, err := testing.StartInstanceWithParams(env, s.callCtx, controllerInstanceName, params)
	c.Assert(err, jc.ErrorIsNil)
	assertPorts(c, env, []portAssertion{
		{NamePrefix: fmt.Sprintf("juju-%s-", uuid), SubnetIDs: []string{"xxx-yyy-zzz"}},
	})

	// The openstack runtime would assign a device_id to a port when it's
	// assigned to an instance. To ensure that all ports are correctly removed
	// when destroying and so we can exercise all the code paths we have to
	// replicate that piece of code.
	// When moving to mocking of providers, this shouldn't be need or required.
	s.assignDeviceIdToPort(c, "1", "1")

	err = controllerEnv.DestroyController(s.callCtx, s.ControllerUUID)
	c.Check(err, jc.ErrorIsNil)
	assertSecurityGroups(c, controllerEnv, []string{"default"})
	assertInstanceIds(c, env, s.callCtx)
	assertInstanceIds(c, controllerEnv, s.callCtx)
	assertPorts(c, env, []portAssertion{})
}

func (s *localServerSuite) assignDeviceIdToPort(c *gc.C, portId, deviceId string) {
	err := s.srv.Nova.AddOSInterface(deviceId, nova.OSInterface{
		FixedIPs: []nova.PortFixedIP{
			{
				IPAddress: "10.0.0.1",
			},
		},
		IPAddress: "10.0.0.1",
	})
	c.Assert(err, jc.ErrorIsNil)

	model := s.srv.Neutron.NeutronModel()
	port, err := model.Port("1")
	c.Assert(err, jc.ErrorIsNil)
	err = model.RemovePort("1")
	c.Assert(err, jc.ErrorIsNil)
	port.DeviceId = "1"
	err = model.AddPort(*port)
	c.Assert(err, jc.ErrorIsNil)
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
	inst, _ := testing.AssertStartInstance(c, s.env, s.callCtx, s.ControllerUUID, "100")
	c.Assert(inst.Status(s.callCtx).Status, gc.Equals, status.Running)
	err := s.env.StopInstances(s.callCtx, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestAllRunningInstancesFloatingIP(c *gc.C) {
	env := s.openEnviron(c, coretesting.Attrs{
		"network":         "private_999",
		"use-floating-ip": true,
	})

	inst0, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, "100")
	inst1, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, "101")
	defer func() {
		err := env.StopInstances(s.callCtx, inst0.Id(), inst1.Id())
		c.Assert(err, jc.ErrorIsNil)
	}()

	allInstances, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	for _, inst := range allInstances {
		c.Assert(*openstack.InstanceFloatingIP(inst), gc.Equals, fmt.Sprintf("10.0.0.%v", inst.Id()))
	}
}

func (s *localServerSuite) assertInstancesGathering(c *gc.C, withFloatingIP bool) {
	env := s.openEnviron(c, coretesting.Attrs{"use-floating-ip": withFloatingIP})

	inst0, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, "100")
	id0 := inst0.Id()
	inst1, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, "101")
	id1 := inst1.Id()
	defer func() {
		err := env.StopInstances(s.callCtx, inst0.Id(), inst1.Id())
		c.Assert(err, jc.ErrorIsNil)
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
		insts, err := env.Instances(s.callCtx, ids)
		c.Assert(err, gc.Equals, test.err)
		if err == environs.ErrNoInstances {
			c.Assert(insts, gc.HasLen, 0)
		} else {
			c.Assert(insts, gc.HasLen, len(test.ids))
		}
		for j, inst := range insts {
			if ids[j] != "" {
				c.Assert(inst.Id(), gc.Equals, ids[j])
				if withFloatingIP {
					c.Assert(*openstack.InstanceFloatingIP(inst), gc.Equals, fmt.Sprintf("10.0.0.%v", inst.Id()))
				} else {
					c.Assert(openstack.InstanceFloatingIP(inst), gc.IsNil)
				}
			} else {
				c.Assert(inst, gc.IsNil)
			}
		}
	}
}

func (s *localServerSuite) TestInstancesGathering(c *gc.C) {
	s.assertInstancesGathering(c, false)
}

func (s *localServerSuite) TestInstancesGatheringWithFloatingIP(c *gc.C) {
	s.assertInstancesGathering(c, true)
}

func (s *localServerSuite) TestInstancesShutoffSuspended(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Nova.RegisterControlPoint(
		"addServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			details := args[0].(*nova.ServerDetail)
			switch {
			case strings.HasSuffix(details.Name, "-100"):
				details.Status = nova.StatusShutoff
			case strings.HasSuffix(details.Name, "-101"):
				details.Status = nova.StatusSuspended
			default:
				c.Fatalf("unexpected instance details: %#v", details)
			}
			return nil
		},
	)
	defer cleanup()
	stateInst1, _ := testing.AssertStartInstance(c, s.env, s.callCtx, s.ControllerUUID, "100")
	stateInst2, _ := testing.AssertStartInstance(c, s.env, s.callCtx, s.ControllerUUID, "101")
	defer func() {
		err := s.env.StopInstances(s.callCtx, stateInst1.Id(), stateInst2.Id())
		c.Assert(err, jc.ErrorIsNil)
	}()

	twoInstances, err := s.env.Instances(s.callCtx, []instance.Id{stateInst1.Id(), stateInst2.Id()})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(twoInstances, gc.HasLen, 2)
	c.Assert(twoInstances[0].Status(s.callCtx).Message, gc.Equals, nova.StatusShutoff)
	c.Assert(twoInstances[1].Status(s.callCtx).Message, gc.Equals, nova.StatusSuspended)
}

func (s *localServerSuite) TestInstancesErrorResponse(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Nova.RegisterControlPoint(
		"server",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("strange error not instance")
		},
	)
	defer cleanup()

	oneInstance, err := s.env.Instances(s.callCtx, []instance.Id{"1"})
	c.Check(oneInstance, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "(?s).*strange error not instance.*")
}

func (s *localServerSuite) TestInstancesMultiErrorResponse(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Nova.RegisterControlPoint(
		"matchServers",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("strange error no instances")
		},
	)
	defer cleanup()

	twoInstances, err := s.env.Instances(s.callCtx, []instance.Id{"1", "2"})
	c.Check(twoInstances, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "(?s).*strange error no instances.*")
}

// TODO (wallyworld) - this test was copied from the ec2 provider.
// It should be moved to environs.jujutests.Tests.
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	// Check that ControllerInstances returns the ID of the bootstrap machine.
	ids, err := s.env.ControllerInstances(s.callCtx, s.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, gc.HasLen, 1)

	allInstances, err := s.env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstances, gc.HasLen, 1)
	c.Check(allInstances[0].Id(), gc.Equals, ids[0])

	addresses, err := allInstances[0].Addresses(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	// TODO(wallyworld) - 2013-03-01 bug=1137005
	// The nova test double needs to be updated to support retrieving instance userData.
	// Until then, we can't check the cloud init script was generated correctly.
	// When we can, we should also check cloudinit for a non-manager node (as in the
	// ec2 tests).
}

func (s *localServerSuite) assertGetImageMetadataSources(c *gc.C, stream, officialSourcePath string) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	// Create a config that matches s.TestConfig but with the specified stream.
	attrs := coretesting.Attrs{}
	if stream != "" {
		attrs = coretesting.Attrs{"image-stream": stream}
	}
	env := s.openEnviron(c, attrs)

	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 3)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		imageURL, err := source.URL("")
		c.Assert(err, jc.ErrorIsNil)
		urls[i] = imageURL
	}
	// The image-metadata-url ends with "/juju-dist-test/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist-test/"), jc.IsTrue)
	// The product-streams URL ends with "/imagemetadata".
	c.Check(strings.HasSuffix(urls[1], "/imagemetadata/"), jc.IsTrue)
	c.Assert(urls[2], gc.Equals, fmt.Sprintf("http://cloud-images.ubuntu.com/%s/", officialSourcePath))
}

func (s *localServerSuite) TestGetImageMetadataSources(c *gc.C) {
	s.assertGetImageMetadataSources(c, "", "releases")
	s.assertGetImageMetadataSources(c, "released", "releases")
	s.assertGetImageMetadataSources(c, "daily", "daily")
}

func (s *localServerSuite) TestGetImageMetadataSourcesNoProductStreams(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.PatchValue(openstack.MakeServiceURL, func(client.AuthenticatingClient, string, string, []string) (string, error) {
		return "", errors.New("cannae do it captain")
	})
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 2)

	// Check that data sources are in the right order
	c.Check(sources[0].Description(), gc.Equals, "image-metadata-url")
	c.Check(sources[1].Description(), gc.Equals, "default ubuntu cloud images")
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.PatchValue(&tools.DefaultBaseURL, "")

	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	sources, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 2)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		metadataURL, err := source.URL("")
		c.Assert(err, jc.ErrorIsNil)
		urls[i] = metadataURL
	}
	// The agent-metadata-url ends with "/juju-dist-test/tools/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist-test/tools/"), jc.IsTrue)
	// Check that the URL from keystone parses.
	_, err = url.Parse(urls[1])
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestSupportsNetworking(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	_, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)
}

func (s *localServerSuite) prepareNetworkingEnviron(c *gc.C, cfg *config.Config) environs.NetworkingEnviron {
	env := s.Open(c, stdcontext.TODO(), cfg)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
	return netenv
}

func (s *localServerSuite) TestSubnetsFindAll(c *gc.C) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	// the environ is opened with network:"private_999" which maps to network id "999"
	obtainedSubnets, err := env.Subnets(s.callCtx, "", []network.Id{})
	c.Assert(err, jc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackSubnets, err := neutronClient.ListSubnetsV2()
	c.Assert(err, jc.ErrorIsNil)

	obtainedSubnetMap := make(map[network.Id]network.SubnetInfo)
	for _, sub := range obtainedSubnets {
		obtainedSubnetMap[sub.ProviderId] = sub
	}

	expectedSubnetMap := make(map[network.Id]network.SubnetInfo)
	for _, subnet := range openstackSubnets {
		if subnet.NetworkId != "999" {
			continue
		}
		net, err := neutronClient.GetNetworkV2(subnet.NetworkId)
		c.Assert(err, jc.ErrorIsNil)
		expectedSubnetMap[network.Id(subnet.Id)] = network.SubnetInfo{
			CIDR:              subnet.Cidr,
			ProviderId:        network.Id(subnet.Id),
			VLANTag:           0,
			AvailabilityZones: net.AvailabilityZones,
			ProviderSpaceId:   "",
		}
	}

	c.Check(obtainedSubnetMap, jc.DeepEquals, expectedSubnetMap)
}

func (s *localServerSuite) TestSubnetsFindAllWithExternal(c *gc.C) {
	cfg := s.env.Config()
	cfg, err := cfg.Apply(map[string]interface{}{"external-network": "ext-net"})
	c.Assert(err, jc.ErrorIsNil)
	env := s.prepareNetworkingEnviron(c, cfg)
	// private_999 is the internal network, 998 is the external network
	// the environ is opened with network:"private_999" which maps to network id "999"
	obtainedSubnets, err := env.Subnets(s.callCtx, "", []network.Id{})
	c.Assert(err, jc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackSubnets, err := neutronClient.ListSubnetsV2()
	c.Assert(err, jc.ErrorIsNil)

	obtainedSubnetMap := make(map[network.Id]network.SubnetInfo)
	for _, sub := range obtainedSubnets {
		obtainedSubnetMap[sub.ProviderId] = sub
	}

	expectedSubnetMap := make(map[network.Id]network.SubnetInfo)
	for _, subnets := range openstackSubnets {
		if subnets.NetworkId != "999" && subnets.NetworkId != "998" {
			continue
		}
		net, err := neutronClient.GetNetworkV2(subnets.NetworkId)
		c.Assert(err, jc.ErrorIsNil)
		expectedSubnetMap[network.Id(subnets.Id)] = network.SubnetInfo{
			CIDR:              subnets.Cidr,
			ProviderId:        network.Id(subnets.Id),
			VLANTag:           0,
			AvailabilityZones: net.AvailabilityZones,
			ProviderSpaceId:   "",
		}
	}

	c.Check(obtainedSubnetMap, jc.DeepEquals, expectedSubnetMap)
}

func (s *localServerSuite) TestFindNetworksInternal(c *gc.C) {
	s.testFindNetworks(c, true)
}

func (s *localServerSuite) TestFindNetworksExternal(c *gc.C) {
	s.testFindNetworks(c, false)
}

func (s *localServerSuite) testFindNetworks(c *gc.C, internal bool) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	obtainedNetworks, err := openstack.FindNetworks(env, internal)
	c.Assert(err, jc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackNetworks, err := neutronClient.ListNetworksV2()
	c.Assert(err, jc.ErrorIsNil)

	expectedNetworks := set.NewStrings()
	for _, oNet := range openstackNetworks {
		if oNet.External == internal {
			continue
		}
		expectedNetworks.Add(oNet.Name)
	}

	c.Check(obtainedNetworks.Values(), jc.SameContents, expectedNetworks.Values())

}

func (s *localServerSuite) TestSubnetsWithMissingSubnet(c *gc.C) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	subnets, err := env.Subnets(s.callCtx, "", []network.Id{"missing"})
	c.Assert(err, gc.ErrorMatches, `failed to find the following subnet ids: \[missing\]`)
	c.Assert(subnets, gc.HasLen, 0)
}

func (s *localServerSuite) TestSuperSubnets(c *gc.C) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	obtainedSubnets, err := env.SuperSubnets(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackSubnets, err := neutronClient.ListSubnetsV2()
	c.Assert(err, jc.ErrorIsNil)

	expectedSubnets := make([]string, 0, len(openstackSubnets))
	for _, subnets := range openstackSubnets {
		if subnets.NetworkId != "999" {
			continue
		}
		expectedSubnets = append(expectedSubnets, subnets.Cidr)
	}
	sort.Strings(obtainedSubnets)
	sort.Strings(expectedSubnets)
	c.Check(obtainedSubnets, jc.DeepEquals, expectedSubnets)
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *gc.C) {
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "")
	env := s.Open(c, stdcontext.TODO(), s.env.Config())

	// An error occurs if no suitable image is found.
	_, err := openstack.FindInstanceSpec(env, "saucy", "amd64", "mem=1G", nil)
	c.Assert(err, gc.ErrorMatches, `no metadata for "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestConstraintsValidator(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	validator, err := env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 virt-type=lxd")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power"})
}

func (s *localServerSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	validator, err := env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")

	cons = constraints.MustParse("virt-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("invalid constraint value: virt-type=foo\nvalid values are: [kvm lxd]"))
}

func (s *localServerSuite) TestConstraintsMerge(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	validator, err := env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	consA := constraints.MustParse("arch=amd64 mem=1G root-disk=10G")
	consB := constraints.MustParse("instance-type=m1.small")
	cons, err := validator.Merge(consA, consB)
	c.Assert(err, jc.ErrorIsNil)
	// NOTE: root-disk and instance-type constraints are checked by PrecheckInstance.
	c.Assert(cons, gc.DeepEquals, constraints.MustParse("arch=amd64 instance-type=m1.small root-disk=10G"))
}

func (s *localServerSuite) TestFindImageInstanceConstraint(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:   "image-id",
		Arch: "amd64",
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTS(), "amd64", "instance-type=m1.tiny",
		imageMetadata,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec.InstanceType.Name, gc.Equals, "m1.tiny")
}

func (s *localServerSuite) TestFindInstanceImageConstraintHypervisor(c *gc.C) {
	testVirtType := "qemu"
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:       "image-id",
		Arch:     "amd64",
		VirtType: testVirtType,
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTS(), "amd64", "virt-type="+testVirtType,
		imageMetadata,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec.InstanceType.VirtType, gc.NotNil)
	c.Assert(*spec.InstanceType.VirtType, gc.Equals, testVirtType)
	c.Assert(spec.InstanceType.Name, gc.Equals, "m1.small")
}

func (s *localServerSuite) TestFindInstanceImageWithHypervisorNoConstraint(c *gc.C) {
	testVirtType := "qemu"
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:       "image-id",
		Arch:     "amd64",
		VirtType: testVirtType,
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTS(), "amd64", "",
		imageMetadata,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec.InstanceType.VirtType, gc.NotNil)
	c.Assert(*spec.InstanceType.VirtType, gc.Equals, testVirtType)
	c.Assert(spec.InstanceType.Name, gc.Equals, "m1.small")
}

func (s *localServerSuite) TestFindInstanceNoConstraint(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:   "image-id",
		Arch: "amd64",
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTS(), "amd64", "",
		imageMetadata,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec.InstanceType.VirtType, gc.IsNil)
	c.Assert(spec.InstanceType.Name, gc.Equals, "m1.small")
}

func (s *localServerSuite) TestFindImageInvalidInstanceConstraint(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:   "image-id",
		Arch: "amd64",
	}}
	_, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTS(), "amd64", "instance-type=m1.large",
		imageMetadata,
	)
	c.Assert(err, gc.ErrorMatches, `no instance types in some-region matching constraints "instance-type=m1.large"`)
}

func (s *localServerSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	cons := constraints.MustParse("instance-type=m1.small")
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	cons := constraints.MustParse("instance-type=m1.large")
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Constraints: cons})
	c.Assert(err, gc.ErrorMatches, `invalid Openstack flavour "m1.large" specified`)
}

func (s *localServerSuite) TestPrecheckInstanceInvalidRootDiskConstraint(c *gc.C) {
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	cons := constraints.MustParse("instance-type=m1.small root-disk=10G")
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Constraints: cons})
	c.Assert(err, gc.ErrorMatches, `constraint root-disk cannot be specified with instance-type unless constraint root-disk-source=volume`)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	placement := "zone=test-available"
	err := s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Placement: placement})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *gc.C) {
	placement := "zone=test-unavailable"
	err := s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Placement: placement})
	c.Assert(err, gc.ErrorMatches, `zone "test-unavailable" is unavailable`)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	placement := "zone=test-unknown"
	err := s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Placement: placement})
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unknown" not valid`)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZonesUnsupported(c *gc.C) {
	s.srv.Nova.SetAvailabilityZones() // no availability zone support
	placement := "zone=test-unknown"
	err := s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.DefaultSupportedLTS(), Placement: placement})
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *localServerSuite) TestPrecheckInstanceVolumeAvailZonesNoPlacement(c *gc.C) {
	s.testPrecheckInstanceVolumeAvailZones(c, "")
}

func (s *localServerSuite) TestPrecheckInstanceVolumeAvailZonesSameZonePlacement(c *gc.C) {
	s.testPrecheckInstanceVolumeAvailZones(c, "zone=az1")
}

func (s *localServerSuite) testPrecheckInstanceVolumeAvailZones(c *gc.C, placement string) {
	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	_, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{
		Series:            jujuversion.DefaultSupportedLTS(),
		Placement:         placement,
		VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZonesConflictsVolume(c *gc.C) {
	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
		nova.AvailabilityZone{
			Name: "az2",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	_, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{
		Series:            jujuversion.DefaultSupportedLTS(),
		Placement:         "zone=az2",
		VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
	})
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=az2": cannot create instance in zone "az2", as this will prevent attaching the requested disks in zone "az1"`)
}

func (s *localServerSuite) TestDeriveAvailabilityZones(c *gc.C) {
	placement := "zone=test-available"
	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{
			Placement: placement,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"test-available"})
}

func (s *localServerSuite) TestDeriveAvailabilityZonesUnavailable(c *gc.C) {
	placement := "zone=test-unavailable"
	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{
			Placement: placement,
		})
	c.Assert(err, gc.ErrorMatches, `zone "test-unavailable" is unavailable`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *localServerSuite) TestDeriveAvailabilityZonesUnknown(c *gc.C) {
	placement := "zone=test-unknown"
	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{
			Placement: placement,
		})
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unknown" not valid`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *localServerSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *gc.C) {
	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: false,
			},
		},
		nova.AvailabilityZone{
			Name: "az2",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	_, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az2",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{
			VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"az2"})
}

func (s *localServerSuite) TestDeriveAvailabilityZonesConflictsVolume(c *gc.C) {
	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
		nova.AvailabilityZone{
			Name: "az2",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	_, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{
			Placement:         "zone=az2",
			VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
		})
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=az2": cannot create instance in zone "az2", as this will prevent attaching the requested disks in zone "az1"`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	params, err := env.(simplestreams.ImageMetadataValidator).ImageMetadataLookupParams("some-region")
	c.Assert(err, jc.ErrorIsNil)
	params.Sources, err = environs.ImageMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	params.Release = "raring"
	imageIDs, _, err := imagemetadata.ValidateImageMetadata(ss, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageIDs, jc.SameContents, []string{"id-y"})
}

func (s *localServerSuite) TestImageMetadataSourceOrder(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	src := func(env environs.Environ) (simplestreams.DataSource, error) {
		return ss.NewDataSource(simplestreams.Config{
			Description:          "my datasource",
			BaseURL:              "bar",
			HostnameVerification: false,
			Priority:             simplestreams.CUSTOM_CLOUD_DATA}), nil
	}
	environs.RegisterUserImageDataSourceFunc("my func", src)
	env := s.Open(c, stdcontext.TODO(), s.env.Config())
	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	var sourceIds []string
	for _, s := range sources {
		sourceIds = append(sourceIds, s.Description())
	}
	c.Assert(sourceIds, jc.DeepEquals, []string{
		"image-metadata-url", "my datasource", "keystone catalog", "default ubuntu cloud images"})
}

// To compare found and expected SecurityGroupRules, convert the rules to RuleInfo, minus
// details we can't predict such as id.
func ruleToRuleInfo(rules []neutron.SecurityGroupRuleV2) []neutron.RuleInfoV2 {
	ruleInfo := make([]neutron.RuleInfoV2, 0, len(rules))
	for _, r := range rules {
		ri := neutron.RuleInfoV2{
			Direction:      r.Direction,
			EthernetType:   r.EthernetType,
			RemoteIPPrefix: r.RemoteIPPrefix,
		}
		if r.IPProtocol != nil {
			ri.IPProtocol = *r.IPProtocol
		}
		if r.PortRangeMax != nil {
			ri.PortRangeMax = *r.PortRangeMax
		}
		if r.PortRangeMin != nil {
			ri.PortRangeMin = *r.PortRangeMin
		}
		ruleInfo = append(ruleInfo, ri)
	}
	return ruleInfo
}

// TestEnsureGroup checks that when creating a duplicate security group, the existing group is
// returned and the existing rules have been left as is.
func (s *localServerSuite) TestEnsureGroup(c *gc.C) {
	rule := []neutron.RuleInfoV2{
		{
			Direction:    "ingress",
			IPProtocol:   "tcp",
			PortRangeMin: 22,
			PortRangeMax: 22,
			EthernetType: "IPv4",
		},
	}

	group, err := openstack.EnsureGroup(s.env, s.callCtx, "test group", rule)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(group.Name, gc.Equals, "test group")

	// Rules created by Neutron when a new Security Group is created
	defaultRules := []neutron.RuleInfoV2{
		{
			Direction:    "egress",
			EthernetType: "IPv4",
		},
		{
			Direction:    "egress",
			EthernetType: "IPv6",
		},
	}
	expectedRules := append(defaultRules, rule[0])
	obtainedRules := ruleToRuleInfo(group.Rules)
	c.Check(obtainedRules, jc.SameContents, expectedRules)
	id := group.Id

	// Do it again and check that the existing group is returned
	// and updated.
	rules := []neutron.RuleInfoV2{
		{
			Direction:    "ingress",
			IPProtocol:   "tcp",
			PortRangeMin: 22,
			PortRangeMax: 22,
			EthernetType: "IPv4",
		},
		{
			Direction:    "ingress",
			IPProtocol:   "icmp",
			EthernetType: "IPv6",
		},
	}
	group, err = openstack.EnsureGroup(s.env, s.callCtx, "test group", rules)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(group.Id, gc.Equals, id)
	c.Assert(group.Name, gc.Equals, "test group")
	c.Check(len(group.Rules), gc.Equals, 4)
	expectedRules = append(defaultRules, rules...)
	obtainedRulesSecondTime := ruleToRuleInfo(group.Rules)
	c.Check(obtainedRulesSecondTime, jc.SameContents, expectedRules)

	// 3rd time with same name, should be back to the original now
	group, err = openstack.EnsureGroup(s.env, s.callCtx, "test group", rule)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(group.Id, gc.Equals, id)
	c.Assert(group.Name, gc.Equals, "test group")
	expectedRules = append(defaultRules, rule[0])
	obtainedRulesThirdTime := ruleToRuleInfo(group.Rules)
	c.Check(obtainedRulesThirdTime, jc.SameContents, expectedRules)
	c.Check(obtainedRulesThirdTime, jc.SameContents, obtainedRules)
}

// TestMatchingGroup checks that you receive the group you expected.  matchingGroup()
// is used by the firewaller when opening and closing ports.  Unit test in response to bug 1675799.
func (s *localServerSuite) TestMatchingGroup(c *gc.C) {
	rule := []neutron.RuleInfoV2{
		{
			Direction:    "ingress",
			IPProtocol:   "tcp",
			PortRangeMin: 22,
			PortRangeMax: 22,
			EthernetType: "IPv4",
		},
	}

	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)
	group1, err := openstack.EnsureGroup(s.env, s.callCtx,
		openstack.MachineGroupName(s.env, s.ControllerUUID, "1"), rule)
	c.Assert(err, jc.ErrorIsNil)
	group2, err := openstack.EnsureGroup(s.env, s.callCtx,
		openstack.MachineGroupName(s.env, s.ControllerUUID, "2"), rule)
	c.Assert(err, jc.ErrorIsNil)
	_, err = openstack.EnsureGroup(s.env, s.callCtx, openstack.MachineGroupName(s.env, s.ControllerUUID, "11"), rule)
	c.Assert(err, jc.ErrorIsNil)
	_, err = openstack.EnsureGroup(s.env, s.callCtx, openstack.MachineGroupName(s.env, s.ControllerUUID, "12"), rule)
	c.Assert(err, jc.ErrorIsNil)

	machineNameRegexp := openstack.MachineGroupRegexp(s.env, "1")
	groupMatched, err := openstack.MatchingGroup(s.env, s.callCtx, machineNameRegexp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(group1.Id, gc.Equals, groupMatched.Id)

	machineNameRegexp = openstack.MachineGroupRegexp(s.env, "2")
	groupMatched, err = openstack.MatchingGroup(s.env, s.callCtx, machineNameRegexp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(group2.Id, gc.Equals, groupMatched.Id)
}

// localHTTPSServerSuite contains tests that run against an Openstack service
// double connected on an HTTPS port with a self-signed certificate. This
// service is set up and torn down for every test.  This should only test
// things that depend on the HTTPS connection, all other functional tests on a
// local connection should be in localServerSuite
type localHTTPSServerSuite struct {
	coretesting.BaseSuite
	attrs   map[string]interface{}
	cred    *identity.Credentials
	srv     localServer
	env     environs.Environ
	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&localHTTPSServerSuite{})

func (s *localHTTPSServerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	overrideCinderProvider(&s.CleanupSuite, &mockAdapter{})
}

func (s *localHTTPSServerSuite) createConfigAttrs(c *gc.C) map[string]interface{} {
	attrs := makeTestConfig(s.cred)
	attrs["agent-version"] = coretesting.FakeVersionNumber.String()
	attrs["authorized-keys"] = "fakekey"
	attrs["network"] = "net"
	// In order to set up and tear down the environment properly, we must
	// disable hostname verification
	attrs["ssl-hostname-verification"] = false
	attrs["auth-url"] = s.cred.URL
	// Now connect and set up test-local tools and image-metadata URLs
	cl := client.NewNonValidatingClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, jc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(containerURL[:8], gc.Equals, "https://")
	attrs["agent-metadata-url"] = containerURL + "/juju-dist-test/tools"
	c.Logf("Set agent-metadata-url=%q", attrs["agent-metadata-url"])
	attrs["image-metadata-url"] = containerURL + "/juju-dist-test"
	c.Logf("Set image-metadata-url=%q", attrs["image-metadata-url"])
	return attrs
}

func (s *localHTTPSServerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.srv.UseTLS = true
	cred := &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some-region",
		TenantName: "some tenant",
	}
	// Note: start() will change cred.URL to point to s.srv.Server.URL
	s.srv.start(c, cred, newFullOpenstackService)
	s.cred = cred
	attrs := s.createConfigAttrs(c)
	c.Assert(attrs["auth-url"].(string)[:8], gc.Equals, "https://")
	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapTODOContext(c),
		jujuclient.NewMemStore(),
		prepareParams(attrs, s.cred),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(environs.Environ)
	s.attrs = s.env.Config().AllAttrs()
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *localHTTPSServerSuite) TearDownTest(c *gc.C) {
	if s.env != nil {
		err := s.env.Destroy(s.callCtx)
		c.Check(err, jc.ErrorIsNil)
		s.env = nil
	}
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
}

func (s *localHTTPSServerSuite) TestSSLVerify(c *gc.C) {
	// If you don't have ssl-hostname-verification set to false, and do have
	// a CA Certificate, then we can connect to the environment. Copy the attrs
	// used by SetUp and force hostname verification.
	env := s.envUsingCertificate(c)
	_, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, gc.IsNil)
}

func (s *localHTTPSServerSuite) TestMustDisableSSLVerify(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	// If you don't have ssl-hostname-verification set to false, then we
	// fail to connect to the environment. Copy the attrs used by SetUp and
	// force hostname verification.
	newattrs := make(map[string]interface{}, len(s.attrs))
	for k, v := range s.attrs {
		newattrs[k] = v
	}
	newattrs["ssl-hostname-verification"] = true
	cfg, err := config.New(config.NoDefaults, newattrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  makeCloudSpec(s.cred),
		Config: cfg,
	})
	c.Assert(err, gc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *localHTTPSServerSuite) TestCanBootstrap(c *gc.C) {
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	defer restoreFinishBootstrap()

	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	toolsMetadataStorage := openstack.MetadataStorage(s.env)
	agentURL, err := toolsMetadataStorage.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("Generating fake tools for: %v", agentURL)
	envtesting.UploadFakeTools(c, toolsMetadataStorage, s.env.Config().AgentStream(), s.env.Config().AgentStream())
	defer envtesting.RemoveFakeTools(c, toolsMetadataStorage, s.env.Config().AgentStream())

	imageMetadataStorage := openstack.ImageMetadataStorage(s.env)
	c.Logf("Generating fake images")
	openstack.UseTestImageData(imageMetadataStorage, s.cred)
	defer openstack.RemoveTestImageData(imageMetadataStorage)

	err = bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localHTTPSServerSuite) TestFetchFromImageMetadataSources(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestSkipVerifyDataSourceFactory())
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(customURL[:8], gc.Equals, "https://")

	envConfig, err := s.env.Config().Apply(
		map[string]interface{}{"image-metadata-url": customURL},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(envConfig)
	c.Assert(err, jc.ErrorIsNil)
	sources, err := environs.ImageMetadataSources(s.env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 3)

	// Make sure there is something to download from each location
	metadata := "metadata-content"
	metadataStorage := openstack.ImageMetadataStorage(s.env)
	err = metadataStorage.Put(metadata, bytes.NewBufferString(metadata), int64(len(metadata)))
	c.Assert(err, jc.ErrorIsNil)

	custom := "custom-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, jc.ErrorIsNil)

	// Produce map of data sources keyed on description
	mappedSources := make(map[string]simplestreams.DataSource, len(sources))
	for i, s := range sources {
		c.Logf("datasource %d: %+v", i, s)
		mappedSources[s.Description()] = s
	}

	// Read from the Config entry's image-metadata-url
	contentReader, imageURL, err := mappedSources["image-metadata-url"].Fetch(custom)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, custom)
	c.Check(imageURL[:8], gc.Equals, "https://")

	// Check the entry we got from keystone
	contentReader, imageURL, err = mappedSources["keystone catalog"].Fetch(metadata)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, metadata)
	c.Check(imageURL[:8], gc.Equals, "https://")
	// Verify that we are pointing at exactly where metadataStorage thinks we are
	metaURL, err := metadataStorage.URL(metadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(imageURL, gc.Equals, metaURL)
}

func (s *localHTTPSServerSuite) TestFetchFromImageMetadataSourcesWithCertificate(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	env := s.envUsingCertificate(c)

	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(env, "custom-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(customURL[:8], gc.Equals, "https://")

	envConfig, err := env.Config().Apply(
		map[string]interface{}{"image-metadata-url": customURL},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(envConfig)
	c.Assert(err, jc.ErrorIsNil)
	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 3)

	// Make sure there is something to download from each location
	metadata := "metadata-content"
	metadataStorage := openstack.ImageMetadataStorage(env)
	err = metadataStorage.Put(metadata, bytes.NewBufferString(metadata), int64(len(metadata)))
	c.Assert(err, jc.ErrorIsNil)

	custom := "custom-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, jc.ErrorIsNil)

	// Produce map of data sources keyed on description
	mappedSources := make(map[string]simplestreams.DataSource, len(sources))
	for i, s := range sources {
		c.Logf("datasource %d: %+v", i, s)
		mappedSources[s.Description()] = s
	}

	// Check the entry we got from keystone
	contentReader, imageURL, err := mappedSources["keystone catalog"].Fetch(metadata)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, metadata)
	c.Check(imageURL[:8], gc.Equals, "https://")

	// Verify that we are pointing at exactly where metadataStorage thinks we are
	metaURL, err := metadataStorage.URL(metadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(imageURL, gc.Equals, metaURL)
}

func (s *localHTTPSServerSuite) TestFetchFromToolsMetadataSources(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestSkipVerifyDataSourceFactory())
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-tools-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(customURL[:8], gc.Equals, "https://")

	envConfig, err := s.env.Config().Apply(
		map[string]interface{}{"agent-metadata-url": customURL},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(envConfig)
	c.Assert(err, jc.ErrorIsNil)
	sources, err := tools.GetMetadataSources(s.env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 3)

	// Make sure there is something to download from each location

	keystone := "keystone-tools-content"
	// The keystone entry just points at the root of the Swift storage, and
	// we have to create a container to upload any data. So we just point
	// into a subdirectory for the data we are downloading
	keystoneContainer := "tools-test"
	keystoneStorage := openstack.CreateCustomStorage(s.env, "tools-test")
	err = keystoneStorage.Put(keystone, bytes.NewBufferString(keystone), int64(len(keystone)))
	c.Assert(err, jc.ErrorIsNil)

	custom := "custom-tools-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, jc.ErrorIsNil)

	// Read from the Config entry's agent-metadata-url
	contentReader, metadataURL, err := sources[0].Fetch(custom)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err := ioutil.ReadAll(contentReader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, custom)
	c.Check(metadataURL[:8], gc.Equals, "https://")

	// Check the entry we got from keystone
	// Now fetch the data, and verify the contents.
	contentReader, metadataURL, err = sources[1].Fetch(keystoneContainer + "/" + keystone)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err = ioutil.ReadAll(contentReader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, keystone)
	c.Check(metadataURL[:8], gc.Equals, "https://")
	keystoneURL, err := keystoneStorage.URL(keystone)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadataURL, gc.Equals, keystoneURL)

	// We *don't* test Fetch for sources[3] because it points to
	// streams.canonical.com
}

func (s *localServerSuite) TestRemoveBlankContainer(c *gc.C) {
	containerStorage := openstack.BlankContainerStorage()
	err := containerStorage.Remove("some-file")
	c.Assert(err, gc.ErrorMatches, `cannot remove "some-file": swift container name is empty`)
}

func (s *localServerSuite) TestAllRunningInstancesIgnoresOtherMachines(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	// Check that we see 1 instance in the environment
	insts, err := s.env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(insts, gc.HasLen, 1)

	// Now start a machine 'manually' in the same account, with a similar
	// but not matching name, and ensure it isn't seen by AllRunningInstances
	// See bug #1257481, for how similar names were causing them to get
	// listed (and thus destroyed) at the wrong time
	existingModelName := s.TestConfig["name"]
	newMachineName := fmt.Sprintf("juju-%s-2-machine-0", existingModelName)

	// We grab the Nova client directly from the env, just to save time
	// looking all the stuff up
	novaClient := openstack.GetNovaClient(s.env)
	entity, err := novaClient.RunServer(nova.RunServerOpts{
		Name:     newMachineName,
		FlavorId: "1", // test service has 1,2,3 for flavor ids
		ImageId:  "1", // UseTestImageData sets up images 1 and 2
		Networks: []nova.ServerNetworks{{NetworkId: "1"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.NotNil)

	// List all servers with no filter, we should see both instances
	servers, err := novaClient.ListServersDetail(nova.NewFilter())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(servers, gc.HasLen, 2)

	insts, err = s.env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(insts, gc.HasLen, 1)
}

func (s *localServerSuite) TestResolveNetworkUUID(c *gc.C) {
	var sampleUUID = "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"
	networkId, err := openstack.ResolveNetwork(s.env, sampleUUID, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networkId, gc.Equals, sampleUUID)
}

func (s *localServerSuite) TestResolveNetworkLabel(c *gc.C) {
	// For now this test has to cheat and use knowledge of goose internals
	var networkLabel = "net"
	var expectNetworkId = "1"
	networkId, err := openstack.ResolveNetwork(s.env, networkLabel, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networkId, gc.Equals, expectNetworkId)
}

func (s *localServerSuite) TestResolveNetworkNotPresent(c *gc.C) {
	var notPresentNetwork = "no-network-with-this-label"
	networkId, err := openstack.ResolveNetwork(s.env, notPresentNetwork, false)
	c.Check(networkId, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `no networks exist with label "no-network-with-this-label"`)
	networkId, err = openstack.ResolveNetwork(s.env, notPresentNetwork, true)
	c.Check(networkId, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `no networks exist with label "no-network-with-this-label"`)
}

// TODO(gz): TestResolveNetworkMultipleMatching when can inject new networks

func (s *localServerSuite) TestStartInstanceAvailZone(c *gc.C) {
	inst, err := s.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openstack.InstanceServerDetail(inst).AvailabilityZone, gc.Equals, "test-available")
}

func (s *localServerSuite) TestStartInstanceAvailZoneUnavailable(c *gc.C) {
	_, err := s.testStartInstanceAvailZone(c, "test-unavailable")
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)
}

func (s *localServerSuite) TestStartInstanceAvailZoneUnknown(c *gc.C) {
	_, err := s.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)
}

func (s *localServerSuite) testStartInstanceAvailZone(c *gc.C, zone string) (instances.Instance, error) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	params := environs.StartInstanceParams{
		ControllerUUID:   s.ControllerUUID,
		AvailabilityZone: zone,
	}
	result, err := testing.StartInstanceWithParams(s.env, s.callCtx, "1", params)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (s *localServerSuite) TestGetAvailabilityZones(c *gc.C) {
	var resultZones []nova.AvailabilityZone
	var resultErr error
	s.PatchValue(openstack.NovaListAvailabilityZones, func(c *nova.Client) ([]nova.AvailabilityZone, error) {
		return append([]nova.AvailabilityZone{}, resultZones...), resultErr
	})
	env := s.env.(common.ZonedEnviron)

	resultErr = fmt.Errorf("failed to get availability zones")
	zones, err := env.AvailabilityZones(s.callCtx)
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zones, gc.IsNil)

	resultErr = nil
	resultZones = make([]nova.AvailabilityZone, 1)
	resultZones[0].Name = "whatever"
	zones, err = env.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	resultErr = fmt.Errorf("failed to get availability zones")
	resultZones[0].Name = "andever"
	zones, err = env.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
}

func (s *localServerSuite) TestGetAvailabilityZonesCommon(c *gc.C) {
	var resultZones []nova.AvailabilityZone
	s.PatchValue(openstack.NovaListAvailabilityZones, func(c *nova.Client) ([]nova.AvailabilityZone, error) {
		return append([]nova.AvailabilityZone{}, resultZones...), nil
	})
	env := s.env.(common.ZonedEnviron)
	resultZones = make([]nova.AvailabilityZone, 2)
	resultZones[0].Name = "az1"
	resultZones[1].Name = "az2"
	resultZones[0].State.Available = true
	resultZones[1].State.Available = false
	zones, err := env.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 2)
	c.Assert(zones[0].Name(), gc.Equals, resultZones[0].Name)
	c.Assert(zones[1].Name(), gc.Equals, resultZones[1].Name)
	c.Assert(zones[0].Available(), jc.IsTrue)
	c.Assert(zones[1].Available(), jc.IsFalse)
}

func (s *localServerSuite) TestStartInstanceWithUnknownAZError(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	s.srv.Nova.SetAvailabilityZones(
		// bootstrap node will be on az1.
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
		// az2 will be made to return an unknown error.
		nova.AvailabilityZone{
			Name: "az2",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	cleanup := s.srv.Nova.RegisterControlPoint(
		"addServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			serverDetail := args[0].(*nova.ServerDetail)
			if serverDetail.AvailabilityZone == "az2" {
				return fmt.Errorf("some unknown error")
			}
			return nil
		},
	)
	defer cleanup()
	_, err = testing.StartInstanceWithParams(s.env, s.callCtx, "1", environs.StartInstanceParams{
		ControllerUUID:   s.ControllerUUID,
		AvailabilityZone: "az2",
	})
	c.Assert(err, gc.ErrorMatches, "(?s).*some unknown error.*")
}

func (s *localServerSuite) testStartInstanceWithParamsDeriveAZ(
	machineId string,
	params environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	zonedEnv := s.env.(common.ZonedEnviron)
	zones, err := zonedEnv.DeriveAvailabilityZones(s.callCtx, params)
	if err != nil {
		return nil, err
	}
	if len(zones) < 1 {
		return nil, errors.New("no zones found")
	}
	params.AvailabilityZone = zones[0]
	return testing.StartInstanceWithParams(s.env, s.callCtx, "1", params)
}

func (s *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZone(c *gc.C) {
	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
		nova.AvailabilityZone{
			Name: "az2",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
		nova.AvailabilityZone{
			Name: "test-available",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az2",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.testStartInstanceWithParamsDeriveAZ("1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		VolumeAttachments: []storage.VolumeAttachmentParams{
			{VolumeId: "foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openstack.InstanceServerDetail(result.Instance).AvailabilityZone, gc.Equals, "az2")
}

func (s *localServerSuite) TestStartInstanceVolumeAttachmentsMultipleAvailZones(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	for _, az := range []string{"az1", "az2"} {
		_, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
			Size:             123,
			Name:             "vol-" + az,
			AvailabilityZone: az,
			Metadata: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	_, err = s.testStartInstanceWithParamsDeriveAZ("1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		VolumeAttachments: []storage.VolumeAttachmentParams{
			{VolumeId: "vol-az1"},
			{VolumeId: "vol-az2"},
		},
	})
	c.Assert(err, gc.ErrorMatches, `cannot attach volumes from multiple availability zones: vol-az1 is in az1, vol-az2 is in az2`)
}

func (s *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZoneConflictsPlacement(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
		nova.AvailabilityZone{
			Name: "az2",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)
	_, err = s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = testing.StartInstanceWithParams(s.env, s.callCtx, "1", environs.StartInstanceParams{
		ControllerUUID:    s.ControllerUUID,
		VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
		AvailabilityZone:  "az2",
	})
	c.Assert(err, gc.ErrorMatches, `cannot create instance in zone "az2", as this will prevent attaching the requested disks in zone "az1"`)
}

// novaInstaceStartedWithOpts exposes run server options used to start an instance.
type novaInstaceStartedWithOpts interface {
	NovaInstanceStartedWithOpts() *nova.RunServerOpts
}

func (s *localServerSuite) TestStartInstanceVolumeRootBlockDevice(c *gc.C) {
	// diskSizeGiB should be equal to the openstack.defaultRootDiskSize
	diskSizeGiB := 30
	env := s.ensureAMDImages(c)

	err := bootstrapEnv(c, env)
	c.Assert(err, jc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk-source=volume arch=amd64")
	c.Assert(err, jc.ErrorIsNil)

	res, err := testing.StartInstanceWithParams(env, s.callCtx, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.NotNil)

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, gc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, gc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, jc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "volume",
		DeleteOnTermination: true,
		VolumeSize:          diskSizeGiB,
	})
}

func (s *localServerSuite) TestStartInstanceVolumeRootBlockDeviceSized(c *gc.C) {
	env := s.ensureAMDImages(c)

	diskSizeGiB := 10

	err := bootstrapEnv(c, env)
	c.Assert(err, jc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk-source=volume root-disk=10G arch=amd64")
	c.Assert(err, jc.ErrorIsNil)

	res, err := testing.StartInstanceWithParams(env, s.callCtx, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.NotNil)

	c.Assert(res.Hardware.RootDisk, gc.NotNil)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, uint64(diskSizeGiB*1024))

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, gc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, gc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, jc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "volume",
		DeleteOnTermination: true,
		VolumeSize:          diskSizeGiB,
	})
}

func (s *localServerSuite) TestStartInstanceLocalRootBlockDevice(c *gc.C) {
	env := s.ensureAMDImages(c)

	err := bootstrapEnv(c, env)
	c.Assert(err, jc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk=1G arch=amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.HasRootDisk(), jc.IsTrue)
	c.Assert(*cons.RootDisk, gc.Equals, uint64(1024))

	res, err := testing.StartInstanceWithParams(env, s.callCtx, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.NotNil)

	c.Assert(res.Hardware.RootDisk, gc.NotNil)
	// Check local disk requirements are met.
	c.Assert(*res.Hardware.RootDisk, jc.GreaterThan, uint64(1024-1))

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, gc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, gc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, jc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "local",
		DeleteOnTermination: true,
		// VolumeSize is 0 when a local disk is used.
		VolumeSize: 0,
	})
}

func (s *localServerSuite) TestInstanceTags(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	allInstances, err := s.env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstances, gc.HasLen, 1)

	c.Assert(
		openstack.InstanceServerDetail(allInstances[0]).Metadata,
		jc.DeepEquals,
		map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
			"juju-is-controller":   "true",
		},
	)
}

func (s *localServerSuite) TestTagInstance(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	assertMetadata := func(extraKey, extraValue string) {
		// Refresh instance
		allInstances, err := s.env.AllRunningInstances(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(allInstances, gc.HasLen, 1)
		c.Assert(
			openstack.InstanceServerDetail(allInstances[0]).Metadata,
			jc.DeepEquals,
			map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
				"juju-is-controller":   "true",
				extraKey:               extraValue,
			},
		)
	}

	allInstances, err := s.env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstances, gc.HasLen, 1)

	extraKey := "extra-k"
	extraValue := "extra-v"
	err = s.env.(environs.InstanceTagger).TagInstance(
		s.callCtx,
		allInstances[0].Id(),
		map[string]string{extraKey: extraValue},
	)
	c.Assert(err, jc.ErrorIsNil)
	assertMetadata(extraKey, extraValue)

	// Ensure that a second call updates existing tags.
	extraValue = "extra-v2"
	err = s.env.(environs.InstanceTagger).TagInstance(
		s.callCtx,
		allInstances[0].Id(),
		map[string]string{extraKey: extraValue},
	)
	c.Assert(err, jc.ErrorIsNil)
	assertMetadata(extraKey, extraValue)
}

func (s *localServerSuite) TestAdoptResources(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	cfg, err := s.env.Config().Apply(map[string]interface{}{
		"uuid": hostedModelUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  makeCloudSpec(s.cred),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	originalController := coretesting.ControllerTag.Id()
	_, _, _, err = testing.StartInstance(env, s.callCtx, originalController, "0")
	c.Assert(err, jc.ErrorIsNil)

	addVolume(c, s.env, s.callCtx, originalController, "99/9")
	addVolume(c, env, s.callCtx, originalController, "23/9")

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, originalController)
	s.checkVolumeTags(c, s.env, originalController)
	s.checkVolumeTags(c, env, originalController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, originalController)

	// Needs to be a correctly formatted uuid so we can get it out of
	// group names.
	newController := "aaaaaaaa-bbbb-cccc-dddd-0123456789ab"
	err = env.AdoptResources(s.callCtx, newController, version.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, newController)
	s.checkVolumeTags(c, s.env, originalController)
	s.checkVolumeTags(c, env, newController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, newController)
}

func (s *localServerSuite) TestAdoptResourcesNoStorage(c *gc.C) {
	// Nova-lxd doesn't support storage. lp:1677225
	s.PatchValue(openstack.NewOpenstackStorage, func(*openstack.Environ) (openstack.OpenstackStorage, error) {
		return nil, errors.NotSupportedf("volumes")
	})
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	cfg, err := s.env.Config().Apply(map[string]interface{}{
		"uuid": hostedModelUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  makeCloudSpec(s.cred),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	originalController := coretesting.ControllerTag.Id()
	_, _, _, err = testing.StartInstance(env, s.callCtx, originalController, "0")
	c.Assert(err, jc.ErrorIsNil)

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, originalController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, originalController)

	// Needs to be a correctly formatted uuid so we can get it out of
	// group names.
	newController := "aaaaaaaa-bbbb-cccc-dddd-0123456789ab"
	err = env.AdoptResources(s.callCtx, newController, version.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, newController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, newController)
}

func addVolume(
	c *gc.C, env environs.Environ, callCtx context.ProviderCallContext, controllerUUID, name string,
) *storage.Volume {
	storageAdapter, err := (*openstack.NewOpenstackStorage)(env.(*openstack.Environ))
	c.Assert(err, jc.ErrorIsNil)
	modelUUID := env.Config().UUID()
	source := openstack.NewCinderVolumeSourceForModel(storageAdapter, modelUUID, env.(common.ZonedEnviron))
	result, err := source.CreateVolumes(callCtx, []storage.VolumeParams{{
		Tag: names.NewVolumeTag(name),
		ResourceTags: tags.ResourceTags(
			names.NewModelTag(modelUUID),
			names.NewControllerTag(controllerUUID),
		),
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, jc.ErrorIsNil)
	return result[0].Volume
}

func (s *localServerSuite) checkInstanceTags(c *gc.C, env environs.Environ, expectedController string) {
	allInstances, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstances, gc.Not(gc.HasLen), 0)
	for _, inst := range allInstances {
		server := openstack.InstanceServerDetail(inst)
		c.Logf(string(inst.Id()))
		c.Check(server.Metadata[tags.JujuController], gc.Equals, expectedController)
	}
}

func (s *localServerSuite) checkVolumeTags(c *gc.C, env environs.Environ, expectedController string) {
	stor, err := (*openstack.NewOpenstackStorage)(env.(*openstack.Environ))
	c.Assert(err, jc.ErrorIsNil)
	source := openstack.NewCinderVolumeSourceForModel(stor, env.Config().UUID(), s.env.(common.ZonedEnviron))
	volumeIds, err := source.ListVolumes(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeIds, gc.Not(gc.HasLen), 0)
	for _, volumeId := range volumeIds {
		c.Logf(volumeId)
		volume, err := stor.GetVolume(volumeId)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(volume.Metadata[tags.JujuController], gc.Equals, expectedController)
	}
}

func (s *localServerSuite) checkGroupController(c *gc.C, env environs.Environ, expectedController string) {
	groupNames, err := openstack.GetModelGroupNames(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groupNames, gc.Not(gc.HasLen), 0)
	extractControllerRe, err := regexp.Compile(openstack.GroupControllerPattern)
	c.Assert(err, jc.ErrorIsNil)
	for _, group := range groupNames {
		c.Logf(group)
		controller := extractControllerRe.ReplaceAllString(group, "$controllerUUID")
		c.Check(controller, gc.Equals, expectedController)
	}
}

func (s *localServerSuite) TestUpdateGroupController(c *gc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, jc.ErrorIsNil)

	groupNames, err := openstack.GetModelGroupNames(s.env)
	c.Assert(err, jc.ErrorIsNil)
	groupNamesBefore := set.NewStrings(groupNames...)
	c.Assert(groupNamesBefore, gc.DeepEquals, set.NewStrings(
		"juju-deadbeef-1bad-500d-9000-4b1d0d06f00d-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"juju-deadbeef-1bad-500d-9000-4b1d0d06f00d-deadbeef-0bad-400d-8000-4b1d0d06f00d-0",
	))

	firewaller := openstack.GetFirewaller(s.env)
	err = firewaller.UpdateGroupController(s.callCtx, "aabbccdd-eeee-ffff-0000-0123456789ab")
	c.Assert(err, jc.ErrorIsNil)

	groupNames, err = openstack.GetModelGroupNames(s.env)
	c.Assert(err, jc.ErrorIsNil)
	groupNamesAfter := set.NewStrings(groupNames...)
	c.Assert(groupNamesAfter, gc.DeepEquals, set.NewStrings(
		"juju-aabbccdd-eeee-ffff-0000-0123456789ab-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"juju-aabbccdd-eeee-ffff-0000-0123456789ab-deadbeef-0bad-400d-8000-4b1d0d06f00d-0",
	))
}

func (s *localServerSuite) ensureAMDImages(c *gc.C) environs.Environ {
	// Ensure amd64 tools are available, to ensure an amd64 image.
	amd64Version := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.AMD64,
	}
	workloadOSList, err := series.AllWorkloadOSTypes("", "")
	c.Assert(err, jc.ErrorIsNil)
	for _, workloadOS := range workloadOSList.Values() {
		amd64Version.Release = workloadOS
		envtesting.AssertUploadFakeToolsVersions(
			c, s.toolsMetadataStorage, s.env.Config().AgentStream(), s.env.Config().AgentStream(), amd64Version)
	}

	// Destroy the old Environ
	err = environs.Destroy(s.env.Config().Name(), s.env, s.callCtx, s.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)

	// Prepare a new Environ
	return s.Prepare(c)
}

// noNeutronSuite is a clone of localServerSuite which hacks the local
// openstack to remove the neutron service from the auth response -
// this causes the client to switch to nova networking.
type noNeutronSuite struct {
	coretesting.BaseSuite
	cred *identity.Credentials
	srv  localServer
}

func (s *noNeutronSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	c.Logf("Running local tests")
}

func (s *noNeutronSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.srv.start(c, s.cred, newNovaNetworkingOpenstackService)

	userPass, ok := s.srv.OpenstackSvc.Identity.(*identityservice.UserPass)
	c.Assert(ok, jc.IsTrue)
	// Ensure that there's nothing returned with a type of "network",
	// so that we switch over to nova networking.
	cleanup := userPass.RegisterControlPoint("authorisation", func(sc hook.ServiceControl, args ...interface{}) error {
		res, ok := args[0].(*identityservice.AccessResponse)
		c.Assert(ok, jc.IsTrue)
		var filtered []identityservice.V2Service
		for _, service := range res.Access.ServiceCatalog {
			if service.Type != "network" {
				filtered = append(filtered, service)
			}
		}
		res.Access.ServiceCatalog = filtered
		return nil
	})
	s.AddCleanup(func(c *gc.C) { cleanup() })
}

func (s *noNeutronSuite) TearDownTest(c *gc.C) {
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
}

func (s *noNeutronSuite) TestSupport(c *gc.C) {
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, jc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":               "sample-no-neutron",
		"type":               "openstack",
		"auth-mode":          "userpass",
		"agent-version":      coretesting.FakeVersionNumber.String(),
		"agent-metadata-url": containerURL + "/juju-dist-test/tools",
		"image-metadata-url": containerURL + "/juju-dist-test",
		"authorized-keys":    "fakekey",
	})
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	_, err = bootstrap.PrepareController(
		false,
		envtesting.BootstrapTODOContext(c),
		jujuclient.NewMemStore(),
		prepareParams(attrs, s.cred),
	)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `OpenStack Neutron service`)
}

func prepareParams(attrs map[string]interface{}, cred *identity.Credentials) bootstrap.PrepareParams {
	return bootstrap.PrepareParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		ModelConfig:      attrs,
		ControllerName:   attrs["name"].(string),
		Cloud:            makeCloudSpec(cred),
		AdminSecret:      testing.AdminSecret,
	}
}

func makeCloudSpec(cred *identity.Credentials) environscloudspec.CloudSpec {
	credential := makeCredential(cred)
	return environscloudspec.CloudSpec{
		Type:       "openstack",
		Name:       "openstack",
		Endpoint:   cred.URL,
		Region:     cred.Region,
		Credential: &credential,
	}
}

func makeCredential(cred *identity.Credentials) cloud.Credential {
	return cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username":    cred.User,
			"password":    cred.Secrets,
			"tenant-name": cred.TenantName,
		},
	)
}

// noSwiftSuite contains tests that run against an OpenStack service double
// that lacks Swift.
type noSwiftSuite struct {
	coretesting.BaseSuite
	cred *identity.Credentials
	srv  localServer
	env  environs.Environ
}

var _ = gc.Suite(&noSwiftSuite{})

func (s *noSwiftSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) { restoreFinishBootstrap() })

	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *noSwiftSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.cred = &identity.Credentials{
		Version:    2,
		User:       "fred",
		Secrets:    "secret",
		Region:     "some-region",
		TenantName: "some tenant",
	}
	s.srv.start(c, s.cred, newNovaOnlyOpenstackService)

	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":            "sample-no-swift",
		"type":            "openstack",
		"auth-mode":       "userpass",
		"agent-version":   coretesting.FakeVersionNumber.String(),
		"authorized-keys": "fakekey",
	})
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	// Serve fake tools and image metadata using "filestorage",
	// rather than Swift as the rest of the tests do.
	storageDir := c.MkDir()
	imagesDir := filepath.Join(storageDir, "images")
	toolsDir := filepath.Join(storageDir, "tools")
	for _, dir := range []string{imagesDir, toolsDir} {
		err := os.MkdirAll(dir, 0755)
		c.Assert(err, jc.ErrorIsNil)
	}
	toolsStorage, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, toolsStorage, "released", "released")
	s.PatchValue(&tools.DefaultBaseURL, storageDir)
	imageStorage, err := filestorage.NewFileStorageWriter(imagesDir)
	c.Assert(err, jc.ErrorIsNil)
	openstack.UseTestImageData(imageStorage, s.cred)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, storageDir)

	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapTODOContext(c),
		jujuclient.NewMemStore(),
		prepareParams(attrs, s.cred),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(environs.Environ)
}

func (s *noSwiftSuite) TearDownTest(c *gc.C) {
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
}

func (s *noSwiftSuite) TestBootstrap(c *gc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that corresponds to a neutron test service network
		"network": "net",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(bootstrapEnv(c, s.env), jc.ErrorIsNil)
}

func newFullOpenstackService(cred *identity.Credentials, auth identity.AuthMode, useTSL bool) (*openstackservice.Openstack, []string) {
	service, logMsg := openstackservice.New(cred, auth, useTSL)
	service.UseNeutronNetworking()
	service.SetupHTTP(nil)
	return service, logMsg
}

func newNovaOnlyOpenstackService(cred *identity.Credentials, auth identity.AuthMode, useTSL bool) (*openstackservice.Openstack, []string) {
	service, logMsg := openstackservice.NewNoSwift(cred, auth, useTSL)
	service.UseNeutronNetworking()
	service.SetupHTTP(nil)
	return service, logMsg
}

func newNovaNetworkingOpenstackService(cred *identity.Credentials, auth identity.AuthMode, useTSL bool) (*openstackservice.Openstack, []string) {
	service, logMsg := openstackservice.New(cred, auth, useTSL)
	service.SetupHTTP(nil)
	return service, logMsg
}
func bootstrapEnv(c *gc.C, env environs.Environ) error {
	return bootstrapEnvWithConstraints(c, env, constraints.Value{})
}

func bootstrapEnvWithConstraints(c *gc.C, env environs.Environ, cons constraints.Value) error {
	return bootstrap.Bootstrap(envtesting.BootstrapTODOContext(c), env,
		context.NewEmptyCloudCallContext(),
		bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
			BootstrapConstraints:     cons,
		})
}
