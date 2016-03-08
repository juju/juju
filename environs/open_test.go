// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type OpenSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&OpenSuite{})

func (s *OpenSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&simplestreams.SimplestreamsJujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *OpenSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *OpenSuite) TestNewDummyEnviron(c *gc.C) {
	s.PatchValue(&version.Current, testing.FakeVersionNumber)
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(c)
	cache := jujuclienttesting.NewMemStore()
	env, err := environs.Prepare(ctx, configstore.NewMem(), cache, cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, cfg.AgentStream(), cfg.AgentStream())
	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	// New controller should have been added to collection.
	uuid, exists := cfg.UUID()
	c.Assert(exists, jc.IsTrue)

	foundController, err := cache.ControllerByName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.DeepEquals, uuid)
}

func (s *OpenSuite) TestUpdateEnvInfo(c *gc.C) {
	store := configstore.NewMem()
	cache := jujuclienttesting.NewMemStore()
	ctx := envtesting.BootstrapContext(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "dummy",
		"name": "admin-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = environs.Prepare(ctx, store, cache, "controller-name", environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, jc.ErrorIsNil)

	info, err := store.ReadInfo("controller-name:admin-model")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.NotNil)
	c.Assert(info.APIEndpoint().CACert, gc.Not(gc.Equals), "")
	c.Assert(info.APIEndpoint().ModelUUID, gc.Not(gc.Equals), "")
	c.Assert(info.APICredentials().Password, gc.Not(gc.Equals), "")
	c.Assert(info.APICredentials().User, gc.Equals, "admin@local")

	foundController, err := cache.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: info.APIEndpoint().ServerUUID,
		CACert:         info.APIEndpoint().CACert,
	})
	foundModel, err := cache.ModelByName("controller-name", "admin@local", "admin-model")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundModel, jc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: foundController.ControllerUUID,
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
		"uuid",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, jc.ErrorIsNil)
	store := configstore.NewMem()
	controllerStore := jujuclienttesting.NewMemStore()
	ctx := envtesting.BootstrapContext(c)
	env, err := environs.Prepare(ctx, store, controllerStore, cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the model info file was correctly created.
	info, err := store.ReadInfo("erewhemos:erewhemos")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Initialized(), jc.IsTrue)
	c.Assert(info.BootstrapConfig(), gc.DeepEquals, env.Config().AllAttrs())
	c.Logf("bootstrap config: %#v", info.BootstrapConfig())

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

	// Check that a uuid was chosen.
	uuid, exists := env.Config().UUID()
	c.Assert(exists, jc.IsTrue)
	c.Assert(uuid, gc.Matches, `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)

	// Check that controller was cached
	foundController, err := controllerStore.ControllerByName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.DeepEquals, uuid)

	// Check we cannot call Prepare again.
	env, err = environs.Prepare(ctx, store, controllerStore, cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
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
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, jc.ErrorIsNil)

	ctx := envtesting.BootstrapContext(c)
	env0, err := environs.Prepare(ctx, configstore.NewMem(), jujuclienttesting.NewMemStore(), cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, jc.ErrorIsNil)
	adminSecret0 := env0.Config().AdminSecret()
	c.Assert(adminSecret0, gc.HasLen, 32)
	c.Assert(adminSecret0, gc.Matches, "^[0-9a-f]*$")

	env1, err := environs.Prepare(ctx, configstore.NewMem(), jujuclienttesting.NewMemStore(), cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
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
	store := configstore.NewMem()
	controllerStore := jujuclienttesting.NewMemStore()
	env, err := environs.Prepare(envtesting.BootstrapContext(c), store, controllerStore, cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, gc.ErrorMatches, "cannot ensure CA certificate: controller configuration with a certificate but no CA private key")
	c.Assert(env, gc.IsNil)
	// Ensure that the config storage info is cleaned up.
	_, err = store.ReadInfo(cfg.Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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
	env, err := environs.Prepare(ctx, configstore.NewMem(), jujuclienttesting.NewMemStore(), cfg.Name(), environs.PrepareForBootstrapParams{Config: cfg})
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
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)

	configstore := configstore.NewMem()
	store := jujuclienttesting.NewMemStore()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	ctx := envtesting.BootstrapContext(c)
	e, err := environs.Prepare(ctx, configstore, store, "controller-name", environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, jc.ErrorIsNil)
	_, err = configstore.ReadInfo("controller-name:erewhemos")
	c.Assert(err, jc.ErrorIsNil)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)

	err = environs.Destroy("controller-name", e, configstore, store)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the environment has actually been destroyed
	// and that the config info has been destroyed too.
	_, err = e.ControllerInstances()
	c.Assert(err, gc.ErrorMatches, "model has been destroyed")
	_, err = configstore.ReadInfo("controller-name:erewhemos")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
