// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/rpc/params"
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
	NetworkInterfaces(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error)
}

// Machine specifies an interface for machine instances processed by the
// instance poller.
type Machine interface {
	Id() string
	InstanceId(ctx context.Context) (instance.Id, error)
	SetProviderNetworkConfig(context.Context, network.InterfaceInfos) (network.ProviderAddresses, bool, error)
	InstanceStatus(ctx context.Context) (params.StatusResult, error)
	SetInstanceStatus(context.Context, status.Status, string, map[string]interface{}) error
	String() string
	Refresh(ctx context.Context) error
	Status(ctx context.Context) (params.StatusResult, error)
	Life() life.Value
	IsManual(ctx context.Context) (bool, error)
}

// FacadeAPI specifies the api-server methods needed by the instance
// poller.
type FacadeAPI interface {
	WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error)
	Machine(ctx context.Context, tag names.MachineTag) (Machine, error)
}

// Config encapsulates the configuration options for instantiating a new
// instance poller worker.
type Config struct {
	Clock   clock.Clock
	Facade  FacadeAPI
	Environ Environ
	Logger  logger.Logger

	CredentialAPI common.CredentialAPI
}

// Validate checks whether the worker configuration settings are valid.
func (config Config) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil clock.Clock")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.CredentialAPI == nil {
		return errors.NotValidf("nil CredentialAPI")
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
	m          Machine
	tag        names.MachineTag
	instanceID instance.Id

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

	pollGroup              [2]map[names.MachineTag]*pollGroupEntry
	instanceIDToGroupEntry map[instance.Id]*pollGroupEntry
	callContextFunc        common.CloudCallContextFunc

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
		pollGroup: [2]map[names.MachineTag]*pollGroupEntry{
			make(map[names.MachineTag]*pollGroupEntry),
			make(map[names.MachineTag]*pollGroupEntry),
		},
		instanceIDToGroupEntry: make(map[instance.Id]*pollGroupEntry),
		callContextFunc:        common.NewCloudCallContextFunc(config.CredentialAPI),
	}
	err := catacomb.Invoke(catacomb.Plan{
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

	watch, err := u.config.Facade.WatchModelMachines(ctx)
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
		case ids, ok := <-watch.Changes():
			if !ok {
				return errors.New("machines watcher closed")
			}

			for i := range ids {
				tag := names.NewMachineTag(ids[i])
				if err := u.queueMachineForPolling(ctx, tag); err != nil {
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

func (u *updaterWorker) queueMachineForPolling(ctx context.Context, tag names.MachineTag) error {
	// If we are already polling this machine, check whether it is still alive
	// and remove it from its poll group if it is now dead.
	if entry, groupType := u.lookupPolledMachine(tag); entry != nil {
		var isDead bool
		if err := entry.m.Refresh(ctx); err != nil {
			// If the machine is not found, this probably means
			// that it is dead and has been removed from the DB.
			if !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			isDead = true
		} else if entry.m.Life() == life.Dead {
			isDead = true
		}

		if isDead {
			u.config.Logger.Debugf(ctx, "removing dead machine %q (instance ID %q)", entry.m, entry.instanceID)
			delete(u.pollGroup[groupType], tag)
			delete(u.instanceIDToGroupEntry, entry.instanceID)
			return nil
		}

		// Something has changed with the machine state. Reset short
		// poll interval for the machine and move it to the short poll
		// group (if not already there) so we immediately poll its
		// status at the next interval.
		u.moveEntryToPollGroup(shortPollGroup, entry)
		if groupType == longPollGroup {
			u.config.Logger.Debugf(ctx, "moving machine %q (instance ID %q) to short poll group", entry.m, entry.instanceID)
		}
		return nil
	}

	// Get information about the machine
	m, err := u.config.Facade.Machine(ctx, tag)
	if err != nil {
		return errors.Trace(err)
	}

	// We don't poll manual machines, instead we're setting the status to 'running'
	// as we don't have any better information from the provider, see lp:1678981
	isManual, err := m.IsManual(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if isManual {
		machineStatus, err := m.InstanceStatus(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		if status.Status(machineStatus.Status) != status.Running {
			if err = m.SetInstanceStatus(ctx, status.Running, "Manually provisioned machine", nil); err != nil {
				u.config.Logger.Errorf(ctx, "cannot set instance status on %q: %v", m, err)
				return err
			}
		}
		return nil
	}

	// Add all new machines to the short poll group and arrange for them to
	// be polled as soon as possible.
	u.appendToShortPollGroup(tag, m)
	return nil
}

func (u *updaterWorker) appendToShortPollGroup(tag names.MachineTag, m Machine) {
	entry := &pollGroupEntry{
		tag: tag,
		m:   m,
	}
	entry.resetShortPollInterval(u.config.Clock)
	u.pollGroup[shortPollGroup][tag] = entry
}

func (u *updaterWorker) moveEntryToPollGroup(toGroup pollGroupType, entry *pollGroupEntry) {
	// Ensure that the entry is not present in the other group
	delete(u.pollGroup[1-toGroup], entry.tag)
	u.pollGroup[toGroup][entry.tag] = entry

	// If moving to the short poll group reset the poll interval
	if toGroup == shortPollGroup {
		entry.resetShortPollInterval(u.config.Clock)
	}
}

func (u *updaterWorker) lookupPolledMachine(tag names.MachineTag) (*pollGroupEntry, pollGroupType) {
	for groupType, members := range u.pollGroup {
		if found := members[tag]; found != nil {
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
			if params.IsCodeNotProvisioned(err) {
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

	infoList, err := u.config.Environ.Instances(u.callContextFunc(ctx), instList)
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

	netList, err := u.config.Environ.NetworkInterfaces(u.callContextFunc(ctx), instList)
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

	providerStatus, providerAddrCount, err := u.processProviderInfo(ctx, entry, info, nics)
	if err != nil {
		return errors.Trace(err)
	}

	machineStatus, err := entry.m.Status(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	u.maybeSwitchPollGroup(ctx, groupType, entry, providerStatus, status.Status(machineStatus.Status), providerAddrCount)
	return nil
}

func (u *updaterWorker) resolveInstanceID(ctx context.Context, entry *pollGroupEntry) error {
	if entry.instanceID != "" {
		return nil // already resolved
	}

	instID, err := entry.m.InstanceId(ctx)
	if err != nil {
		return errors.Annotatef(err, "retrieving instance ID for machine %q", entry.m.Id())
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
) (status.Status, int, error) {
	curStatus, err := entry.m.InstanceStatus(ctx)
	if err != nil {
		// This should never occur since the machine is provisioned. If
		// it does occur, report an unknown status to move the machine to
		// the short poll group.
		u.config.Logger.Warningf(ctx, "cannot get current instance status for machine %v (instance ID %q): %v",
			entry.m.Id(), entry.instanceID, err)

		return status.Unknown, -1, nil
	}

	// Check for status changes
	providerStatus := info.Status(u.callContextFunc(context.Background()))
	curInstStatus := instance.Status{
		Status:  status.Status(curStatus.Status),
		Message: curStatus.Info,
	}

	if providerStatus != curInstStatus {
		u.config.Logger.Infof(ctx, "machine %q (instance ID %q) instance status changed from %q to %q",
			entry.m.Id(), entry.instanceID, curInstStatus, providerStatus)

		if err = entry.m.SetInstanceStatus(ctx, providerStatus.Status, providerStatus.Message, nil); err != nil {
			u.config.Logger.Errorf(ctx, "cannot set instance status on %q: %v", entry.m, err)
			return status.Unknown, -1, errors.Trace(err)
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
	if entry.m.Life() == life.Dead {
		return status.Unknown, -1, nil
	}

	// Check whether the provider addresses for this machine need to be
	// updated.
	addrCount, err := u.syncProviderAddresses(ctx, entry, providerInterfaces)
	if err != nil {
		return status.Unknown, -1, err
	}

	return providerStatus.Status, addrCount, nil
}

// syncProviderAddresses updates the provider addresses for this entry's machine
// using either the provider sourced interface list.
//
// The call returns the count of provider addresses for the machine.
func (u *updaterWorker) syncProviderAddresses(
	ctx context.Context,
	entry *pollGroupEntry, providerIfaceList network.InterfaceInfos,
) (int, error) {
	addrs, modified, err := entry.m.SetProviderNetworkConfig(ctx, providerIfaceList)
	if err != nil {
		return -1, errors.Trace(err)
	} else if modified {
		u.config.Logger.Infof(ctx, "machine %q (instance ID %q) has new addresses: %v",
			entry.m.Id(), entry.instanceID, addrs)
	}

	return len(addrs), nil
}

func (u *updaterWorker) maybeSwitchPollGroup(
	ctx context.Context,
	curGroup pollGroupType,
	entry *pollGroupEntry,
	curProviderStatus,
	curMachineStatus status.Status,
	providerAddrCount int,
) {
	if curProviderStatus == status.Allocating || curProviderStatus == status.Pending {
		// Keep the machine in the short poll group until it settles.
		entry.bumpShortPollInterval(u.config.Clock)
		return
	}

	// If the machine is currently in the long poll group and it has an
	// unknown status or suddenly has no network addresses, move it back to
	// the short poll group.
	if curGroup == longPollGroup && (curProviderStatus == status.Unknown || providerAddrCount == 0) {
		u.moveEntryToPollGroup(shortPollGroup, entry)
		u.config.Logger.Debugf(ctx, "moving machine %q (instance ID %q) back to short poll group", entry.m, entry.instanceID)
		return
	}

	// The machine has started and we have at least one address; move to
	// the long poll group
	if providerAddrCount > 0 && curMachineStatus == status.Started {
		u.moveEntryToPollGroup(longPollGroup, entry)
		if curGroup != longPollGroup {
			u.config.Logger.Debugf(ctx, "moving machine %q (instance ID %q) to long poll group", entry.m, entry.instanceID)
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
