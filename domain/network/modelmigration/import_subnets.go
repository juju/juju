// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"strings"

	"github.com/juju/description/v12"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corenetwork "github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterImportSubnets registers the import operations for spaces and subnets with the given coordinator.
func RegisterImportSubnets(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importSubnetsOperation{
		logger: logger,
	})
}

// SubnetsImportService provides a subset of the network domain service
// methods needed for spaces and subnets import.
type SubnetsImportService interface {
	// AddSpace creates and returns a new space.
	AddSpace(ctx context.Context, args domainnetwork.AddSpaceArgs) (corenetwork.SpaceUUID, error)
	// AddSubnet creates and returns a new subnet.
	AddSubnet(ctx context.Context, args corenetwork.SubnetInfo) (corenetwork.Id, error)
	// GetModelCloudType returns the type of the cloud that is in use by this model.
	GetModelCloudType(context.Context) (string, error)
}

type importSubnetsOperation struct {
	modelmigration.BaseOperation

	importService SubnetsImportService
	logger        logger.Logger
}

// Name returns the name of this operation.
func (i *importSubnetsOperation) Name() string {
	return "import spaces and subnets"
}

// Setup implements Operation.
func (i *importSubnetsOperation) Setup(scope modelmigration.Scope) error {
	st := state.NewState(scope.ModelDB(), i.logger)
	i.importService = service.NewService(
		st,
		i.logger,
	)
	return nil
}

// Execute the import of the spaces and subnets contained in the model.
func (i *importSubnetsOperation) Execute(ctx context.Context, model description.Model) error {
	if model.Type() == description.CAAS {
		// Kubernetes environments do not support spaces or subnets, though
		// we do need to provide a fallback subnets. This is for RI purposes
		// only.
		//
		// Importing only FallbackSubnetInfo here is correct for 3.x -> 4.x
		// CAAS migrations: the target controller re-discovers the real pod
		// subnets via ReloadSpaces() -> the Kubernetes provider's Subnets()
		// discoverer chain on model creation, so any exported subnet data can
		// be safely discarded.
		return i.populateFallbackSubnets(ctx)
	}

	spaceIDsMap, err := i.importSpaces(ctx, model.Spaces())
	if err != nil {
		return errors.Capture(err)
	}
	if err := i.importIAASSubnets(ctx, model.Subnets(), spaceIDsMap); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (i *importSubnetsOperation) importSpaces(ctx context.Context, modelSpaces []description.Space) (map[string]corenetwork.SpaceUUID, error) {
	// CIDRs are not passed here: multiple subnets with the same CIDR may
	// belong to different spaces (distinct provider networks), so the
	// CIDR-based association used by the user-facing API is ambiguous during
	// import. Each subnet is instead linked to its space explicitly by
	// importIAASSubnets via SpaceID.
	spaceIDsMap := make(map[string]corenetwork.SpaceUUID)
	for _, space := range modelSpaces {
		// The default space should not have been exported, but be defensive.
		if space.Name() == corenetwork.AlphaSpaceName.String() {
			continue
		}
		spaceID, err := i.importService.AddSpace(ctx, domainnetwork.AddSpaceArgs{
			UUID:       corenetwork.SpaceUUID(space.UUID()),
			Name:       corenetwork.SpaceName(space.Name()),
			ProviderID: corenetwork.Id(space.ProviderID()),
		})
		if err != nil {
			return nil, errors.Errorf("creating space %s: %w", space.Name(), err)
		}
		// Update the space IDs mapping, which we need for subnets
		// import. We do this for the pre-4.0 migrations, where
		// spaces have their ID set but not their UUID. If their UUID
		// is set then we use it to keep a consistent mapping.
		if space.Id() != "" {
			spaceIDsMap[space.Id()] = spaceID
		} else {
			spaceIDsMap[spaceID.String()] = spaceID
		}
	}
	return spaceIDsMap, nil
}

func (i *importSubnetsOperation) importIAASSubnets(
	ctx context.Context,
	modelSubnets []description.Subnet,
	spaceIDsMap map[string]corenetwork.SpaceUUID,
) error {
	cloudType, err := i.importService.GetModelCloudType(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	for _, subnet := range modelSubnets {
		// Fix subnet data from 3.6 during import, net- is superfluous.
		var providerID, providerNetworkID string
		if cloudType == "lxd" {
			providerNetworkID = strings.TrimPrefix(subnet.ProviderNetworkId(), "net-")
		} else {
			providerID = subnet.ProviderId()
			providerNetworkID = subnet.ProviderNetworkId()
		}

		subnetInfo := corenetwork.SubnetInfo{
			ID:                corenetwork.Id(subnet.UUID()),
			CIDR:              subnet.CIDR(),
			ProviderId:        corenetwork.Id(providerID),
			VLANTag:           subnet.VLANTag(),
			AvailabilityZones: subnet.AvailabilityZones(),
			ProviderNetworkId: corenetwork.Id(providerNetworkID),
		}

		importedSpaceID, ok := spaceIDsMap[subnet.SpaceID()]
		if ok {
			subnetInfo.SpaceID = importedSpaceID
		}

		_, err := i.importService.AddSubnet(ctx, subnetInfo)
		if err != nil {
			return errors.Errorf("creating subnet %s: %w", subnet.CIDR(), err)
		}
	}
	return nil
}

func (i *importSubnetsOperation) populateFallbackSubnets(ctx context.Context) error {
	for _, subnet := range corenetwork.FallbackSubnetInfo {
		_, err := i.importService.AddSubnet(ctx, subnet)
		if err != nil {
			return errors.Errorf("creating fallback subnet %s: %w", subnet.CIDR, err)
		}
	}
	return nil
}
