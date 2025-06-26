// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/port/service"
	"github.com/juju/juju/domain/port/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// PortService provides a subset of the port domain
// service methods needed for open ports import.
type PortService interface {
	// UpdateUnitPorts updates the open ports on the unit with the given UUID.
	UpdateUnitPorts(
		ctx context.Context,
		unitUUID coreunit.UUID,
		openPorts, closePorts network.GroupedPortRanges,
	) error

	// GetUnitUUID returns the UUID of the unit with the given name.
	GetUnitUUID(context.Context, coreunit.Name) (coreunit.UUID, error)
}

type importOperation struct {
	modelmigration.BaseOperation

	logger      logger.Logger
	portService PortService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import open ports"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.portService = service.NewService(
		state.NewState(scope.ModelDB()), i.logger)
	return nil
}

// Execute the import on the open ports on the model applications, machines and
// units.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	// For imports from 3.x ports can either be stored on machines or
	// applications. The ports should be stored on one or the other, but to keep
	// the migration robust, we import everything from both.

	apps := model.Applications()
	for _, app := range apps {
		err := i.importUnitPorts(ctx, app.OpenedPortRanges().ByUnit())
		if err != nil {
			return errors.Errorf("importing open ports of application %s: %w", app.Name(), err)
		}
	}

	machines := model.Machines()
	for _, m := range machines {
		err := i.importUnitPorts(ctx, m.OpenedPortRanges().ByUnit())
		if err != nil {
			return errors.Errorf("importing open ports on machine %s: %w", m.Id(), err)
		}
	}

	return nil
}

func (i *importOperation) importUnitPorts(
	ctx context.Context, ports map[string]description.UnitPortRanges,
) error {
	for unit, unitPorts := range ports {
		unitName, err := coreunit.NewName(unit)
		if err != nil {
			return errors.Errorf("parsing unit name %s: %w", unitName, err)
		}
		openPorts := make(network.GroupedPortRanges)
		for endpointName, portRanges := range unitPorts.ByEndpoint() {
			portRangeList := transform.Slice(portRanges, func(pr description.UnitPortRange) network.PortRange {
				return network.PortRange{
					FromPort: pr.FromPort(),
					ToPort:   pr.ToPort(),
					Protocol: pr.Protocol(),
				}
			})
			openPorts[endpointName] = portRangeList
		}
		unitUUID, err := i.portService.GetUnitUUID(ctx, unitName)
		if err != nil {
			return errors.Errorf("getting uuid for unit %s: %w", unitName, err)
		}
		err = i.portService.UpdateUnitPorts(ctx, unitUUID, openPorts, nil)
		if err != nil {
			return errors.Errorf("setting open ports on unit %s: %w", unitName, err)
		}
	}
	return nil
}
