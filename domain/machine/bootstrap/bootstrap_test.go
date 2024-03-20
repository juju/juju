// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type bootstrapSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertBootstrapMachine(c *gc.C) {
	err := InsertMachine("666")(context.Background(), s.NoopTxnRunner(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var machineId string
	row := s.DB().QueryRow("SELECT machine_id FROM machine")
	c.Assert(row.Scan(&machineId), jc.ErrorIsNil)
	c.Assert(machineId, gc.Equals, "666")
}
