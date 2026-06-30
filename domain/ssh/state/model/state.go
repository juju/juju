// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
)

// State represents model-scoped SSH host key state.
type State struct {
	*domain.StateBase
}

// NewState returns a new model-scoped SSH state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{StateBase: domain.NewStateBase(factory)}
}

// GetMachineVirtualHostKeyByMachineName returns the virtual host key stored for
// the named machine. The boolean indicates whether a key row exists.
func (st *State) GetMachineVirtualHostKeyByMachineName(ctx context.Context, machineName string) (string, bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	nameRec := entityName{Name: machineName}
	getMachineUUIDStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM machine
WHERE name = $entityName.name`, entityUUID{}, entityName{})
	if err != nil {
		return "", false, errors.Capture(err)
	}
	getKeyStmt, err := st.Prepare(`
SELECT ssh_key AS &sshPrivateKey.ssh_key
FROM machine_virtual_ssh_host_key
WHERE machine_uuid = $entityUUID.uuid`, sshPrivateKey{}, entityUUID{})
	if err != nil {
		return "", false, errors.Capture(err)
	}

	var (
		machineUUID entityUUID
		key         sshPrivateKey
		found       bool
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID = entityUUID{}
		key = sshPrivateKey{}
		found = false

		err := tx.Query(ctx, getMachineUUIDStmt, nameRec).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q %w", machineName, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("querying machine %q: %w", machineName, err)
		}

		err = tx.Query(ctx, getKeyStmt, machineUUID).Get(&key)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("querying machine virtual SSH host key for %q: %w", machineName, err)
		}
		found = true
		return nil
	})
	if err != nil {
		return "", false, errors.Capture(err)
	}
	return key.SSHKey, found, nil
}

// EnsureMachineVirtualHostKeyByMachineName persists the virtual host key for
// the named machine when it is missing, otherwise returns the existing key.
func (st *State) EnsureMachineVirtualHostKeyByMachineName(
	ctx context.Context,
	machineName string,
	algorithmTypeID int,
	sshKey string,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	nameRec := entityName{Name: machineName}
	getMachineUUIDStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM machine
WHERE name = $entityName.name`, entityUUID{}, entityName{})
	if err != nil {
		return "", errors.Capture(err)
	}
	getKeyStmt, err := st.Prepare(`
SELECT ssh_key AS &sshPrivateKey.ssh_key
FROM machine_virtual_ssh_host_key
WHERE machine_uuid = $entityUUID.uuid`, sshPrivateKey{}, entityUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}
	insertStmt, err := st.Prepare(`
INSERT INTO machine_virtual_ssh_host_key (machine_uuid, algorithm_type_id, ssh_key)
VALUES ($machineVirtualSSHHostKey.*)`, machineVirtualSSHHostKey{})
	if err != nil {
		return "", errors.Capture(err)
	}

	actualKey := sshKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID := entityUUID{}
		err := tx.Query(ctx, getMachineUUIDStmt, nameRec).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q %w", machineName, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("querying machine %q: %w", machineName, err)
		}

		txKey := sshPrivateKey{}
		err = tx.Query(ctx, getKeyStmt, machineUUID).Get(&txKey)
		if err == nil {
			actualKey = txKey.SSHKey
			return nil
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying machine virtual SSH host key for %q: %w", machineName, err)
		}

		record := machineVirtualSSHHostKey{MachineUUID: machineUUID.UUID, AlgorithmTypeID: algorithmTypeID, SSHKey: sshKey}
		if err := tx.Query(ctx, insertStmt, record).Run(); err != nil {
			return errors.Errorf("persisting machine virtual SSH host key for %q: %w", machineName, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return actualKey, nil
}

// GetUnitVirtualHostKeyByUnitName returns the virtual host key stored for the
// named unit. The boolean indicates whether a key row exists.
func (st *State) GetUnitVirtualHostKeyByUnitName(ctx context.Context, unitName string) (string, bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	nameRec := entityName{Name: unitName}
	getUnitUUIDStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM unit
WHERE name = $entityName.name`, entityUUID{}, entityName{})
	if err != nil {
		return "", false, errors.Capture(err)
	}
	getKeyStmt, err := st.Prepare(`
SELECT ssh_key AS &sshPrivateKey.ssh_key
FROM unit_virtual_ssh_host_key
WHERE unit_uuid = $entityUUID.uuid`, sshPrivateKey{}, entityUUID{})
	if err != nil {
		return "", false, errors.Capture(err)
	}

	var (
		unitUUID entityUUID
		key      sshPrivateKey
		found    bool
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID = entityUUID{}
		key = sshPrivateKey{}
		found = false

		err := tx.Query(ctx, getUnitUUIDStmt, nameRec).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q %w", unitName, applicationerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Errorf("querying unit %q: %w", unitName, err)
		}

		err = tx.Query(ctx, getKeyStmt, unitUUID).Get(&key)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("querying unit virtual SSH host key for %q: %w", unitName, err)
		}
		found = true
		return nil
	})
	if err != nil {
		return "", false, errors.Capture(err)
	}
	return key.SSHKey, found, nil
}

// EnsureUnitVirtualHostKeyByUnitName persists the virtual host key for the
// named unit when it is missing, otherwise returns the existing key.
func (st *State) EnsureUnitVirtualHostKeyByUnitName(
	ctx context.Context,
	unitName string,
	algorithmTypeID int,
	sshKey string,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	nameRec := entityName{Name: unitName}
	getUnitUUIDStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM unit
WHERE name = $entityName.name`, entityUUID{}, entityName{})
	if err != nil {
		return "", errors.Capture(err)
	}
	getKeyStmt, err := st.Prepare(`
SELECT ssh_key AS &sshPrivateKey.ssh_key
FROM unit_virtual_ssh_host_key
WHERE unit_uuid = $entityUUID.uuid`, sshPrivateKey{}, entityUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}
	insertStmt, err := st.Prepare(`
INSERT INTO unit_virtual_ssh_host_key (unit_uuid, algorithm_type_id, ssh_key)
VALUES ($unitVirtualSSHHostKey.*)`, unitVirtualSSHHostKey{})
	if err != nil {
		return "", errors.Capture(err)
	}

	actualKey := sshKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID := entityUUID{}
		err := tx.Query(ctx, getUnitUUIDStmt, nameRec).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q %w", unitName, applicationerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Errorf("querying unit %q: %w", unitName, err)
		}

		txKey := sshPrivateKey{}
		err = tx.Query(ctx, getKeyStmt, unitUUID).Get(&txKey)
		if err == nil {
			actualKey = txKey.SSHKey
			return nil
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying unit virtual SSH host key for %q: %w", unitName, err)
		}

		record := unitVirtualSSHHostKey{UnitUUID: unitUUID.UUID, AlgorithmTypeID: algorithmTypeID, SSHKey: sshKey}
		if err := tx.Query(ctx, insertStmt, record).Run(); err != nil {
			return errors.Errorf("persisting unit virtual SSH host key for %q: %w", unitName, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return actualKey, nil
}

// GetMachineNameForUnit returns the backing machine for a unit when one exists.
// The boolean indicates whether the unit is machine backed.
func (st *State) GetMachineNameForUnit(ctx context.Context, unitName string) (string, bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	nameRec := entityName{Name: unitName}
	stmt, err := st.Prepare(`
SELECT m.name AS &unitMachine.machine_name
FROM unit AS u
LEFT JOIN machine AS m ON m.net_node_uuid = u.net_node_uuid
WHERE u.name = $entityName.name`, unitMachine{}, entityName{})
	if err != nil {
		return "", false, errors.Capture(err)
	}

	var result unitMachine
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = unitMachine{}

		err := tx.Query(ctx, stmt, nameRec).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q %w", unitName, applicationerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Errorf("querying backing machine for unit %q: %w", unitName, err)
		}
		return nil
	})
	if err != nil {
		return "", false, errors.Capture(err)
	}
	if !result.MachineName.Valid {
		return "", false, nil
	}
	return result.MachineName.String, true, nil
}

// InsertSSHConnRequest stores a one-shot SSH connection request.
func (st *State) InsertSSHConnRequest(ctx context.Context, req domainssh.SSHConnRequest, now time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	getMachineUUIDStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM machine
WHERE name = $entityName.name`, entityUUID{}, entityName{})
	if err != nil {
		return errors.Capture(err)
	}
	checkExistsStmt, err := st.Prepare(`
SELECT &tunnelID.*
FROM ssh_connection_request
WHERE tunnel_id = $tunnelID.tunnel_id`, tunnelID{})
	if err != nil {
		return errors.Capture(err)
	}
	insertStmt, err := st.Prepare(`
INSERT INTO ssh_connection_request (*)
VALUES ($sshConnRequestInsert.*)`, sshConnRequestInsert{})
	if err != nil {
		return errors.Capture(err)
	}
	insertAddrStmt, err := st.Prepare(`
INSERT INTO ssh_connection_request_address (*)
VALUES ($sshConnRequestAddress.*)`, sshConnRequestAddress{})
	if err != nil {
		return errors.Capture(err)
	}

	record := sshConnRequestInsert{
		TunnelID:           req.TunnelID,
		ExpiresAt:          req.Expires,
		Username:           req.SSHUsername,
		Password:           req.SSHPassword,
		UnitPort:           req.UnitPort,
		EphemeralPublicKey: req.EphemeralPublicKey,
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.pruneExpiredSSHConnRequests(ctx, tx, now); err != nil {
			return errors.Errorf("pruning expired SSH connection requests: %w", err)
		}

		machineUUID := entityUUID{}
		err := tx.Query(ctx, getMachineUUIDStmt, entityName{Name: req.MachineName}).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q %w", req.MachineName, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("querying machine %q: %w", req.MachineName, err)
		}

		record.MachineUUID = machineUUID.UUID

		existing := tunnelID{}
		err = tx.Query(ctx, checkExistsStmt, tunnelID{TunnelID: req.TunnelID}).Get(&existing)
		if err == nil {
			return errors.Errorf("SSH connection request %q already exists", req.TunnelID).Add(coreerrors.AlreadyExists)
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking SSH connection request %q: %w", req.TunnelID, err)
		}

		if err := tx.Query(ctx, insertStmt, record).Run(); err != nil {
			return errors.Errorf("persisting SSH connection request %q: %w", req.TunnelID, err)
		}

		for i, addr := range req.ControllerAddresses {
			addrRow := sshConnRequestAddress{
				TunnelID:     req.TunnelID,
				IndexID:      i,
				AddressValue: addr.Value,
			}
			if err := tx.Query(ctx, insertAddrStmt, addrRow).Run(); err != nil {
				return errors.Errorf("persisting controller address %d for SSH connection request %q: %w", i, req.TunnelID, err)
			}
		}
		return nil
	}))
}

// GetSSHConnRequest returns a one-shot SSH connection request by tunnel ID.
func (st *State) GetSSHConnRequest(ctx context.Context, requestTunnelID string, now time.Time) (domainssh.SSHConnRequest, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainssh.SSHConnRequest{}, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT scr.tunnel_id AS &sshConnRequestRecord.tunnel_id,
       m.name AS &sshConnRequestRecord.machine_id,
       scr.expires_at AS &sshConnRequestRecord.expires_at,
       scr.username AS &sshConnRequestRecord.username,
       scr.password AS &sshConnRequestRecord.password,
       scr.unit_port AS &sshConnRequestRecord.unit_port,
       scr.ephemeral_public_key AS &sshConnRequestRecord.ephemeral_public_key
FROM ssh_connection_request AS scr
JOIN machine AS m ON m.uuid = scr.machine_uuid
WHERE scr.tunnel_id = $tunnelID.tunnel_id`, sshConnRequestRecord{}, tunnelID{})
	if err != nil {
		return domainssh.SSHConnRequest{}, errors.Capture(err)
	}
	addrStmt, err := st.Prepare(`
SELECT &sshConnRequestAddress.*
FROM ssh_connection_request_address
WHERE tunnel_id = $tunnelID.tunnel_id
ORDER BY index_id ASC`, sshConnRequestAddress{}, tunnelID{})
	if err != nil {
		return domainssh.SSHConnRequest{}, errors.Capture(err)
	}

	var result domainssh.SSHConnRequest
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = domainssh.SSHConnRequest{}

		if err := st.pruneExpiredSSHConnRequests(ctx, tx, now); err != nil {
			return errors.Errorf("pruning expired SSH connection requests: %w", err)
		}

		row := sshConnRequestRecord{}
		err := tx.Query(ctx, stmt, tunnelID{TunnelID: requestTunnelID}).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("SSH connection request %q not found", requestTunnelID).Add(coreerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf("querying SSH connection request %q: %w", requestTunnelID, err)
		}

		var addrRows []sshConnRequestAddress
		if err := tx.Query(ctx, addrStmt, tunnelID{TunnelID: requestTunnelID}).GetAll(&addrRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying controller addresses for SSH connection request %q: %w", requestTunnelID, err)
		}

		controllerAddresses := make(network.SpaceAddresses, len(addrRows))
		for i, a := range addrRows {
			controllerAddresses[i] = network.NewSpaceAddress(a.AddressValue)
		}

		result = domainssh.SSHConnRequest{
			TunnelID:            row.TunnelID,
			MachineName:         row.MachineID,
			Expires:             row.ExpiresAt,
			SSHUsername:         row.Username,
			SSHPassword:         row.Password,
			ControllerAddresses: controllerAddresses,
			UnitPort:            row.UnitPort,
			EphemeralPublicKey:  row.EphemeralPublicKey,
		}
		return nil
	})
	if err != nil {
		return domainssh.SSHConnRequest{}, errors.Capture(err)
	}
	return result, nil
}

// RemoveSSHConnRequest deletes a one-shot SSH connection request.
func (st *State) RemoveSSHConnRequest(ctx context.Context, requestTunnelID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	deleteAddrsStmt, err := st.Prepare(`
DELETE FROM ssh_connection_request_address
WHERE tunnel_id = $tunnelID.tunnel_id`, tunnelID{})
	if err != nil {
		return errors.Capture(err)
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM ssh_connection_request
WHERE tunnel_id = $tunnelID.tunnel_id`, tunnelID{})
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		tid := tunnelID{TunnelID: requestTunnelID}
		if err := tx.Query(ctx, deleteAddrsStmt, tid).Run(); err != nil {
			return errors.Errorf("deleting controller addresses for SSH connection request %q: %w", requestTunnelID, err)
		}
		return tx.Query(ctx, deleteStmt, tid).Run()
	}))
}

// PruneExpiredSSHConnRequests removes all expired SSH connection requests.
func (st *State) PruneExpiredSSHConnRequests(ctx context.Context, now time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.pruneExpiredSSHConnRequests(ctx, tx, now)
	}))
}

// InitialWatchSSHConnRequestsStatement returns the namespace and initial
// statement for SSH connection request watchers.
func (*State) InitialWatchSSHConnRequestsStatement() (string, string) {
	return "ssh_connection_request", "SELECT tunnel_id FROM ssh_connection_request"
}

func (st *State) pruneExpiredSSHConnRequests(ctx context.Context, tx *sqlair.TX, now time.Time) error {
	deleteAddrsStmt, err := st.Prepare(`
DELETE FROM ssh_connection_request_address
WHERE tunnel_id IN (
    SELECT tunnel_id FROM ssh_connection_request WHERE expires_at <= $expiryTime.expires_at
)`, expiryTime{})
	if err != nil {
		return errors.Capture(err)
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM ssh_connection_request
WHERE expires_at <= $expiryTime.expires_at`, expiryTime{})
	if err != nil {
		return errors.Capture(err)
	}
	t := expiryTime{ExpiresAt: now}
	if err := tx.Query(ctx, deleteAddrsStmt, t).Run(); err != nil {
		return errors.Errorf("pruning expired SSH connection request addresses: %w", err)
	}
	return tx.Query(ctx, deleteStmt, t).Run()
}
