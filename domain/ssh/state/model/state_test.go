// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainssh "github.com/juju/juju/domain/ssh"
	sshmodelstate "github.com/juju/juju/domain/ssh/state/model"
	"github.com/juju/juju/internal/uuid"
	internaluuid "github.com/juju/juju/internal/uuid"
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

func (s *stateSuite) TestEnsureAndGetMachineVirtualHostKey(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addMachine(c, "1")

	key, err := st.EnsureMachineVirtualHostKeyByMachineName(c.Context(), "1", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(key, tc.Equals, testPrivateKey)

	var algorithmTypeID int
	row := s.DB().QueryRow(
		`SELECT algorithm_type_id 
		 FROM machine_virtual_ssh_host_key 
		 WHERE machine_uuid = (SELECT uuid FROM machine WHERE name = ?)`, "1")
	c.Assert(row.Scan(&algorithmTypeID), tc.ErrorIsNil)
	c.Check(algorithmTypeID, tc.Equals, domainssh.SSHKeyAlgorithmTypeED25519ID)
}

func (s *stateSuite) TestEnsureMachineVirtualHostKeyMissingMachine(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	key, err := st.EnsureMachineVirtualHostKeyByMachineName(c.Context(), "99", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
	c.Check(key, tc.Equals, "")
}

func (s *stateSuite) TestEnsureMachineVirtualHostKeyReturnsExistingOnConflict(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addMachine(c, "1")

	key, err := st.EnsureMachineVirtualHostKeyByMachineName(c.Context(), "1", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)

	key, err = st.EnsureMachineVirtualHostKeyByMachineName(c.Context(), "1", domainssh.SSHKeyAlgorithmTypeRSAID, "different-key")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *stateSuite) TestGetUnitVirtualHostKeyMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	key, found, err := st.GetUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0")
	c.Check(key, tc.Equals, "")
	c.Check(found, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitVirtualHostKeyMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	key, err := st.EnsureUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitVirtualHostKeyReturnsExistingOnConflict(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addUnit(c, "postgresql/0")

	key, err := st.EnsureUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)

	key, err = st.EnsureUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeRSAID, "different-key")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)

	key, found, err := st.GetUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *stateSuite) TestGetMachineNameForUnitMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))

	machineName, machineBacked, err := st.GetMachineNameForUnit(c.Context(), "postgresql/0")
	c.Check(machineName, tc.Equals, "")
	c.Check(machineBacked, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureAndGetUnitVirtualHostKey(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addUnit(c, "postgresql/0")

	key, err := st.EnsureUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)

	key, found, err := st.GetUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(key, tc.Equals, testPrivateKey)

	var algorithmTypeID int
	row := s.DB().QueryRow(`SELECT algorithm_type_id FROM unit_virtual_ssh_host_key WHERE unit_uuid = (SELECT uuid FROM unit WHERE name = ?)`, "postgresql/0")
	c.Assert(row.Scan(&algorithmTypeID), tc.ErrorIsNil)
	c.Check(algorithmTypeID, tc.Equals, domainssh.SSHKeyAlgorithmTypeED25519ID)
}

func (s *stateSuite) TestInsertAndGetSSHConnRequest(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addMachine(c, "1")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	req := domainssh.SSHConnRequest{
		TunnelID:            "tunnel-0",
		MachineID:           "1",
		Expires:             now.Add(time.Minute),
		Username:            "juju-reverse-tunnel",
		Password:            "secret",
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"),
		UnitPort:            0,
		EphemeralPublicKey:  []byte("pub"),
	}

	err := st.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetSSHConnRequest(c.Context(), req.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.TunnelID, tc.Equals, req.TunnelID)
	c.Check(got.MachineID, tc.Equals, req.MachineID)
	c.Check(got.Expires.Equal(req.Expires), tc.IsTrue)
	c.Check(got.Username, tc.Equals, req.Username)
	c.Check(got.Password, tc.Equals, req.Password)
	c.Check(got.ControllerAddresses.EqualTo(req.ControllerAddresses), tc.IsTrue)
	c.Check(got.UnitPort, tc.Equals, req.UnitPort)
	c.Check(got.EphemeralPublicKey, tc.DeepEquals, req.EphemeralPublicKey)
}

func (s *stateSuite) TestInsertSSHConnRequestMachineNotFound(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	err := st.InsertSSHConnRequest(c.Context(), domainssh.SSHConnRequest{
		TunnelID:  "missing-machine",
		MachineID: "99",
		Expires:   now.Add(time.Minute),
		Username:  "juju-reverse-tunnel",
		Password:  "secret",
	}, now)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestRemoveSSHConnRequest(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addMachine(c, "1")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	req := domainssh.SSHConnRequest{
		TunnelID:           "remove-me",
		MachineID:          "1",
		Expires:            now.Add(time.Minute),
		Username:           "juju-reverse-tunnel",
		Password:           "secret",
		UnitPort:           0,
		EphemeralPublicKey: []byte("pub"),
	}

	err := st.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)
	err = st.RemoveSSHConnRequest(c.Context(), req.TunnelID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetSSHConnRequest(c.Context(), req.TunnelID, now)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *stateSuite) TestPruneExpiredSSHConnRequests(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	s.addMachine(c, "1")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	expiredReq := domainssh.SSHConnRequest{TunnelID: "expired", MachineID: "1", Expires: now.Add(-time.Minute), Username: "juju-reverse-tunnel", Password: "secret", EphemeralPublicKey: []byte("pub")}
	activeReq := domainssh.SSHConnRequest{TunnelID: "active", MachineID: "1", Expires: now.Add(time.Minute), Username: "juju-reverse-tunnel", Password: "secret", EphemeralPublicKey: []byte("pub")}

	err := st.InsertSSHConnRequest(c.Context(), expiredReq, now.Add(-2*time.Minute))
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertSSHConnRequest(c.Context(), activeReq, now)
	c.Assert(err, tc.ErrorIsNil)

	err = st.PruneExpiredSSHConnRequests(c.Context(), now)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetSSHConnRequest(c.Context(), activeReq.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.GetSSHConnRequest(c.Context(), expiredReq.TunnelID, now)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *stateSuite) TestWatchSSHConnRequestStatement(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()))
	table, stmt := st.InitialWatchSSHConnRequestsStatement()
	c.Check(table, tc.Equals, "ssh_connection_request")
	c.Check(stmt, tc.Equals, "SELECT tunnel_id FROM ssh_connection_request")
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

func (s *stateSuite) addUnit(c *tc.C, name string) string {
	unitUUID := uuid.MustNewUUID().String()
	applicationUUID := uuid.MustNewUUID().String()
	charmUUID := uuid.MustNewUUID().String()
	netNodeUUID := uuid.MustNewUUID().String()
	spaceUUID := uuid.MustNewUUID().String()

	_, err := s.DB().ExecContext(c.Context(), `INSERT INTO space (uuid, name) VALUES (?, ?)`, spaceUUID, "space-"+spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id) VALUES (?, 0, 'postgresql', 0, 0)`, charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO charm_metadata (charm_uuid, name) VALUES (?, 'postgresql')`, charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)`, applicationUUID, "postgresql", charmUUID, spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES (?, ?, 0, ?, ?, ?)
`, unitUUID, name, applicationUUID, charmUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID
}

func txRunnerFactory(runner coredatabase.TxnRunner) coredatabase.TxnRunnerFactory {
	return func(context.Context) (coredatabase.TxnRunner, error) {
		return runner, nil
	}
}

func addMachineTxn(c *tc.C, db *sql.DB, name string) string {
	netNodeUUID := internaluuid.MustNewUUID().String()
	machineUUID := internaluuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(), `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(), `
INSERT INTO machine (uuid, name, net_node_uuid, life_id)
VALUES (?, ?, ?, (SELECT id FROM life WHERE value = 'alive'))
`, machineUUID, name, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

const testPrivateKey = "test-private-key"
