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
	"github.com/juju/juju/core/network"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/schema"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainssh "github.com/juju/juju/domain/ssh"
	sshmodelstate "github.com/juju/juju/domain/ssh/state/model"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
	controllerDB     *sql.DB
	controllerRunner coredatabase.TxnRunner
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerRunner, s.controllerDB = s.OpenDB(c)
	s.ApplyDDLForRunner(c, &schematesting.SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	}, s.controllerRunner)
}

func (s *stateSuite) newState() *sshmodelstate.State {
	return sshmodelstate.NewState(
		txRunnerFactory(s.ModelTxnRunner()),
		txRunnerFactory(s.controllerRunner),
	)
}

func (s *stateSuite) addControllerConfig(c *tc.C, key, value string) {
	_, err := s.controllerDB.ExecContext(c.Context(),
		`INSERT INTO controller_config (key, value) VALUES (?, ?)`, key, value)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestGetModelInfo(c *tc.C) {
	st := s.newState()
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, is_controller_model)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ModelUUID(), "controller-uuid", "test-model", "prod", "iaas", "test-cloud", "ec2", true)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, sshmodelstate.ModelInfo{
		Name:              "test-model",
		Type:              "iaas",
		IsControllerModel: true,
	})
}

func (s *stateSuite) TestGetModelInfoNotFound(c *tc.C) {
	_, err := s.newState().GetModelInfo(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetControllerName(c *tc.C) {
	s.addControllerConfig(c, "controller-name", "test-controller")

	got, err := s.newState().GetControllerName(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "test-controller")
}

func (s *stateSuite) TestGetControllerNameNotFound(c *tc.C) {
	_, err := s.newState().GetControllerName(c.Context())
	c.Assert(err, tc.ErrorMatches, `controller name not found`)
}

func (s *stateSuite) TestGetUnitK8sPodInfo(c *tc.C) {
	st := s.newState()
	unitUUID := s.addUnit(c, "postgresql/0")
	_, err := s.DB().ExecContext(c.Context(),
		`INSERT INTO k8s_pod (unit_uuid, provider_id) VALUES (?, ?)`, unitUUID, "pod-0")
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetUnitK8sPodInfo(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "pod-0")
}

func (s *stateSuite) TestGetUnitK8sPodInfoWithoutPod(c *tc.C) {
	s.addUnit(c, "postgresql/0")

	got, err := s.newState().GetUnitK8sPodInfo(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitK8sProviderIDNotAssigned)
	c.Check(got, tc.Equals, "")
}

func (s *stateSuite) TestGetUnitK8sPodInfoDead(c *tc.C) {
	st := s.newState()
	unitUUID := s.addUnit(c, "postgresql/0")
	_, err := s.DB().ExecContext(c.Context(),
		`INSERT INTO k8s_pod (unit_uuid, provider_id) VALUES (?, ?)`, unitUUID, "pod-0")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(),
		`UPDATE unit SET life_id = (SELECT id FROM life WHERE value = 'dead') WHERE uuid = ?`, unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetUnitK8sPodInfo(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitMachineName(c *tc.C) {
	st := s.newState()
	unitUUID := s.addUnit(c, "postgresql/0")
	machineUUID := s.addMachine(c, "1")
	s.connectUnitToMachine(c, unitUUID, machineUUID)

	got, err := st.GetUnitMachineName(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "1")
}

func (s *stateSuite) TestGetUnitMachineNameNotAssigned(c *tc.C) {
	s.addUnit(c, "postgresql/0")

	got, err := s.newState().GetUnitMachineName(c.Context(), "postgresql/0")
	c.Check(got, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

func (s *stateSuite) TestCheckMachineExists(c *tc.C) {
	st := s.newState()
	s.addMachine(c, "1")

	exists, err := st.CheckMachineExists(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.CheckMachineExists(c.Context(), "99")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

func (s *stateSuite) TestCheckMachineExistsDead(c *tc.C) {
	st := s.newState()
	machineUUID := s.addMachine(c, "1")
	_, err := s.DB().ExecContext(c.Context(),
		`UPDATE machine SET life_id = (SELECT id FROM life WHERE value = 'dead') WHERE uuid = ?`, machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.CheckMachineExists(c.Context(), "1")
	c.Check(exists, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineIsDead)
}

func (s *stateSuite) TestGetMachineNameForUnit(c *tc.C) {
	st := s.newState()
	unitUUID := s.addUnit(c, "postgresql/0")
	machineUUID := s.addMachine(c, "1")
	s.connectUnitToMachine(c, unitUUID, machineUUID)

	got, backed, err := st.GetMachineNameForUnit(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "1")
	c.Check(backed, tc.IsTrue)
}

func (s *stateSuite) TestGetMachineNameForUnitNotMachineBacked(c *tc.C) {
	s.addUnit(c, "postgresql/0")

	got, backed, err := s.newState().GetMachineNameForUnit(c.Context(), "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "")
	c.Check(backed, tc.IsFalse)
}

func (s *stateSuite) TestGetMachineVirtualHostKeyMissing(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	machineUUID := s.addMachine(c, "1")
	_ = machineUUID

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
	c.Check(key, tc.Equals, "")
}

func (s *stateSuite) TestGetMachineVirtualHostKeyMissingMachine(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))

	key, found, err := st.GetMachineVirtualHostKeyByMachineName(c.Context(), "99")
	c.Check(key, tc.Equals, "")
	c.Check(found, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestEnsureAndGetMachineVirtualHostKey(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
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
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))

	key, err := st.EnsureMachineVirtualHostKeyByMachineName(c.Context(), "99", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
	c.Check(key, tc.Equals, "")
}

func (s *stateSuite) TestEnsureMachineVirtualHostKeyReturnsExistingOnConflict(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
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
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))

	key, found, err := st.GetUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0")
	c.Check(key, tc.Equals, "")
	c.Check(found, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitVirtualHostKeyMissingUnit(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))

	key, err := st.EnsureUnitVirtualHostKeyByUnitName(c.Context(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeED25519ID, testPrivateKey)
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitVirtualHostKeyReturnsExistingOnConflict(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
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
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))

	machineName, machineBacked, err := st.GetMachineNameForUnit(c.Context(), "postgresql/0")
	c.Check(machineName, tc.Equals, "")
	c.Check(machineBacked, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureAndGetUnitVirtualHostKey(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
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
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	s.addMachine(c, "1")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	req := domainssh.SSHConnRequest{
		TunnelID:            "tunnel-0",
		MachineName:         "1",
		Expires:             now.Add(time.Minute),
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"),
		UnitPort:            0,
		EphemeralPublicKey:  []byte("pub"),
	}

	err := st.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetSSHConnRequest(c.Context(), "1", req.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.TunnelID, tc.Equals, req.TunnelID)
	c.Check(got.MachineName, tc.Equals, req.MachineName)
	c.Check(got.Expires.Equal(req.Expires), tc.IsTrue)
	c.Check(got.ControllerAddresses.EqualTo(req.ControllerAddresses), tc.IsTrue)
	c.Check(got.UnitPort, tc.Equals, req.UnitPort)
	c.Check(got.EphemeralPublicKey, tc.DeepEquals, req.EphemeralPublicKey)
}

// TestGetSSHConnRequestOtherMachineNotFound checks that a request targeting one
// machine cannot be read when scoped to another machine, so a machine agent
// cannot fetch another machine's request (and its credentials).
func (s *stateSuite) TestGetSSHConnRequestOtherMachineNotFound(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	s.addMachine(c, "1")
	s.addMachine(c, "2")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	req := domainssh.SSHConnRequest{
		TunnelID:           "tunnel-machine-1",
		MachineName:        "1",
		Expires:            now.Add(time.Minute),
		EphemeralPublicKey: []byte("pub"),
	}

	err := st.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)

	// Machine "1" can read its own request.
	_, err = st.GetSSHConnRequest(c.Context(), "1", req.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)

	// Machine "2" cannot read machine "1"'s request; it is reported as not
	// found rather than being returned.
	_, err = st.GetSSHConnRequest(c.Context(), "2", req.TunnelID, now)
	c.Assert(err, tc.ErrorMatches, `SSH connection request "tunnel-machine-1" not found`)
}

func (s *stateSuite) TestInsertSSHConnRequestMachineNotFound(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	err := st.InsertSSHConnRequest(c.Context(), domainssh.SSHConnRequest{
		TunnelID:    "missing-machine",
		MachineName: "99",
		Expires:     now.Add(time.Minute),
	}, now)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestRemoveSSHConnRequest(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	s.addMachine(c, "1")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	req := domainssh.SSHConnRequest{
		TunnelID:           "remove-me",
		MachineName:        "1",
		Expires:            now.Add(time.Minute),
		UnitPort:           0,
		EphemeralPublicKey: []byte("pub"),
	}

	err := st.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)
	err = st.RemoveSSHConnRequest(c.Context(), req.TunnelID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetSSHConnRequest(c.Context(), "1", req.TunnelID, now)
	c.Assert(err, tc.ErrorMatches, `SSH connection request "remove-me" not found`)
}

func (s *stateSuite) TestPruneExpiredSSHConnRequests(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	s.addMachine(c, "1")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	expiredReq := domainssh.SSHConnRequest{TunnelID: "expired", MachineName: "1", Expires: now.Add(-time.Minute), EphemeralPublicKey: []byte("pub")}
	activeReq := domainssh.SSHConnRequest{TunnelID: "active", MachineName: "1", Expires: now.Add(time.Minute), EphemeralPublicKey: []byte("pub")}

	err := st.InsertSSHConnRequest(c.Context(), expiredReq, now.Add(-2*time.Minute))
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertSSHConnRequest(c.Context(), activeReq, now)
	c.Assert(err, tc.ErrorIsNil)

	err = st.PruneExpiredSSHConnRequests(c.Context(), now)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetSSHConnRequest(c.Context(), "1", activeReq.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.GetSSHConnRequest(c.Context(), "1", expiredReq.TunnelID, now)
	c.Assert(err, tc.ErrorMatches, `SSH connection request "expired" not found`)
}

func (s *stateSuite) TestWatchSSHConnRequestStatement(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	table, stmt := st.InitialWatchSSHConnRequestsStatement()
	c.Check(table, tc.Equals, "ssh_connection_request")
	c.Check(stmt, tc.Equals, "SELECT tunnel_id FROM ssh_connection_request WHERE machine_uuid = ?")
}

// TestGetMachineUUIDByName checks the machine name is resolved to its UUID and
// that a missing machine yields MachineNotFound.
func (s *stateSuite) TestGetMachineUUIDByName(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	machineUUID := s.addMachine(c, "1")

	got, err := st.GetMachineUUIDByName(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, machineUUID)

	_, err = st.GetMachineUUIDByName(c.Context(), "99")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestFilterSSHConnRequestsForMachine checks that only tunnel IDs belonging to
// the given machine are returned, so a machine cannot observe another machine's
// requests.
func (s *stateSuite) TestFilterSSHConnRequestsForMachine(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	machine1UUID := s.addMachine(c, "1")
	s.addMachine(c, "2")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	req1 := domainssh.SSHConnRequest{TunnelID: "t-machine-1", MachineName: "1", Expires: now.Add(time.Minute), EphemeralPublicKey: []byte("pub")}
	req2 := domainssh.SSHConnRequest{TunnelID: "t-machine-2", MachineName: "2", Expires: now.Add(time.Minute), EphemeralPublicKey: []byte("pub")}
	c.Assert(st.InsertSSHConnRequest(c.Context(), req1, now), tc.ErrorIsNil)
	c.Assert(st.InsertSSHConnRequest(c.Context(), req2, now), tc.ErrorIsNil)

	// Only machine 1's tunnel ID is returned, and an unknown ID is dropped.
	got, err := st.FilterSSHConnRequestsForMachine(
		c.Context(),
		[]string{"t-machine-1", "t-machine-2", "unknown"},
		machine1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []string{"t-machine-1"})
}

// TestFilterSSHConnRequestsForMachineEmpty checks the no-op path for an empty
// input avoids an invalid IN () query.
func (s *stateSuite) TestFilterSSHConnRequestsForMachineEmpty(c *tc.C) {
	st := sshmodelstate.NewState(txRunnerFactory(s.ModelTxnRunner()), txRunnerFactory(s.controllerRunner))
	got, err := st.FilterSSHConnRequestsForMachine(c.Context(), nil, "some-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *stateSuite) addMachine(c *tc.C, name string) string {
	machineUUID := internaluuid.MustNewUUID().String()
	netNodeUUID := internaluuid.MustNewUUID().String()
	_, err := s.DB().ExecContext(c.Context(), `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO machine (uuid, name, net_node_uuid, life_id)
VALUES (?, ?, ?, (SELECT id FROM life WHERE value = 'alive'))
`, machineUUID, name, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func (s *stateSuite) connectUnitToMachine(c *tc.C, unitUUID, machineUUID string) {
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE unit
SET net_node_uuid = (SELECT net_node_uuid FROM machine WHERE uuid = ?)
WHERE uuid = ?`, machineUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) addUnit(c *tc.C, name string) string {
	unitUUID := internaluuid.MustNewUUID().String()
	applicationUUID := internaluuid.MustNewUUID().String()
	charmUUID := internaluuid.MustNewUUID().String()
	netNodeUUID := internaluuid.MustNewUUID().String()
	spaceUUID := internaluuid.MustNewUUID().String()

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

const testPrivateKey = "test-private-key"
