// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The networkconfigapi package implements the network config parts
// common to machiner and provisioner interface

package networkingcommon

import (
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.common.networkingcommon")

type NetworkConfigAPI struct {
	st           *state.State
	getCanModify common.GetAuthFunc
}

func NewNetworkConfigAPI(st *state.State, getCanModify common.GetAuthFunc) *NetworkConfigAPI {
	return &NetworkConfigAPI{
		st:           st,
		getCanModify: getCanModify,
	}
}

// SetObservedNetworkConfig reads the network config for the machine
// identified by the input args.
// This config is merged with the new network config supplied in the
// same args and updated if it has changed.
func (api *NetworkConfigAPI) SetObservedNetworkConfig(args params.SetMachineNetworkConfig) error {
	m, err := api.getMachineForSettingNetworkConfig(args.Tag)
	if err != nil {
		return errors.Trace(err)
	}

	observedConfig := args.Config
	logger.Tracef("observed network config of machine %q: %+v", m.Id(), observedConfig)
	if len(observedConfig) == 0 {
		logger.Infof("not updating machine %q network config: no observed network config found", m.Id())
		return nil
	}

	mergedConfig, err := api.fixUpFanSubnets(observedConfig)
	if err != nil {
		return errors.Trace(err)
	}

	devs := params.InterfaceInfoFromNetworkConfig(mergedConfig)
	if err = devs.Validate(); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(api.st.ApplyOperation(newUpdateMachineLinkLayerOp(&machineShim{m}, devs)))
}

// fixUpFanSubnets takes network config and updates Fan subnets
// with proper CIDR, providerId and providerSubnetId.
// See network/fan.go for more detail on how Fan overlays
// are divided into segments.
func (api *NetworkConfigAPI) fixUpFanSubnets(networkConfig []params.NetworkConfig) ([]params.NetworkConfig, error) {
	subnets, err := api.st.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fanSubnets []*state.Subnet
	var fanCIDRs []*net.IPNet
	for _, subnet := range subnets {
		if subnet.FanOverlay() != "" {
			fanSubnets = append(fanSubnets, subnet)
			_, aNet, err := net.ParseCIDR(subnet.CIDR())
			if err != nil {
				return nil, errors.Trace(err)
			}
			fanCIDRs = append(fanCIDRs, aNet)
		}
	}
	for i := range networkConfig {
		localIP := net.ParseIP(networkConfig[i].Address)
		for j, fanSubnet := range fanSubnets {
			if len(fanCIDRs) >= j && fanCIDRs[j].Contains(localIP) {
				networkConfig[i].CIDR = fanSubnet.CIDR()
				networkConfig[i].ProviderId = string(fanSubnet.ProviderId())
				networkConfig[i].ProviderSubnetId = string(fanSubnet.ProviderNetworkId())
				break
			}
		}
	}
	logger.Tracef("Final network config after fixing up FAN subnets %+v", networkConfig)
	return networkConfig, nil
}

func (api *NetworkConfigAPI) getMachineForSettingNetworkConfig(machineTag string) (*state.Machine, error) {
	canModify, err := api.getCanModify()
	if err != nil {
		return nil, errors.Trace(err)
	}

	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !canModify(tag) {
		return nil, errors.Trace(common.ErrPerm)
	}

	m, err := api.getMachine(tag)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(common.ErrPerm)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return m, nil
}

func (api *NetworkConfigAPI) getMachine(tag names.MachineTag) (*state.Machine, error) {
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

// mergeMachineLinkLayerOp is a model operation used to merge incoming
// provider-sourced network configuration with existing data for a single
// machine/host/container.
type updateMachineLinkLayerOp struct {
	*MachineLinkLayerOp

	// removalCandidates are devices that exist in state, but since have not
	// been observed by the instance-poller or machine agent.
	// We check that these can be deleted after processing all devices.
	removalCandidates []LinkLayerDevice

	// observedParentIDs are the IDs of link-layer devices that are parents of
	// children that we are *not* deleting, thus preventing such parents from
	// being deleted.
	observedParentIDs set.Strings
}

func newUpdateMachineLinkLayerOp(
	machine LinkLayerMachine, incoming network.InterfaceInfos,
) *updateMachineLinkLayerOp {
	return &updateMachineLinkLayerOp{
		MachineLinkLayerOp: NewMachineLinkLayerOp(machine, incoming),
	}
}

// Build (state.ModelOperation) returns the transaction operations used to
// merge incoming provider link-layer data with that in state.
func (o *updateMachineLinkLayerOp) Build(_ int) ([]txn.Op, error) {
	if err := o.PopulateExistingDevices(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := o.PopulateExistingAddresses(); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO: Consider walking the hierarchy here (maybe) and removing incoming
	// child devices that don't have a parent either incoming also, or residing
	// in state. This would prevent us having to check whether removal
	// candidates have an incoming first-time child - however unlikely this is.

	// InterfaceInfo validation is now called before this model Op is
	// validated, but the logic does not yet replicate validation of parents as
	// is currently done is state.

	var ops []txn.Op
	for _, existingDev := range o.ExistingDevices() {
		devOps, err := o.processExistingDevice(existingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, devOps...)
	}

	addOps, err := o.processNewDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, addOps...)

	ops = append(ops, o.processRemovalCandidates()...)

	return ops, nil
}

func (o *updateMachineLinkLayerOp) processExistingDevice(dev LinkLayerDevice) ([]txn.Op, error) {
	incomingDev := o.MatchingIncoming(dev)

	if incomingDev == nil {
		ops, err := o.processExistingDeviceNotObserved(dev)
		return ops, errors.Trace(err)
	}

	// At this point we know that we are dealing with a device that is both in
	// state and observed once again on the machine.
	// We want to ensure that if it has a parent device,
	// then that device can not be deleted.
	parentID := dev.ParentID()
	if parentID != "" {
		o.observedParentIDs.Add(parentID)
	}

	ops := dev.UpdateOps(networkDeviceToStateArgs(*incomingDev))

	incomingAddrs := o.MatchingIncomingAddrs(dev.MACAddress())

	for _, addr := range o.DeviceAddresses(dev) {
		existingAddrOps, err := o.processExistingDeviceAddress(dev, addr, incomingAddrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, existingAddrOps...)
	}

	newAddrOps, err := o.processExistingDeviceNewAddresses(dev, incomingAddrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append(ops, newAddrOps...), nil
}

// processExistingDeviceNotObserved returns transaction operations for
// processing a device we have in state, but that the machine agent no
// longer observes locally.
// The device itself is not marked for deletion now, but for later processing
// to ensure it is not a parent of other observed devices.
func (o *updateMachineLinkLayerOp) processExistingDeviceNotObserved(dev LinkLayerDevice) ([]txn.Op, error) {
	addrs := o.DeviceAddresses(dev)

	var ops []txn.Op
	var removing int
	for _, addr := range addrs {
		// If the machine is the authority for this address,
		// we can delete it; otherwise leave it alone.
		if addr.Origin() == network.OriginMachine {
			logger.Debugf("removing address %q from device %q", addr.Value(), dev.Name())
			ops = append(ops, addr.RemoveOps()...)
			removing++
		}
	}

	// If the device is having all of its addresses removed and is not under
	// the authority of the provider, add it as a candidate for removal.
	// If the device has been relinquished by the provider, the instance-poller
	// will have removed the provider ID - see the instance-poller API facade
	// logic.
	if removing == len(addrs) && dev.ProviderID() == "" {
		o.removalCandidates = append(o.removalCandidates, dev)
	}

	return ops, nil
}

func (o *updateMachineLinkLayerOp) processExistingDeviceAddress(
	dev LinkLayerDevice, addr LinkLayerAddress, incomingAddrs []state.LinkLayerDeviceAddress,
) ([]txn.Op, error) {
	addrValue := addr.Value()

	// If one of the incoming addresses matches the existing one,
	// update it.
	for _, incomingAddr := range incomingAddrs {
		if strings.HasPrefix(incomingAddr.CIDRAddress, addrValue) {
			o.MarkAddrProcessed(dev.MACAddress(), incomingAddr.CIDRAddress)
			ops, err := addr.UpdateOps(incomingAddr)
			return ops, errors.Trace(err)
		}
	}

	// Otherwise if we are the authority, delete it.
	if addr.Origin() == network.OriginMachine {
		logger.Debugf("removing address %q from device %q", addrValue, addr.DeviceName())
		return addr.RemoveOps(), nil
	}

	return nil, nil
}

// processExistingDeviceNewAddresses interrogates the list of incoming
// addresses and adds any that were not processed as already existing.
func (o *updateMachineLinkLayerOp) processExistingDeviceNewAddresses(
	dev LinkLayerDevice, incomingAddrs []state.LinkLayerDeviceAddress,
) ([]txn.Op, error) {
	var ops []txn.Op
	for _, addr := range incomingAddrs {
		if !o.IsAddrProcessed(dev.MACAddress(), addr.CIDRAddress) {
			addOps, err := dev.AddAddressOps(addr)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, addOps...)
		}
	}
	return ops, nil
}

// processNewDevices handles incoming devices that
// did not match any we already have in state.
func (o *updateMachineLinkLayerOp) processNewDevices() ([]txn.Op, error) {
	var ops []txn.Op
	for _, dev := range o.Incoming() {
		if !o.IsDevProcessed(dev) {
			logger.Debugf("adding new device %q (%s) with addresses %v",
				dev.InterfaceName, dev.MACAddress, dev.Addresses)

			addOps, err := o.machine.AddLinkLayerDeviceOps(
				networkDeviceToStateArgs(dev), o.MatchingIncomingAddrs(dev.MACAddress)...)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, addOps...)
		}
	}
	return ops, nil
}

// processRemovalCandidates returns transaction operations for
// removing unobserved devices that it is safe to delete.
// A device is considered safe to delete if it has no children,
// or if all of its children are also candidates for deletion.
// Any device considered here will already have ops generated
// for removing its addresses.
func (o *updateMachineLinkLayerOp) processRemovalCandidates() []txn.Op {
	var ops []txn.Op
	for _, dev := range o.removalCandidates {
		if !o.observedParentIDs.Contains(dev.ID()) {
			ops = append(ops, dev.RemoveOps()...)
		}
	}
	return ops
}
