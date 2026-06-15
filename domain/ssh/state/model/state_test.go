// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	sshmodelstate "github.com/juju/juju/domain/ssh/state/model"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetMachineVirtualHostKeyMissing(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	machineUUID := s.addMachine(c, "1")
	_ = machineUUID

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
	c.Check(key, tc.Equals, "")
}

func (s *stateSuite) TestGetMachineVirtualHostKeyMissingMachine(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "99")
	c.Check(key, tc.Equals, "")
	c.Check(found, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetAndGetMachineVirtualHostKey(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addMachine(c, "1")

	err := st.SetMachineVirtualHostKeyByMachineName(c.Context(), "1", testPrivateKey)
	c.Assert(err, tc.ErrorIsNil)

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *stateSuite) TestSetMachineVirtualHostKeyMissingMachine(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	err := st.SetMachineVirtualHostKeyByMachineName(c.Context(), "99", testPrivateKey)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetUnitVirtualHostKeyMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	key, found, err := st.GetUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0")
	c.Check(key, tc.Equals, "")
	c.Check(found, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestSetUnitVirtualHostKeyMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	err := st.SetUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0", testPrivateKey)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetMachineNameForUnitMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	machineName, machineBacked, err := st.GetMachineNameForUnit(c.Context(), "postgresql/0")
	c.Check(machineName, tc.Equals, "")
	c.Check(machineBacked, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) addMachine(c *tc.C, name string) string {
	machineUUID := uuid.MustNewUUID().String()
	netNodeUUID := uuid.MustNewUUID().String()
	_, err := s.DB().ExecContext(c.Context(), `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO machine (uuid, name, net_node_uuid, life_id)
VALUES (?, ?, ?, (SELECT id FROM life WHERE value = 'alive'))
`, machineUUID, name, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func txRunnerFactory(runner coredatabase.TxnRunner) coredatabase.TxnRunnerFactory {
	return func(context.Context) (coredatabase.TxnRunner, error) {
		return runner, nil
	}
}

const testPrivateKey = "test-private-key"
