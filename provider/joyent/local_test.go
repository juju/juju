// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"net/http"
	"net/http/httptest"

	lc "github.com/joyent/gosdc/localservices/cloudapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/joyent"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

func registerLocalTests() {
	gc.Suite(&localServerSuite{})
	gc.Suite(&localLiveSuite{})
}

type localCloudAPIServer struct {
	Server *httptest.Server
}

func (ca *localCloudAPIServer) setupServer(c *gc.C) {
	// Set up the HTTP server.
	ca.Server = httptest.NewServer(nil)
	c.Assert(ca.Server, gc.NotNil)
	mux := http.NewServeMux()
	ca.Server.Config.Handler = mux

	cloudapi := lc.New(ca.Server.URL, testUser)
	cloudapi.SetupHTTP(mux)
	c.Logf("Started local CloudAPI service at: %v", ca.Server.URL)
}

func (s *localCloudAPIServer) destroyServer(c *gc.C) {
	s.Server.Close()
}

type localLiveSuite struct {
	baseSuite
	jujutest.LiveTests
	cSrv localCloudAPIServer
}

func (s *localLiveSuite) SetUpSuite(c *gc.C) {
	s.baseSuite.SetUpSuite(c)
	s.LiveTests.SetUpSuite(c)
	s.cSrv.setupServer(c)
	s.AddCleanup(s.cSrv.destroyServer)

	s.Credential = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"sdc-user":    testUser,
		"sdc-key-id":  testKeyFingerprint,
		"private-key": testPrivateKey,
		"algorithm":   "rsa-sha256",
	})
	s.CloudEndpoint = s.cSrv.Server.URL
	s.CloudRegion = "some-region"

	s.TestConfig = GetFakeConfig().Merge(coretesting.Attrs{
		"image-metadata-url": "test://host",
	})
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.AddCleanup(func(*gc.C) { envtesting.PatchAttemptStrategies(&joyent.ShortAttempt) })
}

func (s *localLiveSuite) TearDownSuite(c *gc.C) {
	joyent.UnregisterExternalTestImageMetadata()
	s.LiveTests.TearDownSuite(c)
	s.baseSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
	creds := joyent.MakeCredentials(c, s.CloudEndpoint, s.Credential)
	joyent.UseExternalTestImageMetadata(c, creds)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test://host")
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) { restoreFinishBootstrap() })
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
}

func (s *localLiveSuite) TearDownTest(c *gc.C) {
	s.LiveTests.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
}

// localServerSuite contains tests that run against an Joyent service double.
// These tests can test things that would be unreasonably slow or expensive
// to test on a live Joyent server. The service double is started and stopped for
// each test.
type localServerSuite struct {
	baseSuite
	jujutest.Tests
	cSrv localCloudAPIServer
}

func (s *localServerSuite) SetUpSuite(c *gc.C) {
	s.baseSuite.SetUpSuite(c)
	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)

	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) { restoreFinishBootstrap() })
}

func (s *localServerSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.cSrv.setupServer(c)
	s.AddCleanup(s.cSrv.destroyServer)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.Tests.SetUpTest(c)

	s.Credential = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"sdc-user":    testUser,
		"sdc-key-id":  testKeyFingerprint,
		"private-key": testPrivateKey,
		"algorithm":   "rsa-sha256",
	})
	s.CloudEndpoint = s.cSrv.Server.URL
	s.CloudRegion = "some-region"
	s.TestConfig = GetFakeConfig()

	// Put some fake image metadata in place.
	creds := joyent.MakeCredentials(c, s.CloudEndpoint, s.Credential)
	joyent.UseExternalTestImageMetadata(c, creds)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test://host")
}

func (s *localServerSuite) TearDownTest(c *gc.C) {
	joyent.UnregisterExternalTestImageMetadata()
	s.Tests.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
}

func bootstrapContext(c *gc.C) environs.BootstrapContext {
	return envtesting.BootstrapContext(c)
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstance(c *gc.C) {
	env := s.Prepare(c)
	err := bootstrap.Bootstrap(bootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      testing.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, "100")
	err = env.StopInstances(inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestStartInstanceAvailabilityZone(c *gc.C) {
	env := s.Prepare(c)
	err := bootstrap.Bootstrap(bootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      testing.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	inst, hwc := testing.AssertStartInstance(c, env, s.ControllerUUID, "100")
	err = env.StopInstances(inst.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(hwc.AvailabilityZone, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	env := s.Prepare(c)
	err := bootstrap.Bootstrap(bootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      testing.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, env, s.ControllerUUID, "100", constraints.MustParse("mem=1024"))
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
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
	env := s.Prepare(c)
	inst, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, "100")
	c.Assert(inst.Status().Message, gc.Equals, "running")
	err := env.StopInstances(inst.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localServerSuite) TestInstancesGathering(c *gc.C) {
	env := s.Prepare(c)
	inst0, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, "100")
	id0 := inst0.Id()
	inst1, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, "101")
	id1 := inst1.Id()
	c.Logf("id0: %s, id1: %s", id0, id1)
	defer func() {
		// StopInstances deletes machines in parallel but the Joyent
		// API test double isn't goroutine-safe so stop them one at a
		// time. See https://pad.lv/1604514
		c.Check(env.StopInstances(inst0.Id()), jc.ErrorIsNil)
		c.Check(env.StopInstances(inst1.Id()), jc.ErrorIsNil)
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

// It should be moved to environs.jujutests.Tests.
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *gc.C) {
	env := s.Prepare(c)
	err := bootstrap.Bootstrap(bootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      testing.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)

	// check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances(s.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)

	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(instanceIds[0], gc.Equals, insts[0].Id())

	addresses, err := insts[0].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.HasLen, 2)
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	s.PatchValue(&tools.DefaultBaseURL, "")

	env := s.Prepare(c)
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (s *localServerSuite) TestFindInstanceSpec(c *gc.C) {
	env := s.Prepare(c)
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:       "image-id",
		Arch:     "amd64",
		VirtType: "kvm",
	}}
	spec, err := joyent.FindInstanceSpec(env, "trusty", "amd64", "mem=4G", imageMetadata)
	c.Assert(err, gc.IsNil)
	c.Assert(spec.InstanceType.VirtType, gc.NotNil)
	c.Check(spec.Image.Arch, gc.Equals, "amd64")
	c.Check(spec.Image.VirtType, gc.Equals, "kvm")
	c.Check(*spec.InstanceType.VirtType, gc.Equals, "kvm")
	c.Check(spec.InstanceType.CpuCores, gc.Equals, uint64(4))
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *gc.C) {
	env := s.Prepare(c)
	// An error occurs if no suitable image is found.
	_, err := joyent.FindInstanceSpec(env, "saucy", "amd64", "mem=4G", nil)
	c.Assert(err, gc.ErrorMatches, `no "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	env := s.Prepare(c)
	params, err := env.(simplestreams.MetadataValidator).MetadataLookupParams("some-region")
	c.Assert(err, jc.ErrorIsNil)
	params.Sources, err = environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	params.Series = "raring"
	image_ids, _, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(image_ids, gc.DeepEquals, []string{"11223344-0a0a-dd77-33cd-abcd1234e5f6"})
}

func (s *localServerSuite) TestConstraintsValidator(c *gc.C) {
	env := s.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 tags=bar cpu-power=10 virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "tags", "virt-type"})
}

func (s *localServerSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}
