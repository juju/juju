// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
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
	incomingDev := o.MatchingIncoming(dev)

	var ops []txn.Op
	var err error

	// If this device was not observed by the provider,
	// ensure that responsibility for the addresses is relinquished
	// to the machine agent.
	if incomingDev == nil {
		ops, err = o.opsForDeviceOriginRelinquishment(dev)
		return ops, errors.Trace(err)
	}

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

	// Collect normalised addresses for the incoming device.
	// TODO (manadart 2020-07-15): We also need to set shadow addresses.
	// These are sent where appropriate by the provider,
	// but we do not yet process them.
	incomingAddrs := o.MatchingIncomingAddrs(dev.MACAddress())

	for _, addr := range o.DeviceAddresses(dev) {
		addrOps, err := o.processExistingDeviceAddress(dev, addr, incomingAddrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, addrOps...)
	}

	// TODO (manadart 2020-07-15): Process (log) new addresses on the device.

	o.MarkDevProcessed(dev)
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
	dev networkingcommon.LinkLayerDevice,
	addr networkingcommon.LinkLayerAddress,
	incomingAddrs []state.LinkLayerDeviceAddress,
) ([]txn.Op, error) {
	addrValue := addr.Value()
	hwAddr := dev.MACAddress()

	// If one of the incoming addresses matches the existing one,
	// return ops for setting the incoming provider IDs.
	for _, incomingAddr := range incomingAddrs {
		if strings.HasPrefix(incomingAddr.CIDRAddress, addrValue) {
			if o.IsAddrProcessed(hwAddr, addrValue) {
				continue
			}

			ops, err := addr.SetProviderIDOps(incomingAddr.ProviderID)
			if err != nil {
				return nil, errors.Trace(err)
			}

			o.MarkAddrProcessed(hwAddr, addrValue)

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
func (o *mergeMachineLinkLayerOp) processNewDevices() {
	for _, dev := range o.Incoming() {
		if !o.IsDevProcessed(dev) {
			logger.Debugf(
				"ignoring unrecognised device %q (%s) with addresses %v",
				dev.InterfaceName, dev.MACAddress, dev.Addresses,
			)
		}
	}
}
