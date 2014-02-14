// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"bytes"
	"io/ioutil"

	"launchpad.net/gojoyent/client"
	lc "launchpad.net/gojoyent/localservices/cloudapi"
	lm "launchpad.net/gojoyent/localservices/manta"

	gc "launchpad.net/gocheck"

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
	"launchpad.net/juju-core/provider/joyent"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type ProviderSuite struct{}

var _ = gc.Suite(&ProviderSuite{})

func registerLocalTests() {
	gc.Suite(&localServerSuite{})
	gc.Suite(&localLiveSuite{})
}

type localCloudAPIServer struct {
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
	cloudapi *lc.CloudAPI
}

func (ca *localCloudAPIServer) setupServer(c *gc.C) {
	// Set up the HTTP server.
	ca.Server = httptest.NewServer(nil)
	c.Assert(ca.Server, gc.NotNil)
	ca.oldHandler = ca.Server.Config.Handler
	ca.Mux = http.NewServeMux()
	ca.Server.Config.Handler = ca.Mux

	ca.cloudapi = lc.New(ca.Server.URL, testUser)
	ca.cloudapi.SetupHTTP(ca.Mux)
	c.Logf("Started local CloudAPI service at: %v", ca.Server.URL)
}

func (c *localCloudAPIServer) destroyServer() {
	c.Mux = nil
	c.Server.Config.Handler = c.oldHandler
	c.Server.Close()
}

type localMantaServer struct {
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
	manta	 *lm.Manta
}

func (m *localMantaServer) setupServer(c *gc.C) {
	// Set up the HTTP server.
	m.Server = httptest.NewServer(nil)
	c.Assert(m.Server, gc.NotNil)
	m.oldHandler = m.Server.Config.Handler
	m.Mux = http.NewServeMux()
	m.Server.Config.Handler = m.Mux

	m.manta = lm.New(m.Server.URL, testUser)
	m.manta.SetupHTTP(m.Mux)
	c.Logf("Started local Manta service at: %v", m.Server.URL)
}

func (m *localMantaServer) destroyServer() {
	m.Mux = nil
	m.Server.Config.Handler = m.oldHandler
	m.Server.Close()
}

type localLiveSuite struct {
	LiveTests
	cSrv 	   *localCloudAPIServer
	mSrv 	   *localMantaServer
}

func (s *localLiveSuite) SetUpSuite(c *gc.C) {
	CreateTestKey()
	s.cSrv = &localCloudAPIServer{}
	s.mSrv = &localMantaServer{}
	s.cSrv.setupServer(c)
	s.mSrv.setupServer(c)

	s.TestConfig = GetFakeConfig(s.cSrv.Server.URL, s.mSrv.Server.URL)
	s.LiveTests.SetUpSuite(c)

	joyent.UseTestImageData(joyent.ImageMetadataStorage(s.Env), s.Env.(*joyent.JoyentEnviron).Credentials())
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) { restoreFinishBootstrap() })
}

func (s *localLiveSuite) TearDownSuite(c *gc.C) {
	joyent.RemoveTestImageData(joyent.ImageMetadataStorage(s.Env))
	s.LiveTests.TearDownSuite(c)
	s.cSrv.destroyServer()
	s.mSrv.destroyServer()
	RemoveTestKey()
}

func (s *localLiveSuite) SetUpTest(c *gc.C) {
	s.LiveTests.SetUpTest(c)
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
}

func (s *localLiveSuite) TearDownTest(c *gc.C) {
	s.LiveTests.TearDownTest(c)
}

// localServerSuite contains tests that run against an Joyent service double.
// These tests can test things that would be unreasonably slow or expensive
// to test on a live Joyent server. The service double is started and stopped for
// each test.
type localServerSuite struct {
	jujutest.Tests
	cSrv 	   			 *localCloudAPIServer
	mSrv 	   			 *localMantaServer
	toolsMetadataStorage storage.Storage
	imageMetadataStorage storage.Storage
}

func (s *localServerSuite) SetUpSuite(c *gc.C) {
	s.Tests.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) { restoreFinishBootstrap() })
}

func (s *localServerSuite) TearDownSuite(c *gc.C) {
	s.Tests.TearDownSuite(c)
}

func (s *localServerSuite) SetUpTest(c *gc.C) {
	CreateTestKey()

	s.cSrv = &localCloudAPIServer{}
	s.mSrv = &localMantaServer{}
	s.cSrv.setupServer(c)
	s.mSrv.setupServer(c)

	s.TestConfig = GetFakeConfig(s.cSrv.Server.URL, s.mSrv.Server.URL)
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
	s.Tests.SetUpTest(c)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	env := s.Prepare(c)

	cl := client.NewClient(s.mSrv.Server.URL, "", env.(*joyent.JoyentEnviron).Credentials(), nil)
	c.Assert(cl, gc.NotNil)
	containerURL := cl.MakeServiceURL([]string{"object-store", ""})
	s.TestConfig = s.TestConfig.Merge(coretesting.Attrs{
		"tools-metadata-url": containerURL + "/juju-test/tools",
		"image-metadata-url": containerURL + "/juju-test",
	})

	s.toolsMetadataStorage = joyent.MetadataStorage(env)
	// Put some fake metadata in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running can find the metadata.
	envtesting.UploadFakeTools(c, s.toolsMetadataStorage)
	s.imageMetadataStorage = joyent.ImageMetadataStorage(env)
	joyent.UseTestImageData(s.imageMetadataStorage, env.(*joyent.JoyentEnviron).Credentials())
}

func (s *localServerSuite) TearDownTest(c *gc.C) {
	if s.imageMetadataStorage != nil {
		joyent.RemoveTestImageData(s.imageMetadataStorage)
	}
	if s.toolsMetadataStorage != nil {
		envtesting.RemoveFakeToolsMetadata(c, s.toolsMetadataStorage)
	}
	s.Tests.TearDownTest(c)
	s.cSrv.destroyServer()
	s.mSrv.destroyServer()
	RemoveTestKey()
}

func bootstrapContext(c *gc.C) environs.BootstrapContext {
	return envtesting.NewBootstrapContext(coretesting.Context(c))
}

func (s *localServerSuite) TestPrecheck(c *gc.C) {
	var cons constraints.Value
	env := s.Prepare(c)
	prechecker, ok := env.(environs.Prechecker)
	c.Assert(ok, jc.IsTrue)
	err := prechecker.PrecheckInstance("precise", cons)
	c.Check(err, gc.IsNil)
	err = prechecker.PrecheckContainer("precise", instance.LXC)
	c.Check(err, gc.ErrorMatches, "joyent provider does not support containers")
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstance(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, s.TestConfig)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, s.ConfigStore)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(bootstrapContext(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	inst, _ := testing.AssertStartInstance(c, env, "100")
	err = env.StopInstances([]instance.Instance{inst})
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	env := s.Prepare(c)
	err := bootstrap.Bootstrap(bootstrapContext(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	_, hc := testing.AssertStartInstanceWithConstraints(c, env, "100", constraints.MustParse("mem=1024"))
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
	inst, _ := testing.AssertStartInstance(c, env, "100")
	c.Assert(inst.Status(), gc.Equals, "running")
	err := env.StopInstances([]instance.Instance{inst})
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestInstancesGathering(c *gc.C) {
	env := s.Prepare(c)
	inst0, _ := testing.AssertStartInstance(c, env, "100")
	id0 := inst0.Id()
	inst1, _ := testing.AssertStartInstance(c, env, "101")
	id1 := inst1.Id()
	c.Logf("id0: %s, id1: %s", id0, id1)
	defer func() {
		err := env.StopInstances([]instance.Instance{inst0, inst1})
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
		c.Logf("Looking for ids %v, got %v", ids, insts)
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
	err := bootstrap.Bootstrap(bootstrapContext(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := bootstrap.LoadState(env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(stateData.StateInstances, gc.HasLen, 1)

	expectedHardware := instance.MustParseHardware("arch=amd64 cpu-cores=1 mem=512M root-disk=8192M")
	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(stateData.StateInstances[0], gc.Equals, insts[0].Id())
	c.Check(stateData.Characteristics[0], gc.DeepEquals, expectedHardware)

	bootstrapDNS, err := insts[0].DNSName()
	c.Assert(err, gc.IsNil)
	c.Assert(bootstrapDNS, gc.Not(gc.Equals), "")
}

func (s *localServerSuite) TestGetImageMetadataSources(c *gc.C) {
	env := s.Open(c)
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 2)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		urls[i] = url
	}
	// The control bucket URL contains the bucket name.
	c.Check(strings.Contains(urls[0], joyent.ControlBucketName(env)+"/images"), jc.IsTrue)
	c.Assert(urls[1], gc.Equals, imagemetadata.DefaultBaseURL+"/")
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	env := s.Open(c)
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 1)
	url, err := sources[0].URL("")
	// The control bucket URL contains the bucket name.
	c.Assert(strings.Contains(url, joyent.ControlBucketName(env)+"/tools"), jc.IsTrue)
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *gc.C) {
	// Prevent falling over to the public datasource.
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")

	env := s.Open(c)

	// An error occurs if no suitable image is found.
	_, err := joyent.FindInstanceSpec(env, "saucy", "amd64", "mem=4G")
	c.Assert(err, gc.ErrorMatches, `no "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	env := s.Open(c)
	params, err := env.(simplestreams.MetadataValidator).MetadataLookupParams("some-region")
	c.Assert(err, gc.IsNil)
	params.Sources, err = imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	params.Series = "raring"
	image_ids, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(image_ids, gc.DeepEquals, []string{"11223344-0a0a-dd77-33cd-abcd1234e5f6"})
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
