// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/network"
)

// mergeMachineLinkLayerOp is a model operation used to merge incoming
// provider-sourced network configuration with existing data for a single
// machine/host/container.
type mergeMachineLinkLayerOp struct {
	*networkingcommon.MachineLinkLayerOp
}

func newMergeMachineLinkLayerOp(
	machine networkingcommon.LinkLayerMachine, incoming network.InterfaceInfos,
) *mergeMachineLinkLayerOp {
	return &mergeMachineLinkLayerOp{
		networkingcommon.NewMachineLinkLayerOp(machine, incoming),
	}
}

// Build (state.ModelOperation) returns the transaction operations used to
// merge incoming provider link-layer data with that in state.
func (o *mergeMachineLinkLayerOp) Build(_ int) ([]txn.Op, error) {
	if err := o.PopulateExistingDevices(); err != nil {
		return nil, errors.Trace(err)
	}

	// If the machine agent has not yet populated any link-layer devices,
	// then we do nothing here. We have already set addresses directly on the
	// machine document, so the incoming provider-sourced addresses are usable.
	// For now we ensure that the instance poller only adds device information
	// that the machine agent is unaware of.
	if len(o.ExistingDevices()) == 0 {
		return nil, jujutxn.ErrNoOperations
	}

	if err := o.PopulateExistingAddresses(); err != nil {
		return nil, errors.Trace(err)
	}

	var ops []txn.Op
	for _, existingDev := range o.ExistingDevices() {
		devOps, err := o.processExistingDevice(existingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, devOps...)
	}

	o.processNewDevices()

	if len(ops) > 0 {
		return append([]txn.Op{o.AssertAliveOp()}, ops...), nil
	}
	return ops, nil
}

func (o *mergeMachineLinkLayerOp) processExistingDevice(dev networkingcommon.LinkLayerDevice) ([]txn.Op, error) {
	// Match the incoming device by hardware address in order to
	// identify addresses by device name.
	// Not all providers (such as AWS) have names for NIC devices.
	incomingDevs := o.Incoming().GetByHardwareAddress(dev.MACAddress())

	var ops []txn.Op
	var err error

	// If this device was not observed by the provider,
	// ensure that responsibility for the addresses is relinquished
	// to the machine agent.
	if len(incomingDevs) == 0 {
		ops, err = o.opsForDeviceOriginRelinquishment(dev)
		return ops, errors.Trace(err)
	}

	// The MachineLinkLayerOp constructor normalises the incoming interfaces.
	// This means there is at most one interface with any given MAC address.
	incomingDev := incomingDevs[0]

	// Warn the user that we will not change a provider ID that is already set.
	// TODO (manadart 2020-06-09): If this is seen in the wild, we should look
	// into removing/reassigning provider IDs for devices.
	providerID := dev.ProviderID()
	if providerID != "" && providerID != incomingDev.ProviderId {
		logger.Warningf(
			"not changing provider ID for device %s from %q to %q",
			dev.MACAddress(), providerID, incomingDev.ProviderId,
		)
	} else {
		ops, err = dev.SetProviderIDOps(incomingDev.ProviderId)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	for _, addr := range o.DeviceAddresses(dev) {
		addrOps, err := o.processExistingDeviceAddress(addr, incomingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, addrOps...)
	}

	o.MarkProcessed(dev)
	return ops, nil
}

// opsForDeviceOriginRelinquishment returns transaction operations required to
// ensure that a device has no provider ID and that the origin for all
// addresses on the device is relinquished to the machine.
func (o *mergeMachineLinkLayerOp) opsForDeviceOriginRelinquishment(
	dev networkingcommon.LinkLayerDevice,
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
	addr networkingcommon.LinkLayerAddress, incomingDev network.InterfaceInfo,
) ([]txn.Op, error) {
	addrValue := addr.Value()

	// TODO (manadart 2020-06-09): This is where we see a cardinality mismatch.
	// The InterfaceInfo type has one provider address ID, but a collection of
	// addresses and shadow addresses.
	// This should change so that addresses are collections of types that
	// include a provider ID, instead of assuming that the provider ID applies
	// to the primary address (index 0).
	// We should also be adding shadow addresses to the link-layer device here.
	// For now, if the provider does not recognise the address,
	// give responsibility for it to the machine.
	if !set.NewStrings(incomingDev.Addresses.ToIPAddresses()...).Contains(addrValue) {
		return addr.SetOriginOps(network.OriginMachine), nil
	}

	// If the address is the incoming primary address, assign the provider IDs.
	// Otherwise simply indicate that the provider is aware of this address.
	if addrValue == incomingDev.PrimaryAddress().Value {
		ops, err := addr.SetProviderIDOps(incomingDev.ProviderAddressId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return append(ops, addr.SetProviderNetIDsOps(
			incomingDev.ProviderNetworkId, incomingDev.ProviderSubnetId)...), nil
	}
	return addr.SetOriginOps(network.OriginProvider), nil
}

// processNewDevices handles incoming devices that did not match any we already
// have in state.
// TODO (manadart 2020-06-12): It should be unlikely for the provider to be
// aware of devices that the machiner knows nothing about.
// At the time of writing we preserve existing behaviour and do not add them.
// Log for now and consider adding such devices in the future.
func (o *mergeMachineLinkLayerOp) processNewDevices() {
	for _, dev := range o.Incoming() {
		if !o.IsProcessed(dev) {
			logger.Debugf(
				"ignoring unrecognised device %q (%s) with addresses %v",
				dev.InterfaceName, dev.MACAddress, dev.Addresses,
			)
		}
	}
}
