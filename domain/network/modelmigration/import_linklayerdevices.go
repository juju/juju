// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
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

// RegisterLinkLayerDevicesImport registers the import operations with the given coordinator.
func RegisterLinkLayerDevicesImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// LinkLayerDevicesMigrationService defines methods needed to import
// link layer devices as part of model migration.
type LinkLayerDevicesMigrationService interface {
	// GetModelCloudType returns the type of the cloud that is in use by this model.
	GetModelCloudType(context.Context) (string, error)
	// ImportLinkLayerDevices imports the given link layer device data into
	// the model.
	ImportLinkLayerDevices(ctx context.Context, data []internal.ImportLinkLayerDevice) error
}

type importOperation struct {
	modelmigration.BaseOperation

	migrationService LinkLayerDevicesMigrationService
	logger           logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import link layer devices"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	st := state.NewState(scope.ModelDB(), i.logger)
	i.migrationService = service.NewMigrationService(st, i.logger)
	return nil
}

// Execute the import of the link layer devices contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	if err := i.importLinkLayerDevices(ctx, model.LinkLayerDevices(), model.IPAddresses()); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (i *importOperation) importLinkLayerDevices(
	ctx context.Context, modelLLD []description.LinkLayerDevice, modelAddresses []description.IPAddress,
) error {
	if len(modelLLD) == 0 {
		return nil
	}
	lldData, err := i.transformLinkLayerDevices(ctx, modelLLD, modelAddresses)
	if err != nil {
		return errors.Capture(err)
	}
	if err := i.migrationService.ImportLinkLayerDevices(ctx, lldData); err != nil {
		return errors.Errorf("importing link layer devices: %w", err)
	}

	return nil
}

func (i *importOperation) transformLinkLayerDevices(
	ctx context.Context,
	modelLLD []description.LinkLayerDevice,
	addresses []description.IPAddress,
) ([]internal.ImportLinkLayerDevice, error) {
	data := make([]internal.ImportLinkLayerDevice, len(modelLLD))

	cloudType, err := i.migrationService.GetModelCloudType(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Temporary map to index llds by machine and name, to distribute addresses
	type key struct {
		machineID string
		name      string
	}
	llds := make(map[key]*internal.ImportLinkLayerDevice, len(modelLLD))

	for i, lld := range modelLLD {
		lldUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("creating UUID for link layer device %q", lld.Name())
		}
		deviceType, err := encodeDeviceType(lld.Type())
		if err != nil {
			return nil, errors.Errorf("encoding device type for link layer device %q: %w", lld.Name(), err)
		}
		data[i] = internal.ImportLinkLayerDevice{
			UUID:             lldUUID.String(),
			Name:             lld.Name(),
			MachineID:        lld.MachineID(),
			MTU:              nilZeroPtr(int64(lld.MTU())),
			MACAddress:       nilZeroPtr(lld.MACAddress()),
			ProviderID:       nilZeroPtr(lld.ProviderID()),
			Type:             deviceType,
			VirtualPortType:  corenetwork.VirtualPortType(lld.VirtualPortType()),
			IsAutoStart:      lld.IsAutoStart(),
			IsEnabled:        lld.IsUp(),
			ParentDeviceName: lld.ParentName(),
		}
		llds[key{
			machineID: lld.MachineID(),
			name:      lld.Name(),
		}] = &data[i]
	}

	for _, address := range addresses {
		scope := corenetwork.NewMachineAddress(address.Value()).Scope
		if scope == corenetwork.ScopeFanLocal {
			i.logger.Warningf(ctx, "ignoring legacy fan address %q on device %q for machine %q",
				address.Value(), address.DeviceName(), address.MachineID())
			continue
		}

		addressUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("creating UUID for address %q of device %q", address.Value(),
				address.DeviceName())
		}
		device, ok := llds[key{
			machineID: address.MachineID(),
			name:      address.DeviceName(),
		}]
		if !ok {
			return nil, errors.Errorf("address %q for machine %q on device %q not found", address.Value(), address.MachineID(),
				address.DeviceName())
		}
		if !corenetwork.IsValidAddressConfigType(address.ConfigMethod()) {
			return nil, errors.Errorf("invalid address config type %q for address %q of device %q on machine %q",
				address.ConfigMethod(), address.Value(), address.DeviceName(), address.MachineID())
		}

		addrType := corenetwork.DeriveAddressType(address.Value())

		// The ProviderSubnetID is not required. The contrived values for LXD
		// serve no purpose, remove them from the model data from 3.6.
		var providerSubnetID string
		if cloudType != "lxd" {
			providerSubnetID = address.ProviderSubnetID()
		}

		device.Addresses = append(device.Addresses, internal.ImportIPAddress{
			UUID:             addressUUID.String(),
			Type:             addrType,
			Scope:            scope,
			AddressValue:     i.ensureAddressWithCIDR(address, addrType),
			SubnetCIDR:       address.SubnetCIDR(),
			ConfigType:       corenetwork.AddressConfigType(address.ConfigMethod()),
			IsSecondary:      address.IsSecondary(),
			IsShadow:         address.IsShadow(),
			Origin:           corenetwork.Origin(address.Origin()),
			ProviderID:       nilZeroPtr(address.ProviderID()),
			ProviderSubnetID: nilZeroPtr(providerSubnetID),
		})
	}

	return data, nil
}

// ensureAddressWithCIDR ensures that the provided IP address includes
// its CIDR notation and returns it as a string.
func (i *importOperation) ensureAddressWithCIDR(address description.IPAddress, addrType corenetwork.AddressType) string {
	if strings.Contains(address.Value(), "/") {
		return address.Value()
	}
	subnet := strings.Split(address.SubnetCIDR(), "/")
	if len(subnet) == 2 {
		return fmt.Sprintf("%s/%s", address.Value(), subnet[1])
	}
	switch addrType {
	case corenetwork.IPv4Address:
		return fmt.Sprintf("%s/32", address.Value())
	case corenetwork.IPv6Address:
		return fmt.Sprintf("%s/128", address.Value())
	}
	return address.Value()
}

func encodeDeviceType(t string) (network.DeviceType, error) {
	switch t {
	case corenetwork.UnknownDevice.String():
		return network.DeviceTypeUnknown, nil
	case corenetwork.LoopbackDevice.String():
		return network.DeviceTypeLoopback, nil
	case corenetwork.EthernetDevice.String():
		return network.DeviceTypeEthernet, nil
	case corenetwork.VLAN8021QDevice.String():
		return network.DeviceType8021q, nil
	case corenetwork.BondDevice.String():
		return network.DeviceTypeBond, nil
	case corenetwork.BridgeDevice.String():
		return network.DeviceTypeBridge, nil
	case corenetwork.VXLANDevice.String():
		return network.DeviceTypeVXLAN, nil
	// This device type was never actually used in 3.6,
	// but is added here for completeness.
	case corenetwork.VirtualEthernetDevice.String():
		return network.DeviceTypeVeth, nil
	default:
		return -1, errors.Errorf("unknown link layer device type: %q", t)
	}
}

func nilZeroPtr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
