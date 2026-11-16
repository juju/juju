// Copyright 2012, 2013, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
)

type PrepareSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

func TestPrepareSuite(t *stdtesting.T) {
	tc.Run(t, &PrepareSuite{})
}

func (s *PrepareSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *PrepareSuite) TearDownTest(c *tc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *PrepareSuite) TestPrepare(c *tc.C) {
	s.assertPrepare(c, false)
}

func (s *PrepareSuite) TestPrepareSkipVerify(c *tc.C) {
	s.assertPrepare(c, true)
}

func (s *PrepareSuite) assertPrepare(c *tc.C, skipVerify bool) {
	baselineAttrs := testing.FakeConfig().Merge(testing.Attrs{
		"somebool":  false,
		"name":      "erewhemos",
		"test-mode": true,
	}).Delete(
		"admin-secret",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, tc.ErrorIsNil)
	controllerStore := jujuclient.NewMemStore()
	ctx := envtesting.BootstrapTestContext(c)
	controllerCfg := controller.Config{
		controller.ControllerUUIDKey:       testing.ControllerTag.Id(),
		controller.CACertKey:               testing.CACert,
		controller.APIPort:                 17777,
		controller.SetNUMAControlPolicyKey: true,
	}
	cloudSpec := testing.FakeCloudSpec()
	cloudSpec.SkipTLSVerify = skipVerify
	var caCerts []string
	if !skipVerify {
		caCerts = []string{testing.CACert}
		cloudSpec.CACertificates = caCerts
	}
	_, err = bootstrap.PrepareController(false, ctx, controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            cloudSpec,
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check that controller was cached
	foundController, err := controllerStore.ControllerByName(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, tc.DeepEquals, controllerCfg.ControllerUUID())
	c.Assert(foundController.Cloud, tc.Equals, "dummy")
	c.Assert(foundController.CloudType, tc.Equals, "dummy")

	// Check that bootstrap config was written
	bootstrapCfg, err := controllerStore.BootstrapConfigForController(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bootstrapCfg, tc.DeepEquals, &jujuclient.BootstrapConfig{
		ControllerConfig: controller.Config{
			controller.APIPort:                 17777,
			controller.SetNUMAControlPolicyKey: true,
		},
		Config: map[string]any{
			"firewall-mode":             "instance",
			"ssl-hostname-verification": true,
			"logging-config":            "<root>=INFO",
			"type":                      "dummy",
			"name":                      "erewhemos",
			"somebool":                  false,
			"development":               false,
			"test-mode":                 true,
		},
		ControllerModelUUID:   cfg.UUID(),
		Cloud:                 "dummy",
		CloudRegion:           "dummy-region",
		CloudType:             "dummy",
		CloudEndpoint:         "dummy-endpoint",
		CloudIdentityEndpoint: "dummy-identity-endpoint",
		CloudStorageEndpoint:  "dummy-storage-endpoint",
		CloudCACertificates:   caCerts,
		SkipTLSVerify:         skipVerify,
	})

	// Check we cannot call Prepare again.
	_, err = bootstrap.PrepareController(false, ctx, controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIs, errors.AlreadyExists)
	c.Assert(err, tc.ErrorMatches, `controller "erewhemos" already exists`)
}

// TestOpenControllerDoesNotPrepareOrRegister checks that opening a
// controller environ neither prepares it for bootstrapping nor records
// anything in the client store, and that the registration stage then
// writes the expected controller, bootstrap config, account and model
// records.
func (s *PrepareSuite) TestOpenControllerDoesNotPrepareOrRegister(c *tc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"somebool":  false,
		"name":      "erewhemos",
		"test-mode": true,
	}).Delete(
		"admin-secret",
	)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	controllerStore := jujuclient.NewMemStore()
	ctx := envtesting.BootstrapTestContext(c)
	controllerCfg := controller.Config{
		controller.ControllerUUIDKey:       testing.ControllerTag.Id(),
		controller.CACertKey:               testing.CACert,
		controller.APIPort:                 17777,
		controller.SetNUMAControlPolicyKey: true,
	}
	preparation, err := bootstrap.OpenController(false, ctx, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(preparation, tc.NotNil)
	c.Assert(preparation.Environ, tc.NotNil)

	// Opening the controller records nothing in the client store.
	controllers, err := controllerStore.AllControllers()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllers, tc.HasLen, 0)

	// Registering the controller writes the expected records.
	c.Assert(preparation.RegisterController(controllerStore, cfg.Name()), tc.ErrorIsNil)

	// Opening an already-registered controller succeeds and rewrites
	// nothing in the client store.
	secondPreparation, err := bootstrap.OpenController(false, ctx, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            testing.FakeCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(secondPreparation, tc.NotNil)
	c.Assert(secondPreparation.Environ, tc.NotNil)

	foundController, err := controllerStore.ControllerByName(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundController.ControllerUUID, tc.Equals, controllerCfg.ControllerUUID())
	c.Check(foundController.Cloud, tc.Equals, "dummy")
	c.Check(foundController.CloudType, tc.Equals, "dummy")

	bootstrapCfg, err := controllerStore.BootstrapConfigForController(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(bootstrapCfg, tc.NotNil)
	c.Check(bootstrapCfg.ControllerModelUUID, tc.Equals, cfg.UUID())
	c.Check(bootstrapCfg.CloudType, tc.Equals, "dummy")

	account, err := controllerStore.AccountDetails(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(account.User, tc.Equals, environs.AdminUser)
	c.Check(account.Password, tc.Equals, "admin-secret")

	modelName := jujuclient.QualifyModelName(environs.AdminUser, cfg.Name())
	modelDetails, err := controllerStore.ModelByName(cfg.Name(), modelName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelDetails.ModelUUID, tc.Equals, cfg.UUID())

	currentModel, err := controllerStore.CurrentModel(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(currentModel, tc.Equals, modelName)
}
