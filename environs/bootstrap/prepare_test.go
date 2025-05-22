// Copyright 2012, 2013, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
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
		controller.StatePort:               1234,
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
			controller.StatePort:               1234,
			controller.SetNUMAControlPolicyKey: true,
		},
		Config: map[string]interface{}{
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
