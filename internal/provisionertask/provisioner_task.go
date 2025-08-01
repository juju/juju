// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/workerpool"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/password"
	providercommon "github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/rpc/params"
)

type ProvisionerTask interface {
	worker.Worker

	// SetNumProvisionWorkers resizes the pool of provision workers.
	SetNumProvisionWorkers(numWorkers int)
}

// MachinesAPI describes API methods required to access machine provisioning info.
type MachinesAPI interface {
	Machines(context.Context, ...names.MachineTag) ([]apiprovisioner.MachineResult, error)
	MachinesWithTransientErrors(context.Context) ([]apiprovisioner.MachineStatusResult, error)
	WatchMachineErrorRetry(context.Context) (watcher.NotifyWatcher, error)
	WatchModelMachines(context.Context) (watcher.StringsWatcher, error)
	ProvisioningInfo(_ context.Context, machineTags []names.MachineTag) (params.ProvisioningInfoResults, error)
}

// DistributionGroupFinder provides access to machine distribution groups.
type DistributionGroupFinder interface {
	DistributionGroupByMachineId(context.Context, ...names.MachineTag) ([]apiprovisioner.DistributionGroupResult, error)
}

// ToolsFinder is an interface used for finding tools to run on
// provisioned instances.
type ToolsFinder interface {
	// FindTools returns a list of tools matching the specified
	// version, os, and architecture. If arch is empty, the
	// implementation is expected to use a well documented default.
	FindTools(ctx context.Context, version semversion.Number, os string, arch string) (coretools.List, error)
}

// ControllerAPI describes API methods for querying a controller.
type ControllerAPI interface {
	ControllerConfig(context.Context) (controller.Config, error)
	CACert(context.Context) (string, error)
	ModelUUID(context.Context) (string, error)
	ModelConfig(context.Context) (*config.Config, error)
	WatchForModelConfigChanges(context.Context) (watcher.NotifyWatcher, error)
	APIAddresses(context.Context) ([]string, error)
}

// ContainerManagerConfigGetter describes methods for retrieving model config
// needed for configuring the container manager.
type ContainerManagerConfigGetter interface {
	ContainerManagerConfig(context.Context, params.ContainerManagerConfigParams) (params.ContainerManagerConfig, error)
}

// RetryStrategy defines the retry behavior when encountering a retryable
// error during provisioning.
//
// TODO(katco): 2016-08-09: lp:1611427
type RetryStrategy struct {
	RetryDelay time.Duration
	RetryCount int
}

// GetMachineInstanceInfoSetter provides the interface for setting the
// instance info of a machine. It takes a machine provisioner API as input
// so it can be used when we don't have a machine service available.
type GetMachineInstanceInfoSetter func(machineProvisioner apiprovisioner.MachineProvisioner) func(
	ctx context.Context,
	id instance.Id, displayName string, nonce string, characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error

// TaskConfig holds the initialisation data for a ProvisionerTask instance.
type TaskConfig struct {
	ControllerUUID               string
	HostTag                      names.Tag
	Logger                       logger.Logger
	ControllerAPI                ControllerAPI
	MachinesAPI                  MachinesAPI
	GetMachineInstanceInfoSetter GetMachineInstanceInfoSetter
	DistributionGroupFinder      DistributionGroupFinder
	ToolsFinder                  ToolsFinder
	MachineWatcher               watcher.StringsWatcher
	RetryWatcher                 watcher.NotifyWatcher
	Broker                       environs.InstanceBroker
	ImageStream                  string
	RetryStartInstanceStrategy   RetryStrategy
	NumProvisionWorkers          int
	EventProcessedCb             func(string)
}

// NewProvisionerTask creates a new ProvisionerTask instance. The
// MachineWatcher is expected to be started before this function returns.
func NewProvisionerTask(cfg TaskConfig) (ProvisionerTask, error) {
	machineChanges := cfg.MachineWatcher.Changes()
	workers := []worker.Worker{cfg.MachineWatcher}
	var retryChanges watcher.NotifyChannel
	if cfg.RetryWatcher != nil {
		retryChanges = cfg.RetryWatcher.Changes()
		workers = append(workers, cfg.RetryWatcher)
	}
	task := &provisionerTask{
		controllerUUID:               cfg.ControllerUUID,
		hostTag:                      cfg.HostTag,
		logger:                       cfg.Logger,
		controllerAPI:                cfg.ControllerAPI,
		machinesAPI:                  cfg.MachinesAPI,
		getMachineInstanceInfoSetter: cfg.GetMachineInstanceInfoSetter,
		distributionGroupFinder:      cfg.DistributionGroupFinder,
		toolsFinder:                  cfg.ToolsFinder,
		machineChanges:               machineChanges,
		retryChanges:                 retryChanges,
		broker:                       cfg.Broker,
		machines:                     make(map[string]apiprovisioner.MachineProvisioner),
		machinesStarting:             make(map[string]bool),
		machinesStopDeferred:         make(map[string]bool),
		machinesStopping:             make(map[string]bool),
		availabilityZoneMachines:     make([]*AvailabilityZoneMachine, 0),
		imageStream:                  cfg.ImageStream,
		retryStartInstanceStrategy:   cfg.RetryStartInstanceStrategy,
		wp:                           workerpool.NewWorkerPool(cfg.Logger, cfg.NumProvisionWorkers),
		wpSizeChan:                   make(chan int, 1),
		eventProcessedCb:             cfg.EventProcessedCb,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "provisioner-task",
		Site: &task.catacomb,
		Work: task.loop,
		Init: workers,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return task, nil
}

// The list of events that are passed into the eventProcessed callback by the
// main loop.
const (
	eventTypeProcessedMachines         = "processed-machines"
	eventTypeRetriedMachinesWithErrors = "retried-machines-with-errors"
	eventTypeResizedWorkerPool         = "resized-worker-pool"
)

type provisionerTask struct {
	catacomb catacomb.Catacomb

	controllerUUID               string
	hostTag                      names.Tag
	logger                       logger.Logger
	controllerAPI                ControllerAPI
	machinesAPI                  MachinesAPI
	getMachineInstanceInfoSetter GetMachineInstanceInfoSetter
	distributionGroupFinder      DistributionGroupFinder
	toolsFinder                  ToolsFinder
	machineChanges               watcher.StringsChannel
	retryChanges                 watcher.NotifyChannel
	broker                       environs.InstanceBroker
	imageStream                  string
	retryStartInstanceStrategy   RetryStrategy

	machinesMutex            sync.RWMutex
	machines                 map[string]apiprovisioner.MachineProvisioner // machine ID -> machine
	machinesStarting         map[string]bool                              // machine IDs currently being started.
	machinesStopping         map[string]bool                              // machine IDs currently being stopped.
	machinesStopDeferred     map[string]bool                              // machine IDs which were set as dead while starting. They will be stopped once they are online.
	availabilityZoneMachines []*AvailabilityZoneMachine
	instances                map[instance.Id]instances.Instance // instanceID -> instance

	// A worker pool for starting/stopping instances in parallel.
	wp         *workerpool.WorkerPool
	wpSizeChan chan int

	// eventProcessedCb is an optional, externally-registered callback that
	// will be invoked when the task main loop successfully processes an event.
	// The event type is provided as the first arg to the callback.
	eventProcessedCb func(string)
}

// Kill implements worker.Worker.Kill.
func (task *provisionerTask) Kill() {
	task.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (task *provisionerTask) Wait() error {
	return task.catacomb.Wait()
}

func (task *provisionerTask) loop() (taskErr error) {
	ctx := task.scopedContext()

	task.logger.Infof(ctx, "entering provisioner task loop; using provisioner pool with %d workers", task.wp.Size())
	defer func() {
		wpErr := task.wp.Close()
		if taskErr == nil {
			taskErr = wpErr
		}
		task.logger.Infof(ctx, "exiting provisioner task loop; err: %v", taskErr)
	}()

	// When the watcher is started, it will have the initial changes be all
	// the machines that are relevant. Also, since this is available straight
	// away, we know there will be some changes right off the bat.
	for {
		select {
		case <-task.catacomb.Dying():
			return task.catacomb.ErrDying()

		case ids, ok := <-task.machineChanges:
			if !ok {
				return errors.New("machine watcher closed channel")
			}

			if err := task.processMachines(ctx, ids); err != nil {
				return errors.Annotate(err, "processing updated machines")
			}

			task.notifyEventProcessedCallback(eventTypeProcessedMachines)

		case numWorkers := <-task.wpSizeChan:
			if task.wp.Size() == numWorkers {
				continue // nothing to do
			}

			// Stop the current pool (checking for any pending
			// errors) and create a new one.
			task.logger.Infof(ctx, "resizing provision worker pool size to %d", numWorkers)
			if err := task.wp.Close(); err != nil {
				return err
			}
			task.wp = workerpool.NewWorkerPool(task.logger, numWorkers)
			task.notifyEventProcessedCallback(eventTypeResizedWorkerPool)

		case <-task.retryChanges:
			if err := task.processMachinesWithTransientErrors(ctx); err != nil {
				return errors.Annotate(err, "processing machines with transient errors")
			}
			task.notifyEventProcessedCallback(eventTypeRetriedMachinesWithErrors)

		case <-task.wp.Done():
			// The worker pool has detected one or more errors and
			// is in the process of shutting down. Collect and
			// report any emitted errors.
			return task.wp.Close()
		}
	}
}

func (task *provisionerTask) notifyEventProcessedCallback(evtType string) {
	if task.eventProcessedCb != nil {
		task.eventProcessedCb(evtType)
	}
}

// SetNumProvisionWorkers queues a pool resize request to be processed by the
// provisioner task main loop.
func (task *provisionerTask) SetNumProvisionWorkers(numWorkers int) {
	select {
	case task.wpSizeChan <- numWorkers:
	case <-task.catacomb.Dying():
	}
}

func (task *provisionerTask) processMachinesWithTransientErrors(ctx context.Context) error {
	results, err := task.machinesAPI.MachinesWithTransientErrors(ctx)
	if err != nil || len(results) == 0 {
		return nil
	}
	task.logger.Tracef(ctx, "processMachinesWithTransientErrors(%v)", results)
	var pending []apiprovisioner.MachineProvisioner
	for _, result := range results {
		if result.Status.Error != nil {
			task.logger.Errorf(ctx, "cannot retry provisioning of machine %q: %v", result.Machine.Id(), result.Status.Error)
			continue
		}
		machine := result.Machine
		if err := machine.SetStatus(ctx, status.Pending, "", nil); err != nil {
			task.logger.Errorf(ctx, "cannot reset status of machine %q: %v", machine.Id(), err)
			continue
		}
		if err := machine.SetInstanceStatus(ctx, status.Provisioning, "", nil); err != nil {
			task.logger.Errorf(ctx, "cannot reset instance status of machine %q: %v", machine.Id(), err)
			continue
		}
		task.machinesMutex.Lock()
		task.machines[machine.Id()] = machine
		task.machinesMutex.Unlock()
		pending = append(pending, machine)
	}
	return task.queueStartMachines(ctx, pending)
}

func (task *provisionerTask) processMachines(ctx context.Context, ids []string) error {
	task.logger.Debugf(ctx, "processing machines %v", ids)

	// Populate the tasks maps of current instances and machines.
	if err := task.populateMachineMaps(ctx, ids); err != nil {
		return errors.Trace(err)
	}

	// Maintain zone-machine distributions.
	err := task.updateAvailabilityZoneMachines(ctx)
	if err != nil && !errors.Is(err, errors.NotImplemented) {
		return errors.Annotate(err, "updating AZ distributions")
	}

	// Find machines without an instance ID or that are dead.
	pending, dead, err := task.pendingOrDead(ctx, ids)
	if err != nil {
		return errors.Trace(err)
	}

	// Queue removal of any dead machines that are not already being
	// stopped or flagged for deferred stopping once they are online.
	if err := task.filterAndQueueRemovalOfDeadMachines(ctx, dead); err != nil {
		return errors.Trace(err)
	}

	// Queue start requests for any other pending instances.
	return errors.Trace(task.queueStartMachines(ctx, pending))
}

func instanceIds(instances []instances.Instance) []string {
	ids := make([]string, 0, len(instances))
	for _, inst := range instances {
		ids = append(ids, string(inst.Id()))
	}
	return ids
}

// populateMachineMaps updates task.instances. Also updates task.machines map
// if a list of IDs is given.
func (task *provisionerTask) populateMachineMaps(ctx context.Context, ids []string) error {
	allInstances, err := task.broker.AllRunningInstances(ctx)
	if err != nil {
		return errors.Annotate(err, "getting all instances from broker")
	}

	instances := make(map[instance.Id]instances.Instance)
	for _, i := range allInstances {
		instances[i.Id()] = i
	}
	task.machinesMutex.Lock()
	task.instances = instances
	task.machinesMutex.Unlock()

	// Update the machines map with new data for each of the machines in the
	// change list.
	machineTags := make([]names.MachineTag, len(ids))
	for i, id := range ids {
		machineTags[i] = names.NewMachineTag(id)
	}
	machines, err := task.machinesAPI.Machines(ctx, machineTags...)
	if err != nil {
		return errors.Annotatef(err, "getting machines %v", ids)
	}
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for i, result := range machines {
		switch {
		case result.Err == nil:
			task.machines[result.Machine.Id()] = result.Machine
		case params.IsCodeNotFoundOrCodeUnauthorized(result.Err):
			task.logger.Debugf(ctx, "machine %q not found in state", ids[i])
			delete(task.machines, ids[i])
		default:
			return errors.Annotatef(result.Err, "getting machine %v", ids[i])
		}
	}
	task.logger.Tracef(ctx, "provisioner task machine map %v", task.machines)
	return nil
}

// pendingOrDead looks up machines with ids and returns those that do not
// have an instance id assigned yet, and also those that are dead. Any machines
// that are currently being stopped or have been marked for deferred stopping
// once they are online will be skipped.
func (task *provisionerTask) pendingOrDead(
	ctx context.Context, ids []string,
) ([]apiprovisioner.MachineProvisioner, []apiprovisioner.MachineProvisioner, error) {
	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()

	var pending, dead []apiprovisioner.MachineProvisioner
	for _, id := range ids {
		// Ignore machines that have been either queued for deferred
		// stopping or are currently stopping.
		if _, found := task.machinesStopDeferred[id]; found {
			task.logger.Tracef(ctx, "pendingOrDead: ignoring machine %q; machine has deferred stop flag set", id)
			continue // ignore: will be stopped once started
		} else if _, found := task.machinesStopping[id]; found {
			task.logger.Tracef(ctx, "pendingOrDead: ignoring machine %q; machine is currently being stopped", id)
			continue // ignore: currently being stopped.
		}

		machine, found := task.machines[id]
		if !found {
			task.logger.Infof(ctx, "machine %q not found", id)
			continue
		}
		var classification MachineClassification
		classification, err := classifyMachine(ctx, task.logger, machine)
		if err != nil {
			return nil, nil, err
		}
		switch classification {
		case Pending:
			pending = append(pending, machine)
		case Dead:
			dead = append(dead, machine)
		}
	}

	task.logger.Debugf(ctx, "pending: %v, dead: %v", pending, dead)
	return pending, dead, nil
}

func (task *provisionerTask) scopedContext() context.Context {
	return task.catacomb.Context(context.Background())
}

// ClassifiableMachine is an interface that provides methods to classify a
// machine based on its life cycle state and instance ID. It is used to
// determine if a machine is pending, dead, or has no instance ID assigned.
type ClassifiableMachine interface {
	Life() life.Value
	Id() string
	InstanceId(context.Context) (instance.Id, error)
	EnsureDead(context.Context) error
	Status(context.Context) (status.Status, string, error)
	InstanceStatus(context.Context) (status.Status, string, error)
}

// MachineClassification represents the classification of a machine based on
// its life cycle state. It can be None, Pending, or Dead.
type MachineClassification string

const (
	None    MachineClassification = "none"
	Pending MachineClassification = "Pending"
	Dead    MachineClassification = "Dead"
)

func classifyMachine(ctx context.Context, logger logger.Logger, machine ClassifiableMachine) (
	MachineClassification, error) {
	switch machine.Life() {
	case life.Dying:
		if _, err := machine.InstanceId(ctx); err == nil {
			return None, nil
		} else if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "loading dying machine id:%s, details:%v", machine.Id(), machine)
		}
		logger.Infof(ctx, "killing dying, unprovisioned machine %q", machine)
		if err := machine.EnsureDead(ctx); err != nil {
			return None, errors.Annotatef(err, "ensuring machine dead id:%s, details:%v", machine.Id(), machine)
		}
		fallthrough
	case life.Dead:
		return Dead, nil
	}
	instId, err := machine.InstanceId(ctx)
	if err != nil {
		if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "loading machine id:%s, details:%v", machine.Id(), machine)
		}
		machineStatus, _, err := machine.Status(ctx)
		if err != nil {
			logger.Infof(ctx, "cannot get machine id:%s, details:%v, err:%v", machine.Id(), machine, err)
			return None, nil
		}
		if machineStatus == status.Pending {
			logger.Infof(ctx, "found machine pending provisioning id:%s, details:%v", machine.Id(), machine)
			return Pending, nil
		}
		instanceStatus, _, err := machine.InstanceStatus(ctx)
		if err != nil {
			logger.Infof(ctx, "cannot read instance status id:%s, details:%v, err:%v", machine.Id(), machine, err)
			return None, nil
		}
		if instanceStatus == status.Provisioning {
			logger.Infof(ctx, "found machine provisioning id:%s, details:%v", machine.Id(), machine)
			return Pending, nil
		}
		return None, nil
	}
	logger.Infof(ctx, "machine %s already started as instance %q", machine.Id(), instId)

	return None, nil
}

// filterAndQueueRemovalOfDeadMachines scans the list of dead machines and:
//   - Sets the deferred stop flag for machines that are still online
//   - Filters out any machines that are either stopping or have the deferred
//     stop flag set.
//   - Marks the remaining machines as stopping and queues a request for them to
//     be cleaned up.
func (task *provisionerTask) filterAndQueueRemovalOfDeadMachines(ctx context.Context, dead []apiprovisioner.MachineProvisioner) error {
	// Flag any machines in the dead list that are still being started so
	// they will be stopped once they come online.
	task.deferStopForNotYetStartedMachines(dead)

	// Filter the initial dead machine list. Any machines marked for
	// deferred stopping, machines that are already being stopped and
	// machines that have not yet finished provisioning will be removed
	// from the filtered list.
	return task.queueRemovalOfDeadMachines(ctx, task.filterDeadMachines(dead))
}

func (task *provisionerTask) queueRemovalOfDeadMachines(
	ctx context.Context,
	dead []apiprovisioner.MachineProvisioner,
) error {
	if len(dead) == 0 {
		// nothing to do
		return nil
	}

	// Collect the instances for all provisioned machines that are dead.
	stopping := task.instancesForDeadMachines(ctx, dead)
	if len(stopping) == 0 {
		// no instances to stop, as the machines are not provisioned.
		return nil
	}

	provTask := workerpool.Task{
		Type: "stop-instances",
		Process: func() error {
			if len(stopping) > 0 {
				task.logger.Infof(ctx, "stopping known instances %v", instanceIds(stopping))
			}

			// It is important that we stop unknown instances before starting
			// pending ones, because if we start an instance and then fail to
			// set its InstanceId on the machine.
			// We don't want to start a new instance for the same machine ID.
			if err := task.doStopInstances(ctx, stopping); err != nil {
				return errors.Trace(err)
			}

			// Remove any dead machines from state.
			for _, machine := range dead {
				task.logger.Infof(ctx, "removing dead machine %q", machine.Id())
				if err := machine.MarkForRemoval(ctx); err != nil {
					task.logger.Errorf(ctx, "failed to remove dead machine %q: %v", machine.Id(), err)
				}
				task.removeMachineFromAZMap(machine)
				machID := machine.Id()
				task.machinesMutex.Lock()
				delete(task.machines, machID)
				delete(task.machinesStopping, machID)
				task.machinesMutex.Unlock()
			}

			return nil
		},
	}

	select {
	case task.wp.Queue() <- provTask:
		// successfully enqueued removal request
		return nil
	case <-task.catacomb.Dying():
		return task.catacomb.ErrDying()
	case <-task.wp.Done():
		// Capture and surface asynchronous worker pool errors.
		return task.wp.Close()
	}
}

// Filter the provided dead machines and remove any machines marked for
// deferred stopping, machines that are currently being stopped and any
// machines that they have not finished starting.
// This method also marks the filtered list of machines as stopping.
func (task *provisionerTask) filterDeadMachines(dead []apiprovisioner.MachineProvisioner) []apiprovisioner.MachineProvisioner {
	var deadMachines []apiprovisioner.MachineProvisioner

	task.machinesMutex.Lock()
	for _, machine := range dead {
		machID := machine.Id()

		// Ignore any machines for which we have either deferred the
		// stopping of the machine is currently being stopped or they
		// are still being started.
		if task.machinesStopDeferred[machID] || task.machinesStopping[machID] || task.machinesStarting[machID] {
			continue
		}
		task.machinesStopping[machID] = true

		// This machine should be queued for deletion.
		deadMachines = append(deadMachines, machine)
	}
	task.machinesMutex.Unlock()

	return deadMachines
}

// Iterate the list of dead machines and flag the ones that are still being
// started so they can be immediately stopped once they come online.
func (task *provisionerTask) deferStopForNotYetStartedMachines(dead []apiprovisioner.MachineProvisioner) {
	task.machinesMutex.Lock()
	for _, machine := range dead {
		machID := machine.Id()
		if task.machinesStarting[machID] {
			task.machinesStopDeferred[machID] = true
		}
	}
	task.machinesMutex.Unlock()
}

// instancesForDeadMachines returns a list of instances that correspond to
// machines with a life of "dead" in state. Missing machines and machines that
// have not finished starting are omitted from the list.
func (task *provisionerTask) instancesForDeadMachines(ctx context.Context, dead []apiprovisioner.MachineProvisioner) []instances.Instance {
	var deadInstances []instances.Instance
	for _, machine := range dead {
		// Ignore machines that are still provisioning
		task.machinesMutex.RLock()
		if task.machinesStarting[machine.Id()] {
			task.machinesMutex.RUnlock()
			continue
		}
		task.machinesMutex.RUnlock()

		instId, err := machine.InstanceId(ctx)
		if err == nil {
			keep, _ := machine.KeepInstance(ctx)
			if keep {
				task.logger.Debugf(ctx, "machine %v is dead but keep-instance is true", instId)
				continue
			}

			// If the instance is not found we can't stop it.
			task.machinesMutex.RLock()
			if inst, found := task.instances[instId]; found {
				deadInstances = append(deadInstances, inst)
			}
			task.machinesMutex.RUnlock()
		}
	}
	return deadInstances
}

func (task *provisionerTask) doStopInstances(ctx context.Context, instances []instances.Instance) error {
	// Although calling StopInstance with an empty slice should produce no change in the
	// provider, environs like dummy do not consider this a noop.
	if len(instances) == 0 {
		return nil
	}
	if wrench.IsActive("provisioner", "stop-instances") {
		return errors.New("wrench in the works")
	}

	ids := make([]instance.Id, len(instances))
	for i, inst := range instances {
		ids[i] = inst.Id()
	}
	if err := task.broker.StopInstances(ctx, ids...); err != nil {
		return errors.Annotate(err, "stopping instances")
	}
	return nil
}

func (task *provisionerTask) constructInstanceConfig(
	ctx context.Context,
	machine apiprovisioner.MachineProvisioner,
	pInfo *params.ProvisioningInfo,
) (*instancecfg.InstanceConfig, error) {
	apiAddresses, err := task.controllerAPI.APIAddresses(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	caCert, err := task.controllerAPI.CACert(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID, err := task.controllerAPI.ModelUUID(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	password, err := password.RandomPassword()
	if err != nil {
		return nil, fmt.Errorf("cannot make password for machine %v: %v", machine, err)
	}
	if err := machine.SetPassword(ctx, password); err != nil {
		return nil, fmt.Errorf("cannot set API password for machine %v: %v", machine, err)
	}
	apiInfo := &api.Info{
		Addrs:    apiAddresses,
		CACert:   caCert,
		ModelTag: names.NewModelTag(modelUUID),
		Tag:      machine.Tag(),
		Password: password,
	}

	// Generated a nonce for the new instance, with the format: "machine-#:UUID".
	// The first part is a badge, specifying the tag of the machine the provisioner
	// is running on, while the second part is a random UUID.
	uuid, err := uuid.NewUUID()
	if err != nil {
		return nil, errors.Annotate(err, "generating nonce for machine "+machine.Id())
	}

	nonce := fmt.Sprintf("%s:%s", task.hostTag, uuid)
	base, err := corebase.ParseBase(pInfo.Base.Name, pInfo.Base.Channel)
	if err != nil {
		return nil, errors.Annotatef(err, "parsing machine base %q", pInfo.Base)
	}
	instanceConfig, err := instancecfg.NewInstanceConfig(
		names.NewControllerTag(controller.Config(pInfo.ControllerConfig).ControllerUUID()),
		machine.Id(),
		nonce,
		task.imageStream,
		base,
		apiInfo,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instanceConfig.ControllerConfig = make(map[string]interface{})
	for k, v := range pInfo.ControllerConfig {
		instanceConfig.ControllerConfig[k] = v
	}

	instanceConfig.Tags = pInfo.Tags
	if len(pInfo.Jobs) > 0 {
		instanceConfig.Jobs = pInfo.Jobs
	}

	if instanceConfig.IsController() {
		publicKey, err := simplestreams.UserPublicSigningKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
		instanceConfig.PublicImageSigningKey = publicKey
	}

	instanceConfig.CloudInitUserData = pInfo.CloudInitUserData

	return instanceConfig, nil
}

func (task *provisionerTask) constructStartInstanceParams(
	controllerUUID string,
	machine apiprovisioner.MachineProvisioner,
	instanceConfig *instancecfg.InstanceConfig,
	provisioningInfo *params.ProvisioningInfo,
	possibleTools coretools.List,
) (environs.StartInstanceParams, error) {

	volumes := make([]storage.VolumeParams, len(provisioningInfo.Volumes))
	for i, v := range provisioningInfo.Volumes {
		volumeTag, err := names.ParseVolumeTag(v.VolumeTag)
		if err != nil {
			return environs.StartInstanceParams{}, errors.Trace(err)
		}
		if v.Attachment == nil {
			return environs.StartInstanceParams{}, errors.Errorf("volume params missing attachment")
		}
		machineTag, err := names.ParseMachineTag(v.Attachment.MachineTag)
		if err != nil {
			return environs.StartInstanceParams{}, errors.Trace(err)
		}
		if machineTag != machine.Tag() {
			return environs.StartInstanceParams{}, errors.Errorf("volume attachment params has invalid machine tag")
		}
		if v.Attachment.InstanceId != "" {
			return environs.StartInstanceParams{}, errors.Errorf("volume attachment params specifies instance ID")
		}
		volumes[i] = storage.VolumeParams{
			Tag:          volumeTag,
			Size:         v.Size,
			Provider:     storage.ProviderType(v.Provider),
			Attributes:   v.Attributes,
			ResourceTags: v.Tags,
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					Machine:  machineTag,
					ReadOnly: v.Attachment.ReadOnly,
				},
				Volume: volumeTag,
			},
		}
	}
	volumeAttachments := make([]storage.VolumeAttachmentParams, len(provisioningInfo.VolumeAttachments))
	for i, v := range provisioningInfo.VolumeAttachments {
		volumeTag, err := names.ParseVolumeTag(v.VolumeTag)
		if err != nil {
			return environs.StartInstanceParams{}, errors.Trace(err)
		}
		machineTag, err := names.ParseMachineTag(v.MachineTag)
		if err != nil {
			return environs.StartInstanceParams{}, errors.Trace(err)
		}
		if machineTag != machine.Tag() {
			return environs.StartInstanceParams{}, errors.Errorf("volume attachment params has invalid machine tag")
		}
		if v.InstanceId != "" {
			return environs.StartInstanceParams{}, errors.Errorf("volume attachment params specifies instance ID")
		}
		if v.ProviderId == "" {
			return environs.StartInstanceParams{}, errors.Errorf("volume attachment params does not specify volume ID")
		}
		volumeAttachments[i] = storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider: storage.ProviderType(v.Provider),
				Machine:  machineTag,
				ReadOnly: v.ReadOnly,
			},
			Volume:   volumeTag,
			VolumeId: v.ProviderId,
		}
	}

	var endpointBindings map[string]corenetwork.Id
	if len(provisioningInfo.EndpointBindings) != 0 {
		endpointBindings = make(map[string]corenetwork.Id)
		for endpoint, space := range provisioningInfo.EndpointBindings {
			endpointBindings[endpoint] = corenetwork.Id(space)
		}
	}

	possibleImageMetadata := make([]*imagemetadata.ImageMetadata, len(provisioningInfo.ImageMetadata))
	for i, metadata := range provisioningInfo.ImageMetadata {
		possibleImageMetadata[i] = &imagemetadata.ImageMetadata{
			Id:          metadata.ImageId,
			Arch:        metadata.Arch,
			RegionAlias: metadata.Region,
			RegionName:  metadata.Region,
			Storage:     metadata.RootStorageType,
			Stream:      metadata.Stream,
			VirtType:    metadata.VirtType,
			Version:     metadata.Version,
		}
	}

	startInstanceParams := environs.StartInstanceParams{
		ControllerUUID:    controllerUUID,
		Constraints:       provisioningInfo.Constraints,
		Tools:             possibleTools,
		InstanceConfig:    instanceConfig,
		Placement:         provisioningInfo.Placement,
		Volumes:           volumes,
		VolumeAttachments: volumeAttachments,
		SubnetsToZones:    subnetZonesFromNetworkTopology(provisioningInfo.ProvisioningNetworkTopology),
		EndpointBindings:  endpointBindings,
		ImageMetadata:     possibleImageMetadata,
		StatusCallback:    machine.SetInstanceStatus,
		Abort:             task.catacomb.Dying(),
		CharmLXDProfiles:  provisioningInfo.CharmLXDProfiles,
	}
	if provisioningInfo.RootDisk != nil {
		startInstanceParams.RootDisk = &storage.VolumeParams{
			Provider:   storage.ProviderType(provisioningInfo.RootDisk.Provider),
			Attributes: provisioningInfo.RootDisk.Attributes,
		}
	}

	return startInstanceParams, nil
}

// AvailabilityZoneMachine keeps track a single zone and which machines
// are in it, which machines have failed to use it and which machines
// shouldn't use it. This data is used to decide on how to distribute
// machines across availability zones.
//
// Exposed for testing.
type AvailabilityZoneMachine struct {
	ZoneName           string
	MachineIds         set.Strings
	FailedMachineIds   set.Strings
	ExcludedMachineIds set.Strings // Don't use these machines in the zone.
}

// MatchesConstraints against an AZ. If the constraints specifies Zones, make sure
// this AZ matches a listed ZoneName.
func (az *AvailabilityZoneMachine) MatchesConstraints(cons constraints.Value) bool {
	if !cons.HasZones() {
		return true
	}
	for _, zone := range *cons.Zones {
		if az.ZoneName == zone {
			return true
		}
	}
	return false
}

// updateAvailabilityZoneMachines maintains a mapping of AZs to machines
// running in each zone.
// If the provider does not implement the ZonedEnviron interface, return nil.
func (task *provisionerTask) updateAvailabilityZoneMachines(ctx context.Context) error {
	zonedEnv, ok := task.broker.(providercommon.ZonedEnviron)
	if !ok {
		return nil
	}

	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()

	// Only populate from the provider if we have no data.
	// Otherwise, just check that we know all the current AZs.
	if len(task.availabilityZoneMachines) == 0 {
		if err := task.populateAvailabilityZoneMachines(ctx, zonedEnv); err != nil {
			return errors.Trace(err)
		}
	} else {
		if err := task.checkProviderAvailabilityZones(ctx, zonedEnv); err != nil {
			return errors.Trace(err)
		}
	}

	zones := make([]string, len(task.availabilityZoneMachines))
	for i, azm := range task.availabilityZoneMachines {
		zones[i] = azm.ZoneName
	}
	task.logger.Infof(ctx, "provisioning in zones: %v", zones)

	return nil
}

// populateAvailabilityZoneMachines populates the slice,
// availabilityZoneMachines, with each zone and the IDs of
// machines running in that zone, according to the provider.
func (task *provisionerTask) populateAvailabilityZoneMachines(
	ctx context.Context,
	zonedEnv providercommon.ZonedEnviron,
) error {
	availabilityZoneInstances, err := providercommon.AvailabilityZoneAllocations(zonedEnv, ctx, []instance.Id{})
	if err != nil {
		return errors.Trace(err)
	}

	instanceMachines := make(map[instance.Id]string)
	for _, machine := range task.machines {
		instId, err := machine.InstanceId(ctx)
		if err != nil {
			continue
		}
		instanceMachines[instId] = machine.Id()
	}

	// Translate instance IDs to machines IDs to aid distributing
	// to-be-created instances across availability zones.
	task.availabilityZoneMachines = make([]*AvailabilityZoneMachine, len(availabilityZoneInstances))
	for i, azInstances := range availabilityZoneInstances {
		machineIds := set.NewStrings()
		for _, instanceId := range azInstances.Instances {
			if id, ok := instanceMachines[instanceId]; ok {
				machineIds.Add(id)
			}
		}
		task.availabilityZoneMachines[i] = &AvailabilityZoneMachine{
			ZoneName:           azInstances.ZoneName,
			MachineIds:         machineIds,
			FailedMachineIds:   set.NewStrings(),
			ExcludedMachineIds: set.NewStrings(),
		}
	}
	return nil
}

// checkProviderAvailabilityZones queries the known AZs.
// If any are missing from the AZ-machines slice, add them.
// If we have entries that are not known by the provider to be available zones,
// check whether we have machines there.
// If so, log a warning, otherwise we can delete them safely.
func (task *provisionerTask) checkProviderAvailabilityZones(
	ctx context.Context, zonedEnv providercommon.ZonedEnviron,
) error {
	azs, err := zonedEnv.AvailabilityZones(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	zones := set.NewStrings()
	for _, z := range azs {
		if z.Available() {
			zones.Add(z.Name())
		}
	}

	// Process all the zones that the provisioner knows about.
	newAZMs := task.availabilityZoneMachines[:0]
	for _, azm := range task.availabilityZoneMachines {
		// Provider has the zone as available, and we know it. All good.
		if zones.Contains(azm.ZoneName) {
			newAZMs = append(newAZMs, azm)
			zones.Remove(azm.ZoneName)
			continue
		}

		// If the zone isn't available, but we think we have machines there,
		// play it safe and retain the entry.
		if len(azm.MachineIds) > 0 {
			task.logger.Warningf(ctx, "machines %v are in zone %q, which is not available, or not known by the cloud",
				azm.MachineIds.Values(), azm.ZoneName)
			newAZMs = append(newAZMs, azm)
		}

		// Fallthrough is for the zone's entry to be dropped.
		// We don't retain it for newAZMs.
		// The new list is logged by the caller.
	}
	task.availabilityZoneMachines = newAZMs

	// Add any remaining zones to the list.
	// Since this method is only called if we have previously populated the
	// zone-machines slice, we can't have provisioned machines in the zone yet.
	for _, z := range zones.Values() {
		task.availabilityZoneMachines = append(task.availabilityZoneMachines, &AvailabilityZoneMachine{
			ZoneName:           z,
			MachineIds:         set.NewStrings(),
			FailedMachineIds:   set.NewStrings(),
			ExcludedMachineIds: set.NewStrings(),
		})
	}
	return nil
}

// populateDistributionGroupZoneMap returns a zone mapping which only includes
// machines in the same distribution group.  This is used to determine where new
// machines in that distribution group should be placed.
func (task *provisionerTask) populateDistributionGroupZoneMap(machineIds []string) []*AvailabilityZoneMachine {
	var dgAvailabilityZoneMachines []*AvailabilityZoneMachine
	dgSet := set.NewStrings(machineIds...)
	for _, azm := range task.availabilityZoneMachines {
		dgAvailabilityZoneMachines = append(dgAvailabilityZoneMachines, &AvailabilityZoneMachine{
			ZoneName:           azm.ZoneName,
			MachineIds:         azm.MachineIds.Intersection(dgSet),
			FailedMachineIds:   azm.FailedMachineIds,
			ExcludedMachineIds: azm.ExcludedMachineIds,
		})
	}
	return dgAvailabilityZoneMachines
}

// machineAvailabilityZoneDistribution returns a suggested availability zone
// for the specified machine to start in.
// If the current provider does not implement availability zones, "" and no
// error will be returned.
// Machines are spread across availability zones based on lowest population of
// the "available" zones, and any supplied zone constraints.
// Machines in the same DistributionGroup are placed in different zones,
// distributed based on lowest population of machines in that DistributionGroup.
// Machines are not placed in a zone they are excluded from.
// If availability zones are implemented and one isn't found, return NotFound error.
func (task *provisionerTask) machineAvailabilityZoneDistribution(
	ctx context.Context,
	machineId string, distGroupMachineIds []string, cons constraints.Value,
) (string, error) {
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()

	if len(task.availabilityZoneMachines) == 0 {
		return "", nil
	}

	// Assign an initial zone to a machine based on lowest population,
	// accommodating any supplied zone constraints.
	// If the machine has a distribution group, assign based on lowest zone
	// population of the distribution group machine.
	// If more than one zone has the same number of machines, pick one of those at random.
	zoneMachines := task.availabilityZoneMachines
	if len(distGroupMachineIds) > 0 {
		zoneMachines = task.populateDistributionGroupZoneMap(distGroupMachineIds)
	}

	// Make a map of zone machines keyed on count.
	zoneMap := make(map[int][]*AvailabilityZoneMachine)
	for _, zm := range zoneMachines {
		machineCount := zm.MachineIds.Size()
		zoneMap[machineCount] = append(zoneMap[machineCount], zm)
	}
	// Sort the counts we have by size so
	// we can process starting with the lowest.
	var zoneCounts []int
	for k := range zoneMap {
		zoneCounts = append(zoneCounts, k)
	}
	sort.Ints(zoneCounts)

	var machineZone string
done:
	// Starting with the lowest count first, find a suitable AZ.
	for _, count := range zoneCounts {
		zmList := zoneMap[count]
		for len(zmList) > 0 {
			// Pick a random AZ to try.
			index := rand.Intn(len(zmList))
			zoneMachines := zmList[index]
			if !zoneMachines.MatchesConstraints(cons) {
				task.logger.Debugf(ctx, "machine %s does not match az %s: constraints do not match",
					machineId, zoneMachines.ZoneName)
			} else if zoneMachines.FailedMachineIds.Contains(machineId) {
				task.logger.Debugf(ctx, "machine %s does not match az %s: excluded in failed machine ids",
					machineId, zoneMachines.ZoneName)
			} else if zoneMachines.ExcludedMachineIds.Contains(machineId) {
				task.logger.Debugf(ctx, "machine %s does not match az %s: excluded machine id",
					machineId, zoneMachines.ZoneName)
			} else {
				// Success, we're out of here.
				machineZone = zoneMachines.ZoneName
				break done
			}
			// Zone not suitable so remove it from the list and try the next one.
			zmList = append(zmList[:index], zmList[index+1:]...)
		}
	}

	if machineZone == "" {
		return machineZone, errors.NotFoundf("suitable availability zone for machine %v", machineId)
	}

	for _, zoneMachines := range task.availabilityZoneMachines {
		if zoneMachines.ZoneName == machineZone {
			zoneMachines.MachineIds.Add(machineId)
			break
		}
	}
	return machineZone, nil
}

// queueStartMachines resolves the distribution groups for the provided
// machines and enqueues a request for starting each one. If the distribution
// group resolution fails for a particular machine, the method will set the
// machine status and immediately return with an error if that operation fails.
// Any provisioning-related errors are reported asynchronously by the worker
// pool.
func (task *provisionerTask) queueStartMachines(ctx context.Context, machines []apiprovisioner.MachineProvisioner) error {
	if len(machines) == 0 {
		return nil
	}

	// Get the distributionGroups for each machine now to avoid
	// successive calls to DistributionGroupByMachineId which will
	// return the same data.
	machineTags := make([]names.MachineTag, len(machines))
	for i, machine := range machines {
		machineTags[i] = machine.MachineTag()
	}
	machineDistributionGroups, err := task.distributionGroupFinder.DistributionGroupByMachineId(ctx, machineTags...)
	if err != nil {
		return errors.Trace(err)
	}

	// Get all the provisioning info at once, so that we don't make many
	// singular requests in parallel to an API that supports batching.
	// key the results by machine IDs for retrieval in the loop below.
	// We rely here on the API guarantee - that the returned results are
	// ordered to correspond to the call arguments.
	pInfoResults, err := task.machinesAPI.ProvisioningInfo(ctx, machineTags)
	if err != nil {
		return errors.Trace(err)
	}
	task.logger.Debugf(ctx, "obtained provisioning info: %#v", pInfoResults)
	pInfoMap := make(map[string]params.ProvisioningInfoResult, len(pInfoResults.Results))
	for i, tag := range machineTags {
		pInfoMap[tag.Id()] = pInfoResults.Results[i]
	}

	for i, m := range machines {
		if machineDistributionGroups[i].Err != nil {
			if err := task.setErrorStatus(ctx, "fetching distribution groups for machine %q: %v", m, machineDistributionGroups[i].Err); err != nil {
				return errors.Trace(err)
			}
			continue
		}

		// Create and enqueue start instance request.  Keep track of
		// the pending request so that if a deletion request comes in
		// before the machine has completed provisioning we can defer
		// it until it does.
		task.machinesMutex.Lock()
		if _, alreadyStarting := task.machinesStarting[m.Id()]; alreadyStarting {
			task.machinesMutex.Unlock()
			task.logger.Debugf(ctx, "machine %q already being started", m.Id())
			// Already being started, skip.
			continue
		}
		task.machinesStarting[m.Id()] = true
		task.machinesMutex.Unlock()

		// Reassign the loop variable to prevent
		// overwriting the dispatched references.
		machine := m
		distGroup := machineDistributionGroups[i].MachineIds

		provTask := workerpool.Task{
			Type: fmt.Sprintf("start-instance %s", machine.Id()),
			Process: func() error {
				machID := machine.Id()

				if provisionErr := task.doStartMachine(ctx, machine, distGroup, pInfoMap[machID]); provisionErr != nil {
					return provisionErr
				}

				task.machinesMutex.Lock()
				delete(task.machinesStarting, machID)
				// If the provisioning succeeded but a deletion
				// request has been deferred queue it now.
				stopDeferred := task.machinesStopDeferred[machID]
				alreadyStopping := task.machinesStopping[machID]
				if stopDeferred && !alreadyStopping {
					delete(task.machinesStopDeferred, machID)
					task.machinesMutex.Unlock()

					task.logger.Debugf(ctx, "triggering deferred stop of machine %q", machID)
					return task.queueRemovalOfDeadMachines(ctx, task.filterDeadMachines([]apiprovisioner.MachineProvisioner{
						machine,
					}))
				}
				task.machinesMutex.Unlock()

				return nil
			},
		}

		select {
		case task.wp.Queue() <- provTask:
			// successfully enqueued provision request
		case <-task.catacomb.Dying():
			return task.catacomb.ErrDying()
		case <-task.wp.Done():
			// Capture and surface asynchronous worker pool errors.
			return task.wp.Close()
		}
	}

	return nil
}

func (task *provisionerTask) setErrorStatus(ctx context.Context, msg string, machine apiprovisioner.MachineProvisioner, err error) error {
	task.logger.Errorf(ctx, msg, machine, err)
	errForStatus := errors.Cause(err)
	if err2 := machine.SetInstanceStatus(ctx, status.ProvisioningError, errForStatus.Error(), nil); err2 != nil {
		// Something is wrong with this machine, better report it back.
		return errors.Annotatef(err2, "setting error status for machine %q", machine)
	}
	return nil
}

func (task *provisionerTask) doStartMachine(
	ctx context.Context,
	machine apiprovisioner.MachineProvisioner,
	distributionGroupMachineIds []string,
	pInfoResult params.ProvisioningInfoResult,
) (startErr error) {
	defer func() {
		if startErr == nil {
			return
		}

		// Mask the error if the machine has the deferred stop flag set.
		// A stop request will be triggered immediately once this
		// method returns.
		task.machinesMutex.RLock()
		defer task.machinesMutex.RUnlock()
		machID := machine.Id()
		if task.machinesStopDeferred[machID] {
			task.logger.Tracef(ctx, "doStartMachine: ignoring doStartMachine error (%v) for machine %q; machine has been marked dead while it was being started and has the deferred stop flag set", startErr, machID)
			startErr = nil
		}
	}()

	if err := machine.SetInstanceStatus(ctx, status.Provisioning, "starting", nil); err != nil {
		task.logger.Errorf(ctx, "%v", err)
	}

	v, err := machine.ModelAgentVersion(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	startInstanceParams, err := task.setupToStartMachine(ctx, machine, v, pInfoResult)
	if err != nil {
		return errors.Trace(task.setErrorStatus(ctx, "%v %v", machine, err))
	}

	// Figure out if the zones available to use for a new instance are
	// restricted based on placement, and if so exclude those machines
	// from being started in any other zone.
	if err := task.populateExcludedMachines(ctx, machine.Id(), startInstanceParams); err != nil {
		return errors.Trace(err)
	}

	// TODO ProvisionerParallelization 2017-10-03
	// Improve the retry loop, newer methodology
	// Is rate limiting handled correctly?
	var result *environs.StartInstanceResult

	// Attempt creating the instance "retryCount" times. If the provider
	// supports availability zones and we're automatically distributing
	// across the zones, then we try each zone for every attempt, or until
	// one of the StartInstance calls returns an error satisfying
	// Is(err, environs.ErrAvailabilityZoneIndependent)
	for attemptsLeft := task.retryStartInstanceStrategy.RetryCount; attemptsLeft >= 0; {
		if startInstanceParams.AvailabilityZone, err = task.machineAvailabilityZoneDistribution(
			ctx,
			machine.Id(), distributionGroupMachineIds, startInstanceParams.Constraints,
		); err != nil {
			return task.setErrorStatus(ctx, "cannot start instance for machine %q: %v", machine, err)
		}
		if startInstanceParams.AvailabilityZone != "" {
			task.logger.Infof(ctx, "trying machine %s StartInstance in availability zone %s",
				machine, startInstanceParams.AvailabilityZone)
		}

		attemptResult, err := task.broker.StartInstance(ctx, startInstanceParams)
		if err == nil {
			result = attemptResult
			break
		} else if attemptsLeft <= 0 {
			// Set the state to error, so the machine will be skipped
			// next time until the error is resolved.
			task.removeMachineFromAZMap(machine)
			return task.setErrorStatus(ctx, "cannot start instance for machine %q: %v", machine, err)
		} else {
			if startInstanceParams.AvailabilityZone != "" {
				task.logger.Warningf(ctx, "machine %s failed to start in availability zone %s: %v",
					machine, startInstanceParams.AvailabilityZone, err)
			} else {
				task.logger.Warningf(ctx, "machine %s failed to start: %v", machine, err)
			}
		}

		retrying := true
		retryMsg := ""
		if startInstanceParams.AvailabilityZone != "" && !errors.Is(err, environs.ErrAvailabilityZoneIndependent) {
			// We've specified a zone, and the error may be specific to
			// that zone. Retry in another zone if there are any untried.
			azRemaining, err2 := task.markMachineFailedInAZ(machine,
				startInstanceParams.AvailabilityZone, startInstanceParams.Constraints)
			if err2 != nil {
				if err = task.setErrorStatus(ctx, "cannot start instance: %v", machine, err2); err != nil {
					task.logger.Errorf(ctx, "setting error status: %s", err)
				}
				return err2
			}
			if azRemaining {
				retryMsg = fmt.Sprintf(
					"failed to start machine %s in zone %q, retrying in %v with new availability zone: %s",
					machine, startInstanceParams.AvailabilityZone,
					task.retryStartInstanceStrategy.RetryDelay, err,
				)
				task.logger.Debugf(ctx, "%s", retryMsg)
				// There's still more zones to try, so don't decrement "attemptsLeft" yet.
				retrying = false
			} else {
				// All availability zones have been attempted for this iteration,
				// clear the failures for the next time around. A given zone may
				// succeed after a prior failure.
				task.clearMachineAZFailures(machine)
			}
		}
		if retrying {
			retryMsg = fmt.Sprintf(
				"failed to start machine %s (%s), retrying in %v (%d more attempts)",
				machine, err.Error(), task.retryStartInstanceStrategy.RetryDelay, attemptsLeft,
			)
			task.logger.Warningf(ctx, "%s", retryMsg)
			attemptsLeft--
		}

		if err3 := machine.SetInstanceStatus(ctx, status.Provisioning, retryMsg, nil); err3 != nil {
			task.logger.Warningf(ctx, "failed to set instance status: %v", err3)
		}

		select {
		case <-task.catacomb.Dying():
			return task.catacomb.ErrDying()
		case <-time.After(task.retryStartInstanceStrategy.RetryDelay):
		}
	}

	networkConfig := params.NetworkConfigFromInterfaceInfo(result.NetworkInfo)
	volumes := volumesToAPIServer(result.Volumes)
	volumeNameToAttachmentInfo := volumeAttachmentsToAPIServer(result.VolumeAttachments)
	instanceID := result.Instance.Id()

	// TODO(nvinuesa): The charm LXD profiles will have to be re-wired once
	// they are implemented as a dqlite domain.
	// Gather the charm LXD profile names, including the lxd profile names from
	// the container brokers.
	charmLXDProfiles, err := task.gatherCharmLXDProfiles(
		ctx,
		instanceID.String(), machine.Tag().Id(), startInstanceParams.CharmLXDProfiles)
	if err != nil {
		return errors.Trace(err)
	}

	if err := task.getMachineInstanceInfoSetter(machine)(
		ctx,
		instanceID,
		result.DisplayName,
		startInstanceParams.InstanceConfig.MachineNonce,
		result.Hardware,
		networkConfig,
		volumes,
		volumeNameToAttachmentInfo,
		charmLXDProfiles,
	); err != nil {
		// We need to stop the instance right away here, set error status and go on.
		if err2 := task.setErrorStatus(ctx, "cannot register instance for machine %v: %v", machine, err); err2 != nil {
			task.logger.Errorf(ctx, "%v", errors.Annotate(err2, "setting machine status"))
		}
		if err2 := task.broker.StopInstances(ctx, instanceID); err2 != nil {
			task.logger.Errorf(ctx, "%v", errors.Annotate(err2, "after failing to set instance info"))
		}
		return errors.Annotate(err, "setting instance info")
	}
	task.logger.Infof(ctx,
		"started machine %s as instance %s with hardware %q, network config %+v, "+
			"volumes %v, volume attachments %v, subnets to zones %v, lxd profiles %v",
		machine,
		instanceID,
		result.Hardware,
		networkConfig,
		volumes,
		volumeNameToAttachmentInfo,
		startInstanceParams.SubnetsToZones,
		startInstanceParams.CharmLXDProfiles,
	)
	return nil
}

// setupToStartMachine gathers the necessary information,
// based on the specified machine, to create ProvisioningInfo
// and StartInstanceParams to be used by startMachine.
func (task *provisionerTask) setupToStartMachine(
	ctx context.Context,
	machine apiprovisioner.MachineProvisioner, version *semversion.Number, pInfoResult params.ProvisioningInfoResult,
) (environs.StartInstanceParams, error) {
	// Check that we have a result.
	// We should never have an empty result without an error,
	// but we guard for that conservatively.
	if pInfoResult.Error != nil {
		return environs.StartInstanceParams{}, *pInfoResult.Error
	}
	pInfo := pInfoResult.Result
	if pInfo == nil {
		return environs.StartInstanceParams{}, errors.Errorf("no provisioning info for machine %q", machine.Id())
	}

	instanceCfg, err := task.constructInstanceConfig(ctx, machine, pInfo)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "creating instance config for machine %q", machine)
	}

	// We default to amd64 unless otherwise specified.
	agentArch := arch.DefaultArchitecture
	if pInfo.Constraints.Arch != nil {
		agentArch = *pInfo.Constraints.Arch
	}

	possibleTools, err := task.toolsFinder.FindTools(ctx, *version, pInfo.Base.Name, agentArch)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "finding agent binaries for machine %q", machine)
	}

	startInstanceParams, err := task.constructStartInstanceParams(
		task.controllerUUID,
		machine,
		instanceCfg,
		pInfo,
		possibleTools,
	)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "constructing params for machine %q", machine)
	}

	return startInstanceParams, nil
}

// populateExcludedMachines, translates the results of DeriveAvailabilityZones
// into availabilityZoneMachines.ExcludedMachineIds for machines not to be used
// in the given zone.
func (task *provisionerTask) populateExcludedMachines(ctx context.Context, machineId string, startInstanceParams environs.StartInstanceParams) error {
	zonedEnv, ok := task.broker.(providercommon.ZonedEnviron)
	if !ok {
		return nil
	}
	derivedZones, err := zonedEnv.DeriveAvailabilityZones(ctx, startInstanceParams)
	if err != nil {
		return errors.Trace(err)
	}
	if len(derivedZones) == 0 {
		return nil
	}
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	useZones := set.NewStrings(derivedZones...)
	for _, zoneMachines := range task.availabilityZoneMachines {
		if !useZones.Contains(zoneMachines.ZoneName) {
			zoneMachines.ExcludedMachineIds.Add(machineId)
		}
	}
	return nil
}

// gatherCharmLXDProfiles consumes the charms LXD Profiles from the different
// sources. This includes getting the information from the broker.
func (task *provisionerTask) gatherCharmLXDProfiles(
	ctx context.Context,
	instanceID, machineTag string, machineProfiles []string,
) ([]string, error) {
	if !names.IsContainerMachine(machineTag) {
		return machineProfiles, nil
	}

	manager, ok := task.broker.(container.LXDProfileNameRetriever)
	if !ok {
		task.logger.Tracef(ctx, "failed to gather profile names, broker didn't conform to LXDProfileNameRetriever")
		return machineProfiles, nil
	}

	profileNames, err := manager.LXDProfileNames(instanceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return lxdprofile.FilterLXDProfileNames(profileNames), nil
}

// markMachineFailedInAZ moves the machine in zone from MachineIds to FailedMachineIds
// in availabilityZoneMachines, report if there are any availability zones not failed for
// the specified machine.
func (task *provisionerTask) markMachineFailedInAZ(machine apiprovisioner.MachineProvisioner, zone string,
	cons constraints.Value) (bool, error) {
	if zone == "" {
		return false, errors.New("no zone provided")
	}
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for _, zoneMachines := range task.availabilityZoneMachines {
		if zone == zoneMachines.ZoneName {
			zoneMachines.MachineIds.Remove(machine.Id())
			zoneMachines.FailedMachineIds.Add(machine.Id())
			break
		}
	}

	// Check if there are any zones left to try (that also match constraints).
	for _, zoneMachines := range task.availabilityZoneMachines {
		if zoneMachines.MatchesConstraints(cons) &&
			!zoneMachines.FailedMachineIds.Contains(machine.Id()) &&
			!zoneMachines.ExcludedMachineIds.Contains(machine.Id()) {
			return true, nil
		}
	}
	return false, nil
}

func (task *provisionerTask) clearMachineAZFailures(machine apiprovisioner.MachineProvisioner) {
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for _, zoneMachines := range task.availabilityZoneMachines {
		zoneMachines.FailedMachineIds.Remove(machine.Id())
	}
}

// removeMachineFromAZMap removes the specified machine from availabilityZoneMachines.
// It is assumed this is called when the machines are being deleted from state, or failed
// provisioning.
func (task *provisionerTask) removeMachineFromAZMap(machine apiprovisioner.MachineProvisioner) {
	machineId := machine.Id()
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for _, zoneMachines := range task.availabilityZoneMachines {
		zoneMachines.MachineIds.Remove(machineId)
		zoneMachines.FailedMachineIds.Remove(machineId)
	}
}

// subnetZonesFromNetworkTopology denormalises the topology passed from the API
// server into a slice of subnet to AZ list maps, one for each listed space.
func subnetZonesFromNetworkTopology(topology params.ProvisioningNetworkTopology) []map[corenetwork.Id][]string {
	if len(topology.SpaceSubnets) == 0 {
		return nil
	}

	// We want to ensure consistent ordering of the return based on the spaces.
	spaceNames := make([]string, 0, len(topology.SpaceSubnets))
	for spaceName := range topology.SpaceSubnets {
		spaceNames = append(spaceNames, spaceName)
	}
	sort.Strings(spaceNames)

	subnetsToZones := make([]map[corenetwork.Id][]string, 0, len(spaceNames))
	for _, spaceName := range spaceNames {
		subnetAZs := make(map[corenetwork.Id][]string)
		for _, subnet := range topology.SpaceSubnets[spaceName] {
			subnetAZs[corenetwork.Id(subnet)] = topology.SubnetAZs[subnet]
		}
		subnetsToZones = append(subnetsToZones, subnetAZs)
	}
	return subnetsToZones
}

func volumesToAPIServer(volumes []storage.Volume) []params.Volume {
	result := make([]params.Volume, len(volumes))
	for i, v := range volumes {
		result[i] = params.Volume{
			VolumeTag: v.Tag.String(),
			Info: params.VolumeInfo{
				ProviderId: v.VolumeId,
				HardwareId: v.HardwareId,
				WWN:        v.WWN, // pool
				Size:       v.Size,
				Persistent: v.Persistent,
			},
		}
	}
	return result
}

func volumeAttachmentsToAPIServer(attachments []storage.VolumeAttachment) map[string]params.VolumeAttachmentInfo {
	result := make(map[string]params.VolumeAttachmentInfo)
	for _, a := range attachments {

		// Volume attachment plans are used in the OCI provider where actions
		// are required on the instance itself in order to complete attachments
		// of SCSI volumes.
		// TODO (manadart 2020-02-04): I believe this code path to be untested.
		var planInfo *params.VolumeAttachmentPlanInfo
		if a.PlanInfo != nil {
			planInfo = &params.VolumeAttachmentPlanInfo{
				DeviceType:       a.PlanInfo.DeviceType,
				DeviceAttributes: a.PlanInfo.DeviceAttributes,
			}
		}

		result[a.Volume.String()] = params.VolumeAttachmentInfo{
			DeviceName: a.DeviceName,
			DeviceLink: a.DeviceLink,
			BusAddress: a.BusAddress,
			ReadOnly:   a.ReadOnly,
			PlanInfo:   planInfo,
		}
	}
	return result
}
