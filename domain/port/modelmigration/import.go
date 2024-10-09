// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v8"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
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

// ImportService provides a subset of the port domain
// service methods needed for open ports import.
type ImportService interface {
	SetUnitPorts(ctx context.Context, unitName string, openPorts network.GroupedPortRanges) error
}

type importOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import open ports"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB()))
	return nil
}

// Execute the import on the open ports on the model applications, machines and
// units.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {

	// First we need to import endpoints. These are on the applications.

	// For each application, check if the open ports are on unit, application or
	// machines and import appropriately. Main case should be units, but that
	// won't be implemented yet so that should just be a comment.

	// For machines, we will need to match up unit machine tags with machines in
	// the list of machines.

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
	for unitName, unitPorts := range ports {
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
		err := i.service.SetUnitPorts(ctx, unitName, openPorts)
		if err != nil {
			return errors.Errorf("setting open ports on unit %s: %w", unitName, err)
		}
	}
	return nil
}
