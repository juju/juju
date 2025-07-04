// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
}

// CheckMachineIsDead checks to see if a machine is not dead returning
// true when the life of the machine is dead.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided uuid.
func (st *State) CheckMachineIsDead(
	ctx context.Context, uuid machine.UUID,
) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var (
		input       = machineUUID{UUID: uuid.String()}
		machineLife machineLife
	)
	stmt, err := st.Prepare(
		"SELECT &machineLife.* FROM machine WHERE uuid = $machineUUID.uuid",
		input, machineLife,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&machineLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q does not exist", uuid).Add(
				machineerrors.MachineNotFound,
			)
		}
		return err
	})

	if err != nil {
		return false, errors.Capture(err)
	}

	return domainlife.Life(machineLife.LifeId) == domainlife.Dead, nil
}

// GetMachineNetNodeUUID retrieves the net node uuid associated with provided
// machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// uuid.
func (st *State) GetMachineNetNodeUUID(
	ctx context.Context, uuid machine.UUID,
) (domainnetwork.NetNodeUUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		input = machineUUID{UUID: uuid.String()}
		dbVal netNodeUUIDRef
	)
	stmt, err := st.Prepare(
		"SELECT &netNodeUUIDRef.* FROM machine WHERE uuid = $machineUUID.uuid",
		input, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q does not exist", uuid).Add(
				machineerrors.MachineNotFound,
			)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return domainnetwork.NetNodeUUID(dbVal.UUID), nil
}

func (st *State) NamespaceForWatchMachineCloudInstance() string {
	return "machine_cloud_instance"
}

// checkNetNodeExists checks if the provided net node uuid exists in the
// database during a txn. False is returned when the net node does not exist.
func (st *State) checkNetNodeExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) (bool, error) {
	input := netNodeUUID{UUID: uuid}

	checkStmt, err := st.Prepare(
		"SELECT &netNodeUUID.* FROM net_node WHERE uuid = $netNodeUUID.uuid",
		input,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// NewState creates and returns a new [State] for provisioning storage in the
// model.
func NewState(
	factory database.TxnRunnerFactory,
) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
