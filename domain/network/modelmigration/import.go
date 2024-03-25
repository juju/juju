// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/errors"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

// ImportSpaceService provides a subset of the network domain
// service methods needed for spaces import.
type ImportSpaceService interface {
	AddSpace(ctx context.Context, name string, providerID network.Id, subnetIDs []string) (network.Id, error)
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
}

// ImportSpaceService provides a subset of the network domain
// service methods needed for subnets import.
type ImportSubnetService interface {
	AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error)
}

type importOperation struct {
	modelmigration.BaseOperation

	spaceService  ImportSpaceService
	subnetService ImportSubnetService
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.spaceService = service.NewSpaceService(
		state.NewState(scope.ModelDB()),
		logger,
	)
	i.subnetService = service.NewSubnetService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the import of the spaces and subnets contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	spaceIDsMap := make(map[string]string)
	for _, space := range model.Spaces() {
		// The default space should not have been exported, but be defensive.
		if space.Name() == network.AlphaSpaceName {
			continue
		}
		spaceID, err := i.spaceService.AddSpace(ctx, space.Name(), network.Id(space.ProviderID()), nil)
		if err != nil {
			return errors.Annotatef(err, "creating space %s", space.Name())
		}
		// Update the space IDs mapping, which we need for subnets
		// import.
		spaceIDsMap[space.Id()] = spaceID.String()
	}

	// Now import the subnets.
	for _, subnet := range model.Subnets() {
		subnetInfo := network.SubnetInfo{
			CIDR:              subnet.CIDR(),
			ProviderId:        network.Id(subnet.ProviderId()),
			VLANTag:           subnet.VLANTag(),
			AvailabilityZones: subnet.AvailabilityZones(),
			ProviderNetworkId: network.Id(subnet.ProviderNetworkId()),
		}
		if subnet.FanLocalUnderlay() != "" || subnet.FanOverlay() != "" {
			subnetInfo.FanInfo = &network.FanCIDRs{
				FanLocalUnderlay: subnet.FanLocalUnderlay(),
				FanOverlay:       subnet.FanOverlay(),
			}
		}

		importedSpaceID, ok := spaceIDsMap[subnet.SpaceID()]
		if ok {
			space, err := i.spaceService.Space(ctx, importedSpaceID)
			if err != nil {
				return errors.Annotatef(err, "retrieving space with ID %s to import subnet %s", importedSpaceID, subnet.ID())
			}
			subnetInfo.SpaceID = importedSpaceID
			subnetInfo.SpaceName = string(space.Name)
			subnetInfo.ProviderSpaceId = space.ProviderId
		}

		i.subnetService.AddSubnet(ctx, subnetInfo)
	}

	return nil
}
