package state

import (
	"context"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// GetMachinesBoundToSpaces retrieves the machines associated with the
// specified space identifiers.
//
// A machine is bound to a space if either one of both conditions happens:
//
//   - the machine runs a unit belonging to an application with a binding
//     to this space (through application binding or endpoint binding)
//   - the machine has a positive constraint on this space, which implies that
//     it should belong to this space.
func (st *State) GetMachinesBoundToSpaces(ctx context.Context, spaceUUIDs []string) (internal.CheckableMachines, error) {
	if len(spaceUUIDs) == 0 {
		return nil, nil
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Query to find machines bound to spaces either through application bindings or positive constraints
	query := `
WITH bound_machines AS (
    -- Machines with units belonging to applications with bindings to the specified spaces
    SELECT DISTINCT m.uuid AS machine_uuid
    FROM machine m
    JOIN unit u ON m.net_node_uuid = u.net_node_uuid
    JOIN (
        -- Application endpoints bound to the specified spaces
        SELECT application_uuid
        FROM application_endpoint
        WHERE space_uuid IN ($uuids[:])
        UNION
        -- Application extra endpoints bound to the specified spaces
        SELECT application_uuid
        FROM application_extra_endpoint
        WHERE space_uuid IN ($uuids[:])
		UNION
		-- Application with the space_uuid as default
        SELECT uuid as application_uuid
        FROM application
        WHERE space_uuid IN ($uuids[:])
    ) b ON u.application_uuid = b.application_uuid

    UNION

    -- Machines with positive constraints on the specified spaces
    SELECT DISTINCT m.machine_uuid
    FROM machine_constraint m
    JOIN constraint_space cs ON m.constraint_uuid = cs.constraint_uuid
    JOIN space s ON cs.space = s.name
    WHERE s.uuid IN ($uuids[:])
    AND cs.exclude IS FALSE
)
SELECT &machineRow.* FROM bound_machines
JOIN machine AS m ON bound_machines.machine_uuid = m.uuid
LEFT JOIN ip_address AS a ON m.net_node_uuid = a.net_node_uuid`

	type machineRow struct {
		Name    string `db:"name"`
		Address string `db:"address_value"`
	}

	stmt, err := st.Prepare(query, machineRow{}, uuids{})
	if err != nil {
		return nil, errors.Errorf("preparing machines bound to spaces statement: %w", err)
	}

	var rows []machineRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, uuids(spaceUUIDs)).GetAll(&rows); err != nil {
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying machines bound to spaces: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// If no machines are bound to the specified spaces, return a nil slice
	if len(rows) == 0 {
		return nil, nil
	}

	// Create a slice of boundMachine objects
	machines, _ := accumulateToMap(rows, func(row machineRow) (string, string, error) {
		ip, _, _ := strings.Cut(row.Address, "/")
		return row.Name, ip, nil
	})
	return transform.MapToSlice(machines, func(name string,
		addresses []string) []internal.CheckableMachine {
		return []internal.CheckableMachine{
			boundMachine{
				machineName:  name,
				addresses:    filterZeroInPlace(addresses),
				inSpaceUUIDs: spaceUUIDs,
				logger:       st.logger,
			}}
	}), nil
}

// boundMachine implements the internal.CheckableMachine interface.
// It represents a machine that is bound to one or more spaces.
type boundMachine struct {
	machineName  string
	addresses    []string
	inSpaceUUIDs []string
	logger       logger.Logger
}

// Accept checks if the machine is compatible with the new topology.
// A machine is compatible if all of its positive space constraints and
// application bindings are still satisfied in the new topology.
func (m boundMachine) Accept(ctx context.Context, topology network.SpaceInfos) error {
	shouldBeInSpaces := set.NewStrings(m.inSpaceUUIDs...)
	for _, addr := range m.addresses {
		if shouldBeInSpaces.IsEmpty() {
			break // all spaces have an address
		}
		space, err := topology.InferSpaceFromAddress(addr)
		if err != nil {
			m.logger.Warningf(ctx, "failed to infer space from address %q: %s", addr, err)
			continue
		}
		shouldBeInSpaces.Remove(space.ID.String())
	}
	if !shouldBeInSpaces.IsEmpty() {
		spaceNames := transform.Slice(shouldBeInSpaces.Values(), func(s string) string {
			return topology.GetByID(network.SpaceUUID(s)).Name.String()
		})
		slices.Sort(spaceNames)
		return errors.Errorf("machine %q is missing addresses in spaces %s", m.machineName,
			strings.Join(spaceNames, ", "))
	}
	return nil
}

// GetMachinesNotAllowedInSpace retrieves a list of machines that are
// incompatible with the specified space given its UUID.
//
// A machine is not compatible with a space if it has a negative constraint
// against it.
func (st *State) GetMachinesNotAllowedInSpace(ctx context.Context, spaceUUID string) (internal.CheckableMachines, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	type space entityUUID

	query := `
WITH bound_machines AS (
	SELECT DISTINCT machine_uuid
	FROM machine_constraint mc
	JOIN constraint_space cs ON mc.constraint_uuid = cs.constraint_uuid
	JOIN space s ON cs.space = s.name
	WHERE s.uuid = $space.uuid
	AND cs.exclude IS TRUE
)
SELECT &machineRow.* FROM bound_machines
JOIN machine AS m ON bound_machines.machine_uuid = m.uuid
LEFT JOIN ip_address AS a ON m.net_node_uuid = a.net_node_uuid
`

	type machineRow struct {
		Name    string `db:"name"`
		Address string `db:"address_value"`
	}

	stmt, err := st.Prepare(query, machineRow{}, space{})
	if err != nil {
		return nil, errors.Errorf("preparing machines bound to spaces statement: %w", err)
	}

	var rows []machineRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, space{UUID: spaceUUID}).GetAll(&rows); err != nil {
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying machines bound to spaces: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// If no machines are bound to the specified spaces, return a nil slice
	if len(rows) == 0 {
		return nil, nil
	}

	// Create a slice of boundMachine objects
	machines, _ := accumulateToMap(rows, func(row machineRow) (string, string, error) {
		ip, _, _ := strings.Cut(row.Address, "/")
		return row.Name, ip, nil
	})
	return transform.MapToSlice(machines, func(name string,
		addresses []string) []internal.CheckableMachine {
		return []internal.CheckableMachine{
			allergicMachine{
				machineName:      name,
				addresses:        filterZeroInPlace(addresses),
				excludeSpaceUUID: spaceUUID,
				logger:           st.logger,
			}}
	}), nil
}

// allergicMachine implements the internal.CheckableMachine interface.
// It represents a machine that shouldn't have any address into a space.
type allergicMachine struct {
	machineName      string
	addresses        []string
	excludeSpaceUUID string
	logger           logger.Logger
}

// Accept checks if the machine is compatible with the new topology.
// A machine is compatible if all of its negative space constraints
// are still satisfied in the new topology.
func (m allergicMachine) Accept(ctx context.Context, topology network.SpaceInfos) error {
	faultyAddresses := set.NewStrings()
	for _, addr := range m.addresses {
		space, err := topology.InferSpaceFromAddress(addr)
		if err != nil {
			m.logger.Warningf(ctx, "failed to infer space from address %q: %s", addr, err)
			continue
		}
		if space.ID.String() == m.excludeSpaceUUID {
			faultyAddresses.Add(addr)
		}
	}
	if !faultyAddresses.IsEmpty() {
		return errors.Errorf("machine %q would have %d addresses in excluded space %q (%s)", m.machineName,
			len(faultyAddresses),
			topology.GetByID(network.SpaceUUID(m.excludeSpaceUUID)).Name,
			strings.Join(faultyAddresses.SortedValues(), ", "))
	}
	return nil
}

// filterZeroInPlace removes all zero-equivalent elements from the input slice
// in place and returns the filtered slice.
// The zero value for the generic type T is determined using the default initialization of T.
func filterZeroInPlace[T comparable](addresses []T) []T {
	var zero T
	filtered := addresses[:0]
	for _, addr := range addresses {
		if addr != zero {
			filtered = append(filtered, addr)
		}
	}
	if len(filtered) == 0 {
		filtered = nil
	}
	return filtered
}
