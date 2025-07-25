// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
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
	dummy.Reset(c)
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *OpenSuite) TestNewDummyEnviron(c *gc.C) {
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	cache := jujuclient.NewMemStore()
	controllerCfg := testing.FakeControllerConfig()
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, cache, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	env := bootstrapEnviron.(environs.Environ)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, cfg.AgentStream(), cfg.AgentStream())
	err = bootstrap.Bootstrap(ctx, env, context.NewEmptyCloudCallContext(), bootstrap.BootstrapParams{
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
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	uuid := utils.MustNewUUID().String()
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
		Cloud:            dummy.SampleCloudSpec(),
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
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type: "wondercloud",
		},
	})
	c.Assert(err, gc.ErrorMatches, "no registered provider for.*")
	c.Assert(env, gc.IsNil)
}

func (*OpenSuite) TestNewKubernetes(c *gc.C) {
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type: k8sconstants.CAASProviderType,
		},
	})
	c.Assert(err, gc.ErrorMatches, "cloud environ provider provider.kubernetesEnvironProvider not valid")
	c.Assert(env, gc.IsNil)
}

func (*OpenSuite) TestNew(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	e, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  dummy.SampleCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = e.ControllerInstances(context.NewEmptyCloudCallContext(), "uuid")
	c.Assert(err, gc.ErrorMatches, "model is not prepared")
}

func (*OpenSuite) TestDestroy(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"name": "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)

	store := jujuclient.NewMemStore()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	controllerCfg := testing.FakeControllerConfig()
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	e := bootstrapEnviron.(environs.Environ)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)

	callCtx := context.NewEmptyCloudCallContext()
	err = environs.Destroy("controller-name", e, callCtx, store)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the environment has actually been destroyed
	// and that the controller details been removed too.
	_, err = e.ControllerInstances(callCtx, controllerCfg.ControllerUUID())
	c.Assert(err, gc.ErrorMatches, "model is not prepared")
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (*OpenSuite) TestDestroyNotFound(c *gc.C) {
	var env destroyControllerEnv
	store := jujuclient.NewMemStore()
	err := environs.Destroy("fnord", &env, context.NewEmptyCloudCallContext(), store)
	c.Assert(err, jc.ErrorIsNil)
	env.CheckCallNames(c) // no controller details, no call
}

type destroyControllerEnv struct {
	environs.Environ
	gitjujutesting.Stub
}

func (e *destroyControllerEnv) DestroyController(ctx context.ProviderCallContext, uuid string) error {
	e.MethodCall(e, "DestroyController", ctx, uuid)
	return e.NextErr()
}
