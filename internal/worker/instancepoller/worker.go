// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

// ShortPoll and LongPoll hold the polling intervals for the instance
// updater. When a machine has no address or is not started, it will be
// polled at ShortPoll intervals until it does, exponentially backing off
// with an exponent of ShortPollBackoff until a maximum of ShortPollCap is
// reached.
//
// When a machine has an address and is started LongPoll will be used to
// check that the instance address or status has not changed.
var (
	ShortPoll        = 3 * time.Second
	ShortPollBackoff = 2.0
	ShortPollCap     = 1 * time.Minute
	LongPoll         = 15 * time.Minute
)

// Environ specifies the provider-specific methods needed by the instance
// poller.
type Environ interface {
	Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error)
	NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error)
}

// MachineService defines the interface for interacting with the machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// WatchModelMachineLifeAndStartTimes returns a string watcher that emits
	// machine names for changes to machine life or agent start times.
	WatchModelMachineLifeAndStartTimes(context.Context) (watcher.StringsWatcher, error)

	// IsMachineManuallyProvisioned returns whether the machine is a manual machine.
	IsMachineManuallyProvisioned(context.Context, machine.Name) (bool, error)

	// GetMachineLife returns the GetMachineLife status of the specified machine.
	GetMachineLife(context.Context, machine.Name) (corelife.Value, error)

	// GetInstanceIDByMachineName returns the cloud specific instance id for this machine.
	GetInstanceIDByMachineName(context.Context, machine.Name) (instance.Id, error)
}

// StatusService defines the interface for interacting with the status
// service.
type StatusService interface {
	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	GetInstanceStatus(context.Context, machine.Name) (status.StatusInfo, error)

	// SetInstanceStatus sets the cloud specific instance status for this machine.
	SetInstanceStatus(context.Context, machine.Name, status.StatusInfo) error

	// GetMachineStatus returns the status of the specified machine.
	GetMachineStatus(context.Context, machine.Name) (status.StatusInfo, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// SetProviderNetConfig updates the network configuration for a machine using its unique identifier and new interface data.
	SetProviderNetConfig(context.Context, machine.UUID, []domainnetwork.NetInterface) error
}

// Config encapsulates the configuration options for instantiating a new
// instance poller worker.
type Config struct {
	Clock          clock.Clock
	MachineService MachineService
	StatusService  StatusService
	NetworkService NetworkService
	Environ        Environ
	Logger         logger.Logger
}

// Validate checks whether the worker configuration settings are valid.
func (config Config) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil clock.Clock")
	}
	if config.MachineService == nil {
		return errors.NotValidf("nil MachineService")
	}
	if config.StatusService == nil {
		return errors.NotValidf("nil StatusService")
	}
	if config.NetworkService == nil {
		return errors.NotValidf("nil NetworkService")
	}
	if config.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

type pollGroupType uint8

const (
	shortPollGroup pollGroupType = iota
	longPollGroup
	invalidPollGroup
)

type pollGroupEntry struct {
	machineName machine.Name
	instanceID  instance.Id

	shortPollInterval time.Duration
	shortPollAt       time.Time
}

func (e *pollGroupEntry) resetShortPollInterval(clk clock.Clock) {
	e.shortPollInterval = ShortPoll
	e.shortPollAt = clk.Now().Add(e.shortPollInterval)
}

func (e *pollGroupEntry) bumpShortPollInterval(clk clock.Clock) {
	e.shortPollInterval = time.Duration(float64(e.shortPollInterval) * ShortPollBackoff)
	if e.shortPollInterval > ShortPollCap {
		e.shortPollInterval = ShortPollCap
	}
	e.shortPollAt = clk.Now().Add(e.shortPollInterval)
}

type updaterWorker struct {
	config   Config
	catacomb catacomb.Catacomb

	pollGroup              [2]map[machine.Name]*pollGroupEntry
	instanceIDToGroupEntry map[instance.Id]*pollGroupEntry

	// Hook function which tests can use to be notified when the worker
	// has processed a full loop iteration.
	loopCompletedHook func()
}

// NewWorker returns a worker that keeps track of
// the machines in the state and polls their instance
// addresses and status periodically to keep them up to date.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	u := &updaterWorker{
		config: config,
		pollGroup: [2]map[machine.Name]*pollGroupEntry{
			make(map[machine.Name]*pollGroupEntry),
			make(map[machine.Name]*pollGroupEntry),
		},
		instanceIDToGroupEntry: make(map[instance.Id]*pollGroupEntry),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "instance-poller",
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// Kill is part of the worker.Worker interface.
func (u *updaterWorker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *updaterWorker) Wait() error {
	return u.catacomb.Wait()
}

func (u *updaterWorker) loop() error {
	ctx, cancel := u.scopedContext()
	defer cancel()

	watch, err := u.config.MachineService.WatchModelMachineLifeAndStartTimes(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.catacomb.Add(watch); err != nil {
		return errors.Trace(err)
	}

	shortPollTimer := u.config.Clock.NewTimer(ShortPoll)
	longPollTimer := u.config.Clock.NewTimer(LongPoll)
	defer func() {
		_ = shortPollTimer.Stop()
		_ = longPollTimer.Stop()
	}()

	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case names, ok := <-watch.Changes():
			if !ok {
				return errors.New("machines watcher closed")
			}

			for _, name := range names {
				machineName := machine.Name(name)
				if err := machineName.Validate(); err != nil {
					return errors.Annotate(err, "validating emitted machine name")
				}
				if err := u.queueMachineForPolling(ctx, machineName); err != nil {
					return err
				}
			}
		case <-shortPollTimer.Chan():
			if err := u.pollGroupMembers(ctx, shortPollGroup); err != nil {
				return err
			}
			shortPollTimer.Reset(ShortPoll)
		case <-longPollTimer.Chan():
			if err := u.pollGroupMembers(ctx, longPollGroup); err != nil {
				return err
			}
			longPollTimer.Reset(LongPoll)
		}

		if u.loopCompletedHook != nil {
			u.loopCompletedHook()
		}
	}
}

func (u *updaterWorker) queueMachineForPolling(ctx context.Context, machineName machine.Name) error {
	// If we are already polling this machine, check whether it is still alive
	// and remove it from its poll group if it is now dead.
	if entry, groupType := u.lookupPolledMachine(machineName); entry != nil {
		life, err := u.config.MachineService.GetMachineLife(ctx, machineName)
		if life == corelife.Dead || errors.Is(err, machineerrors.MachineNotFound) {
			u.config.Logger.Debugf(ctx, "removing dead machine %q (instance ID %q)", entry.machineName, entry.instanceID)
			delete(u.pollGroup[groupType], machineName)
			delete(u.instanceIDToGroupEntry, entry.instanceID)
			return nil
		} else if err != nil {
			return errors.Trace(err)
		}

		// Something has changed with the machine state. Reset short
		// poll interval for the machine and move it to the short poll
		// group (if not already there) so we immediately poll its
		// status at the next interval.
		u.moveEntryToPollGroup(shortPollGroup, entry)
		if groupType == longPollGroup {
			u.config.Logger.Debugf(ctx, "moving machine %q (instance ID %q) to short poll group", entry.machineName, entry.instanceID)
		}
		return nil
	}

	// We don't poll manual machines, instead we're setting the status to 'running'
	// as we don't have any better information from the provider, see lp:1678981
	isManual, err := u.config.MachineService.IsMachineManuallyProvisioned(ctx, machineName)
	if err != nil {
		return errors.Trace(err)
	}

	if isManual {
		machineStatus, err := u.config.StatusService.GetInstanceStatus(ctx, machineName)
		if err != nil {
			return errors.Trace(err)
		}
		if machineStatus.Status != status.Running {
			now := u.config.Clock.Now()
			if err = u.config.StatusService.SetInstanceStatus(ctx, machineName, status.StatusInfo{
				Status:  status.Running,
				Message: "Manually provisioned machine",
				Since:   &now,
			}); err != nil {
				u.config.Logger.Errorf(ctx, "cannot set instance status on %q: %v", machineName, err)
				return err
			}
		}
		return nil
	}

	// Add all new machines to the short poll group and arrange for them to
	// be polled as soon as possible.
	u.appendToShortPollGroup(machineName)
	return nil
}

func (u *updaterWorker) appendToShortPollGroup(machineName machine.Name) {
	entry := &pollGroupEntry{
		machineName: machineName,
	}
	entry.resetShortPollInterval(u.config.Clock)
	u.pollGroup[shortPollGroup][machineName] = entry
}

func (u *updaterWorker) moveEntryToPollGroup(toGroup pollGroupType, entry *pollGroupEntry) {
	// Ensure that the entry is not present in the other group
	delete(u.pollGroup[1-toGroup], entry.machineName)
	u.pollGroup[toGroup][entry.machineName] = entry

	// If moving to the short poll group reset the poll interval
	if toGroup == shortPollGroup {
		entry.resetShortPollInterval(u.config.Clock)
	}
}

func (u *updaterWorker) lookupPolledMachine(machineName machine.Name) (*pollGroupEntry, pollGroupType) {
	for groupType, members := range u.pollGroup {
		if found := members[machineName]; found != nil {
			return found, pollGroupType(groupType)
		}
	}
	return nil, invalidPollGroup
}

func (u *updaterWorker) pollGroupMembers(ctx context.Context, groupType pollGroupType) error {
	// Build a list of instance IDs to pass as a query to the provider.
	var instList []instance.Id
	now := u.config.Clock.Now()
	for _, entry := range u.pollGroup[groupType] {
		if groupType == shortPollGroup && now.Before(entry.shortPollAt) {
			continue // we shouldn't poll this entry yet
		}

		if err := u.resolveInstanceID(ctx, entry); err != nil {
			if errors.Is(err, machineerrors.NotProvisioned) {
				// machine not provisioned yet; bump its poll
				// interval and re-try later (or as soon as we
				// get a change for the machine)
				entry.bumpShortPollInterval(u.config.Clock)
				continue
			}
			return errors.Trace(err)
		}

		instList = append(instList, entry.instanceID)
	}

	if len(instList) == 0 {
		return nil
	}

	infoList, err := u.config.Environ.Instances(ctx, instList)
	if err != nil {
		switch errors.Cause(err) {
		case environs.ErrPartialInstances:
			// Proceed and process the ones we've found.
		case environs.ErrNoInstances:
			// If there were no instances recognised by the provider, we do not
			// retrieve the network configuration, and will therefore have
			// nothing to update.
			// This can happen when machines do have instance IDs, but the
			// instances themselves are shut down, such as we have seen for
			// dying models.
			// If we're in the short poll group bump all the poll intervals for
			// entries with an instance ID. Any without an instance ID will
			// already have had their intervals bumped above.
			if groupType == shortPollGroup {
				for _, id := range instList {
					u.instanceIDToGroupEntry[id].bumpShortPollInterval(u.config.Clock)
				}
			}

			return nil
		default:
			return errors.Trace(err)
		}
	}

	netList, err := u.config.Environ.NetworkInterfaces(ctx, instList)
	if err != nil && !isPartialOrNoInstancesError(err) {
		// NOTE(achilleasa): 2022-01-24: all existing providers (with the
		// exception of "manual" which we don't care about in this context)
		// implement the NetworkInterfaces method.
		//
		// This error is meant as a hint to folks working on new providers
		// in the future to ensure that they implement this method.
		if errors.Is(err, errors.NotSupported) {
			return errors.Errorf("BUG: substrate does not implement required NetworkInterfaces method")
		}

		return errors.Annotate(err, "enumerating network interface list for instances")
	}

	for idx, info := range infoList {
		var nics network.InterfaceInfos
		if netList != nil {
			nics = netList[idx]
		}

		if err := u.processOneInstance(ctx, instList[idx], info, nics, groupType); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (u *updaterWorker) processOneInstance(
	ctx context.Context,
	id instance.Id, info instances.Instance,
	nics network.InterfaceInfos, groupType pollGroupType,
) error {
	entry := u.instanceIDToGroupEntry[id]

	// If we received ErrPartialInstances, and this ID is one of those not found,
	// and we're in the short poll group, back off the poll interval.
	// This will ensure that instances that have gone away do not cause excessive
	// provider call volumes.
	if info == nil {
		u.config.Logger.Warningf(ctx, "unable to retrieve instance information for instance: %q", id)

		if groupType == shortPollGroup {
			entry.bumpShortPollInterval(u.config.Clock)
		}
		return nil
	}

	providerStatus, err := u.processProviderInfo(ctx, entry, info, nics)
	if err != nil {
		return errors.Trace(err)
	}

	machineStatus, err := u.config.StatusService.GetMachineStatus(ctx, entry.machineName)
	if err != nil {
		return errors.Trace(err)
	}

	u.maybeSwitchPollGroup(ctx, groupType, entry, providerStatus, machineStatus.Status, len(nics))
	return nil
}

func (u *updaterWorker) resolveInstanceID(ctx context.Context, entry *pollGroupEntry) error {
	if entry.instanceID != "" {
		return nil // already resolved
	}

	instID, err := u.config.MachineService.GetInstanceIDByMachineName(ctx, entry.machineName)
	if err != nil {
		return errors.Annotatef(err, "retrieving instance ID for machine %q", entry.machineName)
	}

	entry.instanceID = instID
	u.instanceIDToGroupEntry[instID] = entry
	return nil
}

// processProviderInfo updates an entry's machine status and set of provider
// addresses based on the information collected from the provider. It returns
// the *instance* status and the number of provider addresses currently
// known for the machine.
func (u *updaterWorker) processProviderInfo(
	ctx context.Context,
	entry *pollGroupEntry, info instances.Instance,
	providerInterfaces network.InterfaceInfos,
) (status.Status, error) {
	curStatus, err := u.config.StatusService.GetInstanceStatus(ctx, entry.machineName)
	if err != nil {
		// This should never occur since the machine is provisioned. If
		// it does occur, report an unknown status to move the machine to
		// the short poll group.
		u.config.Logger.Warningf(ctx, "cannot get current instance status for machine %v (instance ID %q): %v",
			entry.machineName, entry.instanceID, err)

		return status.Unknown, nil
	}

	// Check for status changes
	providerStatus := info.Status(ctx)
	curInstStatus := instance.Status{
		Status:  curStatus.Status,
		Message: curStatus.Message,
	}

	if providerStatus != curInstStatus {
		u.config.Logger.Infof(ctx, "machine %q (instance ID %q) instance status changed from %q to %q",
			entry.machineName, entry.instanceID, curInstStatus, providerStatus)

		if err = u.config.StatusService.SetInstanceStatus(ctx, entry.machineName, status.StatusInfo{
			Status:  providerStatus.Status,
			Message: providerStatus.Message,
		}); err != nil {
			u.config.Logger.Errorf(ctx, "cannot set instance status on %q: %v", entry.machineName, err)
			return status.Unknown, errors.Trace(err)
		}

		// If the instance is now running, we should reset the poll
		// interval to make sure we can capture machine status changes
		// as early as possible.
		if providerStatus.Status == status.Running {
			entry.resetShortPollInterval(u.config.Clock)
		}
	}

	// We don't care about dead machines; they will be cleaned up when we
	// process the following machine watcher events.
	life, err := u.config.MachineService.GetMachineLife(ctx, entry.machineName)
	if life == corelife.Dead || errors.Is(err, machineerrors.MachineNotFound) {
		return status.Unknown, nil
	} else if err != nil {
		return status.Unknown, err
	}

	// Check whether the provider addresses for this machine need to be
	// updated.
	err = u.syncProviderAddresses(ctx, entry, providerInterfaces)
	if err != nil {
		return status.Unknown, err
	}

	return providerStatus.Status, nil
}

// syncProviderAddresses updates the provider addresses for this entry's machine
// using either the provider sourced interface list.
//
// The call returns the count of provider addresses for the machine.
func (u *updaterWorker) syncProviderAddresses(
	ctx context.Context,
	entry *pollGroupEntry, providerIfaceList network.InterfaceInfos,
) error {
	uuid, err := u.config.MachineService.GetMachineUUID(ctx, entry.machineName)
	if err != nil {
		return errors.Annotatef(err, "retrieving machine UUID for machine %q", entry.machineName)
	}

	devices := transform.Slice(providerIfaceList, newNetInterface)
	err = u.config.NetworkService.SetProviderNetConfig(ctx, uuid, devices)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (u *updaterWorker) maybeSwitchPollGroup(
	ctx context.Context,
	curGroup pollGroupType,
	entry *pollGroupEntry,
	curProviderStatus,
	curMachineStatus status.Status,
	providerNicCount int,
) {
	if curProviderStatus == status.Allocating || curProviderStatus == status.Pending {
		// Keep the machine in the short poll group until it settles.
		entry.bumpShortPollInterval(u.config.Clock)
		return
	}

	// If the machine is currently in the long poll group and it has an
	// unknown status or suddenly has no network addresses, move it back to
	// the short poll group.
	if curGroup == longPollGroup && (curProviderStatus == status.Unknown || providerNicCount == 0) {
		u.moveEntryToPollGroup(shortPollGroup, entry)
		u.config.Logger.Debugf(ctx, "moving machine %q (instance ID %q) back to short poll group", entry.machineName, entry.instanceID)
		return
	}

	// The machine has started and we have at least one address; move to
	// the long poll group
	if providerNicCount > 0 && curMachineStatus == status.Started {
		u.moveEntryToPollGroup(longPollGroup, entry)
		if curGroup != longPollGroup {
			u.config.Logger.Debugf(ctx, "moving machine %q (instance ID %q) to long poll group", entry.machineName, entry.instanceID)
		}
		return
	}

	// If we are in the short poll group apply exponential backoff to the
	// poll frequency allow time for the machine to boot up.
	if curGroup == shortPollGroup {
		entry.bumpShortPollInterval(u.config.Clock)
	}
}

func (u *updaterWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(u.catacomb.Context(context.Background()))
}

func isPartialOrNoInstancesError(err error) bool {
	cause := errors.Cause(err)
	return cause == environs.ErrPartialInstances || cause == environs.ErrNoInstances
}

func newNetInterface(device network.InterfaceInfo) domainnetwork.NetInterface {
	return domainnetwork.NetInterface{
		Name:             device.InterfaceName,
		MTU:              ptr(int64(device.MTU)),
		MACAddress:       ptr(device.MACAddress),
		ProviderID:       ptr(device.ProviderId),
		Type:             network.LinkLayerDeviceType(device.ConfigType),
		VirtualPortType:  device.VirtualPortType,
		IsAutoStart:      !device.NoAutoStart,
		IsEnabled:        !device.Disabled,
		ParentDeviceName: device.ParentInterfaceName,
		GatewayAddress:   ptr(device.GatewayAddress.Value),
		IsDefaultGateway: device.IsDefaultGateway,
		VLANTag:          uint64(device.VLANTag),
		DNSSearchDomains: device.DNSSearchDomains,
		DNSAddresses:     device.DNSServers,
		Addrs: append(
			transform.Slice(device.Addresses, newNetAddress(device, false)),
			transform.Slice(device.ShadowAddresses, newNetAddress(device, true))...),
	}
}

func newNetAddress(device network.InterfaceInfo, isShadow bool) func(network.ProviderAddress) domainnetwork.NetAddr {
	return func(providerAddr network.ProviderAddress) domainnetwork.NetAddr {
		return domainnetwork.NetAddr{
			InterfaceName:    device.InterfaceName,
			AddressValue:     providerAddr.Value,
			AddressType:      providerAddr.Type,
			ConfigType:       providerAddr.ConfigType,
			Origin:           network.OriginProvider,
			Scope:            providerAddr.Scope,
			IsSecondary:      providerAddr.IsSecondary,
			IsShadow:         isShadow,
			ProviderID:       ptr(device.ProviderAddressId),
			ProviderSubnetID: ptr(device.ProviderSubnetId),
		}
	}
}

func ptr[T comparable](f T) *T {
	var zero T
	if f == zero {
		return nil
	}
	return &f
}
