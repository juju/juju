// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

// ControllerSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.DqliteSuite
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *ControllerSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	})
	err := database.InsertControllerNodeID(c.Context(), s.DqliteSuite.TxnRunner(), 0x2dc171858c3155be)
	c.Assert(err, tc.ErrorIsNil)
}

// ApplyDDLForRunner is responsible for applying the controller schema to the
// given database.
func (s *ControllerSuite) ApplyDDLForRunner(c *tc.C, runner coredatabase.TxnRunner) {
	s.DqliteSuite.ApplyDDLForRunner(c, &SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	}, runner)
	err := database.InsertControllerNodeID(c.Context(), runner, 0x2dc171858c3155be)
	c.Assert(err, tc.ErrorIsNil)
}

// ControllerTxnRunner returns a txn runner attached to the controller database.
func (s *ControllerSuite) ControllerTxnRunner() coredatabase.TxnRunner {
	return s.TxnRunner()
}

// SeedControllerTable sets the uuid in the controller table to the default
// testing value and the controller mode uuid to the supplied value. It does not
// add any other controller config.
func (s *ControllerSuite) SeedControllerTable(c *tc.C, controllerModelUUID coremodel.UUID) (controllerUUID string) {
	controllerUUID = jujutesting.ControllerTag.Id()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO controller (uuid, model_uuid, target_version, api_port, cert, private_key, ca_private_key, system_identity) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`,
			controllerUUID,
			controllerModelUUID,
			jujuversion.Current.String(),
			controller.DefaultAPIPort,
			"test-cert",
			"test-private-key",
			"test-ca-private-key",
			"test-system-identity",
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return controllerUUID
}

func (s *ControllerSuite) SeedControllerUUID(c *tc.C) (controllerUUID string) {
	controllerUUID = jujutesting.ControllerTag.Id()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO controller (uuid, model_uuid, target_version) VALUES (?, ?, ?)`,
			controllerUUID,
			jujutesting.ControllerModelTag.Id(),
			jujuversion.Current.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return controllerUUID
}
