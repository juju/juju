// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
)

// State provides model-backed persistence for SSH connection requests.
type State struct {
	*domain.StateBase
}

// NewState creates a new SSH connection request state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{StateBase: domain.NewStateBase(factory)}
}

// InsertSSHConnRequest stores a one-shot SSH connection request.
func (st *State) InsertSSHConnRequest(ctx context.Context, req ssh.SSHConnRequest, now time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	encodedAddresses, err := marshalControllerAddresses(req.ControllerAddresses)
	if err != nil {
		return errors.Errorf("marshalling controller addresses for tunnel %q: %w", req.TunnelID, err)
	}

	machineStmt, err := st.Prepare(`
SELECT uuid AS &machineUUID.uuid
FROM   machine
WHERE  name = $machineName.name
`, machineUUID{}, machineName{})
	if err != nil {
		return errors.Errorf("preparing machine uuid statement: %w", err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO ssh_connection_request (*) VALUES ($sshConnRequestInsert.*)
`, sshConnRequestInsert{})
	if err != nil {
		return errors.Errorf("preparing insert ssh connection request statement: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.pruneExpiredSSHConnRequests(ctx, tx, now); err != nil {
			return errors.Errorf("pruning expired ssh connection requests: %w", err)
		}

		resolvedMachineUUID := machineUUID{}
		err := tx.Query(ctx, machineStmt, machineName{Name: req.MachineID}).Get(&resolvedMachineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q not found", req.MachineID).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying machine uuid for %q: %w", req.MachineID, err)
		}

		row := sshConnRequestInsert{
			TunnelID:            req.TunnelID,
			MachineUUID:         resolvedMachineUUID.UUID,
			Expires:             req.Expires,
			Username:            req.Username,
			Password:            req.Password,
			ControllerAddresses: encodedAddresses,
			UnitPort:            req.UnitPort,
			EphemeralPublicKey:  req.EphemeralPublicKey,
		}

		if err := tx.Query(ctx, insertStmt, row).Run(); err != nil {
			return errors.Errorf("inserting ssh connection request %q: %w", req.TunnelID, err)
		}
		return nil
	}))
}

// GetSSHConnRequest returns a one-shot SSH connection request by tunnel ID.
func (st *State) GetSSHConnRequest(ctx context.Context, id string, now time.Time) (ssh.SSHConnRequest, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return ssh.SSHConnRequest{}, errors.Capture(err)
	}

	getStmt, err := st.Prepare(`
SELECT scr.tunnel_id AS &sshConnRequestRecord.tunnel_id,
       m.name AS &sshConnRequestRecord.machine_id,
       scr.expires_at AS &sshConnRequestRecord.expires_at,
       scr.username AS &sshConnRequestRecord.username,
       scr.password AS &sshConnRequestRecord.password,
       scr.controller_addresses AS &sshConnRequestRecord.controller_addresses,
       scr.unit_port AS &sshConnRequestRecord.unit_port,
       scr.ephemeral_public_key AS &sshConnRequestRecord.ephemeral_public_key
FROM   ssh_connection_request AS scr
JOIN   machine AS m ON m.uuid = scr.machine_uuid
WHERE  scr.tunnel_id = $tunnelID.tunnel_id
`, sshConnRequestRecord{}, tunnelID{})
	if err != nil {
		return ssh.SSHConnRequest{}, errors.Errorf("preparing get ssh connection request statement: %w", err)
	}

	var result ssh.SSHConnRequest
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = ssh.SSHConnRequest{}

		if err := st.pruneExpiredSSHConnRequests(ctx, tx, now); err != nil {
			return errors.Errorf("pruning expired ssh connection requests: %w", err)
		}

		row := sshConnRequestRecord{}
		err := tx.Query(ctx, getStmt, tunnelID{TunnelID: id}).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("ssh connection request %q not found", id).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("querying ssh connection request %q: %w", id, err)
		}

		controllerAddresses, err := unmarshalControllerAddresses(row.ControllerAddresses)
		if err != nil {
			return errors.Errorf("unmarshalling controller addresses for tunnel %q: %w", id, err)
		}

		result = ssh.SSHConnRequest{
			TunnelID:            row.TunnelID,
			MachineID:           row.MachineID,
			Expires:             row.Expires,
			Username:            row.Username,
			Password:            row.Password,
			ControllerAddresses: controllerAddresses,
			UnitPort:            row.UnitPort,
			EphemeralPublicKey:  row.EphemeralPublicKey,
		}
		return nil
	})
	if err != nil {
		return ssh.SSHConnRequest{}, errors.Capture(err)
	}
	return result, nil
}

// RemoveSSHConnRequest removes a one-shot SSH connection request by tunnel ID.
func (st *State) RemoveSSHConnRequest(ctx context.Context, id string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removeStmt, err := st.Prepare(`
DELETE FROM ssh_connection_request
WHERE tunnel_id = $tunnelID.tunnel_id
`, tunnelID{})
	if err != nil {
		return errors.Errorf("preparing remove ssh connection request statement: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, removeStmt, tunnelID{TunnelID: id}).Run()
	}))
}

// PruneExpiredSSHConnRequests removes expired SSH connection requests.
func (st *State) PruneExpiredSSHConnRequests(ctx context.Context, now time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.pruneExpiredSSHConnRequests(ctx, tx, now)
	}))
}

// InitialWatchSSHConnRequestsStatement returns the namespace and initial watch
// statement for SSH connection requests.
func (*State) InitialWatchSSHConnRequestsStatement() (string, string) {
	return "ssh_connection_request", "SELECT tunnel_id FROM ssh_connection_request"
}

func (st *State) pruneExpiredSSHConnRequests(ctx context.Context, tx *sqlair.TX, now time.Time) error {
	stmt, err := st.Prepare(`
DELETE FROM ssh_connection_request
WHERE expires_at < $expiryTime.expires_at
`, expiryTime{})
	if err != nil {
		return errors.Errorf("preparing prune ssh connection requests statement: %w", err)
	}
	return tx.Query(ctx, stmt, expiryTime{ExpiresAt: now}).Run()
}

func marshalControllerAddresses(addresses network.SpaceAddresses) (string, error) {
	payload, err := json.Marshal(addresses)
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(payload), nil
}

func unmarshalControllerAddresses(payload string) (network.SpaceAddresses, error) {
	var addresses network.SpaceAddresses
	if err := json.Unmarshal([]byte(payload), &addresses); err != nil {
		return nil, errors.Capture(err)
	}
	return addresses, nil
}
