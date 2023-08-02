// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	stdcontext "context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/workerpool"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/wrench"
)

type ProvisionerTask interface {
	worker.Worker

	// SetHarvestMode sets a flag to indicate how the provisioner task
	// should harvest machines. See config.HarvestMode for
	// documentation of behavior.
	SetHarvestMode(mode config.HarvestMode)

	// SetNumProvisionWorkers resizes the pool of provision workers.
	SetNumProvisionWorkers(numWorkers int)
}

// TaskAPI describes API methods required by a ProvisionerTask.
type TaskAPI interface {
	Machines(...names.MachineTag) ([]apiprovisioner.MachineResult, error)
	MachinesWithTransientErrors() ([]apiprovisioner.MachineStatusResult, error)
	ProvisioningInfo(machineTags []names.MachineTag) (params.ProvisioningInfoResults, error)
}

type DistributionGroupFinder interface {
	DistributionGroupByMachineId(...names.MachineTag) ([]apiprovisioner.DistributionGroupResult, error)
}

// ToolsFinder is an interface used for finding tools to run on
// provisioned instances.
type ToolsFinder interface {
	// FindTools returns a list of tools matching the specified
	// version, os, and architecture. If arch is empty, the
	// implementation is expected to use a well documented default.
	FindTools(version version.Number, os string, arch string) (coretools.List, error)
}

// TaskConfig holds the initialisation data for a ProvisionerTask instance.
type TaskConfig struct {
	ControllerUUID             string
	HostTag                    names.Tag
	Logger                     Logger
	HarvestMode                config.HarvestMode
	TaskAPI                    TaskAPI
	DistributionGroupFinder    DistributionGroupFinder
	ToolsFinder                ToolsFinder
	MachineWatcher             watcher.StringsWatcher
	RetryWatcher               watcher.NotifyWatcher
	Broker                     environs.InstanceBroker
	Auth                       authentication.AuthenticationProvider
	ImageStream                string
	RetryStartInstanceStrategy RetryStrategy
	CloudCallContextFunc       common.CloudCallContextFunc
	NumProvisionWorkers        int
	EventProcessedCb           func(string)
}

func NewProvisionerTask(cfg TaskConfig) (ProvisionerTask, error) {
	machineChanges := cfg.MachineWatcher.Changes()
	workers := []worker.Worker{cfg.MachineWatcher}
	var retryChanges watcher.NotifyChannel
	if cfg.RetryWatcher != nil {
		retryChanges = cfg.RetryWatcher.Changes()
		workers = append(workers, cfg.RetryWatcher)
	}
	task := &provisionerTask{
		controllerUUID:             cfg.ControllerUUID,
		hostTag:                    cfg.HostTag,
		logger:                     cfg.Logger,
		taskAPI:                    cfg.TaskAPI,
		distributionGroupFinder:    cfg.DistributionGroupFinder,
		toolsFinder:                cfg.ToolsFinder,
		machineChanges:             machineChanges,
		retryChanges:               retryChanges,
		broker:                     cfg.Broker,
		auth:                       cfg.Auth,
		harvestMode:                cfg.HarvestMode,
		harvestModeChan:            make(chan config.HarvestMode, 1),
		machines:                   make(map[string]apiprovisioner.MachineProvisioner),
		machinesStarting:           make(map[string]bool),
		machinesStopDeferred:       make(map[string]bool),
		machinesStopping:           make(map[string]bool),
		availabilityZoneMachines:   make([]*AvailabilityZoneMachine, 0),
		imageStream:                cfg.ImageStream,
		retryStartInstanceStrategy: cfg.RetryStartInstanceStrategy,
		cloudCallCtxFunc:           cfg.CloudCallContextFunc,
		wp:                         workerpool.NewWorkerPool(cfg.Logger, cfg.NumProvisionWorkers),
		wpSizeChan:                 make(chan int, 1),
		eventProcessedCb:           cfg.EventProcessedCb,
	}
	err := catacomb.Invoke(catacomb.Plan{
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
	eventTypeHarvestModeChanged        = "harvest-mode-changed"
)

type provisionerTask struct {
	controllerUUID             string
	hostTag                    names.Tag
	logger                     Logger
	taskAPI                    TaskAPI
	distributionGroupFinder    DistributionGroupFinder
	toolsFinder                ToolsFinder
	machineChanges             watcher.StringsChannel
	retryChanges               watcher.NotifyChannel
	broker                     environs.InstanceBroker
	catacomb                   catacomb.Catacomb
	auth                       authentication.AuthenticationProvider
	imageStream                string
	harvestMode                config.HarvestMode
	harvestModeChan            chan config.HarvestMode
	retryStartInstanceStrategy RetryStrategy

	machinesMutex            sync.RWMutex
	machines                 map[string]apiprovisioner.MachineProvisioner // machine ID -> machine
	machinesStarting         map[string]bool                              // machine IDs currently being started.
	machinesStopping         map[string]bool                              // machine IDs currently being stopped.
	machinesStopDeferred     map[string]bool                              // machine IDs which were set as dead while starting. They will be stopped once they are online.
	availabilityZoneMachines []*AvailabilityZoneMachine
	instances                map[instance.Id]instances.Instance // instanceID -> instance
	cloudCallCtxFunc         common.CloudCallContextFunc

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
	task.logger.Infof("entering provisioner task loop; using provisioner pool with %d workers", task.wp.Size())
	defer func() {
		wpErr := task.wp.Close()
		if taskErr == nil {
			taskErr = wpErr
		}
		task.logger.Infof("exiting provisioner task loop; err: %v", taskErr)
	}()

	// Don't allow the harvesting mode to change until we have read at
	// least one set of changes, which will populate the task.machines
	// map. Otherwise we will potentially see all legitimate instances
	// as unknown.
	var harvestModeChan chan config.HarvestMode

	// When the watcher is started, it will have the initial changes be all
	// the machines that are relevant. Also, since this is available straight
	// away, we know there will be some changes right off the bat.
	ctx := task.cloudCallCtxFunc(stdcontext.Background())
	for {
		select {
		case ids, ok := <-task.machineChanges:
			if !ok {
				return errors.New("machine watcher closed channel")
			}

			if err := task.processMachines(ctx, ids); err != nil {
				return errors.Annotate(err, "processing updated machines")
			}

			task.notifyEventProcessedCallback(eventTypeProcessedMachines)

			// We've seen a set of changes.
			// Enable modification of harvesting mode.
			harvestModeChan = task.harvestModeChan
		case numWorkers := <-task.wpSizeChan:
			if task.wp.Size() == numWorkers {
				continue // nothing to do
			}

			// Stop the current pool (checking for any pending
			// errors) and create a new one.
			task.logger.Infof("resizing provision worker pool size to %d", numWorkers)
			if err := task.wp.Close(); err != nil {
				return err
			}
			task.wp = workerpool.NewWorkerPool(task.logger, numWorkers)
			task.notifyEventProcessedCallback(eventTypeResizedWorkerPool)
		case harvestMode := <-harvestModeChan:
			if harvestMode == task.harvestMode {
				break
			}
			task.logger.Infof("harvesting mode changed to %s", harvestMode)
			task.harvestMode = harvestMode
			task.notifyEventProcessedCallback(eventTypeHarvestModeChanged)
			if harvestMode.HarvestUnknown() {
				task.logger.Infof("harvesting unknown machines")
				if err := task.processMachines(ctx, nil); err != nil {
					return errors.Annotate(err, "processing machines after safe mode disabled")
				}
				task.notifyEventProcessedCallback(eventTypeProcessedMachines)
			}
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
		case <-task.catacomb.Dying():
			return task.catacomb.ErrDying()
		}
	}
}

func (task *provisionerTask) notifyEventProcessedCallback(evtType string) {
	if task.eventProcessedCb != nil {
		task.eventProcessedCb(evtType)
	}
}

// SetHarvestMode implements ProvisionerTask.SetHarvestMode().
func (task *provisionerTask) SetHarvestMode(mode config.HarvestMode) {
	select {
	case task.harvestModeChan <- mode:
	case <-task.catacomb.Dying():
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

func (task *provisionerTask) processMachinesWithTransientErrors(ctx context.ProviderCallContext) error {
	results, err := task.taskAPI.MachinesWithTransientErrors()
	if err != nil || len(results) == 0 {
		return nil
	}
	task.logger.Tracef("processMachinesWithTransientErrors(%v)", results)
	var pending []apiprovisioner.MachineProvisioner
	for _, result := range results {
		if result.Status.Error != nil {
			task.logger.Errorf("cannot retry provisioning of machine %q: %v", result.Machine.Id(), result.Status.Error)
			continue
		}
		machine := result.Machine
		if err := machine.SetStatus(status.Pending, "", nil); err != nil {
			task.logger.Errorf("cannot reset status of machine %q: %v", machine.Id(), err)
			continue
		}
		if err := machine.SetInstanceStatus(status.Provisioning, "", nil); err != nil {
			task.logger.Errorf("cannot reset instance status of machine %q: %v", machine.Id(), err)
			continue
		}
		if err := machine.SetModificationStatus(status.Idle, "", nil); err != nil {
			task.logger.Errorf("cannot reset modification status of machine %q: %v", machine.Id(), err)
			continue
		}
		task.machinesMutex.Lock()
		task.machines[machine.Tag().String()] = machine
		task.machinesMutex.Unlock()
		pending = append(pending, machine)
	}
	return task.queueStartMachines(ctx, pending)
}

func (task *provisionerTask) processMachines(ctx context.ProviderCallContext, ids []string) error {
	task.logger.Tracef("processMachines(%v)", ids)

	// Populate the tasks maps of current instances and machines.
	if err := task.populateMachineMaps(ctx, ids); err != nil {
		return errors.Trace(err)
	}

	// Maintain zone-machine distributions.
	err := task.updateAvailabilityZoneMachines(ctx)
	if err != nil && !errors.IsNotImplemented(err) {
		return errors.Annotate(err, "updating AZ distributions")
	}

	// Find machines without an instance ID or that are dead.
	pending, dead, err := task.pendingOrDead(ids)
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
func (task *provisionerTask) populateMachineMaps(ctx context.ProviderCallContext, ids []string) error {
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
	machines, err := task.taskAPI.Machines(machineTags...)
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
			task.logger.Debugf("machine %q not found in state", ids[i])
			delete(task.machines, ids[i])
		default:
			return errors.Annotatef(result.Err, "getting machine %v", ids[i])
		}
	}
	return nil
}

// pendingOrDead looks up machines with ids and returns those that do not
// have an instance id assigned yet, and also those that are dead. Any machines
// that are currently being stopped or have been marked for deferred stopping
// once they are online will be skipped.
func (task *provisionerTask) pendingOrDead(
	ids []string,
) (pending, dead []apiprovisioner.MachineProvisioner, err error) {
	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()
	for _, id := range ids {
		// Ignore machines that have been either queued for deferred
		// stopping or they are currently stopping
		if _, found := task.machinesStopDeferred[id]; found {
			task.logger.Tracef("pendingOrDead: ignoring machine %q; machine has deferred stop flag set", id)
			continue // ignore: will be stopped once started
		} else if _, found := task.machinesStopping[id]; found {
			task.logger.Tracef("pendingOrDead: ignoring machine %q; machine is currently being stopped", id)
			continue // ignore: currently being stopped.
		}

		machine, found := task.machines[id]
		if !found {
			task.logger.Infof("machine %q not found", id)
			continue
		}
		var classification MachineClassification
		classification, err = classifyMachine(task.logger, machine)
		if err != nil {
			return // return the error
		}
		switch classification {
		case Pending:
			pending = append(pending, machine)
		case Dead:
			dead = append(dead, machine)
		}
	}
	task.logger.Tracef("pending machines: %v", pending)
	task.logger.Tracef("dead machines: %v", dead)
	return
}

type ClassifiableMachine interface {
	Life() life.Value
	InstanceId() (instance.Id, error)
	EnsureDead() error
	Status() (status.Status, string, error)
	InstanceStatus() (status.Status, string, error)
	Id() string
}

type MachineClassification string

const (
	None    MachineClassification = "none"
	Pending MachineClassification = "Pending"
	Dead    MachineClassification = "Dead"
)

func classifyMachine(logger Logger, machine ClassifiableMachine) (
	MachineClassification, error) {
	switch machine.Life() {
	case life.Dying:
		if _, err := machine.InstanceId(); err == nil {
			return None, nil
		} else if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "loading dying machine id:%s, details:%v", machine.Id(), machine)
		}
		logger.Infof("killing dying, unprovisioned machine %q", machine)
		if err := machine.EnsureDead(); err != nil {
			return None, errors.Annotatef(err, "ensuring machine dead id:%s, details:%v", machine.Id(), machine)
		}
		fallthrough
	case life.Dead:
		return Dead, nil
	}
	instId, err := machine.InstanceId()
	if err != nil {
		if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "loading machine id:%s, details:%v", machine.Id(), machine)
		}
		machineStatus, _, err := machine.Status()
		if err != nil {
			logger.Infof("cannot get machine id:%s, details:%v, err:%v", machine.Id(), machine, err)
			return None, nil
		}
		if machineStatus == status.Pending {
			logger.Infof("found machine pending provisioning id:%s, details:%v", machine.Id(), machine)
			return Pending, nil
		}
		instanceStatus, _, err := machine.InstanceStatus()
		if err != nil {
			logger.Infof("cannot read instance status id:%s, details:%v, err:%v", machine.Id(), machine, err)
			return None, nil
		}
		if instanceStatus == status.Provisioning {
			logger.Infof("found machine provisioning id:%s, details:%v", machine.Id(), machine)
			return Pending, nil
		}
		return None, nil
	}
	logger.Infof("machine %s already started as instance %q", machine.Id(), instId)

	return None, nil
}

// findUnknownInstances finds instances which are not associated with a machine.
func (task *provisionerTask) findUnknownInstances(stopping []instances.Instance) ([]instances.Instance, error) {
	// Make a copy of the instances we know about.
	taskInstances := make(map[instance.Id]instances.Instance)
	for k, v := range task.instances {
		taskInstances[k] = v
	}

	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()
	for _, m := range task.machines {
		instId, err := m.InstanceId()
		switch {
		case err == nil:
			delete(taskInstances, instId)
		case params.IsCodeNotProvisioned(err):
		case params.IsCodeNotFoundOrCodeUnauthorized(err):
		default:
			return nil, err
		}
	}
	// Now remove all those instances that we are stopping already as we
	// know about those and don't want to include them in the unknown list.
	for _, inst := range stopping {
		delete(taskInstances, inst.Id())
	}
	var unknown []instances.Instance
	for _, inst := range taskInstances {
		unknown = append(unknown, inst)
	}
	return unknown, nil
}

// filterAndQueueRemovalOfDeadMachines scans the list of dead machines and:
//   - Sets the deferred stop flag for machines that are still online
//   - Filters out any machines that are either stopping or have the deferred
//     stop flag set.
//   - Marks the remaining machines as stopping and queues a request for them to
//     be cleaned up.
func (task *provisionerTask) filterAndQueueRemovalOfDeadMachines(ctx context.ProviderCallContext, dead []apiprovisioner.MachineProvisioner) error {
	// Flag any machines in the dead list that are still being started so
	// they will be stopped once they come online.
	task.deferStopForNotYetStartedMachines(dead)

	// Filter the initial dead machine list. Any machines marked for
	// deferred stopping, machines that are already being stopped and
	// machines that have not yet finished provisioning will be removed
	// from the filtered list.
	dead = task.filterDeadMachines(dead)

	// The remaining machines will be removed asynchronously and this
	// method can be invoked again concurrently to process another machine
	// change event. To avoid attempts to remove the same machines twice,
	// they are flagged as stopping.
	task.machinesMutex.Lock()
	for _, machine := range dead {
		machID := machine.Id()
		if !task.machinesStopDeferred[machID] {
			task.machinesStopping[machID] = true
		}
	}
	task.machinesMutex.Unlock()
	return task.queueRemovalOfDeadMachines(ctx, dead)
}

func (task *provisionerTask) queueRemovalOfDeadMachines(
	ctx context.ProviderCallContext,
	dead []apiprovisioner.MachineProvisioner,
) error {
	// Collect the instances for all provisioned machines that are dead.
	stopping := task.instancesForDeadMachines(dead)

	// Find running instances that have no machines associated.
	unknown, err := task.findUnknownInstances(stopping)
	if err != nil {
		return errors.Trace(err)
	}

	if !task.harvestMode.HarvestUnknown() && len(unknown) != 0 {
		task.logger.Infof(
			"%s is set to %s; unknown instances not stopped %v",
			config.ProvisionerHarvestModeKey,
			task.harvestMode.String(),
			instanceIds(unknown),
		)
		unknown = nil
	}

	if (task.harvestMode.HarvestNone() || !task.harvestMode.HarvestDestroyed()) && len(stopping) != 0 {
		task.logger.Infof(
			`%s is set to "%s"; will not harvest %s`,
			config.ProvisionerHarvestModeKey,
			task.harvestMode.String(),
			instanceIds(stopping),
		)
		stopping = nil
	}

	if len(dead) == 0 {
		return nil // nothing to do
	}

	provTask := workerpool.Task{
		Type: "stop-instances",
		Process: func() error {
			if len(stopping) > 0 {
				task.logger.Infof("stopping known instances %v", instanceIds(stopping))
			}
			if len(unknown) > 0 {
				task.logger.Infof("stopping unknown instances %v", instanceIds(unknown))
			}

			// It is important that we stop unknown instances before starting
			// pending ones, because if we start an instance and then fail to
			// set its InstanceId on the machine.
			// We don't want to start a new instance for the same machine ID.
			if err := task.doStopInstances(ctx, append(stopping, unknown...)); err != nil {
				return errors.Trace(err)
			}

			// Remove any dead machines from state.
			for _, machine := range dead {
				task.logger.Infof("removing dead machine %q", machine.Id())
				if err := machine.MarkForRemoval(); err != nil {
					task.logger.Errorf("failed to remove dead machine %q", machine.Id())
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
func (task *provisionerTask) instancesForDeadMachines(dead []apiprovisioner.MachineProvisioner) []instances.Instance {
	var deadInstances []instances.Instance
	for _, machine := range dead {
		// Ignore machines that are still provisioning
		task.machinesMutex.RLock()
		if task.machinesStarting[machine.Id()] {
			task.machinesMutex.RUnlock()
			continue
		}
		task.machinesMutex.RUnlock()

		instId, err := machine.InstanceId()
		if err == nil {
			keep, _ := machine.KeepInstance()
			if keep {
				task.logger.Debugf("machine %v is dead but keep-instance is true", instId)
				continue
			}

			// If the instance is not found we can't stop it.
			if inst, found := task.instances[instId]; found {
				deadInstances = append(deadInstances, inst)
			}
		}
	}
	return deadInstances
}

func (task *provisionerTask) doStopInstances(ctx context.ProviderCallContext, instances []instances.Instance) error {
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
	machine apiprovisioner.MachineProvisioner,
	auth authentication.AuthenticationProvider,
	pInfo *params.ProvisioningInfo,
) (*instancecfg.InstanceConfig, error) {

	apiInfo, err := auth.SetupAuthentication(machine)
	if err != nil {
		return nil, errors.Annotate(err, "setting up authentication")
	}

	// Generated a nonce for the new instance, with the format: "machine-#:UUID".
	// The first part is a badge, specifying the tag of the machine the provisioner
	// is running on, while the second part is a random UUID.
	uuid, err := utils.NewUUID()
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
		if v.VolumeId == "" {
			return environs.StartInstanceParams{}, errors.Errorf("volume attachment params does not specify volume ID")
		}
		volumeAttachments[i] = storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider: storage.ProviderType(v.Provider),
				Machine:  machineTag,
				ReadOnly: v.ReadOnly,
			},
			Volume:   volumeTag,
			VolumeId: v.VolumeId,
		}
	}

	var endpointBindings map[string]network.Id
	if len(provisioningInfo.EndpointBindings) != 0 {
		endpointBindings = make(map[string]network.Id)
		for endpoint, space := range provisioningInfo.EndpointBindings {
			endpointBindings[endpoint] = network.Id(space)
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
func (task *provisionerTask) updateAvailabilityZoneMachines(ctx context.ProviderCallContext) error {
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
	task.logger.Infof("provisioning in zones: %v", zones)

	return nil
}

// populateAvailabilityZoneMachines populates the slice,
// availabilityZoneMachines, with each zone and the IDs of
// machines running in that zone, according to the provider.
func (task *provisionerTask) populateAvailabilityZoneMachines(
	ctx context.ProviderCallContext, zonedEnv providercommon.ZonedEnviron,
) error {
	availabilityZoneInstances, err := providercommon.AvailabilityZoneAllocations(zonedEnv, ctx, []instance.Id{})
	if err != nil {
		return errors.Trace(err)
	}

	instanceMachines := make(map[instance.Id]string)
	for _, machine := range task.machines {
		instId, err := machine.InstanceId()
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
	ctx context.ProviderCallContext, zonedEnv providercommon.ZonedEnviron,
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
			task.logger.Warningf("machines %v are in zone %q, which is not available, or not known by the cloud",
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
			azm.ZoneName,
			azm.MachineIds.Intersection(dgSet),
			azm.FailedMachineIds,
			azm.ExcludedMachineIds,
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
				task.logger.Debugf("machine %s does not match az %s: constraints do not match",
					machineId, zoneMachines.ZoneName)
			} else if zoneMachines.FailedMachineIds.Contains(machineId) {
				task.logger.Debugf("machine %s does not match az %s: excluded in failed machine ids",
					machineId, zoneMachines.ZoneName)
			} else if zoneMachines.ExcludedMachineIds.Contains(machineId) {
				task.logger.Debugf("machine %s does not match az %s: excluded machine id",
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
func (task *provisionerTask) queueStartMachines(ctx context.ProviderCallContext, machines []apiprovisioner.MachineProvisioner) error {
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
	machineDistributionGroups, err := task.distributionGroupFinder.DistributionGroupByMachineId(machineTags...)
	if err != nil {
		return errors.Trace(err)
	}

	// Get all the provisioning info at once, so that we don't make many
	// singular requests in parallel to an API that supports batching.
	// key the results by machine IDs for retrieval in the loop below.
	// We rely here on the API guarantee - that the returned results are
	// ordered to correspond to the call arguments.
	pInfoResults, err := task.taskAPI.ProvisioningInfo(machineTags)
	if err != nil {
		return errors.Trace(err)
	}
	pInfoMap := make(map[string]params.ProvisioningInfoResult, len(pInfoResults.Results))
	for i, tag := range machineTags {
		pInfoMap[tag.Id()] = pInfoResults.Results[i]
	}

	for i, m := range machines {
		if machineDistributionGroups[i].Err != nil {
			if err := task.setErrorStatus("fetching distribution groups for machine %q: %v", m, machineDistributionGroups[i].Err); err != nil {
				return errors.Trace(err)
			}
			continue
		}

		// Create and enqueue start instance request.  Keep track of
		// the pending request so that if a deletion request comes in
		// before the machine has completed provisioning we can defer
		// it until it does.
		task.machinesMutex.Lock()
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
				if stopDeferred {
					delete(task.machinesStopDeferred, machID)
					task.machinesStopping[machID] = true
				}
				task.machinesMutex.Unlock()

				if stopDeferred {
					task.logger.Debugf("triggering deferred stop of machine %q", machID)
					return task.queueRemovalOfDeadMachines(ctx, []apiprovisioner.MachineProvisioner{
						machine,
					})
				}

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

func (task *provisionerTask) setErrorStatus(msg string, machine apiprovisioner.MachineProvisioner, err error) error {
	task.logger.Errorf(msg, machine, err)
	errForStatus := errors.Cause(err)
	if err2 := machine.SetInstanceStatus(status.ProvisioningError, errForStatus.Error(), nil); err2 != nil {
		// Something is wrong with this machine, better report it back.
		return errors.Annotatef(err2, "setting error status for machine %q", machine)
	}
	return nil
}

func (task *provisionerTask) doStartMachine(
	ctx context.ProviderCallContext,
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
			task.logger.Tracef("doStartMachine: ignoring doStartMachine error (%v) for machine %q; machine has been marked dead while it was being started and has the deferred stop flag set", startErr, machID)
			startErr = nil
		}
	}()

	if err := machine.SetInstanceStatus(status.Provisioning, "starting", nil); err != nil {
		task.logger.Errorf("%v", err)
	}

	v, err := machine.ModelAgentVersion()
	if err != nil {
		return errors.Trace(err)
	}

	startInstanceParams, err := task.setupToStartMachine(machine, v, pInfoResult)
	if err != nil {
		return errors.Trace(task.setErrorStatus("%v %v", machine, err))
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
	for attemptsLeft := task.retryStartInstanceStrategy.retryCount; attemptsLeft >= 0; {
		if startInstanceParams.AvailabilityZone, err = task.machineAvailabilityZoneDistribution(
			machine.Id(), distributionGroupMachineIds, startInstanceParams.Constraints,
		); err != nil {
			return task.setErrorStatus("cannot start instance for machine %q: %v", machine, err)
		}
		if startInstanceParams.AvailabilityZone != "" {
			task.logger.Infof("trying machine %s StartInstance in availability zone %s",
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
			return task.setErrorStatus("cannot start instance for machine %q: %v", machine, err)
		} else {
			if startInstanceParams.AvailabilityZone != "" {
				task.logger.Warningf("machine %s failed to start in availability zone %s: %v",
					machine, startInstanceParams.AvailabilityZone, err)
			} else {
				task.logger.Warningf("machine %s failed to start: %v", machine, err)
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
				if err = task.setErrorStatus("cannot start instance: %v", machine, err2); err != nil {
					task.logger.Errorf("setting error status: %s", err)
				}
				return err2
			}
			if azRemaining {
				retryMsg = fmt.Sprintf(
					"failed to start machine %s in zone %q, retrying in %v with new availability zone: %s",
					machine, startInstanceParams.AvailabilityZone,
					task.retryStartInstanceStrategy.retryDelay, err,
				)
				task.logger.Debugf("%s", retryMsg)
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
				machine, err.Error(), task.retryStartInstanceStrategy.retryDelay, attemptsLeft,
			)
			task.logger.Warningf("%s", retryMsg)
			attemptsLeft--
		}

		if err3 := machine.SetInstanceStatus(status.Provisioning, retryMsg, nil); err3 != nil {
			task.logger.Warningf("failed to set instance status: %v", err3)
		}

		select {
		case <-task.catacomb.Dying():
			return task.catacomb.ErrDying()
		case <-time.After(task.retryStartInstanceStrategy.retryDelay):
		}
	}

	networkConfig := params.NetworkConfigFromInterfaceInfo(result.NetworkInfo)
	volumes := volumesToAPIServer(result.Volumes)
	volumeNameToAttachmentInfo := volumeAttachmentsToAPIServer(result.VolumeAttachments)
	instanceID := result.Instance.Id()

	// Gather the charm LXD profile names, including the lxd profile names from
	// the container brokers.
	charmLXDProfiles, err := task.gatherCharmLXDProfiles(
		string(instanceID), machine.Tag().Id(), startInstanceParams.CharmLXDProfiles)
	if err != nil {
		return errors.Trace(err)
	}

	if err := machine.SetInstanceInfo(
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
		if err2 := task.setErrorStatus("cannot register instance for machine %v: %v", machine, err); err2 != nil {
			task.logger.Errorf("%v", errors.Annotate(err2, "setting machine status"))
		}
		if err2 := task.broker.StopInstances(ctx, instanceID); err2 != nil {
			task.logger.Errorf("%v", errors.Annotate(err2, "after failing to set instance info"))
		}
		return errors.Annotate(err, "setting instance info")
	}

	task.logger.Infof(
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
	machine apiprovisioner.MachineProvisioner, version *version.Number, pInfoResult params.ProvisioningInfoResult,
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

	instanceCfg, err := task.constructInstanceConfig(machine, task.auth, pInfo)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "creating instance config for machine %q", machine)
	}

	// We default to amd64 unless otherwise specified.
	agentArch := arch.DefaultArchitecture
	if pInfo.Constraints.Arch != nil {
		agentArch = *pInfo.Constraints.Arch
	}

	possibleTools, err := task.toolsFinder.FindTools(*version, pInfo.Base.Name, agentArch)
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
func (task *provisionerTask) populateExcludedMachines(ctx context.ProviderCallContext, machineId string, startInstanceParams environs.StartInstanceParams) error {
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
	instanceID, machineTag string, machineProfiles []string,
) ([]string, error) {
	if !names.IsContainerMachine(machineTag) {
		return machineProfiles, nil
	}

	manager, ok := task.broker.(container.LXDProfileNameRetriever)
	if !ok {
		task.logger.Tracef("failed to gather profile names, broker didn't conform to LXDProfileNameRetriever")
		return machineProfiles, nil
	}

	profileNames, err := manager.LXDProfileNames(instanceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return lxdprofile.LXDProfileNames(profileNames), nil
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
func subnetZonesFromNetworkTopology(topology params.ProvisioningNetworkTopology) []map[network.Id][]string {
	if len(topology.SpaceSubnets) == 0 {
		return nil
	}

	// We want to ensure consistent ordering of the return based on the spaces.
	spaceNames := make([]string, 0, len(topology.SpaceSubnets))
	for spaceName := range topology.SpaceSubnets {
		spaceNames = append(spaceNames, spaceName)
	}
	sort.Strings(spaceNames)

	subnetsToZones := make([]map[network.Id][]string, 0, len(spaceNames))
	for _, spaceName := range spaceNames {
		subnetAZs := make(map[network.Id][]string)
		for _, subnet := range topology.SpaceSubnets[spaceName] {
			subnetAZs[network.Id(subnet)] = topology.SubnetAZs[subnet]
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
				VolumeId:   v.VolumeId,
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
