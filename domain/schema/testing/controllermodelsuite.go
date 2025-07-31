// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
)

// ControllerModelSuite is used to provide a sql.DB reference for tests that
// can both be used for controller and model operations and their separate DB
// requirements.
type ControllerModelSuite struct {
	ControllerSuite
}

// ModelTxnRunner returns a transaction runner on to the model database for the
// provided model uuid.
func (s *ControllerModelSuite) ModelTxnRunner(c *tc.C, modelUUID string) coredatabase.TxnRunner {
	txnRunner, _ := s.OpenDBForNamespace(c, modelUUID, true)
	s.DqliteSuite.ApplyDDLForRunner(c, &SchemaApplier{
		Schema:  schema.ModelDDL(),
		Verbose: s.Verbose,
	}, txnRunner)
	return txnRunner
}
