// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The networkconfigapi package implements the network config parts
// common to machiner and provisioner interface

package networkingcommon

import (
	"context"
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.common.networkingcommon")

type NetworkConfigAPI struct {
	st           LinkLayerAndSubnetsState
	getCanModify common.GetAuthFunc
	getModelOp   func(LinkLayerMachine, network.InterfaceInfos) state.ModelOperation
}

// NewNetworkConfigAPI constructs a new common network configuration API
// and returns its reference.
func NewNetworkConfigAPI(ctx context.Context, st *state.State, cloudService common.CloudService, getCanModify common.GetAuthFunc) (*NetworkConfigAPI, error) {
	// TODO (manadart 2020-08-11): This is a second access of the model when
	// being instantiated by the provisioner API.
	// We should ameliorate repeat model access at some point,
	// as it queries state each time.
	mod, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloud, err := cloudService.Get(ctx, mod.CloudName())
	if err != nil {
		return nil, errors.Trace(err)
	}

	getModelOp := func(machine LinkLayerMachine, incoming network.InterfaceInfos) state.ModelOperation {
		// We discover subnets via reported link-layer devices for the
		// manual provider, which allows us to use spaces there.
		return newUpdateMachineLinkLayerOp(machine, incoming, strings.EqualFold(cloud.Type, "manual"), st)
	}

	return &NetworkConfigAPI{
		st:           &linkLayerState{st},
		getCanModify: getCanModify,
		getModelOp:   getModelOp,
	}, nil
}

// SetObservedNetworkConfig reads the network config for the machine
// identified by the input args.
// This config is merged with the new network config supplied in the
// same args and updated if it has changed.
func (api *NetworkConfigAPI) SetObservedNetworkConfig(ctx context.Context, args params.SetMachineNetworkConfig) error {
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

	return errors.Trace(api.st.ApplyOperation(api.getModelOp(m, devs)))
}

// fixUpFanSubnets takes network config and updates any addresses in Fan
// networks with the CIDR of the zone-specific segments. We need to do this
// because they are detected on-machine as being in the /8 overlay subnet,
// which is a superset of the zone segments.
// See core/network/fan.go for more detail on how Fan overlays are divided
// into segments.
func (api *NetworkConfigAPI) fixUpFanSubnets(networkConfig []params.NetworkConfig) ([]params.NetworkConfig, error) {
	subnets, err := api.st.AllSubnetInfos()
	if err != nil {
		return nil, errors.Trace(err)
	}

	fanSubnets := make(map[string]*net.IPNet)
	for _, subnet := range subnets {
		if subnet.FanOverlay() != "" {
			fanSub, err := subnet.ParsedCIDRNetwork()
			if err != nil {
				return nil, errors.Trace(err)
			}
			fanSubnets[subnet.CIDR] = fanSub
		}
	}

	if len(fanSubnets) == 0 {
		return networkConfig, nil
	}

	for i := range networkConfig {
		for j := range networkConfig[i].Addresses {
			ip := net.ParseIP(networkConfig[i].Addresses[j].Value)
			for cidr, fanNet := range fanSubnets {
				if fanNet != nil && fanNet.Contains(ip) {
					networkConfig[i].Addresses[j].CIDR = cidr
					break
				}
			}
		}
	}

	logger.Tracef("final network config after fixing up Fan subnets %+v", networkConfig)
	return networkConfig, nil
}

func (api *NetworkConfigAPI) getMachineForSettingNetworkConfig(machineTag string) (LinkLayerMachine, error) {
	canModify, err := api.getCanModify()
	if err != nil {
		return nil, errors.Trace(err)
	}

	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !canModify(tag) {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}

	m, err := api.getMachine(tag)
	if errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return m, nil
}

func (api *NetworkConfigAPI) getMachine(tag names.MachineTag) (LinkLayerMachine, error) {
	m, err := api.st.Machine(tag.Id())
	return m, errors.Trace(err)
}

// updateMachineLinkLayerOp is a model operation used to merge incoming
// agent-sourced network configuration with existing data for a single
// machine/host/container.
type updateMachineLinkLayerOp struct {
	*MachineLinkLayerOp

	// removalCandidates are devices that exist in state, but since have not
	// been observed by the instance-poller or machine agent.
	// We check that these can be deleted after processing all devices.
	removalCandidates []LinkLayerDevice

	// observedParentDevices are the names of link-layer devices that are
	// parents of children that we are *not* deleting, thus preventing such
	// parents from being deleted.
	observedParentDevices set.Strings

	// discoverSubnets indicates whether we should add subnets from
	// updated link-layer devices to the state subnets collection.
	discoverSubnets bool

	// st is the state indirection required to persist discovered subnets.
	st AddSubnetsState
}

func newUpdateMachineLinkLayerOp(
	machine LinkLayerMachine, incoming network.InterfaceInfos, discoverSubnets bool, st AddSubnetsState,
) *updateMachineLinkLayerOp {
	return &updateMachineLinkLayerOp{
		MachineLinkLayerOp:    NewMachineLinkLayerOp("agent", machine, incoming),
		observedParentDevices: set.NewStrings(),
		discoverSubnets:       discoverSubnets,
		st:                    st,
	}
}

// Build (state.ModelOperation) returns the transaction operations used to
// merge incoming provider link-layer data with that in state.
func (o *updateMachineLinkLayerOp) Build(_ int) ([]txn.Op, error) {
	o.ClearProcessed()

	if err := o.PopulateExistingDevices(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := o.PopulateExistingAddresses(); err != nil {
		return nil, errors.Trace(err)
	}

	o.noteObservedParentDevices()

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

// noteObservedParentDevices records any parent device names from the incoming
// set. This is used later to ensure that we do not remove unobserved devices
// that are parents of NICs in the incoming set.
// This *should* be impossible, but we can be defensive without throwing an
// error - any unobserved devices will always have their addresses removed
// provided that they are under the authority of the machine.
// See processRemovalCandidates.
func (o *updateMachineLinkLayerOp) noteObservedParentDevices() {
	for _, dev := range o.incoming {
		if dev.ParentInterfaceName != "" {
			o.observedParentDevices.Add(dev.ParentInterfaceName)
		}
	}
}

func (o *updateMachineLinkLayerOp) processExistingDevice(dev LinkLayerDevice) ([]txn.Op, error) {
	incomingDev := o.MatchingIncoming(dev)

	if incomingDev == nil {
		ops, err := o.processExistingDeviceNotObserved(dev)
		return ops, errors.Trace(err)
	}

	ops := dev.UpdateOps(networkDeviceToStateArgs(*incomingDev))

	incomingAddrs := o.MatchingIncomingAddrs(dev.Name())

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

	o.MarkDevProcessed(dev.Name())
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
			logger.Debugf("machine %q: removing address %q from device %q", o.machine.Id(), addr.Value(), dev.Name())
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
		if strings.HasPrefix(incomingAddr.CIDRAddress, addrValue) &&
			!o.IsAddrProcessed(dev.Name(), incomingAddr.CIDRAddress) {
			o.MarkAddrProcessed(dev.Name(), incomingAddr.CIDRAddress)

			ops, err := addr.UpdateOps(incomingAddr)
			return ops, errors.Trace(err)
		}
	}

	// Otherwise if we are the authority, delete it.
	if addr.Origin() == network.OriginMachine {
		logger.Infof("machine %q: removing address %q from device %q", o.machine.Id(), addrValue, addr.DeviceName())
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
		if !o.IsAddrProcessed(dev.Name(), addr.CIDRAddress) {
			logger.Infof("machine %q: adding address %q to device %q", o.machine.Id(), addr.CIDRAddress, dev.Name())

			addOps, err := dev.AddAddressOps(addr)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, addOps...)

			o.MarkAddrProcessed(dev.Name(), addr.CIDRAddress)
		}
	}
	return ops, nil
}

// processNewDevices handles incoming devices that
// did not match any we already have in state.
func (o *updateMachineLinkLayerOp) processNewDevices() ([]txn.Op, error) {
	var ops []txn.Op
	for _, dev := range o.Incoming() {
		if o.IsDevProcessed(dev) {
			continue
		}

		addrs := o.MatchingIncomingAddrs(dev.InterfaceName)
		addrValues := make([]string, len(addrs))
		for i, addr := range addrs {
			addrValues[i] = addr.CIDRAddress
		}

		logger.Infof("machine %q: adding new device %q (%s) with addresses %v",
			o.machine.Id(), dev.InterfaceName, dev.MACAddress, addrValues)

		addOps, err := o.machine.AddLinkLayerDeviceOps(
			networkDeviceToStateArgs(dev), addrs...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, addOps...)

		// Since this is a new device, ensure that we have
		// discovered all the subnets it is connected to.
		if o.discoverSubnets {
			subNetOps, err := o.processSubnets(dev.InterfaceName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, subNetOps...)
		}

		o.MarkDevProcessed(dev.InterfaceName)
	}
	return ops, nil
}

// processSubnets takes an incoming NIC hardware address and ensures that the
// subnets of addresses on the device are present in state.
// Loopback subnets are ignored.
func (o *updateMachineLinkLayerOp) processSubnets(name string) ([]txn.Op, error) {
	// Accrue all incoming CIDRs matching the input device.
	cidrSet := set.NewStrings()
	var isVLAN bool
	for _, matching := range o.Incoming().GetByName(name) {
		if matching.IsVLAN() {
			isVLAN = true
		}

		for _, addr := range matching.Addresses {
			if addr.Scope != network.ScopeMachineLocal && addr.CIDR != "" {
				cidrSet.Add(addr.CIDR)
			}
		}
	}
	cidrs := cidrSet.SortedValues()

	if isVLAN {
		logger.Warningf("ignoring VLAN tag for incoming device subnets: %v", cidrs)
	}

	var ops []txn.Op
	for _, cidr := range cidrs {
		addOps, err := o.st.AddSubnetOps(network.SubnetInfo{CIDR: cidr})
		if err != nil {
			if errors.Is(err, errors.AlreadyExists) {
				continue
			}
			return nil, errors.Trace(err)
		}
		ops = append(ops, addOps...)
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
		if o.observedParentDevices.Contains(dev.Name()) {
			logger.Warningf("machine %q: device %q (%s) not removed; it has incoming child devices",
				o.machine.Id(), dev.Name(), dev.MACAddress())
		} else {
			logger.Infof("machine %q: removing device %q (%s)", o.machine.Id(), dev.Name(), dev.MACAddress())
			ops = append(ops, dev.RemoveOps()...)
		}
	}
	return ops
}
