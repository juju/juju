// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	commonnetwork "github.com/juju/juju/apiserver/common/network"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// mergeMachineLinkLayerOp is a model operation used to merge incoming
// provider-sourced network configuration with existing data for a single
// machine/host/container.
type mergeMachineLinkLayerOp struct {
	*commonnetwork.MachineLinkLayerOp

	// namelessHWAddrs stores the hardware addresses of
	// incoming devices that have no accompanying name.
	namelessHWAddrs set.Strings

	// providerIDs is used for observing ID usage for incoming devices.
	// We consult it to ensure that the same provider ID is not being
	// used for multiple NICs.
	providerIDs map[network.Id]string

	logger corelogger.Logger
}

func newMergeMachineLinkLayerOp(
	machine commonnetwork.LinkLayerMachine, incoming network.InterfaceInfos,
	logger corelogger.Logger,
) *mergeMachineLinkLayerOp {
	return &mergeMachineLinkLayerOp{
		MachineLinkLayerOp: commonnetwork.NewMachineLinkLayerOp("provider", machine, incoming),
		namelessHWAddrs:    set.NewStrings(),
		logger:             logger,
	}
}

// Build (state.ModelOperation) returns the transaction operations used to
// merge incoming provider link-layer data with that in state.
func (o *mergeMachineLinkLayerOp) Build(attempt int) ([]txn.Op, error) {
	o.ClearProcessed()
	o.providerIDs = make(map[network.Id]string)

	if err := o.PopulateExistingDevices(); err != nil {
		return nil, errors.Trace(err)
	}

	// If the machine agent has not yet populated any link-layer devices,
	// then we do nothing here. We have already set addresses directly on the
	// machine document, so the incoming provider-sourced addresses are usable.
	// For now we ensure that the instance-poller only adds device information
	// that the machine agent is unaware of.
	if len(o.ExistingDevices()) == 0 {
		return nil, jujutxn.ErrNoOperations
	}

	if attempt == 0 {
		o.normaliseIncoming()
	}

	if err := o.PopulateExistingAddresses(); err != nil {
		return nil, errors.Trace(err)
	}

	var ops []txn.Op
	for _, existingDev := range o.ExistingDevices() {
		devOps, err := o.processExistingDevice(context.TODO(), existingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, devOps...)
	}

	o.processNewDevices(context.TODO())

	if len(ops) > 0 {
		return append([]txn.Op{o.AssertAliveOp()}, ops...), nil
	}
	return ops, nil
}

// normaliseIncoming is intended accommodate providers such as EC2
// that know device hardware addresses, but not device names.
// We populate names on the incoming data based on
// matching existing devices by hardware address.
// If we locate multiple existing devices with the hardware address,
// such as will be the case for bridged NICs, fallback through the
// following options.
//   - If there is a device that already has a provider ID, use that name.
//   - If the devices are of different types, choose an ethernet device over
//     a bridge (as observed for MAAS).
func (o *mergeMachineLinkLayerOp) normaliseIncoming() {
	incoming := o.Incoming()

	// If the incoming devices have names, no action is required
	// (assuming all or none here per current known provider implementations
	// of `NetworkInterfaces`)
	if len(incoming) > 0 && incoming[0].InterfaceName != "" {
		return
	}

	// First get the best device per hardware address.
	devByHWAddr := make(map[string]commonnetwork.LinkLayerDevice)
	for _, dev := range o.ExistingDevices() {
		hwAddr := dev.MACAddress()

		// If this is the first one we've seen, select it.
		current, ok := devByHWAddr[hwAddr]
		if !ok {
			devByHWAddr[hwAddr] = dev
			continue
		}

		// If we have a matching device that already has a provider ID,
		// I.e. it was previously matched to the hardware address,
		// make sure the same one is resolved thereafter.
		if current.ProviderID() != "" {
			continue
		}

		// Otherwise choose a physical NIC over other device types.
		if dev.Type() == network.EthernetDevice {
			devByHWAddr[hwAddr] = dev
		}
	}

	// Now set the names.
	for i, dev := range incoming {
		if existing, ok := devByHWAddr[dev.MACAddress]; ok && dev.InterfaceName == "" {
			o.namelessHWAddrs.Add(dev.MACAddress)
			incoming[i].InterfaceName = existing.Name()
		}
	}
}

func (o *mergeMachineLinkLayerOp) processExistingDevice(ctx context.Context, dev commonnetwork.LinkLayerDevice) ([]txn.Op, error) {
	incomingDev := o.MatchingIncoming(dev)

	var ops []txn.Op
	var err error

	// If this device was not observed by the provider *and* it is identified
	// by both name and hardware address, ensure that responsibility for the
	// addresses is relinquished to the machine agent.
	if incomingDev == nil {
		// If this device matches an incoming hardware address that we gave a
		// surrogate name to, do not relinquish it.
		if o.namelessHWAddrs.Contains(dev.MACAddress()) {
			return nil, nil
		}

		ops, err = o.opsForDeviceOriginRelinquishment(dev)
		return ops, errors.Trace(err)
	}

	// Log a warning if we are changing a provider ID that is already set.
	providerID := dev.ProviderID()
	if providerID != "" && providerID != incomingDev.ProviderId {
		o.logger.Warningf(ctx,
			"changing provider ID for device %q from %q to %q",
			dev.Name(), providerID, incomingDev.ProviderId,
		)
	}

	// Check that the incoming data is not using a provider ID for more
	// than one device. This is not verified by transaction assertions.
	if incomingDev.ProviderId != "" {
		if usedBy, ok := o.providerIDs[incomingDev.ProviderId]; ok {
			return nil, errors.Errorf(
				"unable to set provider ID %q for multiple devices: %q, %q",
				incomingDev.ProviderId, usedBy, dev.Name(),
			)
		}

		o.providerIDs[incomingDev.ProviderId] = dev.Name()
	}

	ops, err = dev.SetProviderIDOps(incomingDev.ProviderId)
	if err != nil {
		if !state.IsProviderIDNotUniqueError(err) {
			return nil, errors.Trace(err)
		}

		// If this provider ID is already assigned, log a warning and continue.
		// If the ID is moving from one device to another for whatever reason,
		// It will be eventually consistent. E.g. removed from the old device
		// on this pass and added to the new device on the next.
		o.logger.Warningf(ctx,
			"not setting provider ID for device %q to %q; it is assigned to another device",
			dev.Name(), incomingDev.ProviderId,
		)
	}

	// Collect normalised addresses for the incoming device.
	// TODO (manadart 2020-07-15): We also need to set shadow addresses.
	// These are sent where appropriate by the provider,
	// but we do not yet process them.
	incomingAddrs := o.MatchingIncomingAddrs(dev.Name())

	for _, addr := range o.DeviceAddresses(dev) {
		addrOps, err := o.processExistingDeviceAddress(dev, addr, incomingAddrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, addrOps...)
	}

	// TODO (manadart 2020-07-15): Process (log) new addresses on the device.

	o.MarkDevProcessed(dev.Name())
	return ops, nil
}

// opsForDeviceOriginRelinquishment returns transaction operations required to
// ensure that a device has no provider ID and that the origin for all
// addresses on the device is relinquished to the machine.
func (o *mergeMachineLinkLayerOp) opsForDeviceOriginRelinquishment(
	dev commonnetwork.LinkLayerDevice,
) ([]txn.Op, error) {
	ops, err := dev.SetProviderIDOps("")
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, addr := range o.DeviceAddresses(dev) {
		ops = append(ops, addr.SetOriginOps(network.OriginMachine)...)
	}

	return ops, nil
}

func (o *mergeMachineLinkLayerOp) processExistingDeviceAddress(
	dev commonnetwork.LinkLayerDevice,
	addr commonnetwork.LinkLayerAddress,
	incomingAddrs []state.LinkLayerDeviceAddress,
) ([]txn.Op, error) {
	addrValue := addr.Value()
	name := dev.Name()

	// If one of the incoming addresses matches the existing one,
	// return ops for setting the incoming provider IDs.
	for _, incomingAddr := range incomingAddrs {
		if strings.HasPrefix(incomingAddr.CIDRAddress, addrValue) {
			if o.IsAddrProcessed(name, addrValue) {
				continue
			}

			ops, err := addr.SetProviderIDOps(incomingAddr.ProviderID)
			if err != nil {
				return nil, errors.Trace(err)
			}

			o.MarkAddrProcessed(name, addrValue)

			return append(ops, addr.SetProviderNetIDsOps(
				incomingAddr.ProviderNetworkID, incomingAddr.ProviderSubnetID)...), nil
		}
	}

	// Otherwise relinquish responsibility for this device to the machiner.
	return addr.SetOriginOps(network.OriginMachine), nil
}

// processNewDevices handles incoming devices that did not match any we already
// have in state.
// TODO (manadart 2020-06-12): It should be unlikely for the provider to be
// aware of devices that the machiner knows nothing about.
// At the time of writing we preserve existing behaviour and do not add them.
// Log for now and consider adding such devices in the future.
func (o *mergeMachineLinkLayerOp) processNewDevices(ctx context.Context) {
	for _, dev := range o.Incoming() {
		if !o.IsDevProcessed(dev) {
			o.logger.Debugf(ctx,
				"ignoring unrecognised device %q (%s) with addresses %v",
				dev.InterfaceName, dev.MACAddress, dev.Addresses,
			)
		}
	}
}
