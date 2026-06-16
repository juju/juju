// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/ssh"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())

	c.Cleanup(func() {
		s.state = nil
	})
}

func (s *stateSuite) TestInsertAndGetSSHConnRequest(c *tc.C) {
	machineUUID := s.addMachine(c, "0")
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	req := makeSSHConnRequest("tunnel-0", "0", now.Add(time.Minute))

	err := s.state.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.state.GetSSHConnRequest(c.Context(), req.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.TunnelID, tc.Equals, req.TunnelID)
	c.Check(got.MachineID, tc.Equals, req.MachineID)
	c.Check(got.Expires.Equal(req.Expires), tc.IsTrue)
	c.Check(got.Username, tc.Equals, req.Username)
	c.Check(got.Password, tc.Equals, req.Password)
	c.Check(got.ControllerAddresses.EqualTo(req.ControllerAddresses), tc.IsTrue)
	c.Check(got.UnitPort, tc.Equals, req.UnitPort)
	c.Check(got.EphemeralPublicKey, tc.DeepEquals, req.EphemeralPublicKey)
	c.Check(s.machineUUIDForTunnel(c, req.TunnelID), tc.Equals, machineUUID)
}

func (s *stateSuite) TestInsertSSHConnRequestMachineNotFound(c *tc.C) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	req := makeSSHConnRequest("missing-machine", "99", now.Add(time.Minute))

	err := s.state.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestRemoveSSHConnRequest(c *tc.C) {
	s.addMachine(c, "0")
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	req := makeSSHConnRequest("remove-me", "0", now.Add(time.Minute))

	err := s.state.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.RemoveSSHConnRequest(c.Context(), req.TunnelID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetSSHConnRequest(c.Context(), req.TunnelID, now)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	var count int
	s.countRows(c, `SELECT COUNT(*) FROM ssh_connection_request`, &count)
	c.Check(count, tc.Equals, 0)
}

func (s *stateSuite) TestGetSSHConnRequestPrunesExpired(c *tc.C) {
	s.addMachine(c, "0")
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	req := makeSSHConnRequest("expired", "0", now.Add(time.Minute))

	err := s.state.InsertSSHConnRequest(c.Context(), req, now)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetSSHConnRequest(c.Context(), req.TunnelID, now.Add(2*time.Minute))
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)

	var count int
	s.countRows(c, `SELECT COUNT(*) FROM ssh_connection_request`, &count)
	c.Check(count, tc.Equals, 0)
}

func (s *stateSuite) TestPruneExpiredSSHConnRequests(c *tc.C) {
	s.addMachine(c, "0")
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	expiredReq := makeSSHConnRequest("expired", "0", now.Add(-time.Minute))
	activeReq := makeSSHConnRequest("active", "0", now.Add(time.Minute))

	err := s.state.InsertSSHConnRequest(c.Context(), expiredReq, now)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.InsertSSHConnRequest(c.Context(), activeReq, now)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.PruneExpiredSSHConnRequests(c.Context(), now)
	c.Assert(err, tc.ErrorIsNil)

	var count int
	s.countRows(c, `SELECT COUNT(*) FROM ssh_connection_request`, &count)
	c.Check(count, tc.Equals, 1)

	got, err := s.state.GetSSHConnRequest(c.Context(), activeReq.TunnelID, now)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.TunnelID, tc.Equals, activeReq.TunnelID)
}

func makeSSHConnRequest(tunnelID, machineID string, expires time.Time) ssh.SSHConnRequest {
	return ssh.SSHConnRequest{
		TunnelID:            tunnelID,
		MachineID:           machineID,
		Expires:             expires,
		Username:            "juju-reverse-tunnel",
		Password:            "secret",
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"),
		UnitPort:            0,
		EphemeralPublicKey:  []byte("public-key"),
	}
}

func (s *stateSuite) addMachine(c *tc.C, name string) string {
	netNodeUUID := internaluuid.MustNewUUID().String()
	machineUUID := internaluuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ?, ?)`, machineUUID, netNodeUUID, name, 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func (s *stateSuite) machineUUIDForTunnel(c *tc.C, tunnelID string) string {
	var machineUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT machine_uuid FROM ssh_connection_request WHERE tunnel_id = ?`, tunnelID).Scan(&machineUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func (s *stateSuite) countRows(c *tc.C, query string, dest *int) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, query).Scan(dest)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("query %q failed", query))
}
