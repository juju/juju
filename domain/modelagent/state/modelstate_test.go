// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machine"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type modelStateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelStateSuite{})

// TestCheckMachineDoesNotExist is asserting that if no machine exists we get
// back an error satisfying [machineerrors.MachineNotFound].
func (s *modelStateSuite) TestCheckMachineDoesNotExist(c *gc.C) {
	err := NewModelState(s.TxnRunnerFactory()).CheckMachineExists(
		context.Background(),
		machine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCheckUnitDoesNotExist is asserting that if no unit exists we get back an
// error satisfying [applicationerrors.UnitNotFound].
func (s *modelStateSuite) TestCheckUnitDoesNotExist(c *gc.C) {
	err := NewModelState(s.TxnRunnerFactory()).CheckUnitExists(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}
