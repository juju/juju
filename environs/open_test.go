// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

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
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
)

type OpenSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

func TestOpenSuite(t *stdtesting.T) { tc.Run(t, &OpenSuite{}) }
func (s *OpenSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *OpenSuite) TearDownTest(c *tc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *OpenSuite) TestNewDummyEnviron(c *tc.C) {
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(c.Context(), c)
	cache := jujuclient.NewMemStore()
	controllerCfg := testing.FakeControllerConfig()
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, cache, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	env := bootstrapEnviron.(environs.Environ)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")
	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{
		ControllerConfig:        controllerCfg,
		AdminSecret:             "admin-secret",
		CAPrivateKey:            testing.CAKey,
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)

	// New controller should have been added to collection.
	foundController, err := cache.ControllerByName(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, tc.DeepEquals, controllerCfg.ControllerUUID())
}

func (s *OpenSuite) TestUpdateEnvInfo(c *tc.C) {
	store := jujuclient.NewMemStore()
	ctx := envtesting.BootstrapContext(c.Context(), c)
	uuid := uuid.MustNewUUID().String()
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "dummy",
		"name": "admin-model",
		"uuid": uuid,
	})
	c.Assert(err, tc.ErrorIsNil)
	controllerCfg := testing.FakeControllerConfig()
	_, err = bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)

	foundController, err := store.ControllerByName("controller-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, tc.Not(tc.Equals), "")
	c.Assert(foundController.CACert, tc.Not(tc.Equals), "")
	foundModel, err := store.ModelByName("controller-name", "admin/admin-model")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundModel, tc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: cfg.UUID(),
		ModelType: model.IAAS,
	})
}

func (*OpenSuite) TestNewUnknownEnviron(c *tc.C) {
	env, err := environs.New(c.Context(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type: "wondercloud",
		},
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorMatches, "no registered provider for.*")
	c.Assert(env, tc.IsNil)
}

func (*OpenSuite) TestNew(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig().Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, tc.ErrorIsNil)
	ctx := c.Context()
	e, err := environs.New(ctx, environs.OpenParams{
		Cloud:  testing.FakeCloudSpec(),
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	_, err = e.ControllerInstances(ctx, "uuid")
	c.Assert(err, tc.ErrorMatches, "model is not prepared")
}

func (*OpenSuite) TestDestroy(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig().Merge(
		testing.Attrs{
			"name": "erewhemos",
		},
	))
	c.Assert(err, tc.ErrorIsNil)

	store := jujuclient.NewMemStore()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	controllerCfg := testing.FakeControllerConfig()
	ctx := envtesting.BootstrapContext(c.Context(), c)
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	e := bootstrapEnviron.(environs.Environ)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, tc.ErrorIsNil)

	err = environs.Destroy("controller-name", e, c.Context(), store)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the environment has actually been destroyed
	// and that the controller details been removed too.
	_, err = e.ControllerInstances(c.Context(), controllerCfg.ControllerUUID())
	c.Assert(err, tc.ErrorMatches, "model is not prepared")
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (*OpenSuite) TestDestroyNotFound(c *tc.C) {
	var env destroyControllerEnv
	store := jujuclient.NewMemStore()
	err := environs.Destroy("fnord", &env, c.Context(), store)
	c.Assert(err, tc.ErrorIsNil)
	env.CheckCallNames(c) // no controller details, no call
}

type destroyControllerEnv struct {
	environs.Environ
	testhelpers.Stub
}

func (e *destroyControllerEnv) DestroyController(ctx context.Context, uuid string) error {
	e.MethodCall(e, "DestroyController", ctx, uuid)
	return e.NextErr()
}
