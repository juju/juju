// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainssh "github.com/juju/juju/domain/ssh"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents model-scoped SSH host key state.
type State struct {
	*domain.StateBase
	controllerState *domain.StateBase
}

// NewState returns a new model-scoped SSH state.
func NewState(factory database.TxnRunnerFactory, controllerFactory database.TxnRunnerFactory) *State {
	return &State{
		StateBase:       domain.NewStateBase(factory),
		controllerState: domain.NewStateBase(controllerFactory),
	}
}

// GetModelInfo returns the model metadata needed to route SSH destinations.
func (st *State) GetModelInfo(ctx context.Context) (ModelInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return ModelInfo{}, errors.Capture(err)
	}
	row := modelInfo{}
	stmt, err := st.Prepare(`
SELECT name AS &modelInfo.name,
       type AS &modelInfo.type,
       is_controller_model AS &modelInfo.is_controller_model
FROM model`, row)
	if err != nil {
		return ModelInfo{}, errors.Capture(err)
	}
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Get(&row); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("model does not exist").Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("querying model info: %w", err)
		}
		return nil
	}); err != nil {
		return ModelInfo{}, errors.Capture(err)
	}
	return ModelInfo{
		Name:              row.Name,
		Type:              row.Type,
		IsControllerModel: row.IsControllerModel,
	}, nil
}

// GetControllerName returns the controller name from controller state.
func (st *State) GetControllerName(ctx context.Context) (string, error) {
	db, err := st.controllerState.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	row := controllerName{}
	stmt, err := st.controllerState.Prepare(`
SELECT value AS &controllerName.name
FROM v_controller_config
WHERE key = 'controller-name'`, row)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Get(&row); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("controller name not found")
		} else if err != nil {
			return errors.Errorf("querying controller name: %w", err)
		}
		return nil
	})
	return row.Name, errors.Capture(err)
}

// GetUnitK8sPodInfo returns the Kubernetes pod provider ID for a live unit.
func (st *State) GetUnitK8sPodInfo(ctx context.Context, unitName string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := entityName{Name: unitName}
	row := unitK8sPodInfo{}
	stmt, err := st.Prepare(`
SELECT k.provider_id AS &unitK8sPodInfo.provider_id,
       u.life_id AS &unitK8sPodInfo.life_id
FROM unit AS u
LEFT JOIN k8s_pod AS k ON u.uuid = k.unit_uuid
WHERE u.name = $entityName.name`, row, arg)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, arg).Get(&row); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("querying Kubernetes pod info for unit %q: %w", unitName, err)
		}
		if row.LifeID == domainlife.Dead {
			return applicationerrors.UnitIsDead
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	if !row.ProviderID.Valid {
		return "", errors.Errorf("unit %q has no k8s pod", unitName).Add(applicationerrors.UnitK8sProviderIDNotAssigned)
	}
	return row.ProviderID.String, nil
}

// GetUnitMachineName returns the backing machine name for a live unit.
func (st *State) GetUnitMachineName(ctx context.Context, unitName string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := entityName{Name: unitName}
	row := unitMachineRouting{}
	stmt, err := st.Prepare(`
SELECT u.life_id AS &unitMachineRouting.life_id,
       m.name AS &unitMachineRouting.machine_name
FROM unit AS u
LEFT JOIN machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE u.name = $entityName.name`, row, arg)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, arg).Get(&row); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("querying machine for unit %q: %w", unitName, err)
		}
		if row.LifeID == domainlife.Dead {
			return applicationerrors.UnitIsDead
		}
		if !row.MachineName.Valid {
			return applicationerrors.UnitMachineNotAssigned
		}
		return nil
	})
	return row.MachineName.String, errors.Capture(err)
}

// CheckMachineExists reports whether a machine with the supplied name exists
// and is not dead.
func (st *State) CheckMachineExists(ctx context.Context, machineName string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}
	arg := entityName{Name: machineName}
	stmt, err := st.Prepare(`
SELECT uuid AS &machineExists.uuid,
       life_id AS &machineExists.life_id
FROM machine
WHERE name = $entityName.name`, machineExists{}, arg)
	if err != nil {
		return false, errors.Capture(err)
	}
	var row machineExists
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("checking machine %q exists: %w", machineName, err)
		}
		if row.LifeID == domainlife.Dead {
			return errors.Errorf("machine %q is dead", machineName).Add(machineerrors.MachineIsDead)
		}
		return nil
	})
	if err != nil {
		return false, errors.Capture(err)
	}
	return row.UUID != "", nil
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
	insertStmt, err := st.Prepare(`
INSERT INTO machine_virtual_ssh_host_key (machine_uuid, algorithm_type_id, ssh_key)
VALUES ($machineVirtualSSHHostKey.*)`, machineVirtualSSHHostKey{})
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

	actualKey := sshKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID := entityUUID{}
		txKey := sshPrivateKey{}
		err := tx.Query(ctx, getMachineUUIDStmt, nameRec).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q %w", machineName, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("querying machine %q: %w", machineName, err)
		}

		record := machineVirtualSSHHostKey{MachineUUID: machineUUID.UUID, AlgorithmTypeID: algorithmTypeID, SSHKey: sshKey}
		err = tx.Query(ctx, insertStmt, record).Run()
		if internaldatabase.IsErrConstraintPrimaryKey(err) || internaldatabase.IsErrConstraintUnique(err) {
			err = tx.Query(ctx, getKeyStmt, machineUUID).Get(&txKey)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("machine virtual SSH host key for %q not found after concurrent insert", machineName)
			}
			if err != nil {
				return errors.Errorf("querying machine virtual SSH host key for %q after concurrent insert: %w", machineName, err)
			}
			actualKey = txKey.SSHKey
			return nil
		} else if err != nil {
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
	insertStmt, err := st.Prepare(`
INSERT INTO unit_virtual_ssh_host_key (unit_uuid, algorithm_type_id, ssh_key)
VALUES ($unitVirtualSSHHostKey.*)`, unitVirtualSSHHostKey{})
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

	actualKey := sshKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID := entityUUID{}
		txKey := sshPrivateKey{}
		err := tx.Query(ctx, getUnitUUIDStmt, nameRec).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q %w", unitName, applicationerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Errorf("querying unit %q: %w", unitName, err)
		}

		record := unitVirtualSSHHostKey{UnitUUID: unitUUID.UUID, AlgorithmTypeID: algorithmTypeID, SSHKey: sshKey}
		err = tx.Query(ctx, insertStmt, record).Run()
		if internaldatabase.IsErrConstraintPrimaryKey(err) || internaldatabase.IsErrConstraintUnique(err) {
			err = tx.Query(ctx, getKeyStmt, unitUUID).Get(&txKey)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("unit virtual SSH host key for %q not found after concurrent insert", unitName)
			}
			if err != nil {
				return errors.Errorf("querying unit virtual SSH host key for %q after concurrent insert: %w", unitName, err)
			}
			actualKey = txKey.SSHKey
			return nil
		} else if err != nil {
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
	controllerAddresses, err := marshalControllerAddresses(req.ControllerAddresses)
	if err != nil {
		return errors.Errorf("marshalling controller addresses: %w", err)
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

		existing := tunnelID{}
		err = tx.Query(ctx, checkExistsStmt, tunnelID{TunnelID: req.TunnelID}).Get(&existing)
		if err == nil {
			return errors.Errorf("SSH connection request %q already exists", req.TunnelID)
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking SSH connection request %q: %w", req.TunnelID, err)
		}

		record := sshConnRequestInsert{
			TunnelID:            req.TunnelID,
			MachineUUID:         machineUUID.UUID,
			ExpiresAt:           req.Expires,
			ControllerAddresses: controllerAddresses,
			UnitPort:            req.UnitPort,
			EphemeralPublicKey:  req.EphemeralPublicKey,
		}
		if err := tx.Query(ctx, insertStmt, record).Run(); err != nil {
			return errors.Errorf("persisting SSH connection request %q: %w", req.TunnelID, err)
		}

		return nil
	}))
}

// GetSSHConnRequest returns a one-shot SSH connection request by tunnel ID,
// scoped to the named machine. The machine filter is applied in the query so a
// request targeting another machine is reported as not found, rather than being
// fetched and rejected afterwards.
func (st *State) GetSSHConnRequest(ctx context.Context, machineName string, requestTunnelID string, now time.Time) (domainssh.SSHConnRequest, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainssh.SSHConnRequest{}, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT scr.tunnel_id AS &sshConnRequestRecord.tunnel_id,
       m.name AS &sshConnRequestRecord.machine_id,
       scr.expires_at AS &sshConnRequestRecord.expires_at,
       scr.controller_addresses AS &sshConnRequestRecord.controller_addresses,
       scr.unit_port AS &sshConnRequestRecord.unit_port,
       scr.ephemeral_public_key AS &sshConnRequestRecord.ephemeral_public_key
FROM ssh_connection_request AS scr
JOIN machine AS m ON m.uuid = scr.machine_uuid
WHERE scr.tunnel_id = $tunnelID.tunnel_id
AND m.name = $entityName.name`, sshConnRequestRecord{}, tunnelID{}, entityName{})
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
		err := tx.Query(ctx, stmt, tunnelID{TunnelID: requestTunnelID}, entityName{Name: machineName}).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("SSH connection request %q not found", requestTunnelID)
		}
		if err != nil {
			return errors.Errorf("querying SSH connection request %q: %w", requestTunnelID, err)
		}

		controllerAddresses, err := unmarshalControllerAddresses(row.ControllerAddresses)
		if err != nil {
			return errors.Errorf("unmarshalling controller addresses for %q: %w", requestTunnelID, err)
		}

		result = domainssh.SSHConnRequest{
			TunnelID:            row.TunnelID,
			MachineName:         row.MachineID,
			Expires:             row.ExpiresAt,
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

	stmt, err := st.Prepare(`
DELETE FROM ssh_connection_request
WHERE tunnel_id = $tunnelID.tunnel_id`, tunnelID{})
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, tunnelID{TunnelID: requestTunnelID}).Run()
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
// statement for a machine's SSH connection request watcher. The statement is
// parameterised by the machine UUID so that the watcher's initial event only
// reports requests targeting that machine.
func (*State) InitialWatchSSHConnRequestsStatement() (string, string) {
	return "ssh_connection_request", "SELECT tunnel_id FROM ssh_connection_request WHERE machine_uuid = ?"
}

// GetMachineUUIDByName returns the UUID of the named machine.
func (st *State) GetMachineUUIDByName(ctx context.Context, machineName string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM machine
WHERE name = $entityName.name`, entityUUID{}, entityName{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var machineUUID entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, entityName{Name: machineName}).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q %w", machineName, machineerrors.MachineNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying machine %q: %w", machineName, err)
	}
	return machineUUID.UUID, nil
}

// FilterSSHConnRequestsForMachine returns the subset of the supplied tunnel IDs
// that identify SSH connection requests targeting the machine with the given
// UUID.
func (st *State) FilterSSHConnRequestsForMachine(ctx context.Context, tunnelIDs []string, machineUUID string) ([]string, error) {
	if len(tunnelIDs) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := make(sqlair.S, 0, len(tunnelIDs))
	for _, id := range tunnelIDs {
		input = append(input, id)
	}

	stmt, err := st.Prepare(`
SELECT tunnel_id AS &tunnelID.tunnel_id
FROM ssh_connection_request
WHERE machine_uuid = $entityUUID.uuid
AND tunnel_id IN ($S[:])`, tunnelID{}, entityUUID{}, input)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var matched []tunnelID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, entityUUID{UUID: machineUUID}, input).GetAll(&matched)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("filtering SSH connection requests for machine %q: %w", machineUUID, err)
	}

	result := make([]string, len(matched))
	for i, m := range matched {
		result[i] = m.TunnelID
	}
	return result, nil
}

func (st *State) pruneExpiredSSHConnRequests(ctx context.Context, tx *sqlair.TX, now time.Time) error {
	stmt, err := st.Prepare(`
DELETE FROM ssh_connection_request
WHERE expires_at <= $expiryTime.expires_at`, expiryTime{})
	if err != nil {
		return errors.Capture(err)
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
