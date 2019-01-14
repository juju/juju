// Copyright 2012, 2013, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
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
		"test-mode":  true,
	}).Delete(
		"admin-secret",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, jc.ErrorIsNil)
	controllerStore := jujuclient.NewMemStore()
	ctx := envtesting.BootstrapContext(c)
	controllerCfg := controller.Config{
		controller.ControllerUUIDKey:       testing.ControllerTag.Id(),
		controller.CACertKey:               testing.CACert,
		controller.APIPort:                 17777,
		controller.StatePort:               1234,
		controller.SetNUMAControlPolicyKey: true,
	}
	fakeCert := testing.CACert
	cloudSpec := dummy.SampleCloudSpec()
	cloudSpec.CACertificates = []string{fakeCert}
	_, err = bootstrap.PrepareController(false, ctx, controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            cloudSpec,
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that controller was cached
	foundController, err := controllerStore.ControllerByName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, gc.DeepEquals, controllerCfg.ControllerUUID())
	c.Assert(foundController.Cloud, gc.Equals, "dummy")

	// Check that bootstrap config was written
	bootstrapCfg, err := controllerStore.BootstrapConfigForController(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCfg, jc.DeepEquals, &jujuclient.BootstrapConfig{
		ControllerConfig: controller.Config{
			controller.APIPort:                 17777,
			controller.StatePort:               1234,
			controller.SetNUMAControlPolicyKey: true,
		},
		Config: map[string]interface{}{
			"default-series":            "bionic",
			"firewall-mode":             "instance",
			"ssl-hostname-verification": true,
			"logging-config":            "<root>=DEBUG;unit=DEBUG",
			"secret":                    "pork",
			"authorized-keys":           testing.FakeAuthKeys,
			"type":                      "dummy",
			"name":                      "erewhemos",
			"controller":                false,
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
		CloudCACertificates:   []string{fakeCert},
	})

	// Check we cannot call Prepare again.
	_, err = bootstrap.PrepareController(false, ctx, controllerStore, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches, `controller "erewhemos" already exists`)
}
