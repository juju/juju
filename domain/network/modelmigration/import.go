// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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

// ImportService provides a subset of the network domain service
// methods needed for spaces and subnets import.
type ImportService interface {
	AddSpace(ctx context.Context, space corenetwork.SpaceInfo) (corenetwork.SpaceUUID, error)
	Space(ctx context.Context, uuid corenetwork.SpaceUUID) (*corenetwork.SpaceInfo, error)
	AddSubnet(ctx context.Context, args corenetwork.SubnetInfo) (corenetwork.Id, error)
}

// MigrationService defines methods needed to import and export
// link layer devices as part of model migration.
type MigrationService interface {
	// ImportLinkLayerDevices imports the given link layer device data into
	// the model.
	ImportLinkLayerDevices(ctx context.Context, data []internal.ImportLinkLayerDevice) error

	// DeleteImportedLinkLayerDevices removes all link layer device data
	// imported via the ImportLinkLayerDevices method.
	DeleteImportedLinkLayerDevices(ctx context.Context) error
}

type importOperation struct {
	modelmigration.BaseOperation

	importService    ImportService
	migrationService MigrationService
	logger           logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import networks"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	st := state.NewState(scope.ModelDB(), i.logger)
	i.importService = service.NewService(
		st,
		i.logger,
	)
	i.migrationService = service.NewMigrationService(st, i.logger)
	return nil
}

// Execute the import of the spaces, subnets and link layer devices
// contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	spaceIDsMap, err := i.importSpaces(ctx, model.Spaces())
	if err != nil {
		return errors.Capture(err)
	}
	if err := i.importSubnets(ctx, model.Subnets(), spaceIDsMap); err != nil {
		return errors.Capture(err)
	}
	if err := i.importLinkLayerDevices(ctx, model.LinkLayerDevices()); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// Rollback the resource import operation by deleting all data imported
// within the network domain.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	// TODO: 21-May-2025 hml
	// Implement rollback for spaces and subnets.

	if len(model.LinkLayerDevices()) == 0 {
		return nil
	}
	err := i.migrationService.DeleteImportedLinkLayerDevices(ctx)
	if err != nil {
		return errors.Errorf("link layer device import rollback failed: %w", err)
	}
	return nil
}

func (i *importOperation) importSpaces(ctx context.Context, modelSpaces []description.Space) (map[string]corenetwork.SpaceUUID, error) {
	spaceIDsMap := make(map[string]corenetwork.SpaceUUID)
	for _, space := range modelSpaces {
		// The default space should not have been exported, but be defensive.
		if space.Name() == corenetwork.AlphaSpaceName.String() {
			continue
		}
		spaceInfo := corenetwork.SpaceInfo{
			ID:         corenetwork.SpaceUUID(space.UUID()),
			Name:       corenetwork.SpaceName(space.Name()),
			ProviderId: corenetwork.Id(space.ProviderID()),
		}
		spaceID, err := i.importService.AddSpace(ctx, spaceInfo)
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

func (i *importOperation) importSubnets(
	ctx context.Context,
	modelSubnets []description.Subnet,
	spaceIDsMap map[string]corenetwork.SpaceUUID,
) error {

	for _, subnet := range modelSubnets {
		subnetInfo := corenetwork.SubnetInfo{
			ID:                corenetwork.Id(subnet.UUID()),
			CIDR:              subnet.CIDR(),
			ProviderId:        corenetwork.Id(subnet.ProviderId()),
			VLANTag:           subnet.VLANTag(),
			AvailabilityZones: subnet.AvailabilityZones(),
			ProviderNetworkId: corenetwork.Id(subnet.ProviderNetworkId()),
		}

		importedSpaceID, ok := spaceIDsMap[subnet.SpaceID()]
		if ok {
			space, err := i.importService.Space(ctx, importedSpaceID)
			if err != nil {
				return errors.Errorf("retrieving space with ID %s to import subnet %s: %w", importedSpaceID, subnet.ID(), err)
			}
			subnetInfo.SpaceID = importedSpaceID
			subnetInfo.SpaceName = space.Name
			subnetInfo.ProviderSpaceId = space.ProviderId
		}

		_, err := i.importService.AddSubnet(ctx, subnetInfo)
		if err != nil {
			return errors.Errorf("creating subnet %s: %w", subnet.CIDR(), err)
		}
	}
	return nil
}

func (i *importOperation) importLinkLayerDevices(ctx context.Context, modelLLD []description.LinkLayerDevice) error {
	if len(modelLLD) == 0 {
		return nil
	}
	data, err := i.transformLinkLayerDevices(modelLLD)
	if err != nil {
		return err
	}
	if err := i.migrationService.ImportLinkLayerDevices(ctx, data); err != nil {
		return errors.Errorf("importing link layer devices: %w", err)
	}
	return nil
}

func (i *importOperation) transformLinkLayerDevices(modelLLD []description.LinkLayerDevice) ([]internal.ImportLinkLayerDevice, error) {
	data := make([]internal.ImportLinkLayerDevice, len(modelLLD))

	for i, lld := range modelLLD {
		var (
			mac        *string
			mtu        *int64
			providerID *string
		)
		if lld.ProviderID() != "" {
			providerID = ptr(lld.ProviderID())
		}
		if lld.MTU() > 0 {
			mtu = ptr(int64(lld.MTU()))
		}
		if lld.MACAddress() != "" {
			mac = ptr(lld.MACAddress())
		}
		lldUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("creating UUID for link layer device %q", lld.Name())
		}
		data[i] = internal.ImportLinkLayerDevice{
			UUID:             lldUUID.String(),
			Name:             lld.Name(),
			MachineID:        lld.MachineID(),
			MTU:              mtu,
			MACAddress:       mac,
			ProviderID:       providerID,
			Type:             corenetwork.LinkLayerDeviceType(lld.Type()),
			VirtualPortType:  corenetwork.VirtualPortType(lld.VirtualPortType()),
			IsAutoStart:      lld.IsAutoStart(),
			IsEnabled:        lld.IsUp(),
			ParentDeviceName: lld.ParentName(),
		}
	}

	return data, nil
}

// ptr returns a reference to a copied value of type T.
func ptr[T any](i T) *T {
	return &i
}
