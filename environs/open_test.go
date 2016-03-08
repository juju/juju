// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
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
	s.PatchValue(&juju.JujuPublicKey, sstesting.SignedMetadataPublicKey)
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
	ctx := envtesting.BootstrapContext(c)
	cache := jujuclienttesting.NewMemStore()
	env, err := environs.Prepare(ctx, cache, environs.PrepareParams{
		ControllerName: cfg.Name(),
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	err = envtesting.UploadFakeToolsToSimpleStreams(stor, cfg.AgentStream(), cfg.AgentStream())
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	// New controller should have been added to collection.
	foundController, err := cache.ControllerByName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.DeepEquals, cfg.UUID())
}

func (s *OpenSuite) TestUpdateEnvInfo(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	ctx := envtesting.BootstrapContext(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type":            "dummy",
		"name":            "admin-model",
		"controller-uuid": utils.MustNewUUID().String(),
		"uuid":            utils.MustNewUUID().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = environs.Prepare(ctx, store, environs.PrepareParams{
		ControllerName: "controller-name",
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	foundController, err := store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.Equals, cfg.ControllerUUID())
	c.Assert(foundController.CACert, gc.Not(gc.Equals), "")
	foundModel, err := store.ModelByName("controller-name", "admin@local", "admin-model")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundModel, jc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: cfg.UUID(),
	})
}

func (*OpenSuite) TestNewUnknownEnviron(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"type": "wondercloud",
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.ErrorMatches, "no registered provider for.*")
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
	e, err := environs.New(cfg)
	c.Assert(err, gc.ErrorMatches, "model is not prepared")
	c.Assert(e, gc.IsNil)
}

func (*OpenSuite) TestPrepare(c *gc.C) {
	baselineAttrs := dummy.SampleConfig().Merge(testing.Attrs{
		"controller": false,
		"name":       "erewhemos",
	}).Delete(
		"ca-cert",
		"ca-private-key",
		"admin-secret",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, jc.ErrorIsNil)
	controllerStore := jujuclienttesting.NewMemStore()
	ctx := envtesting.BootstrapContext(c)
	env, err := environs.Prepare(ctx, controllerStore, environs.PrepareParams{
		ControllerName: cfg.Name(),
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that an admin-secret was chosen.
	adminSecret := env.Config().AdminSecret()
	c.Assert(adminSecret, gc.HasLen, 32)
	c.Assert(adminSecret, gc.Matches, "^[0-9a-f]*$")

	// Check that the CA cert was generated.
	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, jc.IsTrue)
	c.Assert(cfgKeyOK, jc.IsTrue)

	// Check the common name of the generated cert
	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(caCert.Subject.CommonName, gc.Equals, `juju-generated CA for model "`+testing.SampleModelName+`"`)

	// Check that controller was cached
	foundController, err := controllerStore.ControllerByName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.DeepEquals, cfg.UUID())

	// Check we cannot call Prepare again.
	env, err = environs.Prepare(ctx, controllerStore, environs.PrepareParams{
		ControllerName: cfg.Name(),
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches, `controller "erewhemos" already exists`)
}

func (*OpenSuite) TestPrepareGeneratesDifferentAdminSecrets(c *gc.C) {
	baselineAttrs := dummy.SampleConfig().Merge(testing.Attrs{
		"controller": false,
		"name":       "erewhemos",
	}).Delete(
		"admin-secret",
	)

	ctx := envtesting.BootstrapContext(c)
	env0, err := environs.Prepare(ctx, jujuclienttesting.NewMemStore(), environs.PrepareParams{
		ControllerName: "erewhemos",
		BaseConfig:     baselineAttrs,
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	adminSecret0 := env0.Config().AdminSecret()
	c.Assert(adminSecret0, gc.HasLen, 32)
	c.Assert(adminSecret0, gc.Matches, "^[0-9a-f]*$")

	// Allocate a new UUID, or we'll end up with the same config.
	newUUID := utils.MustNewUUID()
	baselineAttrs[config.UUIDKey] = newUUID.String()
	baselineAttrs[config.ControllerUUIDKey] = newUUID.String()

	env1, err := environs.Prepare(ctx, jujuclienttesting.NewMemStore(), environs.PrepareParams{
		ControllerName: "erewhemos",
		BaseConfig:     baselineAttrs,
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	adminSecret1 := env1.Config().AdminSecret()
	c.Assert(adminSecret1, gc.HasLen, 32)
	c.Assert(adminSecret1, gc.Matches, "^[0-9a-f]*$")

	c.Assert(adminSecret1, gc.Not(gc.Equals), adminSecret0)
}

func (*OpenSuite) TestPrepareWithMissingKey(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Delete("ca-cert", "ca-private-key").Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
			"ca-cert":    string(testing.CACert),
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	controllerStore := jujuclienttesting.NewMemStore()
	env, err := environs.Prepare(envtesting.BootstrapContext(c), controllerStore, environs.PrepareParams{
		ControllerName: cfg.Name(),
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, gc.ErrorMatches, "cannot ensure CA certificate: controller configuration with a certificate but no CA private key")
	c.Assert(env, gc.IsNil)
}

func (*OpenSuite) TestPrepareWithExistingKeyPair(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"controller":     false,
			"name":           "erewhemos",
			"ca-cert":        string(testing.CACert),
			"ca-private-key": string(testing.CAKey),
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(c)
	env, err := environs.Prepare(ctx, jujuclienttesting.NewMemStore(), environs.PrepareParams{
		ControllerName: cfg.Name(),
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, jc.IsTrue)
	c.Assert(cfgKeyOK, jc.IsTrue)
	c.Assert(string(cfgCertPEM), gc.DeepEquals, testing.CACert)
	c.Assert(string(cfgKeyPEM), gc.DeepEquals, testing.CAKey)
}

func (*OpenSuite) TestDestroy(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"name": "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)

	store := jujuclienttesting.NewMemStore()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	ctx := envtesting.BootstrapContext(c)
	e, err := environs.Prepare(ctx, store, environs.PrepareParams{
		ControllerName: "controller-name",
		BaseConfig:     cfg.AllAttrs(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)

	err = environs.Destroy("controller-name", e, store)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the environment has actually been destroyed
	// and that the controller details been removed too.
	_, err = e.ControllerInstances()
	c.Assert(err, gc.ErrorMatches, "model is not prepared")
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
