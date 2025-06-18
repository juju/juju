// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
)

// GetMachinesForExport returns all machines in the model for export.
func (st *State) GetMachinesForExport(ctx context.Context) ([]machine.ExportMachine, error) {
	db, err := st.DB()
	if err != nil {
		return nil, err
	}

	query := `
SELECT &exportMachine.*
FROM machine
JOIN machine_cloud_instance mci ON machine.uuid = mci.machine_uuid
WHERE mci.instance_id IS NOT NULL AND mci.instance_id != '';`

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

	result := make([]machine.ExportMachine, len(machines))
	for i, m := range machines {
		result[i] = machine.ExportMachine{
			Name:         coremachine.Name(m.Name),
			UUID:         coremachine.UUID(m.UUID),
			Nonce:        m.Nonce,
			PasswordHash: m.PasswordHash,
		}
	}
	return result, nil
}
