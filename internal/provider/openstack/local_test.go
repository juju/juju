// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/go-goose/goose/v5/cinder"
	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/go-goose/goose/v5/testservices/hook"
	"github.com/go-goose/goose/v5/testservices/identityservice"
	"github.com/go-goose/goose/v5/testservices/neutronmodel"
	"github.com/go-goose/goose/v5/testservices/neutronservice"
	"github.com/go-goose/goose/v5/testservices/novaservice"
	"github.com/go-goose/goose/v5/testservices/openstackservice"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envstorage "github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
)

type ProviderSuite struct {
	restoreTimeouts func()
}

func TestProviderSuite(t *stdtesting.T) {
	tc.Run(t, &ProviderSuite{})
}

func (s *ProviderSuite) SetUpTest(c *tc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(openstack.ShortAttempt, openstack.StorageAttempt)
}

func (s *ProviderSuite) TearDownTest(c *tc.C) {
	s.restoreTimeouts()
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
	c *tc.C, cred *identity.Credentials, newOpenstackFunc newOpenstackFunc,
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

func (s *localServer) openstackCertificate(c *tc.C) ([]string, error) {
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

func (s *localHTTPSServerSuite) envUsingCertificate(c *tc.C) environs.Environ {
	newattrs := make(map[string]interface{}, len(s.attrs))
	for k, v := range s.attrs {
		newattrs[k] = v
	}
	newattrs["ssl-hostname-verification"] = true
	cfg, err := config.New(config.NoDefaults, newattrs)
	c.Assert(err, tc.ErrorIsNil)

	cloudSpec := makeCloudSpec(s.cred)
	cloudSpec.CACertificates, err = s.srv.openstackCertificate(c)
	c.Assert(err, tc.ErrorIsNil)

	env, err := environs.New(c.Context(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	return env
}

func makeMockAdaptor() *mockAdaptor {
	volumes := make(map[string]*cinder.Volume)
	return &mockAdaptor{
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

func overrideCinderProvider(s *testhelpers.CleanupSuite, adaptor *mockAdaptor) {
	s.PatchValue(openstack.NewOpenstackStorage, func(*openstack.Environ) (openstack.OpenstackStorage, error) {
		return adaptor, nil
	})
}
func TestLocalServerSuite(t *stdtesting.T) {
	tc.Run(t, &localServerSuite{})
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
	storageAdaptor       *mockAdaptor
}

func (s *localServerSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.Tests.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap(c)
	s.AddCleanup(func(*tc.C) { restoreFinishBootstrap() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	s.AddCleanup(func(c *tc.C) {
		server.Close()
	})
	s.PatchValue(&imagemetadata.DefaultUbuntuBaseURL, server.URL)

	c.Logf("Running local tests")
}

var localConfigAttrs = coretesting.FakeConfig().Merge(coretesting.Attrs{
	"name":            "sample",
	"type":            "openstack",
	"auth-mode":       "userpass",
	"agent-version":   coretesting.FakeVersionNumber.String(),
	"authorized-keys": "fakekey",
	"network":         "private_999",
})

func (s *localServerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.cred = &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some-region",
		TenantName: "some tenant",
	}

	s.srv.start(c, s.cred, newFullOpenstackService)

	// Set credentials to use when bootstrapping. Must be done after
	// starting server to get the auth URL.
	s.Credential = makeCredential(s.cred)
	s.CloudEndpoint = s.cred.URL
	s.CloudRegion = s.cred.Region
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, tc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.TestConfig = localConfigAttrs
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
	envtesting.UploadFakeTools(c, s.toolsMetadataStorage, "released")
	s.imageMetadataStorage = openstack.ImageMetadataStorage(s.env)
	openstack.UseTestImageData(s.imageMetadataStorage, s.cred)
	s.storageAdaptor = makeMockAdaptor()
	overrideCinderProvider(&s.CleanupSuite, s.storageAdaptor)
}

func (s *localServerSuite) TearDownTest(c *tc.C) {
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

func (s *localServerSuite) openEnviron(c *tc.C, attrs coretesting.Attrs) environs.Environ {
	cfg, err := config.New(config.NoDefaults, s.TestConfig.Merge(attrs))
	c.Assert(err, tc.ErrorIsNil)
	env, err := environs.New(c.Context(), environs.OpenParams{
		Cloud:          s.CloudSpec(),
		Config:         cfg,
		ControllerUUID: coretesting.FakeControllerConfig().ControllerUUID(),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	return env
}

func (s *localServerSuite) TestBootstrap(c *tc.C) {
	// Tests uses Prepare, so destroy first.
	err := environs.Destroy(s.env.Config().Name(), s.env, c.Context(), s.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)
	s.Tests.TestBootstrap(c)
}

func (t *localServerSuite) TestBootstrapMultiple(c *tc.C) {
	args := bootstrap.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AdminSecret:             testing.AdminSecret,
		CAPrivateKey:            coretesting.CAKey,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	}
	// bootstrap.Bootstrap no longer raises errors if the environment is
	// already up, this has been moved into the bootstrap command.
	err := bootstrap.Bootstrap(t.BootstrapContext, t.Env, args)
	c.Assert(err, tc.ErrorIsNil)

	c.Logf("destroy env")
	env := t.Env
	err = environs.Destroy(t.Env.Config().Name(), t.Env, t.BootstrapContext, t.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)
	err = env.Destroy(t.BootstrapContext) // Again, should work fine and do nothing.
	c.Assert(err, tc.ErrorIsNil)

	// check that we can bootstrap after destroy
	err = bootstrap.Bootstrap(t.BootstrapContext, t.Env, args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartStop(c *tc.C) {
	// Tests uses Prepare, so destroy first.
	err := environs.Destroy(s.env.Config().Name(), s.env, c.Context(), s.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)
	s.Tests.TestStartStop(c)
}

// If the bootstrap node is configured to require a public IP address,
// bootstrapping fails if an address cannot be allocated.
func (s *localServerSuite) TestBootstrapFailsWhenPublicIPError(c *tc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Neutron.RegisterControlPoint(
		"addFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("failed on purpose")
		},
	)
	defer cleanup()

	err := environs.Destroy(s.env.Config().Name(), s.env, c.Context(), s.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)

	env := s.openEnviron(c, coretesting.Attrs{})
	cons := constraints.MustParse("allocate-public-ip=true")
	err = bootstrapEnvWithConstraints(c, env, cons)
	c.Assert(err, tc.ErrorMatches, "(.|\n)*cannot allocate a public IP as needed(.|\n)*")
}

func (s *localServerSuite) TestAddressesWithPublicIPConstraints(c *tc.C) {
	// Floating IP address is 10.0.0.1
	bootstrapFinished := false
	s.PatchValue(&common.FinishBootstrap, func(
		ctx environs.BootstrapContext,
		client ssh.Client,
		env environs.Environ,
		inst instances.Instance,
		instanceConfig *instancecfg.InstanceConfig,
		_ environs.BootstrapDialOpts,
	) error {
		addr, err := inst.Addresses(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(addr, tc.SameContents, network.ProviderAddresses{
			network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			network.NewMachineAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)).AsProviderAddress(),
			network.NewMachineAddress("::face::000f").AsProviderAddress(),
			network.NewMachineAddress("127.10.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			network.NewMachineAddress("::dead:beef:f00d", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		})
		bootstrapFinished = true
		return nil
	})

	env := s.openEnviron(c, coretesting.Attrs{
		"network": "private_999",
	})
	cons := constraints.MustParse("allocate-public-ip=true")
	err := bootstrapEnvWithConstraints(c, env, cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bootstrapFinished, tc.IsTrue)
}

func (s *localServerSuite) TestAddressesWithoutPublicIPConstraints(c *tc.C) {
	bootstrapFinished := false
	s.PatchValue(&common.FinishBootstrap, func(
		ctx environs.BootstrapContext,
		client ssh.Client,
		env environs.Environ,
		inst instances.Instance,
		instanceConfig *instancecfg.InstanceConfig,
		_ environs.BootstrapDialOpts,
	) error {
		addr, err := inst.Addresses(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(addr, tc.SameContents, network.ProviderAddresses{
			network.NewMachineAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)).AsProviderAddress(),
			network.NewMachineAddress("::face::000f").AsProviderAddress(),
			network.NewMachineAddress("127.10.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			network.NewMachineAddress("::dead:beef:f00d", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		})
		bootstrapFinished = true
		return nil
	})

	env := s.openEnviron(c, coretesting.Attrs{})
	cons := constraints.MustParse("allocate-public-ip=false")
	err := bootstrapEnvWithConstraints(c, env, cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bootstrapFinished, tc.IsTrue)
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstanceWithoutPublicIP(c *tc.C) {
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

	err := environs.Destroy(s.env.Config().Name(), s.env, c.Context(), s.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)

	env := s.Prepare(c)
	err = bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, "100")
	err = env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

// If we fail to allocate a floating IP when starting an instance, the nova instance
// should be terminated.
func (s *localServerSuite) TestStartInstanceWhenPublicIPError(c *tc.C) {
	var (
		addServerID        string
		removeServerID     string
		removeServerCalled bool
	)

	cleanup := s.srv.Neutron.RegisterControlPoint(
		"addFloatingIP",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("fail on purpose")
		},
	)
	defer cleanup()
	cleanup = s.srv.Nova.RegisterControlPoint(
		"addServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			addServerID = args[0].(*nova.ServerDetail).Id
			return nil
		},
	)
	defer cleanup()
	cleanup = s.srv.Nova.RegisterControlPoint(
		"removeServer",
		func(sc hook.ServiceControl, args ...interface{}) error {
			removeServerCalled = true
			removeServerID = args[0].(string)
			return nil
		},
	)
	defer cleanup()
	_, _, _, err := testing.StartInstanceWithConstraints(c, s.env, s.ControllerUUID, "100", constraints.MustParse("allocate-public-ip=true"))
	c.Assert(err, tc.ErrorMatches, "(.|\n)*cannot allocate a public IP as needed(.|\n)*")
	c.Assert(removeServerCalled, tc.IsTrue)
	c.Assert(removeServerID, tc.Equals, addServerID)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *tc.C) {
	// Ensure amd64 tools are available, to ensure an amd64 image.
	env := s.ensureAMDImages(c)
	err := bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, env, s.ControllerUUID, "100", constraints.MustParse("mem=1024 arch=amd64"))
	c.Check(*hc.Arch, tc.Equals, "amd64")
	c.Check(*hc.Mem, tc.Equals, uint64(2048))
	c.Check(*hc.CpuCores, tc.Equals, uint64(1))
	c.Assert(hc.CpuPower, tc.IsNil)
}

func (s *localServerSuite) TestInstanceName(c *tc.C) {
	inst, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	serverDetail := openstack.InstanceServerDetail(inst)
	envName := s.env.Config().Name()
	c.Assert(serverDetail.Name, tc.Matches, "juju-06f00d-"+envName+"-100")
}

func (s *localServerSuite) TestStartInstanceNetwork(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that corresponds to a neutron test service network
		"network": "net",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	err = s.env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceMultiNetworkFound(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inst, tc.NotNil)
}

func (s *localServerSuite) TestStartInstanceExternalNetwork(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that corresponds to a neutron test service external network
		"external-network": "ext-net",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("allocate-public-ip=true")
	inst, _ := testing.AssertStartInstanceWithConstraints(c, s.env, s.ControllerUUID, "100", cons)
	err = s.env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkUnknownLabel(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that has no related network in the neutron test service
		"network": "no-network-with-this-label",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Check(inst, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `unable to determine networks for configured list: \[no-network-with-this-label\]`)
}

func (s *localServerSuite) TestStartInstanceExternalNetworkUnknownLabel(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that has no related network in the neutron test service
		"external-network": "no-network-with-this-label",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("allocate-public-ip=true")
	inst, _, _, err := testing.StartInstanceWithConstraints(c, s.env, s.ControllerUUID, "100", cons)
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkUnknownID(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A valid UUID but no related network in the nova test service
		"network": "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Check(inst, tc.IsNil)
	c.Assert(err, tc.ErrorMatches,
		`unable to determine networks for configured list: \[f81d4fae-7dec-11d0-a765-00a0c91e6bf6\]`)
}

func (s *localServerSuite) TestStartInstanceNoNetworksNetworkNotSetNoError(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Check(inst, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceOneNetworkNetworkNotSetNoError(c *tc.C) {
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
			c.Assert(err, tc.ErrorIsNil)
		}
	}
	s.srv.OpenstackSvc.Neutron.AddNeutronModel(model)
	s.srv.OpenstackSvc.Nova.AddNeutronModel(model)

	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Check(inst, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworksDifferentAZ(c *tc.C) {
	// If both the network and external-network config values are
	// specified, there is not check for them being on different
	// network availability zones with allocate-public-ip constraint.
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network":          "net",     // az = nova
		"external-network": "ext-net", // az = test-available
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("allocate-public-ip=true")
	inst, _, _, err := testing.StartInstanceWithConstraints(c, s.env, s.ControllerUUID, "100", cons)
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworksEmptyAZ(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	err = model.AddNetwork(neutron.NetworkV2{
		Id:        "2",
		Name:      "ext-no-az-net",
		SubnetIds: []string{"ext-sub-net"},
		External:  true,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.srv.OpenstackSvc.Neutron.AddNeutronModel(model)
	s.srv.OpenstackSvc.Nova.AddNeutronModel(model)

	// Set floating ip to ensure we try to find the external
	// network.
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "no-az-net", // az = nova
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("allocate-public-ip=true")
	inst, _, _, err := testing.StartInstanceWithConstraints(c, s.env, s.ControllerUUID, "100", cons)
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceNetworkNoExternalNetInAZ(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "net", // az = nova
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("allocate-public-ip=true")
	_, _, _, err = testing.StartInstanceWithConstraints(c, s.env, s.ControllerUUID, "100", cons)
	c.Assert(err, tc.ErrorMatches, "cannot allocate a public IP as needed: could not find an external network in availability zone.*")
}

func (s *localServerSuite) TestStartInstancePortSecurityEnabled(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "net",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Assert(err, tc.ErrorIsNil)
	novaClient := openstack.GetNovaClient(s.env)
	detail, err := novaClient.GetServer(string(inst.Id()))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(detail.Groups, tc.NotNil)
}

func (s *localServerSuite) TestStartInstancePortSecurityDisabled(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		"network": "net-disabled",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Assert(err, tc.ErrorIsNil)
	novaClient := openstack.GetNovaClient(s.env)
	detail, err := novaClient.GetServer(string(inst.Id()))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(detail.Groups, tc.IsNil)
}

func (s *localServerSuite) TestStartInstanceGetServerFail(c *tc.C) {
	// Force an error in waitForActiveServerDetails
	cleanup := s.srv.Nova.RegisterControlPoint(
		"server",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("GetServer failed on purpose")
		},
	)
	defer cleanup()
	inst, _, _, err := testing.StartInstance(c, s.env, s.ControllerUUID, "100")
	c.Check(inst, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "cannot run instance: "+
		"request \\(.*/servers\\) returned unexpected status: "+
		"500; error info: .*GetServer failed on purpose")
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}

func (s *localServerSuite) TestStartInstanceWaitForActiveDetails(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})

	s.srv.Nova.SetServerStatus(nova.StatusBuild)
	defer s.srv.Nova.SetServerStatus("")

	// Make time advance in zero time
	clk := testclock.NewClock(time.Time{})
	clock := testclock.AutoAdvancingClock{Clock: clk, Advance: clk.Advance}
	env.(*openstack.Environ).SetClock(&clock)

	inst, _, _, err := testing.StartInstance(c, env, s.ControllerUUID, "100")
	c.Check(inst, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "cannot run instance: max duration exceeded: instance .* has status BUILD")

	// Ensure that the started instance got terminated.
	insts, err := env.AllInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts, tc.HasLen, 0, tc.Commentf("expected launched instance to be terminated if stuck in BUILD state"))
}

func (s *localServerSuite) TestStartInstanceDeletesMachineSecurityGroupOnInstanceCreateFailure(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})

	// Force an error in waitForActiveServerDetails
	cleanup := s.srv.Nova.RegisterControlPoint(
		"server",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("GetServer failed on purpose")
		},
	)
	defer cleanup()
	inst, _, _, err := testing.StartInstance(c, env, s.ControllerUUID, "100")
	c.Check(inst, tc.IsNil)
	c.Assert(err, tc.NotNil)

	assertSecurityGroups(c, env, []string{
		fmt.Sprintf("juju-%s-%s", coretesting.ControllerTag.Id(), coretesting.ModelTag.Id()),
		"default",
	})
}

func (s *localServerSuite) TestStartInstanceDeletesSecurityGroupsOnFailure(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})

	s.srv.Nova.SetServerStatus(nova.StatusBuild)
	defer s.srv.Nova.SetServerStatus("")

	// Make time advance in zero time
	clk := testclock.NewClock(time.Time{})
	clock := testclock.AutoAdvancingClock{Clock: clk, Advance: clk.Advance}
	env.(*openstack.Environ).SetClock(&clock)

	_, _, _, err := testing.StartInstance(c, env, s.ControllerUUID, "100")
	c.Assert(err, tc.NotNil)

	assertSecurityGroups(c, env, []string{
		fmt.Sprintf("juju-%s-%s", coretesting.ControllerTag.Id(), coretesting.ModelTag.Id()),
		"default",
	})
}

func assertSecurityGroups(c *tc.C, env environs.Environ, expected []string) {
	neutronClient := openstack.GetNeutronClient(env)
	groups, err := neutronClient.ListSecurityGroupsV2()
	c.Assert(err, tc.ErrorIsNil)
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

func assertPorts(c *tc.C, env environs.Environ, expected []portAssertion) {
	neutronClient := openstack.GetNeutronClient(env)
	ports, err := neutronClient.ListPortsV2()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ports, tc.HasLen, len(expected))
	for k, port := range ports {
		c.Assert(port.Name, tc.HasPrefix, expected[k].NamePrefix)
		c.Assert(port.FixedIPs, tc.HasLen, len(expected[k].SubnetIDs))
		for i, ip := range port.FixedIPs {
			c.Assert(ip.SubnetID, tc.Equals, expected[k].SubnetIDs[i])
		}
	}
}

func assertInstanceIds(c *tc.C, env environs.Environ, ctx context.Context, expected ...instance.Id) {
	allInstances, err := env.AllRunningInstances(ctx)
	c.Assert(err, tc.ErrorIsNil)
	instIds := make([]instance.Id, len(allInstances))
	for i, inst := range allInstances {
		instIds[i] = inst.Id()
	}
	c.Assert(instIds, tc.SameContents, expected)
}

func (s *localServerSuite) TestStopInstance(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})
	instanceName := "100"
	inst, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, instanceName)
	// Openstack now has three security groups for the server, the default
	// group, one group for the entire environment, and another for the
	// new instance.
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, instanceName),
	}
	assertSecurityGroups(c, env, allSecurityGroups)
	err := env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
	// The security group for this instance is now removed.
	assertSecurityGroups(c, env, []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
	})
}

// Due to bug #1300755 it can happen that the security group intended for
// an instance is also used as the common security group of another
// environment. If this is the case, the attempt to delete the instance's
// security group fails but StopInstance succeeds.
func (s *localServerSuite) TestStopInstanceSecurityGroupNotDeleted(c *tc.C) {
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
	inst, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, instanceName)
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

	err := env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
	assertSecurityGroups(c, env, allSecurityGroups)
}

func (s *localServerSuite) TestDestroyEnvironmentDeletesSecurityGroupsFWModeInstance(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwInstance})
	instanceName := "100"
	testing.AssertStartInstance(c, env, s.ControllerUUID, instanceName)
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-%v", s.ControllerUUID, modelUUID, instanceName),
	}
	assertSecurityGroups(c, env, allSecurityGroups)
	err := env.Destroy(c.Context())
	c.Check(err, tc.ErrorIsNil)
	assertSecurityGroups(c, env, []string{"default"})
}

func (s *localServerSuite) TestDestroyEnvironmentDeletesSecurityGroupsFWModeGlobal(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"firewall-mode": config.FwGlobal})
	instanceName := "100"
	testing.AssertStartInstance(c, env, s.ControllerUUID, instanceName)
	modelUUID := env.Config().UUID()
	allSecurityGroups := []string{
		"default", fmt.Sprintf("juju-%v-%v", s.ControllerUUID, modelUUID),
		fmt.Sprintf("juju-%v-%v-global", s.ControllerUUID, modelUUID),
	}
	assertSecurityGroups(c, env, allSecurityGroups)
	err := env.Destroy(c.Context())
	c.Check(err, tc.ErrorIsNil)
	assertSecurityGroups(c, env, []string{"default"})
}

func (s *localServerSuite) TestDestroyController(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"uuid": uuid.MustNewUUID().String()})
	controllerEnv := s.env

	controllerInstanceName := "100"
	testing.AssertStartInstance(c, controllerEnv, s.ControllerUUID, controllerInstanceName)
	hostedModelInstanceName := "200"
	testing.AssertStartInstance(c, env, s.ControllerUUID, hostedModelInstanceName)
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

	err := controllerEnv.DestroyController(c.Context(), s.ControllerUUID)
	c.Check(err, tc.ErrorIsNil)
	assertSecurityGroups(c, controllerEnv, []string{"default"})
	assertInstanceIds(c, env, c.Context())
	assertInstanceIds(c, controllerEnv, c.Context())
}

func (s *localServerSuite) TestDestroyHostedModel(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{"uuid": uuid.MustNewUUID().String()})
	controllerEnv := s.env

	controllerInstanceName := "100"
	controllerInstance, _ := testing.AssertStartInstance(c, controllerEnv, s.ControllerUUID, controllerInstanceName)
	hostedModelInstanceName := "200"
	testing.AssertStartInstance(c, env, s.ControllerUUID, hostedModelInstanceName)
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

	err := env.Destroy(c.Context())
	c.Check(err, tc.ErrorIsNil)
	assertSecurityGroups(c, controllerEnv, allControllerSecurityGroups)
	assertInstanceIds(c, env, c.Context())
	assertInstanceIds(c, controllerEnv, c.Context(), controllerInstance.Id())
}

func (s *localServerSuite) TestDestroyControllerSpaceConstraints(c *tc.C) {
	uuid := uuid.MustNewUUID().String()
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
		SubnetsToZones: []map[network.Id][]string{
			{
				"999-01": {"zone-0"},
			},
		},
	}
	_, err := testing.StartInstanceWithParams(c, env, controllerInstanceName, params)
	c.Assert(err, tc.ErrorIsNil)
	assertPorts(c, env, []portAssertion{
		{NamePrefix: fmt.Sprintf("juju-%s-", uuid), SubnetIDs: []string{"999-01"}},
	})

	// The openstack runtime would assign a device_id to a port when it's
	// assigned to an instance. To ensure that all ports are correctly removed
	// when destroying and so we can exercise all the code paths we have to
	// replicate that piece of code.
	// When moving to mocking of providers, this shouldn't be need or required.
	s.assignDeviceIdToPort(c, "1", "1")

	err = controllerEnv.DestroyController(c.Context(), s.ControllerUUID)
	c.Check(err, tc.ErrorIsNil)
	assertSecurityGroups(c, controllerEnv, []string{"default"})
	assertInstanceIds(c, env, c.Context())
	assertInstanceIds(c, controllerEnv, c.Context())
	assertPorts(c, env, []portAssertion{})
}

func (s *localServerSuite) assignDeviceIdToPort(c *tc.C, portId, deviceId string) {
	err := s.srv.Nova.AddOSInterface(deviceId, nova.OSInterface{
		FixedIPs: []nova.PortFixedIP{
			{
				IPAddress: "10.0.0.1",
			},
		},
		IPAddress: "10.0.0.1",
	})
	c.Assert(err, tc.ErrorIsNil)

	model := s.srv.Neutron.NeutronModel()
	port, err := model.Port("1")
	c.Assert(err, tc.ErrorIsNil)
	err = model.RemovePort("1")
	c.Assert(err, tc.ErrorIsNil)
	port.DeviceId = "1"
	err = model.AddPort(*port)
	c.Assert(err, tc.ErrorIsNil)
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

func (s *localServerSuite) TestInstanceStatus(c *tc.C) {
	// goose's test service always returns ACTIVE state.
	inst, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	c.Assert(inst.Status(c.Context()).Status, tc.Equals, status.Running)
	err := s.env.StopInstances(c.Context(), inst.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestAllRunningInstancesFloatingIP(c *tc.C) {
	env := s.openEnviron(c, coretesting.Attrs{
		"network": "private_999",
	})
	cons := constraints.MustParse("allocate-public-ip=true")
	inst0, _ := testing.AssertStartInstanceWithConstraints(c, env, s.ControllerUUID, "100", cons)
	inst1, _ := testing.AssertStartInstanceWithConstraints(c, env, s.ControllerUUID, "101", cons)
	defer func() {
		err := env.StopInstances(c.Context(), inst0.Id(), inst1.Id())
		c.Assert(err, tc.ErrorIsNil)
	}()

	allInstances, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	for _, inst := range allInstances {
		c.Assert(*openstack.InstanceFloatingIP(inst), tc.Equals, fmt.Sprintf("10.0.0.%v", inst.Id()))
	}
}

func (s *localServerSuite) assertInstancesGathering(c *tc.C, withFloatingIP bool) {
	env := s.openEnviron(c, coretesting.Attrs{})

	var cons constraints.Value
	if withFloatingIP {
		cons = constraints.MustParse("allocate-public-ip=true")
	}
	inst0, _ := testing.AssertStartInstanceWithConstraints(c, env, s.ControllerUUID, "100", cons)
	id0 := inst0.Id()
	inst1, _ := testing.AssertStartInstanceWithConstraints(c, env, s.ControllerUUID, "101", cons)
	id1 := inst1.Id()
	defer func() {
		err := env.StopInstances(c.Context(), inst0.Id(), inst1.Id())
		c.Assert(err, tc.ErrorIsNil)
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
		insts, err := env.Instances(c.Context(), ids)
		c.Assert(err, tc.ErrorIs, test.err)
		if errors.Is(err, environs.ErrNoInstances) {
			c.Assert(insts, tc.HasLen, 0)
		} else {
			c.Assert(insts, tc.HasLen, len(test.ids))
		}
		for j, inst := range insts {
			if ids[j] != "" {
				c.Assert(inst.Id(), tc.Equals, ids[j])
				if withFloatingIP {
					c.Assert(*openstack.InstanceFloatingIP(inst), tc.Equals, fmt.Sprintf("10.0.0.%v", inst.Id()))
				} else {
					c.Assert(openstack.InstanceFloatingIP(inst), tc.IsNil)
				}
			} else {
				c.Assert(inst, tc.IsNil)
			}
		}
	}
}

func (s *localServerSuite) TestInstancesGathering(c *tc.C) {
	s.assertInstancesGathering(c, false)
}

func (s *localServerSuite) TestInstancesGatheringWithFloatingIP(c *tc.C) {
	s.assertInstancesGathering(c, true)
}

func (s *localServerSuite) TestInstancesShutoffSuspended(c *tc.C) {
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
	stateInst1, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	stateInst2, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "101")
	defer func() {
		err := s.env.StopInstances(c.Context(), stateInst1.Id(), stateInst2.Id())
		c.Assert(err, tc.ErrorIsNil)
	}()

	twoInstances, err := s.env.Instances(c.Context(), []instance.Id{stateInst1.Id(), stateInst2.Id()})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(twoInstances, tc.HasLen, 2)
	c.Assert(twoInstances[0].Status(c.Context()).Message, tc.Equals, nova.StatusShutoff)
	c.Assert(twoInstances[1].Status(c.Context()).Message, tc.Equals, nova.StatusSuspended)
}

func (s *localServerSuite) TestInstancesErrorResponse(c *tc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Nova.RegisterControlPoint(
		"server",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("strange error not instance")
		},
	)
	defer cleanup()

	oneInstance, err := s.env.Instances(c.Context(), []instance.Id{"1"})
	c.Check(oneInstance, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "(?s).*strange error not instance.*")
}

func (s *localServerSuite) TestInstancesMultiErrorResponse(c *tc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1425242")

	cleanup := s.srv.Nova.RegisterControlPoint(
		"matchServers",
		func(sc hook.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("strange error no instances")
		},
	)
	defer cleanup()

	twoInstances, err := s.env.Instances(c.Context(), []instance.Id{"1", "2"})
	c.Check(twoInstances, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "(?s).*strange error no instances.*")
}

// TODO (wallyworld) - this test was copied from the ec2 provider.
// It should be moved to environs.jujutests.Tests.
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	// Check that ControllerInstances returns the ID of the bootstrap machine.
	ids, err := s.env.ControllerInstances(c.Context(), s.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.HasLen, 1)

	allInstances, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInstances, tc.HasLen, 1)
	c.Check(allInstances[0].Id(), tc.Equals, ids[0])

	addresses, err := allInstances[0].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.Not(tc.HasLen), 0)

	// TODO(wallyworld) - 2013-03-01 bug=1137005
	// The nova test double needs to be updated to support retrieving instance userData.
	// Until then, we can't check the cloud init script was generated correctly.
	// When we can, we should also check cloudinit for a non-manager node (as in the
	// ec2 tests).
}

func (s *localServerSuite) TestGetImageMetadataSources(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	// Create a config that matches s.TestConfig but with the specified stream.
	attrs := coretesting.Attrs{}
	env := s.openEnviron(c, attrs)

	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 3)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		imageURL, err := source.URL("")
		c.Assert(err, tc.ErrorIsNil)
		urls[i] = imageURL
	}
	// The image-metadata-url ends with "/juju-dist-test/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist-test/"), tc.IsTrue)
	// The product-streams URL ends with "/imagemetadata".
	c.Check(strings.HasSuffix(urls[1], "/imagemetadata/"), tc.IsTrue)
	c.Assert(urls[2], tc.HasPrefix, imagemetadata.DefaultUbuntuBaseURL)
}

func (s *localServerSuite) TestGetImageMetadataSourcesNoProductStreams(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.PatchValue(openstack.MakeServiceURL, func(client.AuthenticatingClient, string, string, []string) (string, error) {
		return "", errors.New("cannae do it captain")
	})
	env := s.Open(c, c.Context(), s.env.Config())
	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 2)

	// Check that data sources are in the right order
	c.Check(sources[0].Description(), tc.Equals, "image-metadata-url")
	c.Check(sources[1].Description(), tc.Equals, "default ubuntu cloud images")
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.PatchValue(&tools.DefaultBaseURL, "")

	env := s.Open(c, c.Context(), s.env.Config())
	sources, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 2)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		metadataURL, err := source.URL("")
		c.Assert(err, tc.ErrorIsNil)
		urls[i] = metadataURL
	}
	// The agent-metadata-url ends with "/juju-dist-test/tools/".
	c.Check(strings.HasSuffix(urls[0], "/juju-dist-test/tools/"), tc.IsTrue)
	// Check that the URL from keystone parses.
	_, err = url.Parse(urls[1])
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestSupportsNetworking(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	_, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)
}

func (s *localServerSuite) prepareNetworkingEnviron(c *tc.C, cfg *config.Config) environs.NetworkingEnviron {
	env := s.Open(c, c.Context(), cfg)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, tc.IsTrue)
	return netenv
}

func (s *localServerSuite) TestSubnetsFindAll(c *tc.C) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	// the environ is opened with network:"private_999" which maps to network id "999"
	obtainedSubnets, err := env.Subnets(c.Context(), []network.Id{})
	c.Assert(err, tc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackSubnets, err := neutronClient.ListSubnetsV2()
	c.Assert(err, tc.ErrorIsNil)

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
		c.Assert(err, tc.ErrorIsNil)
		expectedSubnetMap[network.Id(subnet.Id)] = network.SubnetInfo{
			CIDR:              subnet.Cidr,
			ProviderId:        network.Id(subnet.Id),
			ProviderNetworkId: network.Id(subnet.NetworkId),
			VLANTag:           0,
			AvailabilityZones: net.AvailabilityZones,
			ProviderSpaceId:   "",
		}
	}

	c.Check(obtainedSubnetMap, tc.DeepEquals, expectedSubnetMap)
}

func (s *localServerSuite) TestSubnetsFindAllWithExternal(c *tc.C) {
	cfg := s.env.Config()
	cfg, err := cfg.Apply(map[string]interface{}{"external-network": "ext-net"})
	c.Assert(err, tc.ErrorIsNil)
	env := s.prepareNetworkingEnviron(c, cfg)
	// private_999 is the internal network, 998 is the external network
	// the environ is opened with network:"private_999" which maps to network id "999"
	obtainedSubnets, err := env.Subnets(c.Context(), []network.Id{})
	c.Assert(err, tc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackSubnets, err := neutronClient.ListSubnetsV2()
	c.Assert(err, tc.ErrorIsNil)

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
		c.Assert(err, tc.ErrorIsNil)
		expectedSubnetMap[network.Id(subnets.Id)] = network.SubnetInfo{
			CIDR:              subnets.Cidr,
			ProviderId:        network.Id(subnets.Id),
			ProviderNetworkId: network.Id(subnets.NetworkId),
			VLANTag:           0,
			AvailabilityZones: net.AvailabilityZones,
			ProviderSpaceId:   "",
		}
	}

	c.Check(obtainedSubnetMap, tc.DeepEquals, expectedSubnetMap)
}

func (s *localServerSuite) TestFindNetworksInternal(c *tc.C) {
	s.testFindNetworks(c, true)
}

func (s *localServerSuite) TestFindNetworksExternal(c *tc.C) {
	s.testFindNetworks(c, false)
}

func (s *localServerSuite) testFindNetworks(c *tc.C, internal bool) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	obtainedNetworks, err := openstack.FindNetworks(env, internal)
	c.Assert(err, tc.ErrorIsNil)
	neutronClient := openstack.GetNeutronClient(s.env)
	openstackNetworks, err := neutronClient.ListNetworksV2()
	c.Assert(err, tc.ErrorIsNil)

	expectedNetworks := set.NewStrings()
	for _, oNet := range openstackNetworks {
		if oNet.External == internal {
			continue
		}
		expectedNetworks.Add(oNet.Name)
	}

	c.Check(obtainedNetworks.Values(), tc.SameContents, expectedNetworks.Values())

}

func (s *localServerSuite) TestSubnetsWithMissingSubnet(c *tc.C) {
	env := s.prepareNetworkingEnviron(c, s.env.Config())
	subnets, err := env.Subnets(c.Context(), []network.Id{"missing"})
	c.Assert(err, tc.ErrorMatches, `failed to find the following subnet ids: \[missing\]`)
	c.Assert(subnets, tc.HasLen, 0)
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *tc.C) {
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "")
	env := s.Open(c, c.Context(), s.env.Config())

	// An error occurs if no suitable image is found.
	_, err := openstack.FindInstanceSpec(env, corebase.MakeDefaultBase("ubuntu", "15.04"), "amd64", "mem=1G", nil)
	c.Assert(err, tc.ErrorMatches, `no metadata for "ubuntu@15.04" images in some-region with arch amd64`)
}

func (s *localServerSuite) TestConstraintsValidator(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 virt-type=lxd")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unsupported, tc.SameContents, []string{"cpu-power"})
}

func (s *localServerSuite) TestConstraintsValidatorVocab(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, tc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")

	cons = constraints.MustParse("virt-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta("invalid constraint value: virt-type=foo\nvalid values are: kvm lxd"))
}

func (s *localServerSuite) TestConstraintsMerge(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	consA := constraints.MustParse("arch=amd64 mem=1G root-disk=10G")
	consB := constraints.MustParse("instance-type=m1.small")
	cons, err := validator.Merge(consA, consB)
	c.Assert(err, tc.ErrorIsNil)
	// NOTE: root-disk and instance-type constraints are checked by PrecheckInstance.
	c.Assert(cons, tc.DeepEquals, constraints.MustParse("arch=amd64 instance-type=m1.small root-disk=10G"))
}

func (s *localServerSuite) TestFindImageInstanceConstraint(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:   "image-id",
		Arch: "amd64",
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTSBase(), "amd64", "instance-type=m1.tiny",
		imageMetadata,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.InstanceType.Name, tc.Equals, "m1.tiny")
}

func (s *localServerSuite) TestFindInstanceImageConstraintHypervisor(c *tc.C) {
	testVirtType := "qemu"
	env := s.Open(c, c.Context(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:       "image-id",
		Arch:     "amd64",
		VirtType: testVirtType,
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTSBase(), "amd64", "virt-type="+testVirtType,
		imageMetadata,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.InstanceType.VirtType, tc.NotNil)
	c.Assert(*spec.InstanceType.VirtType, tc.Equals, testVirtType)
	c.Assert(spec.InstanceType.Name, tc.Equals, "m1.small")
}

func (s *localServerSuite) TestFindInstanceImageWithHypervisorNoConstraint(c *tc.C) {
	testVirtType := "qemu"
	env := s.Open(c, c.Context(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:       "image-id",
		Arch:     "amd64",
		VirtType: testVirtType,
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTSBase(), "amd64", "",
		imageMetadata,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.InstanceType.VirtType, tc.NotNil)
	c.Assert(*spec.InstanceType.VirtType, tc.Equals, testVirtType)
	c.Assert(spec.InstanceType.Name, tc.Equals, "m1.small")
}

func (s *localServerSuite) TestFindInstanceNoConstraint(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:   "image-id",
		Arch: "amd64",
	}}

	spec, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTSBase(), "amd64", "",
		imageMetadata,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.InstanceType.VirtType, tc.IsNil)
	c.Assert(spec.InstanceType.Name, tc.Equals, "m1.small")
}

func (s *localServerSuite) TestFindImageInvalidInstanceConstraint(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:   "image-id",
		Arch: "amd64",
	}}
	_, err := openstack.FindInstanceSpec(
		env, jujuversion.DefaultSupportedLTSBase(), "amd64", "instance-type=m1.large",
		imageMetadata,
	)
	c.Assert(err, tc.ErrorMatches, `no instance types in some-region matching constraints "arch=amd64 instance-type=m1.large"`)
}

func (s *localServerSuite) TestPrecheckInstanceValidInstanceType(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	cons := constraints.MustParse("instance-type=m1.small")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Constraints: cons})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	cons := constraints.MustParse("instance-type=m1.large")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Constraints: cons})
	c.Assert(err, tc.ErrorMatches, `invalid Openstack flavour "m1.large" specified`)
}

func (s *localServerSuite) TestPrecheckInstanceInvalidRootDiskConstraint(c *tc.C) {
	env := s.Open(c, c.Context(), s.env.Config())
	cons := constraints.MustParse("instance-type=m1.small root-disk=10G")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Constraints: cons})
	c.Assert(err, tc.ErrorMatches, `constraint root-disk cannot be specified with instance-type unless constraint root-disk-source=volume`)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZone(c *tc.C) {
	placement := "zone=test-available"
	err := s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Placement: placement})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *tc.C) {
	placement := "zone=test-unavailable"
	err := s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Placement: placement})
	c.Assert(err, tc.ErrorMatches, `zone "test-unavailable" is unavailable`)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *tc.C) {
	placement := "zone=test-unknown"
	err := s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Placement: placement})
	c.Assert(err, tc.ErrorMatches, `availability zone "test-unknown" not valid`)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZonesUnsupported(c *tc.C) {
	s.srv.Nova.SetAvailabilityZones() // no availability zone support
	placement := "zone=test-unknown"
	err := s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{Base: jujuversion.DefaultSupportedLTSBase(), Placement: placement})
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *localServerSuite) TestPrecheckInstanceVolumeAvailZonesNoPlacement(c *tc.C) {
	s.testPrecheckInstanceVolumeAvailZones(c, "")
}

func (s *localServerSuite) TestPrecheckInstanceVolumeAvailZonesSameZonePlacement(c *tc.C) {
	s.testPrecheckInstanceVolumeAvailZones(c, "zone=az1")
}

func (s *localServerSuite) testPrecheckInstanceVolumeAvailZones(c *tc.C, placement string) {
	s.srv.Nova.SetAvailabilityZones(
		nova.AvailabilityZone{
			Name: "az1",
			State: nova.AvailabilityZoneState{
				Available: true,
			},
		},
	)

	_, err := s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:              jujuversion.DefaultSupportedLTSBase(),
		Placement:         placement,
		VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestPrecheckInstanceAvailZonesConflictsVolume(c *tc.C) {
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

	_, err := s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:              jujuversion.DefaultSupportedLTSBase(),
		Placement:         "zone=az2",
		VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=az2": cannot create instance in zone "az2", as this will prevent attaching the requested disks in zone "az1"`)
}

func (s *localServerSuite) TestDeriveAvailabilityZones(c *tc.C) {
	placement := "zone=test-available"
	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{
			Placement: placement,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"test-available"})
}

func (s *localServerSuite) TestDeriveAvailabilityZonesUnavailable(c *tc.C) {
	placement := "zone=test-unavailable"
	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{
			Placement: placement,
		})
	c.Assert(err, tc.ErrorMatches, `zone "test-unavailable" is unavailable`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *localServerSuite) TestDeriveAvailabilityZonesUnknown(c *tc.C) {
	placement := "zone=test-unknown"
	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{
			Placement: placement,
		})
	c.Assert(err, tc.ErrorMatches, `availability zone "test-unknown" not valid`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *localServerSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *tc.C) {
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

	_, err := s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az2",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{
			VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"az2"})
}

func (s *localServerSuite) TestDeriveAvailabilityZonesConflictsVolume(c *tc.C) {
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

	_, err := s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	env := s.env.(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{
			Placement:         "zone=az2",
			VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
		})
	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=az2": cannot create instance in zone "az2", as this will prevent attaching the requested disks in zone "az1"`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *localServerSuite) TestValidateImageMetadata(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	env := s.Open(c, c.Context(), s.env.Config())
	params, err := env.(simplestreams.ImageMetadataValidator).ImageMetadataLookupParams("some-region")
	c.Assert(err, tc.ErrorIsNil)
	params.Sources, err = environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	params.Release = "13.04"
	imageIDs, _, err := imagemetadata.ValidateImageMetadata(c.Context(), ss, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageIDs, tc.SameContents, []string{"id-y"})
}

func (s *localServerSuite) TestImageMetadataSourceOrder(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	src := func(env environs.Environ) (simplestreams.DataSource, error) {
		return ss.NewDataSource(simplestreams.Config{
			Description:          "my datasource",
			BaseURL:              "file:///bar",
			HostnameVerification: false,
			Priority:             simplestreams.CUSTOM_CLOUD_DATA}), nil
	}
	environs.RegisterUserImageDataSourceFunc("my func", src)
	defer environs.UnregisterImageDataSourceFunc("my func")
	env := s.Open(c, c.Context(), s.env.Config())
	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	var sourceIds []string
	for _, s := range sources {
		sourceIds = append(sourceIds, s.Description())
	}
	c.Assert(sourceIds, tc.DeepEquals, []string{
		"image-metadata-url", "my datasource", "keystone catalog", "default ubuntu cloud images"})
}

// TestEnsureGroup checks that when creating a duplicate security group, the existing group is
// returned.
func (s *localServerSuite) TestEnsureGroup(c *tc.C) {
	group, err := openstack.EnsureGroup(s.env, c.Context(), "test group", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(group.Name, tc.Equals, "test group")
	id := group.Id

	// Do it again and check that the existing group is returned
	group, err = openstack.EnsureGroup(s.env, c.Context(), "test group", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(group.Id, tc.Equals, id)
	c.Assert(group.Name, tc.Equals, "test group")
}

// TestEnsureModelGroup checks that when creating a model security group, the
// group is created with the correct ingress rules
func (s *localServerSuite) TestEnsureModelGroup(c *tc.C) {
	group, err := openstack.EnsureGroup(s.env, c.Context(), "test model group", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(group.Name, tc.Equals, "test model group")

	stringRules := make([]string, 0, len(group.Rules))
	for _, rule := range group.Rules {
		// Skip the default Security Group Rules created by Neutron
		if rule.Direction == "egress" {
			continue
		}
		var minInt int
		if rule.PortRangeMin != nil {
			minInt = *rule.PortRangeMin
		}
		var maxInt int
		if rule.PortRangeMax != nil {
			maxInt = *rule.PortRangeMax
		}
		ruleStr := fmt.Sprintf("%s %s %d %d %s %s %s",
			rule.Direction,
			*rule.IPProtocol,
			minInt, maxInt,
			rule.RemoteIPPrefix,
			rule.EthernetType,
			rule.ParentGroupId,
		)
		stringRules = append(stringRules, ruleStr)
	}
	// We don't care about the ordering, so we sort the result, and compare it.
	expectedRules := []string{
		fmt.Sprintf(`ingress tcp 1 65535  IPv6 %s`, group.Id),
		fmt.Sprintf(`ingress tcp 1 65535  IPv4 %s`, group.Id),
		fmt.Sprintf(`ingress udp 1 65535  IPv6 %s`, group.Id),
		fmt.Sprintf(`ingress udp 1 65535  IPv4 %s`, group.Id),
		fmt.Sprintf(`ingress icmp 0 0  IPv6 %s`, group.Id),
		fmt.Sprintf(`ingress icmp 0 0  IPv4 %s`, group.Id),
	}
	sort.Strings(stringRules)
	sort.Strings(expectedRules)
	c.Check(stringRules, tc.DeepEquals, expectedRules)
}

// TestGetSecurityGroupByName checks that you receive the group you expected.  getSecurityGroupByName()
// is used by the firewaller when opening and closing ports.  Unit test in response to bug 1675799.
func (s *localServerSuite) TestGetSecurityGroupByName(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)
	machineName1 := openstack.MachineGroupName(s.env, s.ControllerUUID, "1")
	group1, err := openstack.EnsureGroup(s.env, c.Context(),
		machineName1, false)
	c.Assert(err, tc.ErrorIsNil)
	machineName2 := openstack.MachineGroupName(s.env, s.ControllerUUID, "2")
	group2, err := openstack.EnsureGroup(s.env, c.Context(),
		machineName2, false)
	c.Assert(err, tc.ErrorIsNil)
	_, err = openstack.EnsureGroup(s.env, c.Context(), openstack.MachineGroupName(s.env, s.ControllerUUID, "11"), false)
	c.Assert(err, tc.ErrorIsNil)
	_, err = openstack.EnsureGroup(s.env, c.Context(), openstack.MachineGroupName(s.env, s.ControllerUUID, "12"), false)
	c.Assert(err, tc.ErrorIsNil)

	groupResult, err := openstack.GetSecurityGroupByName(s.env, c.Context(), machineName1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(group1.Id, tc.Equals, groupResult.Id)

	groupResult, err = openstack.GetSecurityGroupByName(s.env, c.Context(), machineName2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(group2.Id, tc.Equals, groupResult.Id)

	groupResult, err = openstack.GetSecurityGroupByName(s.env, c.Context(), "juju-unknown-machine-name")
	c.Assert(err, tc.ErrorMatches, "failed to find security group with name: juju-unknown-machine-name")
}

func (s *localServerSuite) TestPorts(c *tc.C) {
	inst1, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "1")
	c.Assert(inst1, tc.NotNil)
	defer func() { _ = s.env.StopInstances(c.Context(), inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "2")
	c.Assert(inst2, tc.NotNil)
	fwInst2, ok := inst2.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)
	defer func() { _ = s.env.StopInstances(c.Context(), inst2.Id()) }()

	// Open some ports and check they're there.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	// Check there's no crosstalk to another machine
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check that opening the same port again is ok.
	oldRules, err := fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.DeepEquals, oldRules)

	// Check that opening the same port again and another port is ok.
	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	err = fwInst2.ClosePorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	// Check that we can close ports and that there's no crosstalk.
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check that we can close multiple ports.
	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	// Check that we can close ports that aren't there.
	err = fwInst2.ClosePorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("600-700/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check errors when acting on environment.
	fwEnv, ok := s.env.(environs.Firewaller)
	c.Assert(ok, tc.Equals, true)
	err = fwEnv.OpenPorts(c.Context(), firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "instance" for opening ports on model`)

	err = fwEnv.ClosePorts(c.Context(), firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "instance" for closing ports on model`)

	_, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "instance" for retrieving ingress rules from model`)
}

func (s *localServerSuite) TestGlobalPorts(c *tc.C) {
	// Change configuration.
	oldConfig := s.env.Config()
	defer func() {
		err := s.env.SetConfig(c.Context(), oldConfig)
		c.Assert(err, tc.ErrorIsNil)
	}()

	attrs := s.env.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	newConfig, err := s.env.Config().Apply(attrs)
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), newConfig)
	c.Assert(err, tc.ErrorIsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "1")
	defer func() { _ = s.env.StopInstances(c.Context(), inst1.Id()) }()

	fwEnv, ok := s.env.(environs.Firewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "2")
	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)
	defer func() { _ = s.env.StopInstances(c.Context(), inst2.Id()) }()

	err = fwEnv.OpenPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check closing some ports.
	err = fwEnv.ClosePorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check that we can close ports that aren't there.
	err = fwEnv.ClosePorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)
	// Check errors when acting on instances.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "global" for opening ports on instance`)

	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "global" for closing ports on instance`)

	_, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "global" for retrieving ingress rules from instance`)
}

func (s *localServerSuite) TestModelPorts(c *tc.C) {
	err := bootstrap.Bootstrap(s.BootstrapContext, s.env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	fwModelEnv, ok := s.env.(models.ModelFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	err = fwModelEnv.OpenModelPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	// Check closing some ports.
	err = fwModelEnv.CloseModelPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	// Check that we can close ports that aren't there.
	err = fwModelEnv.CloseModelPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	// Cleanup
	err = fwModelEnv.CloseModelPorts(c.Context(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) TestIngressRulesWithPartiallyMatchingCIDRs(c *tc.C) {
	inst1, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "1")
	c.Assert(inst1, tc.NotNil)
	defer func() { _ = s.env.StopInstances(c.Context(), inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	// Open ports with different CIDRs. Check that rules with same port range
	// get merged.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp")), // open to 0.0.0.0/0
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Open same port with different CIDRs and check that the CIDR gets
	// appended to the existing rule's CIDR list.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24", "192.168.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Close port on a subset of the CIDRs and ensure that that CIDR gets
	// removed from the ingress rules
	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Remove all CIDRs from the rule and check that rules without CIDRs
	// get dropped.
	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
}

func (s *localServerSuite) TestStartInstanceWithEmptyNonceFails(c *tc.C) {
	// Check that we get a consistent error when asking for an instance without
	// a valid machine config.
	machineId := "4"
	apiInfo := testing.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, "",
		"released", corebase.MakeDefaultBase("ubuntu", "22.04"), apiInfo)
	c.Assert(err, tc.ErrorIsNil)

	possibleTools := coretools.List(envtesting.AssertUploadFakeToolsVersions(
		c, s.toolsMetadataStorage, "released", semversion.MustParseBinary("5.4.5-ubuntu-amd64"),
	))
	fakeCallback := func(_ context.Context, _ status.Status, _ string, _ map[string]interface{}) error {
		return nil
	}
	params := environs.StartInstanceParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		StatusCallback: fakeCallback,
	}
	err = testing.SetImageMetadata(
		c,
		s.env,
		simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory()),
		[]string{"22.04"},
		[]string{"amd64"},
		&params.ImageMetadata,
	)
	c.Check(err, tc.ErrorIsNil)
	result, err := s.env.StartInstance(c.Context(), params)
	if result != nil && result.Instance != nil {
		err := s.env.StopInstances(c.Context(), result.Instance.Id())
		c.Check(err, tc.ErrorIsNil)
	}
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, ".*missing machine nonce")
}

func (s *localServerSuite) TestPrechecker(c *tc.C) {
	// All implementations of InstancePrechecker should
	// return nil for empty constraints (excluding the
	// manual provider).
	err := s.env.PrecheckInstance(c.Context(),
		environs.PrecheckInstanceParams{
			Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localServerSuite) assertStartInstanceDefaultSecurityGroup(c *tc.C, useDefault bool) {
	s.CleanupSuite.PatchValue(&s.TestConfig, s.TestConfig.Merge(coretesting.Attrs{
		"use-default-secgroup": useDefault,
	}))

	// Tests uses Prepare, so destroy first.
	err := environs.Destroy(s.env.Config().Name(), s.env, c.Context(), s.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)
	s.env = s.Prepare(c)
	err = bootstrap.Bootstrap(s.BootstrapContext, s.env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	inst, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	// Check whether the instance has the default security group assigned.
	novaClient := openstack.GetNovaClient(s.env)
	groups, err := novaClient.GetServerSecurityGroups(string(inst.Id()))
	c.Assert(err, tc.ErrorIsNil)
	defaultGroupFound := false
	for _, group := range groups {
		if group.Name == "default" {
			defaultGroupFound = true
			break
		}
	}
	c.Assert(defaultGroupFound, tc.Equals, useDefault)
}

func (s *localServerSuite) TestStartInstanceWithDefaultSecurityGroup(c *tc.C) {
	s.assertStartInstanceDefaultSecurityGroup(c, true)
}

func (s *localServerSuite) TestStartInstanceWithoutDefaultSecurityGroup(c *tc.C) {
	s.assertStartInstanceDefaultSecurityGroup(c, false)
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

func TestLocalHTTPSServerSuite(t *stdtesting.T) {
	tc.Run(t, &localHTTPSServerSuite{})
}

func (s *localHTTPSServerSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	overrideCinderProvider(&s.CleanupSuite, &mockAdaptor{})
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "")
}

func (s *localHTTPSServerSuite) createConfigAttrs(c *tc.C) map[string]interface{} {
	attrs := localConfigAttrs
	// In order to set up and tear down the environment properly, we must
	// disable hostname verification
	attrs["ssl-hostname-verification"] = false
	attrs["auth-url"] = s.cred.URL
	// Now connect and set up test-local tools and image-metadata URLs
	cl := client.NewNonValidatingClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, tc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containerURL[:8], tc.Equals, "https://")
	attrs["agent-metadata-url"] = containerURL + "/juju-dist-test/tools"
	c.Logf("Set agent-metadata-url=%q", attrs["agent-metadata-url"])
	attrs["image-metadata-url"] = containerURL + "/juju-dist-test"
	c.Logf("Set image-metadata-url=%q", attrs["image-metadata-url"])
	return attrs
}

func (s *localHTTPSServerSuite) SetUpTest(c *tc.C) {
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
	c.Assert(attrs["auth-url"].(string)[:8], tc.Equals, "https://")
	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapTestContext(c),
		jujuclient.NewMemStore(),
		prepareParams(attrs, s.cred),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.env = env.(environs.Environ)
	s.attrs = s.env.Config().AllAttrs()
}

func (s *localHTTPSServerSuite) TearDownTest(c *tc.C) {
	if s.env != nil {
		err := s.env.Destroy(c.Context())
		c.Check(err, tc.ErrorIsNil)
		s.env = nil
	}
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
}

func (s *localHTTPSServerSuite) TestSSLVerify(c *tc.C) {
	// If you don't have ssl-hostname-verification set to false, and do have
	// a CA Certificate, then we can connect to the environment. Copy the attrs
	// used by SetUp and force hostname verification.
	env := s.envUsingCertificate(c)
	_, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.IsNil)
}

func (s *localHTTPSServerSuite) TestMustDisableSSLVerify(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	_, err = environs.New(c.Context(), environs.OpenParams{
		Cloud:  makeCloudSpec(s.cred),
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *localHTTPSServerSuite) TestCanBootstrap(c *tc.C) {
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap(c)
	defer restoreFinishBootstrap()

	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	toolsMetadataStorage := openstack.MetadataStorage(s.env)
	agentURL, err := toolsMetadataStorage.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("Generating fake tools for: %v", agentURL)
	envtesting.UploadFakeTools(c, toolsMetadataStorage, "released")
	defer envtesting.RemoveFakeTools(c, toolsMetadataStorage, s.env.Config().AgentStream())

	imageMetadataStorage := openstack.ImageMetadataStorage(s.env)
	c.Logf("Generating fake images")
	openstack.UseTestImageData(imageMetadataStorage, s.cred)
	defer openstack.RemoveTestImageData(imageMetadataStorage)

	err = bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localHTTPSServerSuite) TestFetchFromImageMetadataSources(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestSkipVerifyDataSourceFactory())
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(customURL[:8], tc.Equals, "https://")

	envConfig, err := s.env.Config().Apply(
		map[string]interface{}{"image-metadata-url": customURL},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), envConfig)
	c.Assert(err, tc.ErrorIsNil)
	sources, err := environs.ImageMetadataSources(s.env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 2)

	// Make sure there is something to download from each location
	metadata := "metadata-content"
	metadataStorage := openstack.ImageMetadataStorage(s.env)
	err = metadataStorage.Put(metadata, bytes.NewBufferString(metadata), int64(len(metadata)))
	c.Assert(err, tc.ErrorIsNil)

	custom := "custom-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, tc.ErrorIsNil)

	// Produce map of data sources keyed on description
	mappedSources := make(map[string]simplestreams.DataSource, len(sources))
	for i, s := range sources {
		c.Logf("datasource %d: %+v", i, s)
		mappedSources[s.Description()] = s
	}

	// Read from the Config entry's image-metadata-url
	contentReader, imageURL, err := mappedSources["image-metadata-url"].Fetch(c.Context(), custom)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err := io.ReadAll(contentReader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, custom)
	c.Check(imageURL[:8], tc.Equals, "https://")

	// Check the entry we got from keystone
	contentReader, imageURL, err = mappedSources["keystone catalog"].Fetch(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err = io.ReadAll(contentReader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, metadata)
	c.Check(imageURL[:8], tc.Equals, "https://")
	// Verify that we are pointing at exactly where metadataStorage thinks we are
	metaURL, err := metadataStorage.URL(metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(imageURL, tc.Equals, metaURL)
}

func (s *localHTTPSServerSuite) TestFetchFromImageMetadataSourcesWithCertificate(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	env := s.envUsingCertificate(c)

	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(env, "custom-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(customURL[:8], tc.Equals, "https://")

	envConfig, err := env.Config().Apply(
		map[string]interface{}{"image-metadata-url": customURL},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = env.SetConfig(c.Context(), envConfig)
	c.Assert(err, tc.ErrorIsNil)
	sources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 2)

	// Make sure there is something to download from each location
	metadata := "metadata-content"
	metadataStorage := openstack.ImageMetadataStorage(env)
	err = metadataStorage.Put(metadata, bytes.NewBufferString(metadata), int64(len(metadata)))
	c.Assert(err, tc.ErrorIsNil)

	custom := "custom-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, tc.ErrorIsNil)

	// Produce map of data sources keyed on description
	mappedSources := make(map[string]simplestreams.DataSource, len(sources))
	for i, s := range sources {
		c.Logf("datasource %d: %+v", i, s)
		mappedSources[s.Description()] = s
	}

	// Check the entry we got from keystone
	contentReader, imageURL, err := mappedSources["keystone catalog"].Fetch(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err := io.ReadAll(contentReader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, metadata)
	c.Check(imageURL[:8], tc.Equals, "https://")

	// Verify that we are pointing at exactly where metadataStorage thinks we are
	metaURL, err := metadataStorage.URL(metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(imageURL, tc.Equals, metaURL)
}

func (s *localHTTPSServerSuite) TestFetchFromToolsMetadataSources(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestSkipVerifyDataSourceFactory())
	// Setup a custom URL for image metadata
	customStorage := openstack.CreateCustomStorage(s.env, "custom-tools-metadata")
	customURL, err := customStorage.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(customURL[:8], tc.Equals, "https://")

	envConfig, err := s.env.Config().Apply(
		map[string]interface{}{"agent-metadata-url": customURL},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), envConfig)
	c.Assert(err, tc.ErrorIsNil)
	sources, err := tools.GetMetadataSources(s.env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 3)

	// Make sure there is something to download from each location

	keystone := "keystone-tools-content"
	// The keystone entry just points at the root of the Swift storage, and
	// we have to create a container to upload any data. So we just point
	// into a subdirectory for the data we are downloading
	keystoneContainer := "tools-test"
	keystoneStorage := openstack.CreateCustomStorage(s.env, "tools-test")
	err = keystoneStorage.Put(keystone, bytes.NewBufferString(keystone), int64(len(keystone)))
	c.Assert(err, tc.ErrorIsNil)

	custom := "custom-tools-content"
	err = customStorage.Put(custom, bytes.NewBufferString(custom), int64(len(custom)))
	c.Assert(err, tc.ErrorIsNil)

	// Read from the Config entry's agent-metadata-url
	contentReader, metadataURL, err := sources[0].Fetch(c.Context(), custom)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err := io.ReadAll(contentReader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, custom)
	c.Check(metadataURL[:8], tc.Equals, "https://")

	// Check the entry we got from keystone
	// Now fetch the data, and verify the contents.
	contentReader, metadataURL, err = sources[1].Fetch(c.Context(), keystoneContainer+"/"+keystone)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = contentReader.Close() }()
	content, err = io.ReadAll(contentReader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, keystone)
	c.Check(metadataURL[:8], tc.Equals, "https://")
	keystoneURL, err := keystoneStorage.URL(keystone)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(metadataURL, tc.Equals, keystoneURL)

	// We *don't* test Fetch for sources[3] because it points to
	// streams.canonical.com
}

func (s *localServerSuite) TestRemoveBlankContainer(c *tc.C) {
	containerStorage := openstack.BlankContainerStorage()
	err := containerStorage.Remove("some-file")
	c.Assert(err, tc.ErrorMatches, `cannot remove "some-file": swift container name is empty`)
}

func (s *localServerSuite) TestAllRunningInstancesIgnoresOtherMachines(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	// Check that we see 1 instance in the environment
	insts, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(insts, tc.HasLen, 1)

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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity, tc.NotNil)

	// List all servers with no filter, we should see both instances
	servers, err := novaClient.ListServersDetail(nova.NewFilter())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(servers, tc.HasLen, 2)

	insts, err = s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(insts, tc.HasLen, 1)
}

func (s *localServerSuite) TestResolveNetworkUUID(c *tc.C) {
	var sampleUUID = "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"

	err := s.srv.Neutron.NeutronModel().AddNetwork(neutron.NetworkV2{Id: sampleUUID})
	c.Assert(err, tc.ErrorIsNil)

	networkIDs, err := openstack.ResolveNetworkIDs(s.env, sampleUUID, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networkIDs, tc.DeepEquals, []string{sampleUUID})
}

func (s *localServerSuite) TestResolveNetworkLabel(c *tc.C) {
	// For now this test has to cheat and use knowledge of goose internals
	var networkLabel = "net"
	var expectNetworkIDs = []string{"1"}
	networkIDs, err := openstack.ResolveNetworkIDs(s.env, networkLabel, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networkIDs, tc.DeepEquals, expectNetworkIDs)
}

func (s *localServerSuite) TestResolveNetworkLabelMultiple(c *tc.C) {
	var networkLabel = "multi"

	err := s.srv.Neutron.NeutronModel().AddNetwork(neutron.NetworkV2{
		Id:   "multi-666",
		Name: networkLabel,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.srv.Neutron.NeutronModel().AddNetwork(neutron.NetworkV2{
		Id:   "multi-999",
		Name: networkLabel,
	})
	c.Assert(err, tc.ErrorIsNil)

	var expectNetworkIDs = []string{"multi-666", "multi-999"}
	networkIDs, err := openstack.ResolveNetworkIDs(s.env, networkLabel, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networkIDs, tc.SameContents, expectNetworkIDs)
}

func (s *localServerSuite) TestResolveNetworkNotPresent(c *tc.C) {
	networkIDs, err := openstack.ResolveNetworkIDs(s.env, "no-network-with-this-label", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(networkIDs, tc.HasLen, 0)
}

func (s *localServerSuite) TestStartInstanceAvailZone(c *tc.C) {
	inst, err := s.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(openstack.InstanceServerDetail(inst).AvailabilityZone, tc.Equals, "test-available")
}

func (s *localServerSuite) TestStartInstanceAvailZoneUnavailable(c *tc.C) {
	_, err := s.testStartInstanceAvailZone(c, "test-unavailable")
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
}

func (s *localServerSuite) TestStartInstanceAvailZoneUnknown(c *tc.C) {
	_, err := s.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
}

func (s *localServerSuite) testStartInstanceAvailZone(c *tc.C, zone string) (instances.Instance, error) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	params := environs.StartInstanceParams{
		ControllerUUID:   s.ControllerUUID,
		AvailabilityZone: zone,
	}
	result, err := testing.StartInstanceWithParams(c, s.env, "1", params)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (s *localServerSuite) TestGetAvailabilityZones(c *tc.C) {
	var resultZones []nova.AvailabilityZone
	var resultErr error
	s.PatchValue(openstack.NovaListAvailabilityZones, func(c *nova.Client) ([]nova.AvailabilityZone, error) {
		return append([]nova.AvailabilityZone{}, resultZones...), resultErr
	})
	env := s.env.(common.ZonedEnviron)

	resultErr = fmt.Errorf("failed to get availability zones")
	zones, err := env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIs, resultErr)
	c.Assert(zones, tc.IsNil)

	resultErr = nil
	resultZones = make([]nova.AvailabilityZone, 1)
	resultZones[0].Name = "whatever"
	zones, err = env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.HasLen, 1)
	c.Assert(zones[0].Name(), tc.Equals, "whatever")
}

func (s *localServerSuite) TestGetAvailabilityZonesCommon(c *tc.C) {
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
	zones, err := env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.HasLen, 2)
	c.Assert(zones[0].Name(), tc.Equals, resultZones[0].Name)
	c.Assert(zones[1].Name(), tc.Equals, resultZones[1].Name)
	c.Assert(zones[0].Available(), tc.IsTrue)
	c.Assert(zones[1].Available(), tc.IsFalse)
}

func (s *localServerSuite) TestStartInstanceWithUnknownAZError(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

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
	_, err = testing.StartInstanceWithParams(c, s.env, "1", environs.StartInstanceParams{
		ControllerUUID:   s.ControllerUUID,
		AvailabilityZone: "az2",
	})
	c.Assert(err, tc.ErrorMatches, "(?s).*some unknown error.*")
}

func (s *localServerSuite) testStartInstanceWithParamsDeriveAZ(
	c *tc.C, machineId string,
	params environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	zonedEnv := s.env.(common.ZonedEnviron)
	zones, err := zonedEnv.DeriveAvailabilityZones(c.Context(), params)
	if err != nil {
		return nil, err
	}
	if len(zones) < 1 {
		return nil, errors.New("no zones found")
	}
	params.AvailabilityZone = zones[0]
	return testing.StartInstanceWithParams(c, s.env, "1", params)
}

func (s *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZone(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az2",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	result, err := s.testStartInstanceWithParamsDeriveAZ(c, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		VolumeAttachments: []storage.VolumeAttachmentParams{
			{VolumeId: "foo"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(openstack.InstanceServerDetail(result.Instance).AvailabilityZone, tc.Equals, "az2")
}

func (s *localServerSuite) TestStartInstanceVolumeAttachmentsMultipleAvailZones(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	for _, az := range []string{"az1", "az2"} {
		_, err := s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
			Size:             123,
			Name:             "vol-" + az,
			AvailabilityZone: az,
			Metadata: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	_, err = s.testStartInstanceWithParamsDeriveAZ(c, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		VolumeAttachments: []storage.VolumeAttachmentParams{
			{VolumeId: "vol-az1"},
			{VolumeId: "vol-az2"},
		},
	})
	c.Assert(err, tc.ErrorMatches, `cannot attach volumes from multiple availability zones: vol-az1 is in az1, vol-az2 is in az2`)
}

func (s *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZoneConflictsPlacement(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

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
	_, err = s.storageAdaptor.CreateVolume(cinder.CreateVolumeVolumeParams{
		Size:             123,
		Name:             "foo",
		AvailabilityZone: "az1",
		Metadata: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = testing.StartInstanceWithParams(c, s.env, "1", environs.StartInstanceParams{
		ControllerUUID:    s.ControllerUUID,
		VolumeAttachments: []storage.VolumeAttachmentParams{{VolumeId: "foo"}},
		AvailabilityZone:  "az2",
	})
	c.Assert(err, tc.ErrorMatches, `cannot create instance in zone "az2", as this will prevent attaching the requested disks in zone "az1"`)
}

// novaInstaceStartedWithOpts exposes run server options used to start an instance.
type novaInstaceStartedWithOpts interface {
	NovaInstanceStartedWithOpts() *nova.RunServerOpts
}

func (s *localServerSuite) TestStartInstanceWithImageIDConstraint(c *tc.C) {
	env := s.ensureAMDImages(c)

	err := bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)

	cons, err := constraints.Parse("image-id=ubuntu-bf2")
	c.Assert(err, tc.ErrorIsNil)

	res, err := testing.StartInstanceWithParams(c, env, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, tc.NotNil)
	c.Assert(runOpts.ImageId, tc.NotNil)
	c.Assert(runOpts.ImageId, tc.Equals, "ubuntu-bf2")
}

func (s *localServerSuite) TestStartInstanceVolumeRootBlockDevice(c *tc.C) {
	// diskSizeGiB should be equal to the openstack.defaultRootDiskSize
	diskSizeGiB := 30
	env := s.ensureAMDImages(c)

	err := bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk-source=volume arch=amd64")
	c.Assert(err, tc.ErrorIsNil)

	res, err := testing.StartInstanceWithParams(c, env, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, tc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, tc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, tc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "volume",
		DeleteOnTermination: true,
		VolumeSize:          diskSizeGiB,
	})
}

func (s *localServerSuite) TestStartInstanceVolumeRootBlockDeviceSized(c *tc.C) {
	env := s.ensureAMDImages(c)

	diskSizeGiB := 10

	err := bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk-source=volume root-disk=10G arch=amd64")
	c.Assert(err, tc.ErrorIsNil)

	res, err := testing.StartInstanceWithParams(c, env, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)

	c.Assert(res.Hardware.RootDisk, tc.NotNil)
	c.Assert(*res.Hardware.RootDisk, tc.Equals, uint64(diskSizeGiB*1024))

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, tc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, tc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, tc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "volume",
		DeleteOnTermination: true,
		VolumeSize:          diskSizeGiB,
	})
}

func (s *localServerSuite) TestStartInstanceLocalRootBlockDeviceConstraint(c *tc.C) {
	env := s.ensureAMDImages(c)

	err := bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk-source=local root-disk=1G arch=amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons.HasRootDisk(), tc.IsTrue)
	c.Assert(*cons.RootDisk, tc.Equals, uint64(1024))

	res, err := testing.StartInstanceWithParams(c, env, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)

	c.Assert(res.Hardware.RootDisk, tc.NotNil)
	// Check local disk requirements are met.
	c.Assert(*res.Hardware.RootDisk, tc.GreaterThan, uint64(1024-1))

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, tc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, tc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, tc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "local",
		DeleteOnTermination: true,
		// VolumeSize is 0 when a local disk is used.
		VolumeSize: 0,
	})
}

func (s *localServerSuite) TestStartInstanceLocalRootBlockDevice(c *tc.C) {
	env := s.ensureAMDImages(c)

	err := bootstrapEnv(c, env)
	c.Assert(err, tc.ErrorIsNil)

	cons, err := constraints.Parse("root-disk=1G arch=amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons.HasRootDisk(), tc.IsTrue)
	c.Assert(*cons.RootDisk, tc.Equals, uint64(1024))

	res, err := testing.StartInstanceWithParams(c, env, "1", environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		Constraints:    cons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)

	c.Assert(res.Hardware.RootDisk, tc.NotNil)
	// Check local disk requirements are met.
	c.Assert(*res.Hardware.RootDisk, tc.GreaterThan, uint64(1024-1))

	runOpts := res.Instance.(novaInstaceStartedWithOpts).NovaInstanceStartedWithOpts()
	c.Assert(runOpts, tc.NotNil)
	c.Assert(runOpts.BlockDeviceMappings, tc.NotNil)
	deviceMapping := runOpts.BlockDeviceMappings[0]
	c.Assert(deviceMapping, tc.DeepEquals, nova.BlockDeviceMapping{
		BootIndex:           0,
		UUID:                "1",
		SourceType:          "image",
		DestinationType:     "local",
		DeleteOnTermination: true,
		// VolumeSize is 0 when a local disk is used.
		VolumeSize: 0,
	})
}

func (s *localServerSuite) TestInstanceTags(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	allInstances, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInstances, tc.HasLen, 1)

	c.Assert(
		openstack.InstanceServerDetail(allInstances[0]).Metadata,
		tc.DeepEquals,
		map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
			"juju-is-controller":   "true",
		},
	)
}

func (s *localServerSuite) TestTagInstance(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	assertMetadata := func(extraKey, extraValue string) {
		// Refresh instance
		allInstances, err := s.env.AllRunningInstances(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(allInstances, tc.HasLen, 1)
		c.Assert(
			openstack.InstanceServerDetail(allInstances[0]).Metadata,
			tc.DeepEquals,
			map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
				"juju-is-controller":   "true",
				extraKey:               extraValue,
			},
		)
	}

	allInstances, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInstances, tc.HasLen, 1)

	extraKey := "extra-k"
	extraValue := "extra-v"
	err = s.env.(environs.InstanceTagger).TagInstance(
		c.Context(),
		allInstances[0].Id(),
		map[string]string{extraKey: extraValue},
	)
	c.Assert(err, tc.ErrorIsNil)
	assertMetadata(extraKey, extraValue)

	// Ensure that a second call updates existing tags.
	extraValue = "extra-v2"
	err = s.env.(environs.InstanceTagger).TagInstance(
		c.Context(),
		allInstances[0].Id(),
		map[string]string{extraKey: extraValue},
	)
	c.Assert(err, tc.ErrorIsNil)
	assertMetadata(extraKey, extraValue)
}

func (s *localServerSuite) TestAdoptResources(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	cfg, err := s.env.Config().Apply(map[string]interface{}{
		"uuid": hostedModelUUID,
	})
	c.Assert(err, tc.ErrorIsNil)
	env, err := environs.New(c.Context(), environs.OpenParams{
		Cloud:  makeCloudSpec(s.cred),
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	originalController := coretesting.ControllerTag.Id()
	_, _, _, err = testing.StartInstance(c, env, originalController, "0")
	c.Assert(err, tc.ErrorIsNil)

	addVolume(c, s.env, originalController, "99/9")
	addVolume(c, env, originalController, "23/9")

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, originalController)
	s.checkVolumeTags(c, s.env, originalController)
	s.checkVolumeTags(c, env, originalController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, originalController)

	// Needs to be a correctly formatted uuid so we can get it out of
	// group names.
	newController := "aaaaaaaa-bbbb-cccc-dddd-0123456789ab"
	err = env.AdoptResources(c.Context(), newController, semversion.MustParse("1.2.3"))
	c.Assert(err, tc.ErrorIsNil)

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, newController)
	s.checkVolumeTags(c, s.env, originalController)
	s.checkVolumeTags(c, env, newController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, newController)
}

func (s *localServerSuite) TestAdoptResourcesNoStorage(c *tc.C) {
	// Nova-lxd doesn't support storage. lp:1677225
	s.PatchValue(openstack.NewOpenstackStorage, func(*openstack.Environ) (openstack.OpenstackStorage, error) {
		return nil, errors.NotSupportedf("volumes")
	})
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	cfg, err := s.env.Config().Apply(map[string]interface{}{
		"uuid": hostedModelUUID,
	})
	c.Assert(err, tc.ErrorIsNil)
	env, err := environs.New(c.Context(), environs.OpenParams{
		Cloud:  makeCloudSpec(s.cred),
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	originalController := coretesting.ControllerTag.Id()
	_, _, _, err = testing.StartInstance(c, env, originalController, "0")
	c.Assert(err, tc.ErrorIsNil)

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, originalController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, originalController)

	// Needs to be a correctly formatted uuid so we can get it out of
	// group names.
	newController := "aaaaaaaa-bbbb-cccc-dddd-0123456789ab"
	err = env.AdoptResources(c.Context(), newController, semversion.MustParse("1.2.3"))
	c.Assert(err, tc.ErrorIsNil)

	s.checkInstanceTags(c, s.env, originalController)
	s.checkInstanceTags(c, env, newController)
	s.checkGroupController(c, s.env, originalController)
	s.checkGroupController(c, env, newController)
}

func addVolume(
	c *tc.C, env environs.Environ, controllerUUID, name string,
) *storage.Volume {
	storageAdaptor, err := (*openstack.NewOpenstackStorage)(env.(*openstack.Environ))
	c.Assert(err, tc.ErrorIsNil)
	modelUUID := env.Config().UUID()
	source := openstack.NewCinderVolumeSourceForModel(storageAdaptor, modelUUID, env.(common.ZonedEnviron), env.(common.CredentialInvalidator))
	result, err := source.CreateVolumes(c.Context(), []storage.VolumeParams{{
		Tag: names.NewVolumeTag(name),
		ResourceTags: tags.ResourceTags(
			names.NewModelTag(modelUUID),
			names.NewControllerTag(controllerUUID),
		),
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result[0].Error, tc.ErrorIsNil)
	return result[0].Volume
}

func (s *localServerSuite) checkInstanceTags(c *tc.C, env environs.Environ, expectedController string) {
	allInstances, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInstances, tc.Not(tc.HasLen), 0)
	for _, inst := range allInstances {
		server := openstack.InstanceServerDetail(inst)
		c.Logf("%s", string(inst.Id()))
		c.Check(server.Metadata[tags.JujuController], tc.Equals, expectedController)
	}
}

func (s *localServerSuite) checkVolumeTags(c *tc.C, env environs.Environ, expectedController string) {
	stor, err := (*openstack.NewOpenstackStorage)(env.(*openstack.Environ))
	c.Assert(err, tc.ErrorIsNil)
	source := openstack.NewCinderVolumeSourceForModel(stor, env.Config().UUID(), s.env.(common.ZonedEnviron), s.env.(common.CredentialInvalidator))
	volumeIds, err := source.ListVolumes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeIds, tc.Not(tc.HasLen), 0)
	for _, volumeId := range volumeIds {
		c.Logf("%s", volumeId)
		volume, err := stor.GetVolume(volumeId)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(volume.Metadata[tags.JujuController], tc.Equals, expectedController)
	}
}

func (s *localServerSuite) checkGroupController(c *tc.C, env environs.Environ, expectedController string) {
	groupNames, err := openstack.GetModelGroupNames(env)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(groupNames, tc.Not(tc.HasLen), 0)
	extractControllerRe, err := regexp.Compile(openstack.GroupControllerPattern)
	c.Assert(err, tc.ErrorIsNil)
	for _, group := range groupNames {
		c.Logf("%s", group)
		controller := extractControllerRe.ReplaceAllString(group, "$controllerUUID")
		c.Check(controller, tc.Equals, expectedController)
	}
}

func (s *localServerSuite) TestUpdateGroupController(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	groupNames, err := openstack.GetModelGroupNames(s.env)
	c.Assert(err, tc.ErrorIsNil)
	groupNamesBefore := set.NewStrings(groupNames...)
	c.Assert(groupNamesBefore, tc.DeepEquals, set.NewStrings(
		fmt.Sprintf("juju-%s-%s", coretesting.ControllerTag.Id(), coretesting.ModelTag.Id()),
		fmt.Sprintf("juju-%s-%s-0", coretesting.ControllerTag.Id(), coretesting.ModelTag.Id()),
	))

	firewaller := openstack.GetFirewaller(s.env)
	err = firewaller.UpdateGroupController(c.Context(), "aabbccdd-eeee-ffff-0000-0123456789ab")
	c.Assert(err, tc.ErrorIsNil)

	groupNames, err = openstack.GetModelGroupNames(s.env)
	c.Assert(err, tc.ErrorIsNil)
	groupNamesAfter := set.NewStrings(groupNames...)
	c.Assert(groupNamesAfter, tc.DeepEquals, set.NewStrings(
		fmt.Sprintf("juju-aabbccdd-eeee-ffff-0000-0123456789ab-%s", coretesting.ModelTag.Id()),
		fmt.Sprintf("juju-aabbccdd-eeee-ffff-0000-0123456789ab-%s-0", coretesting.ModelTag.Id()),
	))
}

func (s *localServerSuite) TestICMPFirewallRules(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	inst, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	firewaller := openstack.GetFirewaller(s.env)
	err = firewaller.OpenInstancePorts(c.Context(), inst, "100", firewall.IngressRules{
		{
			PortRange: network.PortRange{
				FromPort: -1,
				ToPort:   -1,
				Protocol: "icmp",
			},
			SourceCIDRs: set.NewStrings("0.0.0.0/0"),
		},
		{
			PortRange: network.PortRange{
				FromPort: -1,
				ToPort:   -1,
				Protocol: "ipv6-icmp",
			},
			SourceCIDRs: set.NewStrings("::/0"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	rules, err := firewaller.InstanceIngressRules(c.Context(), inst, "100")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(len(rules), tc.Equals, 1)
	c.Assert(rules[0].PortRange.FromPort, tc.Equals, -1)
	c.Assert(rules[0].PortRange.ToPort, tc.Equals, -1)
	c.Assert(rules[0].PortRange.Protocol, tc.Equals, "icmp")
	c.Assert(rules[0].SourceCIDRs.Size(), tc.Equals, 2)
	c.Assert(rules[0].SourceCIDRs.Contains("0.0.0.0/0"), tc.IsTrue)
	c.Assert(rules[0].SourceCIDRs.Contains("::/0"), tc.IsTrue)
}

// TestIPv6RuleCreationForEmptyCIDR is a regression test for lp1709312
func (s *localServerSuite) TestIPv6RuleCreationForEmptyCIDR(c *tc.C) {
	err := bootstrapEnv(c, s.env)
	c.Assert(err, tc.ErrorIsNil)

	inst, _ := testing.AssertStartInstance(c, s.env, s.ControllerUUID, "100")
	firewaller := openstack.GetFirewaller(s.env)
	err = firewaller.OpenInstancePorts(c.Context(), inst, "100", firewall.IngressRules{
		{
			PortRange: network.PortRange{
				FromPort: 443,
				ToPort:   443,
				Protocol: "tcp",
			},
			SourceCIDRs: set.NewStrings(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	rules, err := firewaller.InstanceIngressRules(c.Context(), inst, "100")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(len(rules), tc.Equals, 1)
	c.Assert(rules[0].PortRange.FromPort, tc.Equals, 443)
	c.Assert(rules[0].PortRange.ToPort, tc.Equals, 443)
	c.Assert(rules[0].PortRange.Protocol, tc.Equals, "tcp")
	c.Assert(rules[0].SourceCIDRs.Size(), tc.Equals, 2)
	c.Assert(rules[0].SourceCIDRs.Contains("0.0.0.0/0"), tc.IsTrue)
	c.Assert(rules[0].SourceCIDRs.Contains("::/0"), tc.IsTrue)
}

func (s *localServerSuite) ensureAMDImages(c *tc.C) environs.Environ {
	// Ensure amd64 tools are available, to ensure an amd64 image.
	amd64Version := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.AMD64,
		Release: corebase.UbuntuOS,
	}
	envtesting.AssertUploadFakeToolsVersions(
		c, s.toolsMetadataStorage, "released", amd64Version)

	// Destroy the old Environ
	err := environs.Destroy(s.env.Config().Name(), s.env, c.Context(), s.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)

	// Prepare a new Environ
	return s.Prepare(c)
}
func TestNoNeutronSuite(t *stdtesting.T) {
	tc.Run(t, &noNeutronSuite{})
}

// noNeutronSuite is a clone of localServerSuite which hacks the local
// openstack to remove the neutron service from the auth response -
// this causes the client to switch to nova networking.
type noNeutronSuite struct {
	coretesting.BaseSuite
	cred *identity.Credentials
	srv  localServer
}

func (s *noNeutronSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.cred = &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some-region",
		TenantName: "some tenant",
	}
	c.Logf("Running local tests")
}

func (s *noNeutronSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.srv.start(c, s.cred, newNovaNetworkingOpenstackService)

	userPass, ok := s.srv.OpenstackSvc.Identity.(*identityservice.UserPass)
	c.Assert(ok, tc.IsTrue)
	// Ensure that there's nothing returned with a type of "network",
	// so that we switch over to nova networking.
	cleanup := userPass.RegisterControlPoint("authorisation", func(sc hook.ServiceControl, args ...interface{}) error {
		res, ok := args[0].(*identityservice.AccessResponse)
		c.Assert(ok, tc.IsTrue)
		var filtered []identityservice.V2Service
		for _, service := range res.Access.ServiceCatalog {
			if service.Type != "network" {
				filtered = append(filtered, service)
			}
		}
		res.Access.ServiceCatalog = filtered
		return nil
	})
	s.AddCleanup(func(c *tc.C) { cleanup() })
}

func (s *noNeutronSuite) TearDownTest(c *tc.C) {
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
}

func (s *noNeutronSuite) TestSupport(c *tc.C) {
	cl := client.NewClient(s.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, tc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, tc.ErrorIsNil)
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
		envtesting.BootstrapTestContext(c),
		jujuclient.NewMemStore(),
		prepareParams(attrs, s.cred),
	)
	c.Check(err, tc.ErrorIs, errors.NotFound)
	c.Assert(err, tc.ErrorMatches, `OpenStack Neutron service`)
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

func TestNoSwiftSuite(t *stdtesting.T) {
	tc.Run(t, &noSwiftSuite{})
}

func (s *noSwiftSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap(c)
	s.AddCleanup(func(*tc.C) { restoreFinishBootstrap() })

	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *noSwiftSuite) SetUpTest(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
	}
	toolsStorage, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, toolsStorage, "released")
	s.PatchValue(&tools.DefaultBaseURL, storageDir)
	imageStorage, err := filestorage.NewFileStorageWriter(imagesDir)
	c.Assert(err, tc.ErrorIsNil)
	openstack.UseTestImageData(imageStorage, s.cred)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, storageDir)

	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapTestContext(c),
		jujuclient.NewMemStore(),
		prepareParams(attrs, s.cred),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.env = env.(environs.Environ)
}

func (s *noSwiftSuite) TearDownTest(c *tc.C) {
	s.srv.stop()
	s.BaseSuite.TearDownTest(c)
}

func (s *noSwiftSuite) TestBootstrap(c *tc.C) {
	cfg, err := s.env.Config().Apply(coretesting.Attrs{
		// A label that corresponds to a neutron test service network
		"network": "net",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(bootstrapEnv(c, s.env), tc.ErrorIsNil)
}

func newFullOpenstackService(cred *identity.Credentials, auth identity.AuthMode, useTSL bool) (
	*openstackservice.Openstack, []string,
) {
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

func bootstrapEnv(c *tc.C, env environs.Environ) error {
	return bootstrapEnvWithConstraints(c, env, constraints.Value{})
}

func bootstrapEnvWithConstraints(c *tc.C, env environs.Environ, cons constraints.Value) error {
	return bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			BootstrapConstraints:    cons,
		})
}
