// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"

	lm "github.com/joyent/gomanta/localservices/manta"
	lc "github.com/joyent/gosdc/localservices/cloudapi"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/arch"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/joyent"
	coretesting "launchpad.net/juju-core/testing"
)

func registerLocalTests() {
	gc.Suite(&localServerSuite{})
	gc.Suite(&localLiveSuite{})
}

type localCloudAPIServer struct {
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
	cloudapi   *lc.CloudAPI
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
	manta      *lm.Manta
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
	providerSuite
	jujutest.LiveTests
	cSrv *localCloudAPIServer
	mSrv *localMantaServer
}

func (s *localLiveSuite) SetUpSuite(c *gc.C) {
	s.providerSuite.SetUpSuite(c)
	s.cSrv = &localCloudAPIServer{}
	s.mSrv = &localMantaServer{}
	s.cSrv.setupServer(c)
	s.mSrv.setupServer(c)

	s.TestConfig = GetFakeConfig(s.cSrv.Server.URL, s.mSrv.Server.URL)
	s.TestConfig = s.TestConfig.Merge(coretesting.Attrs{
		"image-metadata-url": "test://host",
	})
	s.LiveTests.UploadArches = []string{arch.AMD64}
	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *gc.C) {
	joyent.UnregisterExternalTestImageMetadata()
	s.LiveTests.TearDownSuite(c)
	s.cSrv.destroyServer()
	s.mSrv.destroyServer()
	s.providerSuite.TearDownSuite(c)
}

func (s *localLiveSuite) SetUpTest(c *gc.C) {
	s.providerSuite.SetUpTest(c)
	creds := joyent.MakeCredentials(c, s.TestConfig)
	joyent.UseExternalTestImageMetadata(creds)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) { restoreFinishBootstrap() })
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *gc.C) {
	s.LiveTests.TearDownTest(c)
	s.providerSuite.TearDownTest(c)
}

// localServerSuite contains tests that run against an Joyent service double.
// These tests can test things that would be unreasonably slow or expensive
// to test on a live Joyent server. The service double is started and stopped for
// each test.
type localServerSuite struct {
	providerSuite
	jujutest.Tests
	cSrv *localCloudAPIServer
	mSrv *localMantaServer
}

func (s *localServerSuite) SetUpSuite(c *gc.C) {
	s.providerSuite.SetUpSuite(c)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) { restoreFinishBootstrap() })
}

func (s *localServerSuite) SetUpTest(c *gc.C) {
	s.providerSuite.SetUpTest(c)

	s.cSrv = &localCloudAPIServer{}
	s.mSrv = &localMantaServer{}
	s.cSrv.setupServer(c)
	s.mSrv.setupServer(c)

	s.Tests.ToolsFixture.UploadArches = []string{arch.AMD64}
	s.Tests.SetUpTest(c)
	s.TestConfig = GetFakeConfig(s.cSrv.Server.URL, s.mSrv.Server.URL)
	// Put some fake image metadata in place.
	creds := joyent.MakeCredentials(c, s.TestConfig)
	joyent.UseExternalTestImageMetadata(creds)
}

func (s *localServerSuite) TearDownTest(c *gc.C) {
	joyent.UnregisterExternalTestImageMetadata()
	s.Tests.TearDownTest(c)
	s.cSrv.destroyServer()
	s.mSrv.destroyServer()
	s.providerSuite.TearDownTest(c)
}

func bootstrapContext(c *gc.C) environs.BootstrapContext {
	return coretesting.Context(c)
}

// If the environment is configured not to require a public IP address for nodes,
// bootstrapping and starting an instance should occur without any attempt to
// allocate a public address.
func (s *localServerSuite) TestStartInstance(c *gc.C) {
	env := s.Prepare(c)
	s.Tests.UploadFakeTools(c, env.Storage())
	err := bootstrap.Bootstrap(bootstrapContext(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)
	inst, _ := testing.AssertStartInstance(c, env, "100")
	err = env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	env := s.Prepare(c)
	s.Tests.UploadFakeTools(c, env.Storage())
	err := bootstrap.Bootstrap(bootstrapContext(c), env, environs.BootstrapParams{})
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
	s.Tests.UploadFakeTools(c, env.Storage())
	inst, _ := testing.AssertStartInstance(c, env, "100")
	c.Assert(inst.Status(), gc.Equals, "running")
	err := env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)
}

func (s *localServerSuite) TestInstancesGathering(c *gc.C) {
	env := s.Prepare(c)
	s.Tests.UploadFakeTools(c, env.Storage())
	inst0, _ := testing.AssertStartInstance(c, env, "100")
	id0 := inst0.Id()
	inst1, _ := testing.AssertStartInstance(c, env, "101")
	id1 := inst1.Id()
	c.Logf("id0: %s, id1: %s", id0, id1)
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

// It should be moved to environs.jujutests.Tests.
func (s *localServerSuite) TestBootstrapInstanceUserDataAndState(c *gc.C) {
	env := s.Prepare(c)
	s.Tests.UploadFakeTools(c, env.Storage())
	err := bootstrap.Bootstrap(bootstrapContext(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	// check that the state holds the id of the bootstrap machine.
	stateData, err := bootstrap.LoadState(env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(stateData.StateInstances, gc.HasLen, 1)

	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(stateData.StateInstances[0], gc.Equals, insts[0].Id())

	bootstrapDNS, err := insts[0].DNSName()
	c.Assert(err, gc.IsNil)
	c.Assert(bootstrapDNS, gc.Not(gc.Equals), "")
}

func (s *localServerSuite) TestGetImageMetadataSources(c *gc.C) {
	env := s.Prepare(c)
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
	c.Assert(strings.Contains(urls[0], joyent.ControlBucketName(env)+"/images"), jc.IsTrue)
}

func (s *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	env := s.Prepare(c)
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 1)
	var urls = make([]string, len(sources))
	for i, source := range sources {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		urls[i] = url
	}
	// The control bucket URL contains the bucket name.
	c.Assert(strings.Contains(urls[0], joyent.ControlBucketName(env)+"/tools"), jc.IsTrue)
}

func (s *localServerSuite) TestFindImageBadDefaultImage(c *gc.C) {
	env := s.Prepare(c)
	// An error occurs if no suitable image is found.
	_, err := joyent.FindInstanceSpec(env, "saucy", "amd64", "mem=4G")
	c.Assert(err, gc.ErrorMatches, `no "saucy" images in some-region with arches \[amd64\]`)
}

func (s *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	env := s.Prepare(c)
	params, err := env.(simplestreams.MetadataValidator).MetadataLookupParams("some-region")
	c.Assert(err, gc.IsNil)
	params.Sources, err = imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	params.Series = "raring"
	image_ids, _, err := imagemetadata.ValidateImageMetadata(params)
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

func (s *localServerSuite) TestConstraintsValidator(c *gc.C) {
	env := s.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=amd64 tags=bar cpu-power=10")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, gc.IsNil)
	c.Assert(unsupported, gc.DeepEquals, []string{"cpu-power", "tags"})
}

func (s *localServerSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=ppc64")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64\nvalid values are:.*")
	cons = constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}
