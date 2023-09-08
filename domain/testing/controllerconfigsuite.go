// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	databasetesting "github.com/juju/juju/database/testing"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	"github.com/juju/juju/domain/controllerconfig/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/juju/testing"
)

// ControllerConfigSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller config.
type ControllerConfigSuite struct {
	schematesting.ControllerSuite

	ControllerConfig controller.Config
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller config.
func (s *ControllerConfigSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.SeedControllerCloud(c, s.TxnRunner())
	s.SeedControllerConfig(c, s.TxnRunner(), s.ControllerConfig)
}

// SeedControllerConfig is responsible for applying the controller config to
// the given database.
func (s *ControllerConfigSuite) SeedControllerConfig(c *gc.C, runner coredatabase.TxnRunner, config controller.Config) {
	err := bootstrap.InsertInitialControllerConfig(config)(context.Background(), runner)
	c.Assert(err, jc.ErrorIsNil)
}

// SeedControllerCloud is responsible for applying the controller cloud to
// the given database.
func (s *ControllerConfigSuite) SeedControllerCloud(c *gc.C, runner coredatabase.TxnRunner) {
	err := databasetesting.DummyCloudOpt(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	err = cloudbootstrap.InsertInitialControllerCloud(testing.DefaultCloud)(context.Background(), runner)
	c.Assert(err, jc.ErrorIsNil)
}
