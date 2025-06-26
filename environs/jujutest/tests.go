// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	stdcontext "context"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with the given configuration. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	TestConfig     coretesting.Attrs
	Credential     cloud.Credential
	CloudEndpoint  string
	CloudRegion    string
	ControllerUUID string
	Env            environs.Environ
	envtesting.ToolsFixture
	sstesting.TestDataSuite

	// ControllerStore holds the controller related information
	// such as controllers, accounts, etc., used when preparing
	// the environment. This is initialized by SetUpSuite.
	ControllerStore jujuclient.ClientStore
	toolsStorage    storage.Storage

	// ProviderCallContext holds the context to be used to make
	// calls to a cloud provider.
	ProviderCallContext context.ProviderCallContext

	// BootstrapContext holds the context to bootstrap a test environment.
	BootstrapContext environs.BootstrapContext
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *gc.C, ctx stdcontext.Context, cfg *config.Config) environs.Environ {
	e, err := environs.New(ctx, environs.OpenParams{
		ControllerUUID: t.ControllerUUID,
		Cloud:          t.CloudSpec(),
		Config:         cfg,
	})
	c.Assert(err, gc.IsNil, gc.Commentf("opening environ %#v", cfg.AllAttrs()))
	c.Assert(e, gc.NotNil)
	return e
}

func (t *Tests) CloudSpec() environscloudspec.CloudSpec {
	credential := t.Credential
	if credential.AuthType() == "" {
		credential = cloud.NewEmptyCredential()
	}
	return environscloudspec.CloudSpec{
		Type:       t.TestConfig["type"].(string),
		Name:       t.TestConfig["type"].(string),
		Region:     t.CloudRegion,
		Endpoint:   t.CloudEndpoint,
		Credential: &credential,
	}
}

// PrepareParams returns the environs.PrepareParams that will be used to call
// environs.Prepare.
func (t *Tests) PrepareParams(c *gc.C) bootstrap.PrepareParams {
	testConfigCopy := t.TestConfig.Merge(nil)

	return bootstrap.PrepareParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		ModelConfig:      testConfigCopy,
		Cloud:            t.CloudSpec(),
		ControllerName:   t.TestConfig["name"].(string),
		AdminSecret:      AdminSecret,
	}
}

// Prepare prepares an instance of the testing environment.
func (t *Tests) Prepare(c *gc.C) environs.Environ {
	t.Env = t.PrepareWithParams(c, t.PrepareParams(c))
	return t.Env
}

// PrepareWithParams prepares an instance of the testing environment.
func (t *Tests) PrepareWithParams(c *gc.C, params bootstrap.PrepareParams) environs.Environ {
	e, err := bootstrap.PrepareController(false, t.BootstrapContext, t.ControllerStore, params)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", params.ModelConfig))
	c.Assert(e, gc.NotNil)
	t.Env = e.(environs.Environ)
	return t.Env
}

func (t *Tests) AssertPrepareFailsWithConfig(c *gc.C, badConfig coretesting.Attrs, errorMatches string) error {
	args := t.PrepareParams(c)
	args.ModelConfig = coretesting.Attrs(args.ModelConfig).Merge(badConfig)

	e, err := bootstrap.PrepareController(false, t.BootstrapContext, t.ControllerStore, args)
	c.Assert(err, gc.ErrorMatches, errorMatches)
	c.Assert(e, gc.IsNil)
	return err
}

func (t *Tests) SetUpTest(c *gc.C) {
	storageDir := c.MkDir()
	baseURLPath := filepath.Join(storageDir, "tools")
	t.DefaultBaseURL = utils.MakeFileURL(baseURLPath)
	t.ToolsFixture.SetUpTest(c)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	t.UploadFakeTools(c, stor, "released", "released")
	t.toolsStorage = stor
	t.ControllerStore = jujuclient.NewMemStore()
	t.ControllerUUID = coretesting.FakeControllerConfig().ControllerUUID()

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	ctx := stdcontext.WithValue(stdcontext.TODO(), bootstrap.SimplestreamsFetcherContextKey, ss)
	t.BootstrapContext = envtesting.BootstrapContext(ctx, c)
	t.ProviderCallContext = context.NewCloudCallContext(ctx)
}

func (t *Tests) TearDownTest(c *gc.C) {
	t.ToolsFixture.TearDownTest(c)
}

func (t *Tests) TestStartStop(c *gc.C) {
	e := t.Prepare(c)
	cfg, err := e.Config().Apply(map[string]interface{}{
		"agent-version": jujuversion.Current.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = e.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	insts, err := e.Instances(t.ProviderCallContext, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 0)

	inst0, hc := testing.AssertStartInstance(c, e, t.ProviderCallContext, t.ControllerUUID, "0")
	c.Assert(inst0, gc.NotNil)
	id0 := inst0.Id()
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(hc.Mem, gc.NotNil)
	c.Assert(hc.CpuCores, gc.NotNil)

	inst1, _ := testing.AssertStartInstance(c, e, t.ProviderCallContext, t.ControllerUUID, "1")
	c.Assert(inst1, gc.NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances(t.ProviderCallContext, []instance.Id{id0, id1})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Not(gc.Equals), insts[1].Id())

	err = e.StopInstances(t.ProviderCallContext, inst0.Id())
	c.Assert(err, jc.ErrorIsNil)

	insts, err = e.Instances(t.ProviderCallContext, []instance.Id{id0, id1})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts[0], gc.IsNil)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	insts, err = e.AllInstances(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts[0].Id(), gc.Equals, id1)
}

func (t *Tests) TestBootstrap(c *gc.C) {
	credential := t.Credential
	if credential.AuthType() == "" {
		credential = cloud.NewEmptyCredential()
	}

	var regions []cloud.Region
	if t.CloudRegion != "" {
		regions = []cloud.Region{{
			Name:     t.CloudRegion,
			Endpoint: t.CloudEndpoint,
		}}
	}

	args := bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		Cloud: cloud.Cloud{
			Name:      t.TestConfig["type"].(string),
			Type:      t.TestConfig["type"].(string),
			AuthTypes: []cloud.AuthType{credential.AuthType()},
			Regions:   regions,
			Endpoint:  t.CloudEndpoint,
		},
		CloudRegion:             t.CloudRegion,
		CloudCredential:         &credential,
		CloudCredentialName:     "credential",
		AdminSecret:             AdminSecret,
		CAPrivateKey:            coretesting.CAKey,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		SSHServerHostKey:        coretesting.SSHServerHostKey,
	}

	e := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, e, t.ProviderCallContext, args)
	c.Assert(err, jc.ErrorIsNil)

	controllerInstances, err := e.ControllerInstances(t.ProviderCallContext, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInstances, gc.Not(gc.HasLen), 0)

	e2 := t.Open(c, t.BootstrapContext.Context(), e.Config())
	controllerInstances2, err := e2.ControllerInstances(t.ProviderCallContext, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInstances2, gc.Not(gc.HasLen), 0)
	c.Assert(controllerInstances2, jc.SameContents, controllerInstances)

	err = environs.Destroy(e2.Config().Name(), e2, t.ProviderCallContext, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)

	// Prepare again because Destroy invalidates old environments.
	e3 := t.Prepare(c)

	err = bootstrap.Bootstrap(t.BootstrapContext, e3, t.ProviderCallContext, args)
	c.Assert(err, jc.ErrorIsNil)

	err = environs.Destroy(e3.Config().Name(), e3, t.ProviderCallContext, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
}
