// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/set"

	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

type ProvisionerTask interface {
	worker.Worker

	// SetHarvestMode sets a flag to indicate how the provisioner task
	// should harvest machines. See config.HarvestMode for
	// documentation of behavior.
	SetHarvestMode(mode config.HarvestMode)
}

type MachineGetter interface {
	Machine(names.MachineTag) (*apiprovisioner.Machine, error)
	MachinesWithTransientErrors() ([]*apiprovisioner.Machine, []params.StatusResult, error)
}

// ToolsFinder is an interface used for finding tools to run on
// provisioned instances.
type ToolsFinder interface {
	// FindTools returns a list of tools matching the specified
	// version, series, and architecture. If arch is empty, the
	// implementation is expected to use a well documented default.
	FindTools(version version.Number, series string, arch string) (coretools.List, error)
}

var _ MachineGetter = (*apiprovisioner.State)(nil)
var _ ToolsFinder = (*apiprovisioner.State)(nil)

func NewProvisionerTask(
	machineTag names.MachineTag,
	harvestMode config.HarvestMode,
	machineGetter MachineGetter,
	toolsFinder ToolsFinder,
	machineWatcher watcher.StringsWatcher,
	retryWatcher watcher.NotifyWatcher,
	broker environs.InstanceBroker,
	auth authentication.AuthenticationProvider,
	imageStream string,
	secureServerConnection bool,
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
		machineTag:                 machineTag,
		machineGetter:              machineGetter,
		toolsFinder:                toolsFinder,
		machineChanges:             machineChanges,
		retryChanges:               retryChanges,
		broker:                     broker,
		auth:                       auth,
		harvestMode:                harvestMode,
		harvestModeChan:            make(chan config.HarvestMode, 1),
		machines:                   make(map[string]*apiprovisioner.Machine),
		imageStream:                imageStream,
		secureServerConnection:     secureServerConnection,
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
	return task, nil
}

type provisionerTask struct {
	machineTag                 names.MachineTag
	machineGetter              MachineGetter
	toolsFinder                ToolsFinder
	machineChanges             watcher.StringsChannel
	retryChanges               watcher.NotifyChannel
	broker                     environs.InstanceBroker
	catacomb                   catacomb.Catacomb
	auth                       authentication.AuthenticationProvider
	imageStream                string
	secureServerConnection     bool
	harvestMode                config.HarvestMode
	harvestModeChan            chan config.HarvestMode
	retryStartInstanceStrategy RetryStrategy
	// instance id -> instance
	instances map[instance.Id]instance.Instance
	// machine id -> machine
	machines map[string]*apiprovisioner.Machine
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
	machines, statusResults, err := task.machineGetter.MachinesWithTransientErrors()
	if err != nil {
		return nil
	}
	logger.Tracef("processMachinesWithTransientErrors(%v)", statusResults)
	var pending []*apiprovisioner.Machine
	for i, status := range statusResults {
		if status.Error != nil {
			logger.Errorf("cannot retry provisioning of machine %q: %v", status.Id, status.Error)
			continue
		}
		machine := machines[i]
		if err := machine.SetStatus(params.StatusPending, "", nil); err != nil {
			logger.Errorf("cannot reset status of machine %q: %v", status.Id, err)
			continue
		}
		task.machines[machine.Tag().String()] = machine
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
	stopping := task.instancesForMachines(dead)

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
		if err := machine.Remove(); err != nil {
			logger.Errorf("failed to remove dead machine %q", machine)
		}
		delete(task.machines, machine.Id())
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
	// TODO(thumper): update for API server later to get all machines in one go.
	for _, id := range ids {
		machineTag := names.NewMachineTag(id)
		machine, err := task.machineGetter.Machine(machineTag)
		switch {
		case params.IsCodeNotFoundOrCodeUnauthorized(err):
			logger.Debugf("machine %q not found in state", id)
			delete(task.machines, id)
		case err == nil:
			task.machines[id] = machine
		default:
			return errors.Annotatef(err, "failed to get machine %v", id)
		}
	}
	return nil
}

// pendingOrDead looks up machines with ids and returns those that do not
// have an instance id assigned yet, and also those that are dead.
func (task *provisionerTask) pendingOrDeadOrMaintain(ids []string) (pending, dead, maintain []*apiprovisioner.Machine, err error) {
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
	Status() (params.Status, string, error)
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
	if instId, err := machine.InstanceId(); err != nil {
		if !params.IsCodeNotProvisioned(err) {
			return None, errors.Annotatef(err, "failed to load machine id:%s, details:%v", machine.Id(), machine)
		}
		status, _, err := machine.Status()
		if err != nil {
			logger.Infof("cannot get machine id:%s, details:%v, err:%v", machine.Id(), machine, err)
			return None, nil
		}
		if status == params.StatusPending {
			logger.Infof("found machine pending provisioning id:%s, details:%v", machine.Id(), machine)
			return Pending, nil
		}
	} else {
		logger.Infof("machine %s already started as instance %q", machine.Id(), instId)
		if err != nil {
			logger.Infof("Error fetching provisioning info")
		} else {
			isLxc := regexp.MustCompile(`\d+/lxc/\d+`)
			isKvm := regexp.MustCompile(`\d+/kvm/\d+`)
			if isLxc.MatchString(machine.Id()) || isKvm.MatchString(machine.Id()) {
				return Maintain, nil
			}
		}
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

// instancesForMachines returns a list of instance.Instance that represent
// the list of machines running in the provider. Missing machines are
// omitted from the list.
func (task *provisionerTask) instancesForMachines(machines []*apiprovisioner.Machine) []instance.Instance {
	var instances []instance.Instance
	for _, machine := range machines {
		instId, err := machine.InstanceId()
		if err == nil {
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

	publicKey, err := simplestreams.UserPublicSigningKey()
	if err != nil {
		return nil, err
	}
	nonce := fmt.Sprintf("%s:%s", task.machineTag, uuid)
	return instancecfg.NewInstanceConfig(
		machine.Id(),
		nonce,
		task.imageStream,
		pInfo.Series,
		publicKey,
		task.secureServerConnection,
		nil,
		stateInfo,
		apiInfo,
	)
}

func constructStartInstanceParams(
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

	return environs.StartInstanceParams{
		Constraints:       provisioningInfo.Constraints,
		Tools:             possibleTools,
		InstanceConfig:    instanceConfig,
		Placement:         provisioningInfo.Placement,
		DistributionGroup: machine.DistributionGroup,
		Volumes:           volumes,
		SubnetsToZones:    subnetsToZones,
		EndpointBindings:  endpointBindings,
		ImageMetadata:     possibleImageMetadata,
	}, nil
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

func (task *provisionerTask) startMachines(machines []*apiprovisioner.Machine) error {
	for _, m := range machines {

		pInfo, err := m.ProvisioningInfo()
		if err != nil {
			return task.setErrorStatus("fetching provisioning info for machine %q: %v", m, err)
		}

		instanceCfg, err := task.constructInstanceConfig(m, task.auth, pInfo)
		if err != nil {
			return task.setErrorStatus("creating instance config for machine %q: %v", m, err)
		}

		assocProvInfoAndMachCfg(pInfo, instanceCfg)

		var arch string
		if pInfo.Constraints.Arch != nil {
			arch = *pInfo.Constraints.Arch
		}

		possibleTools, err := task.toolsFinder.FindTools(
			version.Current,
			pInfo.Series,
			arch,
		)
		if err != nil {
			return task.setErrorStatus("cannot find tools for machine %q: %v", m, err)
		}

		startInstanceParams, err := constructStartInstanceParams(
			m,
			instanceCfg,
			pInfo,
			possibleTools,
		)
		if err != nil {
			return task.setErrorStatus("cannot construct params for machine %q: %v", m, err)
		}

		if err := task.startMachine(m, pInfo, startInstanceParams); err != nil {
			return errors.Annotatef(err, "cannot start machine %v", m)
		}
	}
	return nil
}

func (task *provisionerTask) setErrorStatus(message string, machine *apiprovisioner.Machine, err error) error {
	logger.Errorf(message, machine, err)
	if err1 := machine.SetStatus(params.StatusError, err.Error(), nil); err1 != nil {
		// Something is wrong with this machine, better report it back.
		return errors.Annotatef(err1, "cannot set error status for machine %q", machine)
	}
	return nil
}

func (task *provisionerTask) prepareNetworkAndInterfaces(networkInfo []network.InterfaceInfo) (
	networks []params.Network, ifaces []params.NetworkInterface, err error) {
	if len(networkInfo) == 0 {
		return nil, nil, nil
	}
	visitedNetworks := set.NewStrings()
	for _, info := range networkInfo {
		// TODO(dimitern): The following few fields are required, but no longer
		// matter and will be dropped or changed soon as part of making spaces
		// and subnets usable across the board.
		if info.NetworkName == "" {
			info.NetworkName = network.DefaultPrivate
		}
		if info.ProviderId == "" {
			info.ProviderId = network.DefaultPrivate
		}
		if info.CIDR == "" {
			// TODO(dimitern): This is only when NOT using addressable
			// containers, as we don't fetch the subnet details, but since
			// networks in state are going away real soon, it's not important.
			info.CIDR = "0.0.0.0/32"
		}
		if !names.IsValidNetwork(info.NetworkName) {
			return nil, nil, errors.Errorf("invalid network name %q", info.NetworkName)
		}
		networkTag := names.NewNetworkTag(info.NetworkName).String()
		if !visitedNetworks.Contains(networkTag) {
			networks = append(networks, params.Network{
				Tag:        networkTag,
				ProviderId: string(info.ProviderId),
				CIDR:       info.CIDR,
				VLANTag:    info.VLANTag,
			})
			visitedNetworks.Add(networkTag)
		}
		ifaces = append(ifaces, params.NetworkInterface{
			InterfaceName: info.ActualInterfaceName(),
			MACAddress:    info.MACAddress,
			NetworkTag:    networkTag,
			IsVirtual:     info.IsVirtual(),
			Disabled:      info.Disabled,
		})
	}
	return networks, ifaces, nil
}

func (task *provisionerTask) startMachine(
	machine *apiprovisioner.Machine,
	provisioningInfo *params.ProvisioningInfo,
	startInstanceParams environs.StartInstanceParams,
) error {

	result, err := task.broker.StartInstance(startInstanceParams)
	if err != nil {
		if !instance.IsRetryableCreationError(errors.Cause(err)) {
			// Set the state to error, so the machine will be skipped next
			// time until the error is resolved, but don't return an
			// error; just keep going with the other machines.
			return task.setErrorStatus("cannot start instance for machine %q: %v", machine, err)
		}
		logger.Infof("retryable error received on start instance: %v", err)
		for count := task.retryStartInstanceStrategy.retryCount; count > 0; count-- {
			if task.retryStartInstanceStrategy.retryDelay > 0 {
				select {
				case <-task.catacomb.Dying():
					return task.catacomb.ErrDying()
				case <-time.After(task.retryStartInstanceStrategy.retryDelay):
				}

			}
			result, err = task.broker.StartInstance(startInstanceParams)
			if err == nil {
				break
			}
			// If this was the last attempt and an error was received, set the error
			// status on the machine.
			if count == 1 {
				return task.setErrorStatus("cannot start instance for machine %q: %v", machine, err)
			}
		}
	}

	inst := result.Instance
	hardware := result.Hardware
	nonce := startInstanceParams.InstanceConfig.MachineNonce
	networks, ifaces, err := task.prepareNetworkAndInterfaces(result.NetworkInfo)
	if err != nil {
		return task.setErrorStatus("cannot prepare network for machine %q: %v", machine, err)
	}
	volumes := volumesToApiserver(result.Volumes)
	volumeAttachments := volumeAttachmentsToApiserver(result.VolumeAttachments)

	// TODO(dimitern) In a newer Provisioner API version, change
	// SetInstanceInfo or add a new method that takes and saves in
	// state all the information available on a network.InterfaceInfo
	// for each interface, so we can later manage interfaces
	// dynamically at run-time.
	err = machine.SetInstanceInfo(inst.Id(), nonce, hardware, networks, ifaces, volumes, volumeAttachments)
	if err == nil {
		logger.Infof(
			"started machine %s as instance %s with hardware %q, networks %v, interfaces %v, volumes %v, volume attachments %v, subnets to zones %v",
			machine, inst.Id(), hardware,
			networks, ifaces,
			volumes, volumeAttachments,
			startInstanceParams.SubnetsToZones,
		)
		return nil
	}
	// We need to stop the instance right away here, set error status and go on.
	task.setErrorStatus("cannot register instance for machine %v: %v", machine, err)
	if err := task.broker.StopInstances(inst.Id()); err != nil {
		// We cannot even stop the instance, log the error and quit.
		return errors.Annotatef(err, "cannot stop instance %q for machine %v", inst.Id(), machine)
	}
	return nil
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

	instanceConfig.Networks = provInfo.Networks
	instanceConfig.Tags = provInfo.Tags

	if len(provInfo.Jobs) > 0 {
		instanceConfig.Jobs = provInfo.Jobs
	}

	return &provisioningInfo{
		Constraints:    provInfo.Constraints,
		Series:         provInfo.Series,
		Placement:      provInfo.Placement,
		InstanceConfig: instanceConfig,
		SubnetsToZones: provInfo.SubnetsToZones,
	}
}

func volumesToApiserver(volumes []storage.Volume) []params.Volume {
	result := make([]params.Volume, len(volumes))
	for i, v := range volumes {
		result[i] = params.Volume{
			v.Tag.String(),
			params.VolumeInfo{
				v.VolumeId,
				v.HardwareId,
				v.Size,
				v.Persistent,
			},
		}
	}
	return result
}

func volumeAttachmentsToApiserver(attachments []storage.VolumeAttachment) map[string]params.VolumeAttachmentInfo {
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
