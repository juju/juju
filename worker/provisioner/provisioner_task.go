// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/wrench"
)

type ProvisionerTask interface {
	worker.Worker

	// SetHarvestMode sets a flag to indicate how the provisioner task
	// should harvest machines. See config.HarvestMode for
	// documentation of behavior.
	SetHarvestMode(mode config.HarvestMode)
}

type MachineGetter interface {
	Machines(...names.MachineTag) ([]apiprovisioner.MachineResult, error)
	MachinesWithTransientErrors() ([]apiprovisioner.MachineStatusResult, error)
}

type DistributionGroupFinder interface {
	DistributionGroupByMachineId(...names.MachineTag) ([]apiprovisioner.DistributionGroupResult, error)
}

// ToolsFinder is an interface used for finding tools to run on
// provisioned instances.
type ToolsFinder interface {
	// FindTools returns a list of tools matching the specified
	// version, series, and architecture. If arch is empty, the
	// implementation is expected to use a well documented default.
	FindTools(version version.Number, series string, arch string) (coretools.List, error)
}

func NewProvisionerTask(
	controllerUUID string,
	machineTag names.MachineTag,
	harvestMode config.HarvestMode,
	machineGetter MachineGetter,
	distributionGroupFinder DistributionGroupFinder,
	toolsFinder ToolsFinder,
	machineWatcher watcher.StringsWatcher,
	retryWatcher watcher.NotifyWatcher,
	broker environs.InstanceBroker,
	auth authentication.AuthenticationProvider,
	imageStream string,
	retryStartInstanceStrategy RetryStrategy,
) (ProvisionerTask, error) {
	machineChanges := machineWatcher.Changes()
	workers := []worker.Worker{machineWatcher}
	var retryChanges watcher.NotifyChannel
	if retryWatcher != nil {
		retryChanges = retryWatcher.Changes()
		workers = append(workers, retryWatcher)
	}
	task := &provisionerTask{
		controllerUUID:             controllerUUID,
		machineTag:                 machineTag,
		machineGetter:              machineGetter,
		distributionGroupFinder:    distributionGroupFinder,
		toolsFinder:                toolsFinder,
		machineChanges:             machineChanges,
		retryChanges:               retryChanges,
		broker:                     broker,
		auth:                       auth,
		harvestMode:                harvestMode,
		harvestModeChan:            make(chan config.HarvestMode, 1),
		machines:                   make(map[string]*apiprovisioner.Machine),
		availabilityZoneMachines:   make([]*AvailabilityZoneMachine, 0),
		imageStream:                imageStream,
		retryStartInstanceStrategy: retryStartInstanceStrategy,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &task.catacomb,
		Work: task.loop,
		Init: workers,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Get existing machine distributions.
	err = task.populateAvailabilityZoneMachines()
	// Not all providers implement ZonedEnviron
	if err != nil && !errors.IsNotImplemented(err) {
		return nil, errors.Trace(err)
	}
	return task, nil
}

type provisionerTask struct {
	controllerUUID             string
	machineTag                 names.MachineTag
	machineGetter              MachineGetter
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
	// instance id -> instance
	instances map[instance.Id]instance.Instance
	// machine id -> machine
	machines                 map[string]*apiprovisioner.Machine
	machinesMutex            sync.RWMutex
	availabilityZoneMachines []*AvailabilityZoneMachine
}

// Kill implements worker.Worker.Kill.
func (task *provisionerTask) Kill() {
	task.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (task *provisionerTask) Wait() error {
	return task.catacomb.Wait()
}

func (task *provisionerTask) loop() error {

	// Don't allow the harvesting mode to change until we have read at
	// least one set of changes, which will populate the task.machines
	// map. Otherwise we will potentially see all legitimate instances
	// as unknown.
	var harvestModeChan chan config.HarvestMode

	// When the watcher is started, it will have the initial changes be all
	// the machines that are relevant. Also, since this is available straight
	// away, we know there will be some changes right off the bat.
	for {
		select {
		case <-task.catacomb.Dying():
			logger.Infof("Shutting down provisioner task %s", task.machineTag)
			return task.catacomb.ErrDying()
		case ids, ok := <-task.machineChanges:
			if !ok {
				return errors.New("machine watcher closed channel")
			}
			if err := task.processMachines(ids); err != nil {
				return errors.Annotate(err, "failed to process updated machines")
			}
			// We've seen a set of changes. Enable modification of
			// harvesting mode.
			harvestModeChan = task.harvestModeChan
		case harvestMode := <-harvestModeChan:
			if harvestMode == task.harvestMode {
				break
			}
			logger.Infof("harvesting mode changed to %s", harvestMode)
			task.harvestMode = harvestMode
			if harvestMode.HarvestUnknown() {
				logger.Infof("harvesting unknown machines")
				if err := task.processMachines(nil); err != nil {
					return errors.Annotate(err, "failed to process machines after safe mode disabled")
				}
			}
		case <-task.retryChanges:
			if err := task.processMachinesWithTransientErrors(); err != nil {
				return errors.Annotate(err, "failed to process machines with transient errors")
			}
		}
	}
}

// SetHarvestMode implements ProvisionerTask.SetHarvestMode().
func (task *provisionerTask) SetHarvestMode(mode config.HarvestMode) {
	select {
	case task.harvestModeChan <- mode:
	case <-task.catacomb.Dying():
	}
}

func (task *provisionerTask) processMachinesWithTransientErrors() error {
	results, err := task.machineGetter.MachinesWithTransientErrors()
	if err != nil {
		return nil
	}
	logger.Tracef("processMachinesWithTransientErrors(%v)", results)
	var pending []*apiprovisioner.Machine
	for _, result := range results {
		if result.Status.Error != nil {
			logger.Errorf("cannot retry provisioning of machine %q: %v", result.Machine.Id(), result.Status.Error)
			continue
		}
		machine := result.Machine
		if err := machine.SetStatus(status.Pending, "", nil); err != nil {
			logger.Errorf("cannot reset status of machine %q: %v", machine.Id(), err)
			continue
		}
		if err := machine.SetInstanceStatus(status.Provisioning, "", nil); err != nil {
			logger.Errorf("cannot reset instance status of machine %q: %v", machine.Id(), err)
			continue
		}
		task.machinesMutex.Lock()
		task.machines[machine.Tag().String()] = machine
		task.machinesMutex.Unlock()
		pending = append(pending, machine)
	}
	return task.startMachines(pending)
}

func (task *provisionerTask) processMachines(ids []string) error {
	logger.Tracef("processMachines(%v)", ids)

	// Populate the tasks maps of current instances and machines.
	if err := task.populateMachineMaps(ids); err != nil {
		return err
	}

	// Find machines without an instance id or that are dead
	pending, dead, maintain, err := task.pendingOrDeadOrMaintain(ids)
	if err != nil {
		return err
	}

	// Stop all machines that are dead
	stopping := task.instancesForDeadMachines(dead)

	// Find running instances that have no machines associated
	unknown, err := task.findUnknownInstances(stopping)
	if err != nil {
		return err
	}
	if !task.harvestMode.HarvestUnknown() {
		logger.Infof(
			"%s is set to %s; unknown instances not stopped %v",
			config.ProvisionerHarvestModeKey,
			task.harvestMode.String(),
			instanceIds(unknown),
		)
		unknown = nil
	}
	if task.harvestMode.HarvestNone() || !task.harvestMode.HarvestDestroyed() {
		logger.Infof(
			`%s is set to "%s"; will not harvest %s`,
			config.ProvisionerHarvestModeKey,
			task.harvestMode.String(),
			instanceIds(stopping),
		)
		stopping = nil
	}

	if len(stopping) > 0 {
		logger.Infof("stopping known instances %v", stopping)
	}
	if len(unknown) > 0 {
		logger.Infof("stopping unknown instances %v", instanceIds(unknown))
	}
	// It's important that we stop unknown instances before starting
	// pending ones, because if we start an instance and then fail to
	// set its InstanceId on the machine we don't want to start a new
	// instance for the same machine ID.
	if err := task.stopInstances(append(stopping, unknown...)); err != nil {
		return err
	}

	// Remove any dead machines from state.
	for _, machine := range dead {
		logger.Infof("removing dead machine %q", machine)
		if err := machine.MarkForRemoval(); err != nil {
			logger.Errorf("failed to remove dead machine %q", machine)
		}
		task.removeMachineFromAZMap(machine)
		task.machinesMutex.Lock()
		delete(task.machines, machine.Id())
		task.machinesMutex.Unlock()
	}

	// Any machines that require maintenance get pinged
	task.maintainMachines(maintain)

	// Start an instance for the pending ones
	return task.startMachines(pending)
}

func instanceIds(instances []instance.Instance) []string {
	ids := make([]string, 0, len(instances))
	for _, inst := range instances {
		ids = append(ids, string(inst.Id()))
	}
	return ids
}

// populateMachineMaps updates task.instances. Also updates
// task.machines map if a list of IDs is given.
func (task *provisionerTask) populateMachineMaps(ids []string) error {
	task.instances = make(map[instance.Id]instance.Instance)

	instances, err := task.broker.AllInstances()
	if err != nil {
		return errors.Annotate(err, "failed to get all instances from broker")
	}
	for _, i := range instances {
		task.instances[i.Id()] = i
	}

	// Update the machines map with new data for each of the machines in the
	// change list.
	machineTags := make([]names.MachineTag, len(ids))
	for i, id := range ids {
		machineTags[i] = names.NewMachineTag(id)
	}
	machines, err := task.machineGetter.Machines(machineTags...)
	if err != nil {
		return errors.Annotatef(err, "failed to get machines %v", ids)
	}
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for i, result := range machines {
		switch {
		case result.Err == nil:
			task.machines[result.Machine.Id()] = result.Machine
		case params.IsCodeNotFoundOrCodeUnauthorized(result.Err):
			logger.Debugf("machine %q not found in state", ids[i])
			delete(task.machines, ids[i])
		default:
			return errors.Annotatef(result.Err, "failed to get machine %v", ids[i])
		}
	}
	return nil
}

// pendingOrDead looks up machines with ids and returns those that do not
// have an instance id assigned yet, and also those that are dead.
func (task *provisionerTask) pendingOrDeadOrMaintain(ids []string) (pending, dead, maintain []*apiprovisioner.Machine, err error) {
	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()
	for _, id := range ids {
		machine, found := task.machines[id]
		if !found {
			logger.Infof("machine %q not found", id)
			continue
		}
		var classification MachineClassification
		classification, err = classifyMachine(machine)
		if err != nil {
			return // return the error
		}
		switch classification {
		case Pending:
			pending = append(pending, machine)
		case Dead:
			dead = append(dead, machine)
		case Maintain:
			maintain = append(maintain, machine)
		}
	}
	logger.Tracef("pending machines: %v", pending)
	logger.Tracef("dead machines: %v", dead)
	return
}

type ClassifiableMachine interface {
	Life() params.Life
	InstanceId() (instance.Id, error)
	EnsureDead() error
	Status() (status.Status, string, error)
	InstanceStatus() (status.Status, string, error)
	Id() string
}

type MachineClassification string

const (
	None     MachineClassification = "none"
	Pending  MachineClassification = "Pending"
	Dead     MachineClassification = "Dead"
	Maintain MachineClassification = "Maintain"
)

func classifyMachine(machine ClassifiableMachine) (
	MachineClassification, error) {
	switch machine.Life() {
	case params.Dying:
		if _, err := machine.InstanceId(); err == nil {
			return None, nil
		} else if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "failed to load dying machine id:%s, details:%v", machine.Id(), machine)
		}
		logger.Infof("killing dying, unprovisioned machine %q", machine)
		if err := machine.EnsureDead(); err != nil {
			return None, errors.Annotatef(err, "failed to ensure machine dead id:%s, details:%v", machine.Id(), machine)
		}
		fallthrough
	case params.Dead:
		return Dead, nil
	}
	instId, err := machine.InstanceId()
	if err != nil {
		if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "failed to load machine id:%s, details:%v", machine.Id(), machine)
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

	if state.ContainerTypeFromId(machine.Id()) != "" {
		return Maintain, nil
	}
	return None, nil
}

// findUnknownInstances finds instances which are not associated with a machine.
func (task *provisionerTask) findUnknownInstances(stopping []instance.Instance) ([]instance.Instance, error) {
	// Make a copy of the instances we know about.
	instances := make(map[instance.Id]instance.Instance)
	for k, v := range task.instances {
		instances[k] = v
	}

	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()
	for _, m := range task.machines {
		instId, err := m.InstanceId()
		switch {
		case err == nil:
			delete(instances, instId)
		case params.IsCodeNotProvisioned(err):
		case params.IsCodeNotFoundOrCodeUnauthorized(err):
		default:
			return nil, err
		}
	}
	// Now remove all those instances that we are stopping already as we
	// know about those and don't want to include them in the unknown list.
	for _, inst := range stopping {
		delete(instances, inst.Id())
	}
	var unknown []instance.Instance
	for _, inst := range instances {
		unknown = append(unknown, inst)
	}
	return unknown, nil
}

// instancesForDeadMachines returns a list of instance.Instance that represent
// the list of dead machines running in the provider. Missing machines are
// omitted from the list.
func (task *provisionerTask) instancesForDeadMachines(deadMachines []*apiprovisioner.Machine) []instance.Instance {
	var instances []instance.Instance
	for _, machine := range deadMachines {
		instId, err := machine.InstanceId()
		if err == nil {
			keep, _ := machine.KeepInstance()
			if keep {
				logger.Debugf("machine %v is dead but keep-instance is true", instId)
				continue
			}
			instance, found := task.instances[instId]
			// If the instance is not found we can't stop it.
			if found {
				instances = append(instances, instance)
			}
		}
	}
	return instances
}

func (task *provisionerTask) stopInstances(instances []instance.Instance) error {
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
	if err := task.broker.StopInstances(ids...); err != nil {
		return errors.Annotate(err, "broker failed to stop instances")
	}
	return nil
}

func (task *provisionerTask) constructInstanceConfig(
	machine *apiprovisioner.Machine,
	auth authentication.AuthenticationProvider,
	pInfo *params.ProvisioningInfo,
) (*instancecfg.InstanceConfig, error) {

	stateInfo, apiInfo, err := auth.SetupAuthentication(machine)
	if err != nil {
		return nil, errors.Annotate(err, "failed to setup authentication")
	}

	// Generated a nonce for the new instance, with the format: "machine-#:UUID".
	// The first part is a badge, specifying the tag of the machine the provisioner
	// is running on, while the second part is a random UUID.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate a nonce for machine "+machine.Id())
	}

	nonce := fmt.Sprintf("%s:%s", task.machineTag, uuid)
	instanceConfig, err := instancecfg.NewInstanceConfig(
		names.NewControllerTag(controller.Config(pInfo.ControllerConfig).ControllerUUID()),
		machine.Id(),
		nonce,
		task.imageStream,
		pInfo.Series,
		apiInfo,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instanceConfig.Tags = pInfo.Tags
	if len(pInfo.Jobs) > 0 {
		instanceConfig.Jobs = pInfo.Jobs
	}

	if multiwatcher.AnyJobNeedsState(instanceConfig.Jobs...) {
		publicKey, err := simplestreams.UserPublicSigningKey()
		if err != nil {
			return nil, err
		}
		instanceConfig.Controller = &instancecfg.ControllerConfig{
			PublicImageSigningKey: publicKey,
			MongoInfo:             stateInfo,
		}
		instanceConfig.Controller.Config = make(map[string]interface{})
		for k, v := range pInfo.ControllerConfig {
			instanceConfig.Controller.Config[k] = v
		}
	}

	instanceConfig.CloudInitUserData = pInfo.CloudInitUserData

	return instanceConfig, nil
}

func (task *provisionerTask) constructStartInstanceParams(
	controllerUUID string,
	machine *apiprovisioner.Machine,
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
			volumeTag,
			v.Size,
			storage.ProviderType(v.Provider),
			v.Attributes,
			v.Tags,
			&storage.VolumeAttachmentParams{
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

	var subnetsToZones map[network.Id][]string
	if provisioningInfo.SubnetsToZones != nil {
		// Convert subnet provider ids from string to network.Id.
		subnetsToZones = make(map[network.Id][]string, len(provisioningInfo.SubnetsToZones))
		for providerId, zones := range provisioningInfo.SubnetsToZones {
			subnetsToZones[network.Id(providerId)] = zones
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
		SubnetsToZones:    subnetsToZones,
		EndpointBindings:  endpointBindings,
		ImageMetadata:     possibleImageMetadata,
		StatusCallback:    machine.SetInstanceStatus,
	}

	return startInstanceParams, nil
}

func (task *provisionerTask) maintainMachines(machines []*apiprovisioner.Machine) error {
	for _, m := range machines {
		logger.Infof("maintainMachines: %v", m)
		startInstanceParams := environs.StartInstanceParams{}
		startInstanceParams.InstanceConfig = &instancecfg.InstanceConfig{}
		startInstanceParams.InstanceConfig.MachineId = m.Id()
		if err := task.broker.MaintainInstance(startInstanceParams); err != nil {
			return errors.Annotatef(err, "cannot maintain machine %v", m)
		}
	}
	return nil
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

// populateAvailabilityZoneMachines fills in the map, availabilityZoneMachines,
// if empty, with a current mapping of availability zone to IDs of machines
// running in that zone.  If the provider does not implement the ZonedEnviron
// interface, return nil.
func (task *provisionerTask) populateAvailabilityZoneMachines() error {
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()

	if len(task.availabilityZoneMachines) > 0 {
		return nil
	}

	zonedEnv, ok := task.broker.(providercommon.ZonedEnviron)
	if !ok {
		// Nothing to do if the provider doesn't implement AvailabilityZonesAllocations
		return nil
	}

	// In this case, AvailabilityZoneAllocations() will return all of the "available"
	// availability zones and their instance allocations.
	availabilityZoneInstances, err := providercommon.AvailabilityZoneAllocations(zonedEnv, []instance.Id{})
	if err != nil {
		return err
	}

	instanceMachines := make(map[instance.Id]string)
	for _, machine := range task.machines {
		instId, err := machine.InstanceId()
		if err != nil {
			continue
		}
		instanceMachines[instId] = machine.Id()
	}

	// convert instances IDs to machines IDs to aid distributing
	// not yet created instances across availability zones.
	task.availabilityZoneMachines = make([]*AvailabilityZoneMachine, len(availabilityZoneInstances))
	for i, instances := range availabilityZoneInstances {
		machineIds := set.NewStrings()
		for _, instanceId := range instances.Instances {
			if id, ok := instanceMachines[instanceId]; ok {
				machineIds.Add(id)
			}
		}
		task.availabilityZoneMachines[i] = &AvailabilityZoneMachine{
			ZoneName:           instances.ZoneName,
			MachineIds:         machineIds,
			FailedMachineIds:   set.NewStrings(),
			ExcludedMachineIds: set.NewStrings(),
		}
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
// for the specified machine to start in.  If the current provider does not
// implement availability zones, "" and no error will be returned. Machines are
// spread across availability zones based on lowest population of the "available" zones.
// Machines in the same DistributionGroup are placed in different zones, spread
// across availability zones based on lowest population of machines in that
// DistributionGroup.  Machines are not placed in a zone they are excluded from.
// If availability zones are implemented and one isn't found, return NotFound error.
func (task *provisionerTask) machineAvailabilityZoneDistribution(machineId string, distributionGroupMachineIds []string) (string, error) {
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()

	if len(task.availabilityZoneMachines) == 0 {
		return "", nil
	}

	var machineZone string
	// assign an initial az to a machine based on lowest population.
	// if the machine has a distribution group, assign based on lowest
	// az population of the distribution group machine.
	if len(distributionGroupMachineIds) > 0 {
		dgZoneMap := task.populateDistributionGroupZoneMap(distributionGroupMachineIds)
		sort.Sort(byPopulationThenNames(dgZoneMap))

		for _, dgZoneMachines := range dgZoneMap {
			if !dgZoneMachines.FailedMachineIds.Contains(machineId) &&
				!dgZoneMachines.ExcludedMachineIds.Contains(machineId) {
				machineZone = dgZoneMachines.ZoneName
				for _, azm := range task.availabilityZoneMachines {
					if azm.ZoneName == dgZoneMachines.ZoneName {
						azm.MachineIds.Add(machineId)
						break
					}
				}
				break
			}
		}
	} else {
		sort.Sort(byPopulationThenNames(task.availabilityZoneMachines))
		for _, zoneMachines := range task.availabilityZoneMachines {
			if !zoneMachines.FailedMachineIds.Contains(machineId) &&
				!zoneMachines.ExcludedMachineIds.Contains(machineId) {
				machineZone = zoneMachines.ZoneName
				zoneMachines.MachineIds.Add(machineId)
				break
			}
		}
	}
	if machineZone == "" {
		return machineZone, errors.NotFoundf("suitable availability zone for machine %v", machineId)
	}
	return machineZone, nil
}

type byPopulationThenNames []*AvailabilityZoneMachine

func (b byPopulationThenNames) Len() int {
	return len(b)
}

func (b byPopulationThenNames) Less(i, j int) bool {
	switch {
	case b[i].MachineIds.Size() < b[j].MachineIds.Size():
		return true
	case b[i].MachineIds.Size() == b[j].MachineIds.Size():
		return b[i].ZoneName < b[j].ZoneName
	}
	return false
}

func (b byPopulationThenNames) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

// startMachines starts a goroutine for each specified machine to
// start it.  Errors from individual start machine attempts will be logged.
func (task *provisionerTask) startMachines(machines []*apiprovisioner.Machine) error {
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
		return err
	}

	var wg sync.WaitGroup
	errMachines := make([]error, len(machines))
	for i, m := range machines {
		if machineDistributionGroups[i].Err != nil {
			task.setErrorStatus(
				"fetching distribution groups for machine %q: %v",
				m, machineDistributionGroups[i].Err,
			)
			continue
		}
		wg.Add(1)
		go func(machine *apiprovisioner.Machine, dg []string, index int) {
			defer wg.Done()
			if err := task.startMachine(machine, dg); err != nil {
				task.removeMachineFromAZMap(machine)
				errMachines[index] = err
			}
		}(m, machineDistributionGroups[i].MachineIds, i)
	}

	wg.Wait()
	select {
	case <-task.catacomb.Dying():
		return task.catacomb.ErrDying()
	default:
	}
	var errorStrings []string
	for _, err := range errMachines {
		if err != nil {
			errorStrings = append(errorStrings, err.Error())
		}
	}
	if errorStrings != nil {
		return errors.New(strings.Join(errorStrings, "\n"))
	}
	return nil
}

func (task *provisionerTask) setErrorStatus(message string, machine *apiprovisioner.Machine, err error) error {
	logger.Errorf(message, machine, err)
	errForStatus := errors.Cause(err)
	if err2 := machine.SetInstanceStatus(status.ProvisioningError, errForStatus.Error(), nil); err2 != nil {
		// Something is wrong with this machine, better report it back.
		return errors.Annotatef(err2, "cannot set error status for machine %q", machine)
	}
	return nil
}

// setupToStartMachine gathers the nessecessary information, based on the specified
// machine, to create ProvisioningInfo and StartInstanceParms to be used by startMachine.
func (task *provisionerTask) setupToStartMachine(machine *apiprovisioner.Machine, version *version.Number) (
	environs.StartInstanceParams,
	error,
) {
	pInfo, err := machine.ProvisioningInfo()
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "fetching provisioning info for machine %q", machine)
	}

	instanceCfg, err := task.constructInstanceConfig(machine, task.auth, pInfo)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "creating instance config for machine %q", machine)
	}

	assocProvInfoAndMachCfg(pInfo, instanceCfg)

	var arch string
	if pInfo.Constraints.Arch != nil {
		arch = *pInfo.Constraints.Arch
	}

	possibleTools, err := task.toolsFinder.FindTools(
		*version,
		pInfo.Series,
		arch,
	)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "cannot find agent binaries for machine %q", machine)
	}

	startInstanceParams, err := task.constructStartInstanceParams(
		task.controllerUUID,
		machine,
		instanceCfg,
		pInfo,
		possibleTools,
	)
	if err != nil {
		return environs.StartInstanceParams{}, errors.Annotatef(err, "cannot construct params for machine %q", machine)
	}

	return startInstanceParams, nil
}

// populateExcludedMachines, translates the results of DeriveAvailabilityZones
// into availabilityZoneMachines.ExcludedMachineIds for machines not to be used
// in the given zone.
func (task *provisionerTask) populateExcludedMachines(machineId string, startInstanceParams environs.StartInstanceParams) error {
	zonedEnv, ok := task.broker.(providercommon.ZonedEnviron)
	if !ok {
		return nil
	}
	derivedZones, err := zonedEnv.DeriveAvailabilityZones(startInstanceParams)
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

func (task *provisionerTask) startMachine(
	machine *apiprovisioner.Machine,
	distributionGroupMachineIds []string,
) error {
	v, err := machine.ModelAgentVersion()
	if err != nil {
		return err
	}
	startInstanceParams, err := task.setupToStartMachine(machine, v)
	if err != nil {
		return task.setErrorStatus("%v", machine, err)
	}

	// Figure out if the zones available to use for a new instance are
	// restricted based on placement, and if so exclude those machines
	// from being started in any other zone.
	if err := task.populateExcludedMachines(machine.Id(), startInstanceParams); err != nil {
		return err
	}

	// TODO (jam): 2017-01-19 Should we be setting this earlier in the cycle?
	if err := machine.SetInstanceStatus(status.Provisioning, "starting", nil); err != nil {
		logger.Errorf("%v", err)
	}

	// TODO ProvisionerParallelization 2017-10-03
	// Improve the retry loop, newer methodology
	// Is rate limiting handled correctly?
	var result *environs.StartInstanceResult

	// Attempt creating the instance "retryCount" times. If the provider
	// supports availability zones and we're automatically distributing
	// across the zones, then we try each zone for every attempt, or until
	// one of the StartInstance calls returns an error satisfying
	// environs.IsAvailabilityZoneIndependent.
	for attemptsLeft := task.retryStartInstanceStrategy.retryCount; attemptsLeft >= 0; {
		startInstanceParams.AvailabilityZone, err = task.machineAvailabilityZoneDistribution(machine.Id(), distributionGroupMachineIds)
		if err != nil {
			return task.setErrorStatus("cannot start instance for machine %q: %v", machine, err)
		}
		if startInstanceParams.AvailabilityZone != "" {
			logger.Infof("trying machine %s StartInstance in availability zone %s", machine, startInstanceParams.AvailabilityZone)
		}

		attemptResult, err := task.broker.StartInstance(startInstanceParams)
		if err == nil {
			result = attemptResult
			break
		} else if attemptsLeft <= 0 {
			// Set the state to error, so the machine will be skipped
			// next time until the error is resolved.
			task.removeMachineFromAZMap(machine)
			return task.setErrorStatus("cannot start instance for machine %q: %v", machine, err)
		}

		retrying := true
		retryMsg := ""
		if startInstanceParams.AvailabilityZone != "" && !environs.IsAvailabilityZoneIndependent(err) {
			// We've specified a zone, and the error may be specific to
			// that zone. Retry in another zone if there are any untried.
			azRemaining, err2 := task.markMachineFailedInAZ(machine, startInstanceParams.AvailabilityZone)
			if err2 != nil {
				if err = task.setErrorStatus("cannot start instance: %v", machine, err2); err != nil {
					logger.Errorf("setting error status: %s", err)
				}
				return err2
			}
			if azRemaining {
				retryMsg = fmt.Sprintf(
					"failed to start machine %s in zone %q, retrying in %v with new availability zone: %s",
					machine, startInstanceParams.AvailabilityZone,
					task.retryStartInstanceStrategy.retryDelay, err,
				)
				logger.Debugf("%s", retryMsg)
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
			logger.Warningf("%s", retryMsg)
			attemptsLeft--
		}

		if err3 := machine.SetInstanceStatus(status.Provisioning, retryMsg, nil); err3 != nil {
			logger.Warningf("failed to set instance status: %v", err3)
		}

		select {
		case <-task.catacomb.Dying():
			return task.catacomb.ErrDying()
		case <-time.After(task.retryStartInstanceStrategy.retryDelay):
		}
	}

	networkConfig := networkingcommon.NetworkConfigFromInterfaceInfo(result.NetworkInfo)
	volumes := volumesToAPIserver(result.Volumes)
	volumeNameToAttachmentInfo := volumeAttachmentsToAPIserver(result.VolumeAttachments)

	if err := machine.SetInstanceInfo(
		result.Instance.Id(),
		startInstanceParams.InstanceConfig.MachineNonce,
		result.Hardware,
		networkConfig,
		volumes,
		volumeNameToAttachmentInfo,
	); err != nil {
		// We need to stop the instance right away here, set error status and go on.
		if err2 := task.setErrorStatus("cannot register instance for machine %v: %v", machine, err); err2 != nil {
			logger.Errorf("%v", errors.Annotate(err2, "cannot set machine's status"))
		}
		if err2 := task.broker.StopInstances(result.Instance.Id()); err2 != nil {
			logger.Errorf("%v", errors.Annotate(err2, "after failing to set instance info"))
		}
		return errors.Annotate(err, "cannot set instance info")
	}

	logger.Infof(
		"started machine %s as instance %s with hardware %q, network config %+v, volumes %v, volume attachments %v, subnets to zones %v",
		machine,
		result.Instance.Id(),
		result.Hardware,
		networkConfig,
		volumes,
		volumeNameToAttachmentInfo,
		startInstanceParams.SubnetsToZones,
	)
	return nil
}

// markMachineFailedInAZ moves the machine in zone from MachineIds to FailedMachineIds
// in availabilityZoneMachines, report if there are any availability zones not failed for
// the specified machine.
func (task *provisionerTask) markMachineFailedInAZ(machine *apiprovisioner.Machine, zone string) (bool, error) {
	if zone == "" {
		return false, errors.New("no zone provided")
	}
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	azRemaining := false
	for _, zoneMachines := range task.availabilityZoneMachines {
		if zone == zoneMachines.ZoneName {
			zoneMachines.MachineIds.Remove(machine.Id())
			zoneMachines.FailedMachineIds.Add(machine.Id())
			if azRemaining {
				break
			}
		}
		if !zoneMachines.FailedMachineIds.Contains(machine.Id()) &&
			!zoneMachines.ExcludedMachineIds.Contains(machine.Id()) {
			azRemaining = true
		}
	}
	return azRemaining, nil
}

func (task *provisionerTask) clearMachineAZFailures(machine *apiprovisioner.Machine) {
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for _, zoneMachines := range task.availabilityZoneMachines {
		zoneMachines.FailedMachineIds.Remove(machine.Id())
	}
}

func (task *provisionerTask) addMachinetoAZMap(machine *apiprovisioner.Machine, zoneName string) {
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for _, zoneMachines := range task.availabilityZoneMachines {
		if zoneName == zoneMachines.ZoneName {
			zoneMachines.MachineIds.Add(machine.Id())
			break
		}
	}
	return
}

// removeMachineFromAZMap removes the specified machine from availabilityZoneMachines.
// It is assumed this is called when the machines are being deleted from state, or failed
// provisioning.
func (task *provisionerTask) removeMachineFromAZMap(machine *apiprovisioner.Machine) {
	machineId := machine.Id()
	task.machinesMutex.Lock()
	defer task.machinesMutex.Unlock()
	for _, zoneMachines := range task.availabilityZoneMachines {
		zoneMachines.MachineIds.Remove(machineId)
		zoneMachines.FailedMachineIds.Remove(machineId)
	}
}

type provisioningInfo struct {
	Constraints    constraints.Value
	Series         string
	Placement      string
	InstanceConfig *instancecfg.InstanceConfig
	SubnetsToZones map[string][]string
}

func assocProvInfoAndMachCfg(
	provInfo *params.ProvisioningInfo,
	instanceConfig *instancecfg.InstanceConfig,
) *provisioningInfo {
	return &provisioningInfo{
		Constraints:    provInfo.Constraints,
		Series:         provInfo.Series,
		Placement:      provInfo.Placement,
		InstanceConfig: instanceConfig,
		SubnetsToZones: provInfo.SubnetsToZones,
	}
}

func volumesToAPIserver(volumes []storage.Volume) []params.Volume {
	result := make([]params.Volume, len(volumes))
	for i, v := range volumes {
		result[i] = params.Volume{
			v.Tag.String(),
			params.VolumeInfo{
				v.VolumeId,
				v.HardwareId,
				v.WWN,
				"", // pool
				v.Size,
				v.Persistent,
			},
		}
	}
	return result
}

func volumeAttachmentsToAPIserver(attachments []storage.VolumeAttachment) map[string]params.VolumeAttachmentInfo {
	result := make(map[string]params.VolumeAttachmentInfo)
	for _, a := range attachments {
		result[a.Volume.String()] = params.VolumeAttachmentInfo{
			a.DeviceName,
			a.DeviceLink,
			a.BusAddress,
			a.ReadOnly,
		}
	}
	return result
}
