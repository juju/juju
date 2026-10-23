// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// GetOtherControllerMachines retrieves controller machines other than the
// replacement. The view includes machines that also host workload units.
func (st *State) GetOtherControllerMachines(ctx context.Context, machineName string) ([]string, error) {
	type machine struct {
		Name string `db:"name"`
	}
	input := machine{Name: machineName}
	stmt, err := st.Prepare(`
SELECT m.name AS &machine.name FROM machine AS m
JOIN v_machine_is_controller AS vic ON vic.machine_uuid = m.uuid
WHERE m.name != $machine.name ORDER BY m.name`, input)
	if err != nil {
		return nil, errors.Capture(err)
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var rows []machine
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.Name
	}
	return result, nil
}

// GetRecoveryCounts retrieves lifecycle and scheduled removal counts.
func (st *State) GetRecoveryCounts(ctx context.Context) (domainrecovery.RecoveryCounts, error) {
	statements := []string{
		`SELECT COUNT(*) AS &recoveryCounts.machines FROM machine AS m WHERE m.life_id != 0`,
		`SELECT COUNT(*) AS &recoveryCounts.applications FROM application AS a WHERE a.life_id != 0`,
		`SELECT COUNT(*) AS &recoveryCounts.units FROM unit AS u WHERE u.life_id != 0`,
		`SELECT COUNT(*) AS &recoveryCounts.removals FROM removal AS r`,
	}
	var prepared []*sqlair.Statement
	for _, query := range statements {
		stmt, err := st.Prepare(query, recoveryCounts{})
		if err != nil {
			return domainrecovery.RecoveryCounts{}, errors.Capture(err)
		}
		prepared = append(prepared, stmt)
	}
	db, err := st.DB(ctx)
	if err != nil {
		return domainrecovery.RecoveryCounts{}, errors.Capture(err)
	}
	var result recoveryCounts
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, stmt := range prepared {
			if err := tx.Query(ctx, stmt).Get(&result); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	return domainrecovery.RecoveryCounts{
		MachinesNotAlive:     result.MachinesNotAlive,
		ApplicationsNotAlive: result.ApplicationsNotAlive,
		UnitsNotAlive:        result.UnitsNotAlive,
		PendingRemovals:      result.PendingRemovals,
	}, errors.Capture(err)
}

// recoveryCounts is the Sqlair representation of the aggregate query results.
type recoveryCounts struct {
	MachinesNotAlive     int `db:"machines"`
	ApplicationsNotAlive int `db:"applications"`
	UnitsNotAlive        int `db:"units"`
	PendingRemovals      int `db:"removals"`
}
