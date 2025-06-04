// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the network domain
// service methods needed for spaces/subnets export.
type ExportService interface {
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	exportService ExportService
	logger        logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export networks"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.exportService = service.NewService(
		state.NewState(scope.ModelDB(), e.logger),
		e.logger,
	)
	return nil
}

// Execute the migration export, which adds the spaces and subnets to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	spaces, err := e.exportService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	for _, space := range spaces {
		// We do not export the alpha space because it is created by default
		// with the new model. This is OK, because it is immutable.
		// Any subnets added to the space will still be exported.
		if space.ID == network.AlphaSpaceId {
			continue
		}

		model.AddSpace(description.SpaceArgs{
			UUID:       space.ID.String(),
			Name:       space.Name.String(),
			ProviderID: string(space.ProviderId),
		})
	}

	// Export subnets.
	subnets, err := e.exportService.GetAllSubnets(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	for _, subnet := range subnets {
		args := description.SubnetArgs{
			UUID:              string(subnet.ID),
			CIDR:              subnet.CIDR,
			ProviderId:        string(subnet.ProviderId),
			ProviderSpaceId:   string(subnet.ProviderSpaceId),
			ProviderNetworkId: string(subnet.ProviderNetworkId),
			VLANTag:           subnet.VLANTag,
			SpaceID:           subnet.SpaceID.String(),
			SpaceName:         subnet.SpaceName.String(),
			AvailabilityZones: subnet.AvailabilityZones,
		}
		model.AddSubnet(args)
	}

	return nil
}
