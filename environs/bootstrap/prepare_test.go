// Copyright 2012, 2013, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type PrepareSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&PrepareSuite{})

func (s *PrepareSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *PrepareSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (*PrepareSuite) TestPrepare(c *gc.C) {
	baselineAttrs := dummy.SampleConfig().Merge(testing.Attrs{
		"controller": false,
		"name":       "erewhemos",
	}).Delete(
		"admin-secret",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, jc.ErrorIsNil)
	controllerStore := jujuclienttesting.NewMemStore()
	ctx := envtesting.BootstrapContext(c)
	controllerCfg := controller.Config{controller.ControllerUUIDKey: testing.ModelTag.Id()}
	env, err := bootstrap.Prepare(ctx, controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		BaseConfig:       cfg.AllAttrs(),
		CloudName:        "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that an admin-secret was chosen.
	adminSecret := env.Config().AdminSecret()
	c.Assert(adminSecret, gc.HasLen, 32)
	c.Assert(adminSecret, gc.Matches, "^[0-9a-f]*$")

	// Check that the CA cert was generated.
	cfgCertPEM, cfgCertOK := controllerCfg.CACert()
	cfgKeyPEM, cfgKeyOK := controllerCfg.CAPrivateKey()
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
	c.Assert(foundController.Cloud, gc.Equals, "dummy")

	// Check we cannot call Prepare again.
	env, err = bootstrap.Prepare(ctx, controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		BaseConfig:       cfg.AllAttrs(),
		CloudName:        "dummy",
	})
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches, `controller "erewhemos" already exists`)
}

func (*PrepareSuite) TestPrepareGeneratesDifferentAdminSecrets(c *gc.C) {
	baselineAttrs := dummy.SampleConfig().Merge(testing.Attrs{
		"controller": false,
		"name":       "erewhemos",
	}).Delete(
		"admin-secret",
	)

	ctx := envtesting.BootstrapContext(c)
	env0, err := bootstrap.Prepare(ctx, jujuclienttesting.NewMemStore(), bootstrap.PrepareParams{
		ControllerConfig: testing.FakeControllerBootstrapConfig(),
		ControllerName:   "erewhemos",
		BaseConfig:       baselineAttrs,
		CloudName:        "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	adminSecret0 := env0.Config().AdminSecret()
	c.Assert(adminSecret0, gc.HasLen, 32)
	c.Assert(adminSecret0, gc.Matches, "^[0-9a-f]*$")

	// Allocate a new UUID, or we'll end up with the same config.
	newUUID := utils.MustNewUUID()
	baselineAttrs[config.UUIDKey] = newUUID.String()
	controllerCfg := testing.FakeControllerBootstrapConfig()
	controllerCfg[controller.ControllerUUIDKey] = newUUID.String()

	env1, err := bootstrap.Prepare(ctx, jujuclienttesting.NewMemStore(), bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "erewhemos",
		BaseConfig:       baselineAttrs,
		CloudName:        "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	adminSecret1 := env1.Config().AdminSecret()
	c.Assert(adminSecret1, gc.HasLen, 32)
	c.Assert(adminSecret1, gc.Matches, "^[0-9a-f]*$")

	c.Assert(adminSecret1, gc.Not(gc.Equals), adminSecret0)
}

func (*PrepareSuite) TestPrepareWithMissingKey(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	controllerStore := jujuclienttesting.NewMemStore()
	controllerCfg := testing.FakeControllerBootstrapConfig()
	delete(controllerCfg, "ca-private-key")
	env, err := bootstrap.Prepare(envtesting.BootstrapContext(c), controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		BaseConfig:       cfg.AllAttrs(),
		CloudName:        "dummy",
	})
	c.Assert(err, gc.ErrorMatches, "cannot ensure CA certificate: controller configuration with a certificate but no CA private key")
	c.Assert(env, gc.IsNil)
}

func (*PrepareSuite) TestPrepareWithExistingKeyPair(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(c)
	controllerCfg := testing.FakeControllerBootstrapConfig()
	_, err = bootstrap.Prepare(ctx, jujuclienttesting.NewMemStore(), bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		BaseConfig:       cfg.AllAttrs(),
		CloudName:        "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	cfgCertPEM, cfgCertOK := controllerCfg.CACert()
	cfgKeyPEM, cfgKeyOK := controllerCfg.CAPrivateKey()
	c.Assert(cfgCertOK, jc.IsTrue)
	c.Assert(cfgKeyOK, jc.IsTrue)
	c.Assert(string(cfgCertPEM), gc.DeepEquals, testing.CACert)
	c.Assert(string(cfgKeyPEM), gc.DeepEquals, testing.CAKey)
}
