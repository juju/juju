// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"context"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
)

type OpenSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&OpenSuite{})

func (s *OpenSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *OpenSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *OpenSuite) TestNewDummyEnviron(c *gc.C) {
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(context.Background(), c)
	cache := jujuclient.NewMemStore()
	controllerCfg := testing.FakeControllerConfig()
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, cache, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	env := bootstrapEnviron.(environs.Environ)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, cfg.AgentStream(), cfg.AgentStream())
	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{
		ControllerConfig:        controllerCfg,
		AdminSecret:             "admin-secret",
		CAPrivateKey:            testing.CAKey,
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	})
	c.Assert(err, jc.ErrorIsNil)

	// New controller should have been added to collection.
	foundController, err := cache.ControllerByName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.DeepEquals, controllerCfg.ControllerUUID())
}

func (s *OpenSuite) TestUpdateEnvInfo(c *gc.C) {
	store := jujuclient.NewMemStore()
	ctx := envtesting.BootstrapContext(context.Background(), c)
	uuid := uuid.MustNewUUID().String()
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "dummy",
		"name": "admin-model",
		"uuid": uuid,
	})
	c.Assert(err, jc.ErrorIsNil)
	controllerCfg := testing.FakeControllerConfig()
	_, err = bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	foundController, err := store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.Not(gc.Equals), "")
	c.Assert(foundController.CACert, gc.Not(gc.Equals), "")
	foundModel, err := store.ModelByName("controller-name", "admin/admin-model")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundModel, jc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: cfg.UUID(),
		ModelType: model.IAAS,
	})
}

func (*OpenSuite) TestNewUnknownEnviron(c *gc.C) {
	env, err := environs.New(context.Background(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type: "wondercloud",
		},
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, gc.ErrorMatches, "no registered provider for.*")
	c.Assert(env, gc.IsNil)
}

func (*OpenSuite) TestNew(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig().Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	ctx := context.Background()
	e, err := environs.New(ctx, environs.OpenParams{
		Cloud:  testing.FakeCloudSpec(),
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, jc.ErrorIsNil)
	_, err = e.ControllerInstances(ctx, "uuid")
	c.Assert(err, gc.ErrorMatches, "model is not prepared")
}

func (*OpenSuite) TestDestroy(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig().Merge(
		testing.Attrs{
			"name": "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)

	store := jujuclient.NewMemStore()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	controllerCfg := testing.FakeControllerConfig()
	ctx := envtesting.BootstrapContext(context.Background(), c)
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	e := bootstrapEnviron.(environs.Environ)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)

	err = environs.Destroy("controller-name", e, context.Background(), store)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the environment has actually been destroyed
	// and that the controller details been removed too.
	_, err = e.ControllerInstances(context.Background(), controllerCfg.ControllerUUID())
	c.Assert(err, gc.ErrorMatches, "model is not prepared")
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (*OpenSuite) TestDestroyNotFound(c *gc.C) {
	var env destroyControllerEnv
	store := jujuclient.NewMemStore()
	err := environs.Destroy("fnord", &env, context.Background(), store)
	c.Assert(err, jc.ErrorIsNil)
	env.CheckCallNames(c) // no controller details, no call
}

type destroyControllerEnv struct {
	environs.Environ
	jujutesting.Stub
}

func (e *destroyControllerEnv) DestroyController(ctx context.Context, uuid string) error {
	e.MethodCall(e, "DestroyController", ctx, uuid)
	return e.NextErr()
}
