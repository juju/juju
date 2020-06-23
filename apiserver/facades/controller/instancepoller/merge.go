// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

// mergeMachineLinkLayerOp is a model operation used to merge incoming
// provider-sourced network configuration with existing data for a single
// machine/host/container.
type mergeMachineLinkLayerOp struct {
	// machine is the machine for which this operation
	// sets link-layer device information.
	machine StateMachine

	// incoming is the network interface information incoming from the provider.
	incoming network.InterfaceInfos

	// processed is the set of hardware IDs that we have
	// processed from the incoming interfaces.
	processed set.Strings

	existingDevs  []StateLinkLayerDevice
	existingAddrs []StateLinkLayerDeviceAddress
}

func newMergeMachineLinkLayerOp(machine StateMachine, incoming network.InterfaceInfos) *mergeMachineLinkLayerOp {
	return &mergeMachineLinkLayerOp{
		machine:   machine,
		incoming:  incoming,
		processed: set.NewStrings(),
	}
}

func (o *mergeMachineLinkLayerOp) Build(_ int) ([]txn.Op, error) {
	var err error

	if o.existingDevs, err = o.machine.AllLinkLayerDevices(); err != nil {
		return nil, errors.Trace(err)
	}

	// If the machine agent has not yet populated any link-layer devices,
	// then we do nothing here. We have already set addresses directly on the
	// machine document, so the incoming provider-sourced addresses are usable.
	// For now we ensure that the instance poller only adds device information
	// that the machine agent is unaware of.
	if len(o.existingDevs) == 0 {
		return nil, jujutxn.ErrNoOperations
	}

	if o.existingAddrs, err = o.machine.AllAddresses(); err != nil {
		return nil, errors.Trace(err)
	}

	var ops []txn.Op
	for _, existingDev := range o.existingDevs {
		devOps, err := o.processExistingDevice(existingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, devOps...)
	}

	o.processNewDevices()

	if len(ops) > 0 {
		return append([]txn.Op{o.machine.AssertAliveOp()}, ops...), nil
	}
	return ops, nil
}

func (o *mergeMachineLinkLayerOp) processExistingDevice(dev StateLinkLayerDevice) ([]txn.Op, error) {
	// Match the incoming device by hardware address in order to
	// identify addresses by device name.
	// Not all providers (such as AWS) have names for NIC devices.
	incomingDev := o.incoming.GetByHardwareAddress(dev.MACAddress())

	// If this device was not observed by the provider,
	// ensure that responsibility for the addresses is relinquished
	// to the machine agent.
	if incomingDev == nil {
		return o.opsForDeviceOriginRelinquishment(dev), nil
	}

	// Warn the user that we will not change a provider ID that is already set.
	// The ops generated for this scenario will be nil.
	// TODO (manadart 2020-06-09): If this is seen in the wild, we should look
	// into removing/reassigning provider IDs for devices.
	providerID := dev.ProviderID()
	if providerID != "" && providerID != incomingDev.ProviderId {
		logger.Warningf(
			"not changing provider ID for device %s from %q to %q",
			dev.MACAddress(), providerID, incomingDev.ProviderId,
		)
	}

	ops, err := dev.SetProviderIDOps(incomingDev.ProviderId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, addr := range o.deviceAddrs(dev) {
		addrOps, err := o.processExistingDeviceAddress(addr, incomingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, addrOps...)
	}

	o.processed.Add(dev.MACAddress())
	return ops, nil
}

// opsForDeviceOriginRelinquishment returns transaction operations required to
// ensure that the origin for all addresses on the device is relinquished to
// the machine.
func (o *mergeMachineLinkLayerOp) opsForDeviceOriginRelinquishment(dev StateLinkLayerDevice) []txn.Op {
	var ops []txn.Op
	for _, addr := range o.deviceAddrs(dev) {
		ops = append(ops, addr.SetOriginOps(network.OriginMachine)...)
	}
	return ops
}

func (o *mergeMachineLinkLayerOp) deviceAddrs(dev StateLinkLayerDevice) []StateLinkLayerDeviceAddress {
	var addrs []StateLinkLayerDeviceAddress
	for _, addr := range o.existingAddrs {
		if addr.DeviceName() == dev.Name() {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

func (o *mergeMachineLinkLayerOp) processExistingDeviceAddress(
	addr StateLinkLayerDeviceAddress, incomingDev *network.InterfaceInfo,
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
	for _, dev := range o.incoming {
		if !o.processed.Contains(dev.MACAddress) {
			logger.Debugf(
				"ignoring unrecognised device %q (%s) with addresses %v",
				dev.InterfaceName, dev.MACAddress, dev.Addresses,
			)
		}
	}
}

// Done (state.ModelOperation) returns the result of running the operation.
func (o *mergeMachineLinkLayerOp) Done(err error) error {
	return err
}
