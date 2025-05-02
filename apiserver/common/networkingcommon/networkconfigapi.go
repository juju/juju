// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The networkconfigapi package implements the network config parts
// common to machiner and provisioner interface

package networkingcommon

import (
	"context"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	internalerrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.common.networkingcommon")

// ModelInfoService is the interface that is used to ask questions about the
// current model.
type ModelInfoService interface {
	// GetModelCloudType returns the type of the cloud that is in use by this model.
	GetModelCloudType(context.Context) (string, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// AddSubnet creates and returns a new subnet.
	AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error)
}

type NetworkConfigAPI struct {
	st             LinkLayerAndSubnetsState
	networkService NetworkService
	getCanModify   common.GetAuthFunc
	getModelOp     func(LinkLayerMachine, network.InterfaceInfos) state.ModelOperation
}

// NewNetworkConfigAPI constructs a new common network configuration API
// and returns its reference.
func NewNetworkConfigAPI(
	ctx context.Context,
	st *state.State,
	modelInfoService ModelInfoService,
	networkService NetworkService,
	getCanModify common.GetAuthFunc,
) (*NetworkConfigAPI, error) {
	cloudType, err := modelInfoService.GetModelCloudType(ctx)
	if err != nil {
		return nil, internalerrors.Errorf("getting model cloud type: %w", err)
	}

	getModelOp := func(machine LinkLayerMachine, incoming network.InterfaceInfos) state.ModelOperation {
		// We discover subnets via reported link-layer devices for the
		// manual provider, which allows us to use spaces there.
		return newUpdateMachineLinkLayerOp(machine, networkService, incoming, strings.EqualFold(cloudType, "manual"))
	}

	return &NetworkConfigAPI{
		st:             &linkLayerState{State: st},
		networkService: networkService,
		getCanModify:   getCanModify,
		getModelOp:     getModelOp,
	}, nil
}

// SetObservedNetworkConfig reads the network config for the machine
// identified by the input args.
// This config is merged with the new network config supplied in the
// same args and updated if it has changed.
func (api *NetworkConfigAPI) SetObservedNetworkConfig(ctx context.Context, args params.SetMachineNetworkConfig) error {
	m, err := api.getMachineForSettingNetworkConfig(ctx, args.Tag)
	if err != nil {
		return errors.Trace(err)
	}

	observedConfig := args.Config
	logger.Tracef(ctx, "observed network config of machine %q: %+v", m.Id(), observedConfig)
	if len(observedConfig) == 0 {
		logger.Infof(ctx, "not updating machine %q network config: no observed network config found", m.Id())
		return nil
	}

	devs := params.InterfaceInfoFromNetworkConfig(observedConfig)
	if err = devs.Validate(); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(api.st.ApplyOperation(api.getModelOp(m, devs)))
}

func (api *NetworkConfigAPI) getMachineForSettingNetworkConfig(ctx context.Context, machineTag string) (LinkLayerMachine, error) {
	canModify, err := api.getCanModify(ctx)
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

	networkService NetworkService

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
}

func newUpdateMachineLinkLayerOp(
	machine LinkLayerMachine, networkService NetworkService, incoming network.InterfaceInfos, discoverSubnets bool,
) *updateMachineLinkLayerOp {
	return &updateMachineLinkLayerOp{
		MachineLinkLayerOp:    NewMachineLinkLayerOp("agent", machine, incoming),
		networkService:        networkService,
		observedParentDevices: set.NewStrings(),
		discoverSubnets:       discoverSubnets,
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

	ctx := context.TODO()

	var ops []txn.Op
	for _, existingDev := range o.ExistingDevices() {
		devOps, err := o.processExistingDevice(ctx, existingDev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, devOps...)
	}

	addOps, err := o.processNewDevices(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, addOps...)

	ops = append(ops, o.processRemovalCandidates(ctx)...)

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

func (o *updateMachineLinkLayerOp) processExistingDevice(ctx context.Context, dev LinkLayerDevice) ([]txn.Op, error) {
	incomingDev := o.MatchingIncoming(dev)

	if incomingDev == nil {
		ops, err := o.processExistingDeviceNotObserved(ctx, dev)
		return ops, errors.Trace(err)
	}

	ops := dev.UpdateOps(networkDeviceToStateArgs(*incomingDev))

	incomingAddrs := o.MatchingIncomingAddrs(dev.Name())

	for _, addr := range o.DeviceAddresses(dev) {
		existingAddrOps, err := o.processExistingDeviceAddress(ctx, dev, addr, incomingAddrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, existingAddrOps...)
	}

	newAddrOps, err := o.processExistingDeviceNewAddresses(ctx, dev, incomingAddrs)
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
func (o *updateMachineLinkLayerOp) processExistingDeviceNotObserved(ctx context.Context, dev LinkLayerDevice) ([]txn.Op, error) {
	addrs := o.DeviceAddresses(dev)

	var ops []txn.Op
	var removing int
	for _, addr := range addrs {
		// If the machine is the authority for this address,
		// we can delete it; otherwise leave it alone.
		if addr.Origin() == network.OriginMachine {
			logger.Debugf(ctx, "machine %q: removing address %q from device %q", o.machine.Id(), addr.Value(), dev.Name())
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
	ctx context.Context,
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
		logger.Infof(ctx, "machine %q: removing address %q from device %q", o.machine.Id(), addrValue, addr.DeviceName())
		return addr.RemoveOps(), nil
	}

	return nil, nil
}

// processExistingDeviceNewAddresses interrogates the list of incoming
// addresses and adds any that were not processed as already existing.
// If there are new address to add then also the subnets are processed
// to make sure they are updated on the state as well.
func (o *updateMachineLinkLayerOp) processExistingDeviceNewAddresses(
	ctx context.Context,
	dev LinkLayerDevice, incomingAddrs []state.LinkLayerDeviceAddress,
) ([]txn.Op, error) {
	var ops []txn.Op
	for _, addr := range incomingAddrs {
		if !o.IsAddrProcessed(dev.Name(), addr.CIDRAddress) {
			logger.Infof(ctx, "machine %q: adding address %q to device %q", o.machine.Id(), addr.CIDRAddress, dev.Name())

			addOps, err := dev.AddAddressOps(addr)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, addOps...)

			// Since this is a new address, ensure that we have
			// discovered all the subnets for the device.
			if o.discoverSubnets {
				if err := o.processSubnets(ctx, dev.Name()); err != nil {
					return nil, errors.Trace(err)
				}
			}

			o.MarkAddrProcessed(dev.Name(), addr.CIDRAddress)
		}
	}
	return ops, nil
}

// processNewDevices handles incoming devices that
// did not match any we already have in state.
func (o *updateMachineLinkLayerOp) processNewDevices(ctx context.Context) ([]txn.Op, error) {
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

		logger.Infof(ctx, "machine %q: adding new device %q (%s) with addresses %v",
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
			if err := o.processSubnets(ctx, dev.InterfaceName); err != nil {
				return nil, errors.Trace(err)
			}
		}

		o.MarkDevProcessed(dev.InterfaceName)
	}
	return ops, nil
}

// processSubnets takes an incoming NIC hardware address and ensures that the
// subnets of addresses on the device are present in state.
// Loopback subnets are ignored.
func (o *updateMachineLinkLayerOp) processSubnets(ctx context.Context, name string) error {
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
		logger.Warningf(ctx, "ignoring VLAN tag for incoming device subnets: %v", cidrs)
	}

	for _, cidr := range cidrs {
		_, err := o.networkService.AddSubnet(ctx, network.SubnetInfo{CIDR: cidr})
		if err != nil {
			if errors.Is(err, errors.AlreadyExists) {
				continue
			}
			return errors.Trace(err)
		}
	}
	return nil
}

// processRemovalCandidates returns transaction operations for
// removing unobserved devices that it is safe to delete.
// A device is considered safe to delete if it has no children,
// or if all of its children are also candidates for deletion.
// Any device considered here will already have ops generated
// for removing its addresses.
func (o *updateMachineLinkLayerOp) processRemovalCandidates(ctx context.Context) []txn.Op {
	var ops []txn.Op
	for _, dev := range o.removalCandidates {
		if o.observedParentDevices.Contains(dev.Name()) {
			logger.Warningf(ctx, "machine %q: device %q (%s) not removed; it has incoming child devices",
				o.machine.Id(), dev.Name(), dev.MACAddress())
		} else {
			logger.Infof(ctx, "machine %q: removing device %q (%s)", o.machine.Id(), dev.Name(), dev.MACAddress())
			ops = append(ops, dev.RemoveOps()...)
		}
	}
	return ops
}
