// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	internaldatabase "github.com/juju/juju/internal/database"
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
SELECT uuid AS &entityUUID.uuid
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
SELECT uuid AS &entityUUID.uuid
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

	var key sshPrivateKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID := entityUUID{}
		key = sshPrivateKey{}
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
			err = tx.Query(ctx, getKeyStmt, machineUUID).Get(&key)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("machine virtual SSH host key for %q not found after concurrent insert", machineName)
			}
			if err != nil {
				return errors.Errorf("querying machine virtual SSH host key for %q after concurrent insert: %w", machineName, err)
			}
			return nil
		} else if err != nil {
			return errors.Errorf("persisting machine virtual SSH host key for %q: %w", machineName, err)
		}
		key = sshPrivateKey{SSHKey: sshKey}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return key.SSHKey, nil
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
SELECT uuid AS &entityUUID.uuid
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
SELECT uuid AS &entityUUID.uuid
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

	var key sshPrivateKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID := entityUUID{}
		key = sshPrivateKey{}
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
			err = tx.Query(ctx, getKeyStmt, unitUUID).Get(&key)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("unit virtual SSH host key for %q not found after concurrent insert", unitName)
			}
			if err != nil {
				return errors.Errorf("querying unit virtual SSH host key for %q after concurrent insert: %w", unitName, err)
			}
			return nil
		} else if err != nil {
			return errors.Errorf("persisting unit virtual SSH host key for %q: %w", unitName, err)
		}
		key = sshPrivateKey{SSHKey: sshKey}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return key.SSHKey, nil
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
