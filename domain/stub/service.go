// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// StubService is a special service that collects temporary methods required for
// wiring together domains which not completely implemented or wired up.
//
// Given the temporary nature of this service, we have not implemented the full
// service/state layer indirection. Instead, the service directly uses a transaction
// runner.
//
// Deprecated: All methods here should be thrown away as soon as we're done with
// then.
type StubService struct {
	*domain.StateBase
}

// NewStubService returns a new StubService.
func NewStubService(
	factory database.TxnRunnerFactory,
) *StubService {
	return &StubService{
		StateBase: domain.NewStateBase(factory),
	}
}

// AssignUnitsToMachines assigns the given units to the given machines but setting
// unit net node to the machine net node.
//
// Deprecated: AssignUnitsToMachines will become redundant once the machine and
// application domains have become fully implemented.
func (s *StubService) AssignUnitsToMachines(ctx context.Context, groupedUnitsByMachine map[string][]unit.Name) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	getNetNodeQuery, err := s.Prepare(`
SELECT &netNodeUUID.*
FROM machine
WHERE name = $machine.name
`, netNodeUUID{}, machine{})
	if err != nil {
		return errors.Errorf("preparing machine query: %v", err)
	}

	verifyUnitsExistQuery, err := s.Prepare(`
SELECT COUNT(*) AS &count.count
FROM unit
WHERE name IN ($units[:])
`, count{}, units{})
	if err != nil {
		return errors.Errorf("preparing verify units exist query: %v", err)
	}

	setUnitsNetNodeQuery, err := s.Prepare(`
UPDATE unit
SET net_node_uuid = $netNodeUUID.net_node_uuid
WHERE name IN ($units[:])
`, netNodeUUID{}, units{})
	if err != nil {
		return errors.Errorf("preparing set units query: %v", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for machine, units := range encodeGroupedUnitsByMachine(groupedUnitsByMachine) {
			var netNodeUUID netNodeUUID
			err = tx.Query(ctx, getNetNodeQuery, machine).Get(&netNodeUUID)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("%w: %v", machineerrors.MachineNotFound, machine.MachineName)
			}
			if err != nil {
				return errors.Errorf("getting machine net node: %v", err)
			}

			var count count
			err = tx.Query(ctx, verifyUnitsExistQuery, units).Get(&count)
			if err != nil {
				return errors.Errorf("verifying units exist: %v", err)
			}
			if count.Count != len(units) {
				return errors.Errorf("not all units found %q", units).
					Add(applicationerrors.UnitNotFound)
			}

			err = tx.Query(ctx, setUnitsNetNodeQuery, netNodeUUID, units).Run()
			if err != nil {
				return errors.Errorf("setting unit: %v", err)
			}
		}
		return nil
	})

	if err != nil {
		return errors.Errorf("assigning units to machines: %w", err)
	}
	return err
}
