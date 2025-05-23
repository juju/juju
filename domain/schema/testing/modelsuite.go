// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/uuid"
)

// ModelSuite is used to provide an in-memory sql.DB reference to tests.
// It is pre-populated with the model schema.
type ModelSuite struct {
	testing.DqliteSuite

	modelUUID string
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the model schema.
func (s *ModelSuite) SetUpTest(c *tc.C) {
	s.modelUUID = uuid.MustNewUUID().String()

	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &SchemaApplier{
		Schema:  schema.ModelDDL(),
		Verbose: s.Verbose,
	})
}

func (s *ModelSuite) ModelUUID() string {
	return s.modelUUID
}

// ModelTxnRunner returns a txn runner attached to the model database.
func (s *ModelSuite) ModelTxnRunner() coredatabase.TxnRunner {
	return s.TxnRunner()
}

// ControllerTxnRunner returns a txn runner attached to the controller database.
func (s *ModelSuite) ControllerTxnRunner() coredatabase.TxnRunner {
	return s.TxnRunner()
}
