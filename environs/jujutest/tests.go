// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"context"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
)

const (
	AdminSecret = "admin-secret"
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

	// BootstrapContext holds the context to bootstrap a test environment.
	BootstrapContext environs.BootstrapContext
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *tc.C, ctx context.Context, cfg *config.Config) environs.Environ {
	e, err := environs.New(ctx, environs.OpenParams{
		ControllerUUID: t.ControllerUUID,
		Cloud:          t.CloudSpec(),
		Config:         cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.IsNil, tc.Commentf("opening environ %#v", cfg.AllAttrs()))
	c.Assert(e, tc.NotNil)
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
func (t *Tests) PrepareParams(c *tc.C) bootstrap.PrepareParams {
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
func (t *Tests) Prepare(c *tc.C) environs.Environ {
	t.Env = t.PrepareWithParams(c, t.PrepareParams(c))
	return t.Env
}

// PrepareWithParams prepares an instance of the testing environment.
func (t *Tests) PrepareWithParams(c *tc.C, params bootstrap.PrepareParams) environs.Environ {
	e, err := bootstrap.PrepareController(false, t.BootstrapContext, t.ControllerStore, params)
	c.Assert(err, tc.IsNil, tc.Commentf("preparing environ %#v", params.ModelConfig))
	c.Assert(e, tc.NotNil)
	t.Env = e.(environs.Environ)
	return t.Env
}

func (t *Tests) AssertPrepareFailsWithConfig(c *tc.C, badConfig coretesting.Attrs, errorMatches string) error {
	args := t.PrepareParams(c)
	args.ModelConfig = coretesting.Attrs(args.ModelConfig).Merge(badConfig)

	e, err := bootstrap.PrepareController(false, t.BootstrapContext, t.ControllerStore, args)
	c.Assert(err, tc.ErrorMatches, errorMatches)
	c.Assert(e, tc.IsNil)
	return err
}

func (t *Tests) SetUpTest(c *tc.C) {
	storageDir := c.MkDir()
	baseURLPath := filepath.Join(storageDir, "tools")
	t.DefaultBaseURL = utils.MakeFileURL(baseURLPath)
	t.ToolsFixture.SetUpTest(c)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	t.UploadFakeTools(c, stor, "released")
	t.toolsStorage = stor
	t.ControllerStore = jujuclient.NewMemStore()
	t.ControllerUUID = coretesting.FakeControllerConfig().ControllerUUID()

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	ctx := context.WithValue(c.Context(), bootstrap.SimplestreamsFetcherContextKey, ss)
	t.BootstrapContext = envtesting.BootstrapContext(ctx, c)
}

func (t *Tests) TearDownTest(c *tc.C) {
	t.ToolsFixture.TearDownTest(c)
}

func (t *Tests) TestStartStop(c *tc.C) {
	e := t.Prepare(c)
	cfg, err := e.Config().Apply(map[string]interface{}{
		"agent-version": jujuversion.Current.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = e.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	insts, err := e.Instances(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts, tc.HasLen, 0)

	inst0, hc := testing.AssertStartInstance(c, e, t.ControllerUUID, "0")
	c.Assert(inst0, tc.NotNil)
	id0 := inst0.Id()
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, tc.NotNil)
	c.Assert(hc.Mem, tc.NotNil)
	c.Assert(hc.CpuCores, tc.NotNil)

	inst1, _ := testing.AssertStartInstance(c, e, t.ControllerUUID, "1")
	c.Assert(inst1, tc.NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances(c.Context(), []instance.Id{id0, id1})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts, tc.HasLen, 2)
	c.Assert(insts[0].Id(), tc.Equals, id0)
	c.Assert(insts[1].Id(), tc.Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts, tc.HasLen, 2)
	c.Assert(insts[0].Id(), tc.Not(tc.Equals), insts[1].Id())

	err = e.StopInstances(c.Context(), inst0.Id())
	c.Assert(err, tc.ErrorIsNil)

	insts, err = e.Instances(c.Context(), []instance.Id{id0, id1})
	c.Assert(err, tc.ErrorIs, environs.ErrPartialInstances)
	c.Assert(insts, tc.HasLen, 2)
	c.Assert(insts[0], tc.IsNil)
	c.Assert(insts[1].Id(), tc.Equals, id1)

	insts, err = e.AllInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts[0].Id(), tc.Equals, id1)
}

func (t *Tests) TestBootstrap(c *tc.C) {
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
	err := bootstrap.Bootstrap(t.BootstrapContext, e, args)
	c.Assert(err, tc.ErrorIsNil)

	controllerInstances, err := e.ControllerInstances(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerInstances, tc.Not(tc.HasLen), 0)

	e2 := t.Open(c, t.BootstrapContext, e.Config())
	controllerInstances2, err := e2.ControllerInstances(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerInstances2, tc.Not(tc.HasLen), 0)
	c.Assert(controllerInstances2, tc.SameContents, controllerInstances)

	err = environs.Destroy(e2.Config().Name(), e2, c.Context(), t.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)

	// Prepare again because Destroy invalidates old environments.
	e3 := t.Prepare(c)

	err = bootstrap.Bootstrap(t.BootstrapContext, e3, args)
	c.Assert(err, tc.ErrorIsNil)

	err = environs.Destroy(e3.Config().Name(), e3, c.Context(), t.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)
}
