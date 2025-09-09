// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	corebase "github.com/juju/juju/core/base"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// checkIfMachineNameInUse checks the model to see if the provided machine name
// is in use or not.
func checkIfMachineNameInUse(
	ctx context.Context,
	tx *sqlair.TX,
	preparer domain.Preparer,
	mName string,
) (bool, error) {
	input := machineName{Name: mName}
	query := "SELECT &machineName.* FROM machine WHERE name = $machineName.name"
	stmt, err := preparer.Prepare(query, input)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// GetMachinesForExport returns all machines in the model for export.
func (st *State) GetMachinesForExport(ctx context.Context) ([]machine.ExportMachine, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, err
	}

	query := `
SELECT    &exportMachine.*
FROM      machine AS m
JOIN      machine_cloud_instance mci ON m.uuid = mci.machine_uuid
LEFT JOIN machine_placement AS mp ON m.uuid = mp.machine_uuid
LEFT JOIN v_machine_platform AS mpl ON m.uuid = mpl.machine_uuid
WHERE     mci.instance_id IS NOT NULL AND mci.instance_id != '';`
	stmt, err := st.Prepare(query, exportMachine{})
	if err != nil {
		return nil, errors.Errorf("preparing query for machine export: %w", err)
	}

	var machines []exportMachine
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&machines); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("querying machines for export: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]machine.ExportMachine, len(machines))
	for i, m := range machines {
		base, err := corebase.ParseBase(m.OSName, m.Channel)
		if err != nil {
			return nil, errors.Errorf("parsing base: %w", err)
		}
		result[i] = machine.ExportMachine{
			Name:         coremachine.Name(m.Name),
			UUID:         coremachine.UUID(m.UUID),
			Nonce:        m.Nonce,
			PasswordHash: m.PasswordHash,
			Placement:    m.PlacementDirective,
			Base:         base.String(),
			InstanceID:   m.InstanceID,
		}
	}
	return result, nil
}

// InsertMigratingMachine inserts a machine which is taken from the description
// model during migration into the machine table.
//
// The following errors may be returned:
// - [machineerrors.MachineAlreadyExists] if a machine with the same name
// already exists.
func (st *State) InsertMigratingMachine(ctx context.Context, machineName string, args machine.CreateMachineArgs) error {
	db, err := st.DB(ctx)
	if err != nil {
		return err
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		inUse, err := checkIfMachineNameInUse(ctx, tx, st, machineName)
		if err != nil {
			return errors.Errorf(
				"checking if machine name %q already in use: %w", machineName, err,
			)
		}

		if inUse {
			return errors.Errorf(
				"machine %q already exists in model", machineName,
			).Add(machineerrors.MachineAlreadyExists)
		}
		return CreateMachineWithName(ctx, tx, st, st.clock, machineName, createMachineArgs{
			MachineUUID: args.MachineUUID.String(),
			Platform:    args.Platform,
			Nonce:       args.Nonce,
		})
	})
	return errors.Capture(err)
}
