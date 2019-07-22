// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/storage/poolmanager"
)

const maxStatusHistoryEntries = 20

// ExportConfig allows certain aspects of the model to be skipped
// during the export. The intent of this is to be able to get a partial
// export to support other API calls, like status.
type ExportConfig struct {
	SkipActions              bool
	SkipAnnotations          bool
	SkipCloudImageMetadata   bool
	SkipCredentials          bool
	SkipIPAddresses          bool
	SkipSettings             bool
	SkipSSHHostKeys          bool
	SkipStatusHistory        bool
	SkipLinkLayerDevices     bool
	SkipUnitAgentBinaries    bool
	SkipMachineAgentBinaries bool
	SkipRelationData         bool
	SkipInstanceData         bool
	SkipApplicationOffers    bool
}

// ExportPartial the current model for the State optionally skipping
// aspects as defined by the ExportConfig.
func (st *State) ExportPartial(cfg ExportConfig) (description.Model, error) {
	return st.exportImpl(cfg)
}

// Export the current model for the State.
func (st *State) Export() (description.Model, error) {
	return st.exportImpl(ExportConfig{})
}

func (st *State) exportImpl(cfg ExportConfig) (description.Model, error) {
	dbModel, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	export := exporter{
		st:      st,
		cfg:     cfg,
		dbModel: dbModel,
		logger:  loggo.GetLogger("juju.state.export-model"),
	}
	if err := export.readAllStatuses(); err != nil {
		return nil, errors.Annotate(err, "reading statuses")
	}
	if err := export.readAllStatusHistory(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllSettings(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllStorageConstraints(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllAnnotations(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllConstraints(); err != nil {
		return nil, errors.Trace(err)
	}

	modelConfig, found := export.modelSettings[modelGlobalKey]
	if !found && !cfg.SkipSettings {
		return nil, errors.New("missing model config")
	}
	delete(export.modelSettings, modelGlobalKey)

	blocks, err := export.readBlocks()
	if err != nil {
		return nil, errors.Trace(err)
	}

	args := description.ModelArgs{
		Type:               string(dbModel.Type()),
		Cloud:              dbModel.Cloud(),
		CloudRegion:        dbModel.CloudRegion(),
		Owner:              dbModel.Owner(),
		Config:             modelConfig.Settings,
		LatestToolsVersion: dbModel.LatestToolsVersion(),
		EnvironVersion:     dbModel.EnvironVersion(),
		Blocks:             blocks,
	}
	export.model = description.NewModel(args)
	if credsTag, credsSet := dbModel.CloudCredential(); credsSet && !cfg.SkipCredentials {
		creds, err := st.CloudCredential(credsTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		export.model.SetCloudCredential(description.CloudCredentialArgs{
			Owner:      credsTag.Owner(),
			Cloud:      credsTag.Cloud(),
			Name:       credsTag.Name(),
			AuthType:   creds.AuthType,
			Attributes: creds.Attributes,
		})
	}
	modelKey := dbModel.globalKey()
	export.model.SetAnnotations(export.getAnnotations(modelKey))
	if err := export.sequences(); err != nil {
		return nil, errors.Trace(err)
	}
	constraintsArgs, err := export.constraintsArgs(modelKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	export.model.SetConstraints(constraintsArgs)
	if err := export.modelStatus(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.modelUsers(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.machines(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.applications(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.remoteApplications(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.relations(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.spaces(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.subnets(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.ipaddresses(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.linklayerdevices(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.sshHostKeys(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.actions(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.cloudimagemetadata(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.storage(); err != nil {
		return nil, errors.Trace(err)
	}

	// If we are doing a partial export, it doesn't really make sense
	// to validate the model.
	fullExport := ExportConfig{}
	if cfg == fullExport {
		if err := export.model.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	export.model.SetSLA(dbModel.SLALevel(), dbModel.SLAOwner(), string(dbModel.SLACredential()))
	export.model.SetMeterStatus(dbModel.MeterStatus().Code.String(), dbModel.MeterStatus().Info)

	if featureflag.Enabled(feature.StrictMigration) {
		if err := export.checkUnexportedValues(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return export.model, nil
}

type exporter struct {
	cfg     ExportConfig
	st      *State
	dbModel *Model
	model   description.Model
	logger  loggo.Logger

	annotations             map[string]annotatorDoc
	constraints             map[string]bson.M
	modelSettings           map[string]settingsDoc
	modelStorageConstraints map[string]storageConstraintsDoc
	status                  map[string]bson.M
	statusHistory           map[string][]historicalStatusDoc
	// Map of application name to units. Populated as part
	// of the applications export.
	units map[string][]*Unit
}

func (e *exporter) sequences() error {
	sequences, err := e.st.Sequences()
	if err != nil {
		return errors.Trace(err)
	}

	for name, value := range sequences {
		e.model.SetSequence(name, value)
	}
	return nil
}

func (e *exporter) readBlocks() (map[string]string, error) {
	blocks, closer := e.st.db().GetCollection(blocksC)
	defer closer()

	var docs []blockDoc
	if err := blocks.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]string)
	for _, doc := range docs {
		// We don't care about the id, uuid, or tag.
		// The uuid and tag both refer to the model uuid, and the
		// id is opaque - even though it is sequence generated.
		result[doc.Type.MigrationValue()] = doc.Message
	}
	return result, nil
}

func (e *exporter) modelStatus() error {
	statusArgs, err := e.statusArgs(modelGlobalKey)
	if err != nil {
		return errors.Annotatef(err, "status for model")
	}

	e.model.SetStatus(statusArgs)
	e.model.SetStatusHistory(e.statusHistoryArgs(modelGlobalKey))
	return nil
}

func (e *exporter) modelUsers() error {
	users, err := e.dbModel.Users()
	if err != nil {
		return errors.Trace(err)
	}
	lastConnections, err := e.readLastConnectionTimes()
	if err != nil {
		return errors.Trace(err)
	}
	for _, user := range users {
		lastConn := lastConnections[strings.ToLower(user.UserName)]
		arg := description.UserArgs{
			Name:           user.UserTag,
			DisplayName:    user.DisplayName,
			CreatedBy:      user.CreatedBy,
			DateCreated:    user.DateCreated,
			LastConnection: lastConn,
			Access:         string(user.Access),
		}
		e.model.AddUser(arg)
	}
	return nil
}

func (e *exporter) machines() error {
	machines, err := e.st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("found %d machines", len(machines))

	instances, err := e.loadMachineInstanceData()
	if err != nil {
		return errors.Trace(err)
	}
	blockDevices, err := e.loadMachineBlockDevices()
	if err != nil {
		return errors.Trace(err)
	}

	// Read all the open ports documents.
	openedPorts, closer := e.st.db().GetCollection(openedPortsC)
	defer closer()
	var portsData []portsDoc
	if err := openedPorts.Find(nil).All(&portsData); err != nil {
		return errors.Annotate(err, "opened ports")
	}
	e.logger.Debugf("found %d openedPorts docs", len(portsData))

	// We are iterating through a flat list of machines, but the migration
	// model stores the nesting. The AllMachines method assures us that the
	// machines are returned in an order so the parent will always before
	// any children.
	machineMap := make(map[string]description.Machine)

	for _, machine := range machines {
		e.logger.Debugf("export machine %s", machine.Id())

		var exParent description.Machine
		if parentId := ParentId(machine.Id()); parentId != "" {
			var found bool
			exParent, found = machineMap[parentId]
			if !found {
				return errors.Errorf("machine %s missing parent", machine.Id())
			}
		}

		exMachine, err := e.newMachine(exParent, machine, instances, portsData, blockDevices)
		if err != nil {
			return errors.Trace(err)
		}
		machineMap[machine.Id()] = exMachine
	}

	return nil
}

func (e *exporter) loadMachineInstanceData() (map[string]instanceData, error) {
	instanceDataCollection, closer := e.st.db().GetCollection(instanceDataC)
	defer closer()

	var instData []instanceData
	instances := make(map[string]instanceData)
	if err := instanceDataCollection.Find(nil).All(&instData); err != nil {
		return nil, errors.Annotate(err, "instance data")
	}
	e.logger.Debugf("found %d instanceData", len(instData))
	for _, data := range instData {
		instances[data.MachineId] = data
	}
	return instances, nil
}

func (e *exporter) loadMachineBlockDevices() (map[string][]BlockDeviceInfo, error) {
	coll, closer := e.st.db().GetCollection(blockDevicesC)
	defer closer()

	var deviceData []blockDevicesDoc
	result := make(map[string][]BlockDeviceInfo)
	if err := coll.Find(nil).All(&deviceData); err != nil {
		return nil, errors.Annotate(err, "block devices")
	}
	e.logger.Debugf("found %d block device records", len(deviceData))
	for _, data := range deviceData {
		result[data.Machine] = data.BlockDevices
	}
	return result, nil
}

func (e *exporter) newMachine(exParent description.Machine, machine *Machine, instances map[string]instanceData, portsData []portsDoc, blockDevices map[string][]BlockDeviceInfo) (description.Machine, error) {
	args := description.MachineArgs{
		Id:            machine.MachineTag(),
		Nonce:         machine.doc.Nonce,
		PasswordHash:  machine.doc.PasswordHash,
		Placement:     machine.doc.Placement,
		Series:        machine.doc.Series,
		ContainerType: machine.doc.ContainerType,
	}

	if supported, ok := machine.SupportedContainers(); ok {
		containers := make([]string, len(supported))
		for i, containerType := range supported {
			containers[i] = string(containerType)
		}
		args.SupportedContainers = &containers
	}

	for _, job := range machine.Jobs() {
		args.Jobs = append(args.Jobs, job.MigrationValue())
	}

	// A null value means that we don't yet know which containers
	// are supported. An empty slice means 'no containers are supported'.
	var exMachine description.Machine
	if exParent == nil {
		exMachine = e.model.AddMachine(args)
	} else {
		exMachine = exParent.AddContainer(args)
	}
	exMachine.SetAddresses(
		e.newAddressArgsSlice(machine.doc.MachineAddresses),
		e.newAddressArgsSlice(machine.doc.Addresses))
	exMachine.SetPreferredAddresses(
		e.newAddressArgs(machine.doc.PreferredPublicAddress),
		e.newAddressArgs(machine.doc.PreferredPrivateAddress))

	// We fully expect the machine to have tools set, and that there is
	// some instance data.
	if !e.cfg.SkipInstanceData {
		instData, found := instances[machine.doc.Id]
		if !found {
			return nil, errors.NotValidf("missing instance data for machine %s", machine.Id())
		}
		exMachine.SetInstance(e.newCloudInstanceArgs(instData))
		instance := exMachine.Instance()
		instanceKey := machine.globalInstanceKey()
		statusArgs, err := e.statusArgs(instanceKey)
		if err != nil {
			return nil, errors.Annotatef(err, "status for machine instance %s", machine.Id())
		}
		instance.SetStatus(statusArgs)
		instance.SetStatusHistory(e.statusHistoryArgs(instanceKey))
		// Extract the modification status from the status dataset
		modificationInstanceKey := machine.globalModificationKey()
		modificationStatusArgs, err := e.statusArgs(modificationInstanceKey)
		if err != nil {
			return nil, errors.Annotatef(err, "modification status for machine instance %s", machine.Id())
		}
		instance.SetModificationStatus(modificationStatusArgs)
	}

	// We don't rely on devices being there. If they aren't, we get an empty slice,
	// which is fine to iterate over with range.
	for _, device := range blockDevices[machine.doc.Id] {
		exMachine.AddBlockDevice(description.BlockDeviceArgs{
			Name:           device.DeviceName,
			Links:          device.DeviceLinks,
			Label:          device.Label,
			UUID:           device.UUID,
			HardwareID:     device.HardwareId,
			WWN:            device.WWN,
			BusAddress:     device.BusAddress,
			Size:           device.Size,
			FilesystemType: device.FilesystemType,
			InUse:          device.InUse,
			MountPoint:     device.MountPoint,
		})
	}

	// Find the current machine status.
	globalKey := machine.globalKey()
	statusArgs, err := e.statusArgs(globalKey)
	if err != nil {
		return nil, errors.Annotatef(err, "status for machine %s", machine.Id())
	}
	exMachine.SetStatus(statusArgs)
	exMachine.SetStatusHistory(e.statusHistoryArgs(globalKey))

	if !e.cfg.SkipMachineAgentBinaries {
		tools, err := machine.AgentTools()
		if err != nil {
			// This means the tools aren't set, but they should be.
			return nil, errors.Trace(err)
		}

		exMachine.SetTools(description.AgentToolsArgs{
			Version: tools.Version,
			URL:     tools.URL,
			SHA256:  tools.SHA256,
			Size:    tools.Size,
		})
	}

	for _, args := range e.openedPortsArgsForMachine(machine.Id(), portsData) {
		exMachine.AddOpenedPorts(args)
	}

	exMachine.SetAnnotations(e.getAnnotations(globalKey))

	constraintsArgs, err := e.constraintsArgs(globalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	exMachine.SetConstraints(constraintsArgs)

	return exMachine, nil
}

func (e *exporter) openedPortsArgsForMachine(machineId string, portsData []portsDoc) []description.OpenedPortsArgs {
	var result []description.OpenedPortsArgs
	for _, doc := range portsData {
		// Don't bother including a subnet if there are no ports open on it.
		if doc.MachineID == machineId && len(doc.Ports) > 0 {
			args := description.OpenedPortsArgs{SubnetID: doc.SubnetID}
			for _, p := range doc.Ports {
				args.OpenedPorts = append(args.OpenedPorts, description.PortRangeArgs{
					UnitName: p.UnitName,
					FromPort: p.FromPort,
					ToPort:   p.ToPort,
					Protocol: p.Protocol,
				})
			}
			result = append(result, args)
		}
	}
	return result
}

func (e *exporter) newAddressArgsSlice(a []address) []description.AddressArgs {
	result := []description.AddressArgs{}
	for _, addr := range a {
		result = append(result, e.newAddressArgs(addr))
	}
	return result
}

func (e *exporter) newAddressArgs(a address) description.AddressArgs {
	return description.AddressArgs{
		Value:  a.Value,
		Type:   a.AddressType,
		Scope:  a.Scope,
		Origin: a.Origin,
	}
}

func (e *exporter) newCloudInstanceArgs(data instanceData) description.CloudInstanceArgs {
	inst := description.CloudInstanceArgs{
		InstanceId: string(data.InstanceId),
	}
	if data.Arch != nil {
		inst.Architecture = *data.Arch
	}
	if data.Mem != nil {
		inst.Memory = *data.Mem
	}
	if data.RootDisk != nil {
		inst.RootDisk = *data.RootDisk
	}
	if data.RootDiskSource != nil {
		inst.RootDiskSource = *data.RootDiskSource
	}
	if data.CpuCores != nil {
		inst.CpuCores = *data.CpuCores
	}
	if data.CpuPower != nil {
		inst.CpuPower = *data.CpuPower
	}
	if data.Tags != nil {
		inst.Tags = *data.Tags
	}
	if data.AvailZone != nil {
		inst.AvailabilityZone = *data.AvailZone
	}
	if len(data.CharmProfiles) > 0 {
		inst.CharmProfiles = data.CharmProfiles
	}
	return inst
}

func (e *exporter) applications() error {
	applications, err := e.st.AllApplications()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("found %d applications", len(applications))

	e.units, err = e.readAllUnits()
	if err != nil {
		return errors.Trace(err)
	}

	meterStatus, err := e.readAllMeterStatus()
	if err != nil {
		return errors.Trace(err)
	}

	bindings, err := e.readAllEndpointBindings()
	if err != nil {
		return errors.Trace(err)
	}

	leaders, err := e.st.ApplicationLeaders()
	if err != nil {
		return errors.Trace(err)
	}

	payloads, err := e.readAllPayloads()
	if err != nil {
		return errors.Trace(err)
	}

	podSpecs, err := e.readAllPodSpecs()
	if err != nil {
		return errors.Trace(err)
	}
	cloudServices, err := e.readAllCloudServices()
	if err != nil {
		return errors.Trace(err)
	}
	cloudContainers, err := e.readAllCloudContainers()
	if err != nil {
		return errors.Trace(err)
	}

	resourcesSt, err := e.st.Resources()
	if err != nil {
		return errors.Trace(err)
	}

	appOfferMap, err := e.groupOffersByApplicationName()
	if err != nil {
		return errors.Trace(err)
	}

	for _, application := range applications {
		applicationUnits := e.units[application.Name()]
		leader := leaders[application.Name()]
		resources, err := resourcesSt.ListResources(application.Name())
		if err != nil {
			return errors.Trace(err)
		}
		appCtx := addApplicationContext{
			application:      application,
			units:            applicationUnits,
			meterStatus:      meterStatus,
			podSpecs:         podSpecs,
			cloudServices:    cloudServices,
			cloudContainers:  cloudContainers,
			leader:           leader,
			payloads:         payloads,
			resources:        resources,
			endpoingBindings: bindings,
		}

		if appOfferMap != nil {
			appCtx.offers = appOfferMap[application.Name()]
		}

		if err := e.addApplication(appCtx); err != nil {
			return errors.Trace(err)
		}

	}
	return nil
}

func (e *exporter) readAllStorageConstraints() error {
	coll, closer := e.st.db().GetCollection(storageConstraintsC)
	defer closer()

	storageConstraints := make(map[string]storageConstraintsDoc)
	var doc storageConstraintsDoc
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		storageConstraints[e.st.localID(doc.DocID)] = doc
	}
	if err := iter.Close(); err != nil {
		return errors.Annotate(err, "failed to read storage constraints")
	}
	e.logger.Debugf("read %d storage constraint documents", len(storageConstraints))
	e.modelStorageConstraints = storageConstraints
	return nil
}

func (e *exporter) storageConstraints(doc storageConstraintsDoc) map[string]description.StorageConstraintArgs {
	result := make(map[string]description.StorageConstraintArgs)
	for key, value := range doc.Constraints {
		result[key] = description.StorageConstraintArgs{
			Pool:  value.Pool,
			Size:  value.Size,
			Count: value.Count,
		}
	}
	return result
}

func (e *exporter) readAllPayloads() (map[string][]payload.FullPayloadInfo, error) {
	result := make(map[string][]payload.FullPayloadInfo)
	all, err := ModelPayloads{db: e.st.database}.ListAll()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, payload := range all {
		result[payload.Unit] = append(result[payload.Unit], payload)
	}
	return result, nil
}

type addApplicationContext struct {
	application      *Application
	units            []*Unit
	meterStatus      map[string]*meterStatusDoc
	leader           string
	payloads         map[string][]payload.FullPayloadInfo
	resources        resource.ApplicationResources
	endpoingBindings map[string]bindingsMap

	// CAAS
	podSpecs        map[string]string
	cloudServices   map[string]*cloudServiceDoc
	cloudContainers map[string]*cloudContainerDoc

	// Offers
	offers []*crossmodel.ApplicationOffer
}

func (e *exporter) addApplication(ctx addApplicationContext) error {
	application := ctx.application
	appName := application.Name()
	globalKey := application.globalKey()
	charmConfigKey := application.charmConfigKey()
	appConfigKey := application.applicationConfigKey()
	leadershipKey := leadershipSettingsKey(appName)
	storageConstraintsKey := application.storageConstraintsKey()

	applicationCharmSettingsDoc, found := e.modelSettings[charmConfigKey]
	if !found && !e.cfg.SkipSettings {
		return errors.Errorf("missing charm settings for application %q", appName)
	}
	delete(e.modelSettings, charmConfigKey)
	applicationConfigDoc, found := e.modelSettings[appConfigKey]
	if !found && !e.cfg.SkipSettings {
		return errors.Errorf("missing config for application %q", appName)
	}
	delete(e.modelSettings, appConfigKey)
	leadershipSettingsDoc, found := e.modelSettings[leadershipKey]
	if !found && !e.cfg.SkipSettings {
		return errors.Errorf("missing leadership settings for application %q", appName)
	}
	delete(e.modelSettings, leadershipKey)

	args := description.ApplicationArgs{
		Tag:                  application.ApplicationTag(),
		Type:                 e.model.Type(),
		Series:               application.doc.Series,
		Subordinate:          application.doc.Subordinate,
		CharmURL:             application.doc.CharmURL.String(),
		Channel:              application.doc.Channel,
		CharmModifiedVersion: application.doc.CharmModifiedVersion,
		ForceCharm:           application.doc.ForceCharm,
		Exposed:              application.doc.Exposed,
		PasswordHash:         application.doc.PasswordHash,
		Placement:            application.doc.Placement,
		DesiredScale:         application.doc.DesiredScale,
		MinUnits:             application.doc.MinUnits,
		EndpointBindings:     map[string]string(ctx.endpoingBindings[globalKey]),
		ApplicationConfig:    applicationConfigDoc.Settings,
		CharmConfig:          applicationCharmSettingsDoc.Settings,
		Leader:               ctx.leader,
		LeadershipSettings:   leadershipSettingsDoc.Settings,
		MetricsCredentials:   application.doc.MetricCredentials,
		PodSpec:              ctx.podSpecs[application.globalKey()],
	}

	if cloudService, found := ctx.cloudServices[application.globalKey()]; found {
		args.CloudService = e.cloudService(cloudService)
	}
	if constraints, found := e.modelStorageConstraints[storageConstraintsKey]; found {
		args.StorageConstraints = e.storageConstraints(constraints)
	}
	exApplication := e.model.AddApplication(args)

	// Populate offer list
	for _, offer := range ctx.offers {
		endpoints := make([]string, 0, len(offer.Endpoints))
		for _, ep := range offer.Endpoints {
			endpoints = append(endpoints, ep.Name)
		}

		userMap, err := e.st.GetOfferUsers(offer.OfferUUID)
		if err != nil {
			return errors.Annotatef(err, "ACL for offer %s", offer.OfferName)
		}

		var acl map[string]string
		if len(userMap) != 0 {
			acl = make(map[string]string, len(userMap))
			for user, access := range userMap {
				acl[user] = accessToString(access)
			}
		}

		_ = exApplication.AddOffer(description.ApplicationOfferArgs{
			OfferName: offer.OfferName,
			Endpoints: endpoints,
			ACL:       acl,
		})
	}

	// Find the current application status.
	statusArgs, err := e.statusArgs(globalKey)
	if err != nil {
		return errors.Annotatef(err, "status for application %s", appName)
	}
	exApplication.SetStatus(statusArgs)
	exApplication.SetStatusHistory(e.statusHistoryArgs(globalKey))
	exApplication.SetAnnotations(e.getAnnotations(globalKey))

	globalAppWorkloadKey := applicationGlobalOperatorKey(appName)
	operatorStatusArgs, err := e.statusArgs(globalAppWorkloadKey)
	if err != nil {
		if !errors.IsNotFound(err) {
			return errors.Annotatef(err, "application operator status for application %s", appName)
		}
	}
	exApplication.SetOperatorStatus(operatorStatusArgs)
	e.statusHistoryArgs(globalAppWorkloadKey)

	constraintsArgs, err := e.constraintsArgs(globalKey)
	if err != nil {
		return errors.Trace(err)
	}
	exApplication.SetConstraints(constraintsArgs)

	if err := e.setResources(exApplication, ctx.resources); err != nil {
		return errors.Trace(err)
	}

	for _, unit := range ctx.units {
		agentKey := unit.globalAgentKey()
		unitMeterStatus, found := ctx.meterStatus[agentKey]
		if !found {
			return errors.Errorf("missing meter status for unit %s", unit.Name())
		}

		workloadVersion, err := e.unitWorkloadVersion(unit)
		if err != nil {
			return errors.Trace(err)
		}
		args := description.UnitArgs{
			Tag:             unit.UnitTag(),
			Type:            string(unit.modelType),
			Machine:         names.NewMachineTag(unit.doc.MachineId),
			WorkloadVersion: workloadVersion,
			PasswordHash:    unit.doc.PasswordHash,
			MeterStatusCode: unitMeterStatus.Code,
			MeterStatusInfo: unitMeterStatus.Info,
		}
		if principalName, isSubordinate := unit.PrincipalName(); isSubordinate {
			args.Principal = names.NewUnitTag(principalName)
		}
		if subs := unit.SubordinateNames(); len(subs) > 0 {
			for _, subName := range subs {
				args.Subordinates = append(args.Subordinates, names.NewUnitTag(subName))
			}
		}
		if cloudContainer, found := ctx.cloudContainers[unit.globalKey()]; found {
			args.CloudContainer = e.cloudContainer(cloudContainer)
		}
		exUnit := exApplication.AddUnit(args)

		e.setUnitResources(exUnit, ctx.resources.UnitResources)

		if err := e.setUnitPayloads(exUnit, ctx.payloads[unit.UnitTag().Id()]); err != nil {
			return errors.Trace(err)
		}

		// workload uses globalKey, agent uses globalAgentKey,
		// workload version uses globalWorkloadVersionKey.
		globalKey := unit.globalKey()
		statusArgs, err := e.statusArgs(globalKey)
		if err != nil {
			return errors.Annotatef(err, "workload status for unit %s", unit.Name())
		}
		exUnit.SetWorkloadStatus(statusArgs)
		exUnit.SetWorkloadStatusHistory(e.statusHistoryArgs(globalKey))

		statusArgs, err = e.statusArgs(agentKey)
		if err != nil {
			return errors.Annotatef(err, "agent status for unit %s", unit.Name())
		}
		exUnit.SetAgentStatus(statusArgs)
		exUnit.SetAgentStatusHistory(e.statusHistoryArgs(agentKey))

		workloadVersionKey := unit.globalWorkloadVersionKey()
		exUnit.SetWorkloadVersionHistory(e.statusHistoryArgs(workloadVersionKey))

		if e.dbModel.Type() != ModelTypeCAAS && !e.cfg.SkipUnitAgentBinaries {
			tools, err := unit.AgentTools()
			if err != nil {
				// This means the tools aren't set, but they should be.
				return errors.Trace(err)
			}
			exUnit.SetTools(description.AgentToolsArgs{
				Version: tools.Version,
				URL:     tools.URL,
				SHA256:  tools.SHA256,
				Size:    tools.Size,
			})
		} else {
			// TODO(caas) - Actually use the exported cloud container details and status history.
			// Currently these are only grabbed to make the MigrationExportSuite tests happy.
			globalCCKey := unit.globalCloudContainerKey()
			_, err = e.statusArgs(globalCCKey)
			if err != nil {
				if !errors.IsNotFound(err) {
					return errors.Annotatef(err, "cloud container workload status for unit %s", unit.Name())
				}
			}
			e.statusHistoryArgs(globalCCKey)
		}
		exUnit.SetAnnotations(e.getAnnotations(globalKey))

		constraintsArgs, err := e.constraintsArgs(agentKey)
		if err != nil {
			return errors.Trace(err)
		}
		exUnit.SetConstraints(constraintsArgs)
	}

	// Set Tools for application - this is only for CAAS models.
	if e.dbModel.Type() == ModelTypeCAAS {
		tools, err := ctx.application.AgentTools()
		if err != nil {
			// This means the tools aren't set, but they should be.
			return errors.Trace(err)
		}
		exApplication.SetTools(description.AgentToolsArgs{
			Version: tools.Version,
		})
	}

	return nil
}

func (e *exporter) unitWorkloadVersion(unit *Unit) (string, error) {
	// Rather than call unit.WorkloadVersion(), which does a database
	// query, we go directly to the status value that is stored.
	key := unit.globalWorkloadVersionKey()
	info, err := e.statusArgs(key)
	if err != nil {
		return "", errors.Trace(err)
	}
	return info.Message, nil
}

func (e *exporter) setResources(exApp description.Application, resources resource.ApplicationResources) error {
	if len(resources.Resources) != len(resources.CharmStoreResources) {
		return errors.New("number of resources don't match charm store resources")
	}

	for i, resource := range resources.Resources {
		exResource := exApp.AddResource(description.ResourceArgs{
			Name: resource.Name,
		})
		exResource.SetApplicationRevision(description.ResourceRevisionArgs{
			Revision:       resource.Revision,
			Type:           resource.Type.String(),
			Path:           resource.Path,
			Description:    resource.Description,
			Origin:         resource.Origin.String(),
			FingerprintHex: resource.Fingerprint.Hex(),
			Size:           resource.Size,
			Timestamp:      resource.Timestamp,
			Username:       resource.Username,
		})
		csResource := resources.CharmStoreResources[i]
		exResource.SetCharmStoreRevision(description.ResourceRevisionArgs{
			Revision:       csResource.Revision,
			Type:           csResource.Type.String(),
			Path:           csResource.Path,
			Description:    csResource.Description,
			Origin:         csResource.Origin.String(),
			Size:           csResource.Size,
			FingerprintHex: csResource.Fingerprint.Hex(),
		})
	}

	return nil
}

func (e *exporter) setUnitResources(exUnit description.Unit, allResources []resource.UnitResources) {
	for _, resource := range findUnitResources(exUnit.Name(), allResources) {
		exUnit.AddResource(description.UnitResourceArgs{
			Name: resource.Name,
			RevisionArgs: description.ResourceRevisionArgs{
				Revision:       resource.Revision,
				Type:           resource.Type.String(),
				Path:           resource.Path,
				Description:    resource.Description,
				Origin:         resource.Origin.String(),
				FingerprintHex: resource.Fingerprint.Hex(),
				Size:           resource.Size,
				Timestamp:      resource.Timestamp,
				Username:       resource.Username,
			},
		})
	}
}

func findUnitResources(unitName string, allResources []resource.UnitResources) []resource.Resource {
	for _, unitResources := range allResources {
		if unitResources.Tag.Id() == unitName {
			return unitResources.Resources
		}
	}
	return nil
}

func (e *exporter) setUnitPayloads(exUnit description.Unit, payloads []payload.FullPayloadInfo) error {
	if len(payloads) == 0 {
		return nil
	}
	unitID := exUnit.Tag().Id()
	machineID := exUnit.Machine().Id()
	for _, payload := range payloads {
		if payload.Machine != machineID {
			return errors.NotValidf("payload for unit %q reports wrong machine %q (should be %q)", unitID, payload.Machine, machineID)
		}
		args := description.PayloadArgs{
			Name:   payload.Name,
			Type:   payload.Type,
			RawID:  payload.ID,
			State:  payload.Status,
			Labels: payload.Labels,
		}
		exUnit.AddPayload(args)
	}
	return nil
}

func (e *exporter) relations() error {
	rels, err := e.st.AllRelations()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d relations", len(rels))

	relationScopes := set.NewStrings()
	if !e.cfg.SkipRelationData {
		relationScopes, err = e.readAllRelationScopes()
		if err != nil {
			return errors.Trace(err)
		}
	}

	remoteApps := make(set.Strings)
	for _, a := range e.model.RemoteApplications() {
		remoteApps.Add(a.Name())
	}
	for _, relation := range rels {
		exRelation := e.model.AddRelation(description.RelationArgs{
			Id:  relation.Id(),
			Key: relation.String(),
		})
		globalKey := relation.globalScope()
		statusArgs, err := e.statusArgs(globalKey)
		if err == nil {
			exRelation.SetStatus(statusArgs)
		} else if !errors.IsNotFound(err) {
			return errors.Annotatef(err, "status for relation %v", relation.Id())
		}

		isRemote := false
		for _, ep := range relation.Endpoints() {
			if remoteApps.Contains(ep.ApplicationName) {
				isRemote = true
				break
			}
		}
		for _, ep := range relation.Endpoints() {
			exEndPoint := exRelation.AddEndpoint(description.EndpointArgs{
				ApplicationName: ep.ApplicationName,
				Name:            ep.Name,
				Role:            string(ep.Role),
				Interface:       ep.Interface,
				Optional:        ep.Optional,
				Limit:           ep.Limit,
				Scope:           string(ep.Scope),
			})
			// We expect a relationScope and settings for each of the
			// units of the specified application, unless it is a
			// remote application.
			if isRemote {
				continue
			}
			units := e.units[ep.ApplicationName]
			for _, unit := range units {
				ru, err := relation.Unit(unit)
				if err != nil {
					return errors.Trace(err)
				}
				valid, err := ru.Valid()
				if err != nil {
					return errors.Trace(err)
				}
				if !valid {
					// It doesn't make sense for this application to have a
					// relations scope for this endpoint. For example the
					// situation where we have a subordinate charm related to
					// two different principals.
					continue
				}
				key := ru.key()
				if !e.cfg.SkipRelationData && !relationScopes.Contains(key) {
					return errors.Errorf("missing relation scope for %s and %s", relation, unit.Name())
				}
				settingsDoc, found := e.modelSettings[key]
				if !found && !e.cfg.SkipSettings && !e.cfg.SkipRelationData {
					return errors.Errorf("missing relation settings for %s and %s", relation, unit.Name())
				}
				delete(e.modelSettings, key)
				exEndPoint.SetUnitSettings(unit.Name(), settingsDoc.Settings)
			}
		}
	}
	return nil
}

func (e *exporter) spaces() error {
	spaces, err := e.st.AllSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d spaces", len(spaces))

	for _, space := range spaces {
		// We do not export the default space because it is created by default
		// with the new model. This is OK, because it is immutable.
		// Any subnets added to the space will still be exported.
		if space.Name() == environs.DefaultSpaceName {
			continue
		}

		// TODO (manadart 2019-07-12): Update juju/description and export IDs.
		e.model.AddSpace(description.SpaceArgs{
			Name:       space.Name(),
			Public:     space.IsPublic(),
			ProviderID: string(space.ProviderId()),
		})
	}
	return nil
}

func (e *exporter) linklayerdevices() error {
	if e.cfg.SkipLinkLayerDevices {
		return nil
	}
	linklayerdevices, err := e.st.AllLinkLayerDevices()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d ip devices", len(linklayerdevices))
	for _, device := range linklayerdevices {
		e.model.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
			ProviderID:  string(device.ProviderID()),
			MachineID:   device.MachineID(),
			Name:        device.Name(),
			MTU:         device.MTU(),
			Type:        string(device.Type()),
			MACAddress:  device.MACAddress(),
			IsAutoStart: device.IsAutoStart(),
			IsUp:        device.IsUp(),
			ParentName:  device.ParentName(),
		})
	}
	return nil
}

func (e *exporter) subnets() error {
	subnets, err := e.st.AllSubnets()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d subnets", len(subnets))

	for _, subnet := range subnets {
		args := description.SubnetArgs{
			CIDR:              subnet.CIDR(),
			ProviderId:        string(subnet.ProviderId()),
			ProviderNetworkId: string(subnet.ProviderNetworkId()),
			VLANTag:           subnet.VLANTag(),
			SpaceName:         subnet.SpaceName(),
			FanLocalUnderlay:  subnet.FanLocalUnderlay(),
			FanOverlay:        subnet.FanOverlay(),
		}
		// TODO(babbageclunk): at the moment state.Subnet only stores
		// one AZ.
		az := subnet.AvailabilityZone()
		if az != "" {
			args.AvailabilityZones = []string{az}
		}
		e.model.AddSubnet(args)
	}
	return nil
}

func (e *exporter) ipaddresses() error {
	if e.cfg.SkipIPAddresses {
		return nil
	}
	ipaddresses, err := e.st.AllIPAddresses()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d ip addresses", len(ipaddresses))
	for _, addr := range ipaddresses {
		e.model.AddIPAddress(description.IPAddressArgs{
			ProviderID:       string(addr.ProviderID()),
			DeviceName:       addr.DeviceName(),
			MachineID:        addr.MachineID(),
			SubnetCIDR:       addr.SubnetCIDR(),
			ConfigMethod:     string(addr.ConfigMethod()),
			Value:            addr.Value(),
			DNSServers:       addr.DNSServers(),
			DNSSearchDomains: addr.DNSSearchDomains(),
			GatewayAddress:   addr.GatewayAddress(),
		})
	}
	return nil
}

func (e *exporter) sshHostKeys() error {
	if e.cfg.SkipSSHHostKeys {
		return nil
	}
	machines, err := e.st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	for _, machine := range machines {
		keys, err := e.st.GetSSHHostKeys(machine.MachineTag())
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		if len(keys) == 0 {
			continue
		}
		e.model.AddSSHHostKey(description.SSHHostKeyArgs{
			MachineID: machine.Id(),
			Keys:      keys,
		})
	}
	return nil
}

func (e *exporter) cloudimagemetadata() error {
	if e.cfg.SkipCloudImageMetadata {
		return nil
	}
	cloudimagemetadata, err := e.st.CloudImageMetadataStorage.AllCloudImageMetadata()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d cloudimagemetadata", len(cloudimagemetadata))
	for _, metadata := range cloudimagemetadata {
		e.model.AddCloudImageMetadata(description.CloudImageMetadataArgs{
			Stream:          metadata.Stream,
			Region:          metadata.Region,
			Version:         metadata.Version,
			Series:          metadata.Series,
			Arch:            metadata.Arch,
			VirtType:        metadata.VirtType,
			RootStorageType: metadata.RootStorageType,
			RootStorageSize: metadata.RootStorageSize,
			DateCreated:     metadata.DateCreated,
			Source:          metadata.Source,
			Priority:        metadata.Priority,
			ImageId:         metadata.ImageId,
		})
	}
	return nil
}

func (e *exporter) actions() error {
	if e.cfg.SkipActions {
		return nil
	}

	m, err := e.st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	actions, err := m.AllActions()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d actions", len(actions))
	for _, action := range actions {
		results, message := action.Results()
		e.model.AddAction(description.ActionArgs{
			Receiver:   action.Receiver(),
			Name:       action.Name(),
			Parameters: action.Parameters(),
			Enqueued:   action.Enqueued(),
			Started:    action.Started(),
			Completed:  action.Completed(),
			Status:     string(action.Status()),
			Results:    results,
			Message:    message,
			Id:         action.Id(),
		})
	}
	return nil
}

func (e *exporter) readAllRelationScopes() (set.Strings, error) {
	relationScopes, closer := e.st.db().GetCollection(relationScopesC)
	defer closer()

	docs := []relationScopeDoc{}
	err := relationScopes.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all relation scopes")
	}
	e.logger.Debugf("found %d relationScope docs", len(docs))

	result := set.NewStrings()
	for _, doc := range docs {
		result.Add(doc.Key)
	}
	return result, nil
}

func (e *exporter) readAllUnits() (map[string][]*Unit, error) {
	unitsCollection, closer := e.st.db().GetCollection(unitsC)
	defer closer()

	docs := []unitDoc{}
	err := unitsCollection.Find(nil).Sort("name").All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all units")
	}
	e.logger.Debugf("found %d unit docs", len(docs))
	model, err := e.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string][]*Unit)
	for _, doc := range docs {
		units := result[doc.Application]
		result[doc.Application] = append(units, newUnit(e.st, model.Type(), &doc))
	}
	return result, nil
}

func (e *exporter) readAllEndpointBindings() (map[string]bindingsMap, error) {
	bindings, closer := e.st.db().GetCollection(endpointBindingsC)
	defer closer()

	docs := []endpointBindingsDoc{}
	err := bindings.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all application endpoint bindings")
	}
	e.logger.Debugf("found %d application endpoint binding docs", len(docs))
	result := make(map[string]bindingsMap)
	for _, doc := range docs {
		result[e.st.localID(doc.DocID)] = doc.Bindings
	}
	return result, nil
}

func (e *exporter) readAllMeterStatus() (map[string]*meterStatusDoc, error) {
	meterStatuses, closer := e.st.db().GetCollection(meterStatusC)
	defer closer()

	docs := []meterStatusDoc{}
	err := meterStatuses.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all meter status docs")
	}
	e.logger.Debugf("found %d meter status docs", len(docs))
	result := make(map[string]*meterStatusDoc)
	for _, doc := range docs {
		result[e.st.localID(doc.DocID)] = &doc
	}
	return result, nil
}

func (e *exporter) readAllPodSpecs() (map[string]string, error) {
	specs, closer := e.st.db().GetCollection(podSpecsC)
	defer closer()

	docs := []containerSpecDoc{}
	err := specs.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all pod spec docs")
	}
	e.logger.Debugf("found %d pod spec docs", len(docs))
	result := make(map[string]string)
	for _, doc := range docs {
		result[e.st.localID(doc.Id)] = doc.Spec
	}
	return result, nil
}

func (e *exporter) readAllCloudServices() (map[string]*cloudServiceDoc, error) {
	cloudServices, closer := e.st.db().GetCollection(cloudServicesC)
	defer closer()

	docs := []cloudServiceDoc{}
	err := cloudServices.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all cloud service docs")
	}
	e.logger.Debugf("found %d cloud service docs", len(docs))
	result := make(map[string]*cloudServiceDoc)
	for _, doc := range docs {
		result[e.st.localID(doc.DocID)] = &doc
	}
	return result, nil
}

func (e *exporter) cloudService(doc *cloudServiceDoc) *description.CloudServiceArgs {
	return &description.CloudServiceArgs{
		ProviderId: doc.ProviderId,
		Addresses:  e.newAddressArgsSlice(doc.Addresses),
	}
}

func (e *exporter) readAllCloudContainers() (map[string]*cloudContainerDoc, error) {
	cloudContainers, closer := e.st.db().GetCollection(cloudContainersC)
	defer closer()

	docs := []cloudContainerDoc{}
	err := cloudContainers.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all cloud container docs")
	}
	e.logger.Debugf("found %d cloud container docs", len(docs))
	result := make(map[string]*cloudContainerDoc)
	for _, doc := range docs {
		result[e.st.localID(doc.Id)] = &doc
	}
	return result, nil
}

func (e *exporter) cloudContainer(doc *cloudContainerDoc) *description.CloudContainerArgs {
	result := &description.CloudContainerArgs{
		ProviderId: doc.ProviderId,
		Ports:      doc.Ports,
	}
	if doc.Address != nil {
		result.Address = e.newAddressArgs(*doc.Address)
	}
	return result
}

func (e *exporter) readLastConnectionTimes() (map[string]time.Time, error) {
	lastConnections, closer := e.st.db().GetCollection(modelUserLastConnectionC)
	defer closer()

	var docs []modelUserLastConnectionDoc
	if err := lastConnections.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]time.Time)
	for _, doc := range docs {
		result[doc.UserName] = doc.LastConnection.UTC()
	}
	return result, nil
}

func (e *exporter) readAllAnnotations() error {
	e.annotations = make(map[string]annotatorDoc)
	if e.cfg.SkipAnnotations {
		return nil
	}

	annotations, closer := e.st.db().GetCollection(annotationsC)
	defer closer()

	var docs []annotatorDoc
	if err := annotations.Find(nil).All(&docs); err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d annotations docs", len(docs))

	for _, doc := range docs {
		e.annotations[doc.GlobalKey] = doc
	}
	return nil
}

func (e *exporter) readAllConstraints() error {
	constraintsCollection, closer := e.st.db().GetCollection(constraintsC)
	defer closer()

	// Since the constraintsDoc doesn't include any global key or _id
	// fields, we can't just deserialize the entire collection into a slice
	// of docs, so we get them all out with bson maps.
	var docs []bson.M
	err := constraintsCollection.Find(nil).All(&docs)
	if err != nil {
		return errors.Annotate(err, "failed to read constraints collection")
	}

	e.logger.Debugf("read %d constraints docs", len(docs))
	e.constraints = make(map[string]bson.M)
	for _, doc := range docs {
		docId, ok := doc["_id"].(string)
		if !ok {
			return errors.Errorf("expected string, got %s (%T)", doc["_id"], doc["_id"])
		}
		id := e.st.localID(docId)
		e.constraints[id] = doc
		e.logger.Debugf("doc[%q] = %#v", id, doc)
	}
	return nil
}

// getAnnotations doesn't really care if there are any there or not
// for the key, but if they were there, they are removed so we can
// check at the end of the export for anything we have forgotten.
func (e *exporter) getAnnotations(key string) map[string]string {
	result, found := e.annotations[key]
	if found {
		delete(e.annotations, key)
	}
	return result.Annotations
}

func (e *exporter) readAllSettings() error {
	e.modelSettings = make(map[string]settingsDoc)
	if e.cfg.SkipSettings {
		return nil
	}

	settings, closer := e.st.db().GetCollection(settingsC)
	defer closer()

	var docs []settingsDoc
	if err := settings.Find(nil).All(&docs); err != nil {
		return errors.Trace(err)
	}

	for _, doc := range docs {
		key := e.st.localID(doc.DocID)
		e.modelSettings[key] = doc
	}
	return nil
}

func (e *exporter) readAllStatuses() error {
	statuses, closer := e.st.db().GetCollection(statusesC)
	defer closer()

	var docs []bson.M
	err := statuses.Find(nil).All(&docs)
	if err != nil {
		return errors.Annotate(err, "failed to read status collection")
	}

	e.logger.Debugf("read %d status documents", len(docs))
	e.status = make(map[string]bson.M)
	for _, doc := range docs {
		docId, ok := doc["_id"].(string)
		if !ok {
			return errors.Errorf("expected string, got %s (%T)", doc["_id"], doc["_id"])
		}
		id := e.st.localID(docId)
		e.status[id] = doc
	}

	return nil
}

func (e *exporter) readAllStatusHistory() error {
	statuses, closer := e.st.db().GetCollection(statusesHistoryC)
	defer closer()

	count := 0
	e.statusHistory = make(map[string][]historicalStatusDoc)
	if e.cfg.SkipStatusHistory {
		return nil
	}
	var doc historicalStatusDoc
	// In tests, sorting by time can leave the results
	// underconstrained - include document id for deterministic
	// ordering in those cases.
	iter := statuses.Find(nil).Sort("-updated", "-_id").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		history := e.statusHistory[doc.GlobalKey]
		e.statusHistory[doc.GlobalKey] = append(history, doc)
		count++
	}

	if err := iter.Close(); err != nil {
		return errors.Annotate(err, "failed to read status history collection")
	}

	e.logger.Debugf("read %d status history documents", count)

	return nil
}

func (e *exporter) statusArgs(globalKey string) (description.StatusArgs, error) {
	result := description.StatusArgs{}
	statusDoc, found := e.status[globalKey]
	if !found {
		return result, errors.NotFoundf("status data for %s", globalKey)
	}
	delete(e.status, globalKey)

	status, ok := statusDoc["status"].(string)
	if !ok {
		return result, errors.Errorf("expected string for status, got %T", statusDoc["status"])
	}
	info, ok := statusDoc["statusinfo"].(string)
	if !ok {
		return result, errors.Errorf("expected string for statusinfo, got %T", statusDoc["statusinfo"])
	}
	// data is an embedded map and comes out as a bson.M
	// A bson.M is map[string]interface{}, so we can type cast it.
	data, ok := statusDoc["statusdata"].(bson.M)
	if !ok {
		return result, errors.Errorf("expected map for data, got %T", statusDoc["statusdata"])
	}
	dataMap := map[string]interface{}(data)
	updated, ok := statusDoc["updated"].(int64)
	if !ok {
		return result, errors.Errorf("expected int64 for updated, got %T", statusDoc["updated"])
	}
	neverset, ok := statusDoc["neverset"].(bool)
	if !ok {
		return result, errors.Errorf("expected neverset for updated, got %T", statusDoc["neverset"])
	}

	result.Value = status
	result.Message = info
	result.Data = dataMap
	result.Updated = time.Unix(0, updated)
	result.NeverSet = neverset
	return result, nil
}

func (e *exporter) statusHistoryArgs(globalKey string) []description.StatusArgs {
	history := e.statusHistory[globalKey]
	e.logger.Tracef("found %d status history docs for %s", len(history), globalKey)
	if len(history) > maxStatusHistoryEntries {
		history = history[:maxStatusHistoryEntries]
	}
	result := make([]description.StatusArgs, len(history))
	for i, doc := range history {
		result[i] = description.StatusArgs{
			Value:   string(doc.Status),
			Message: doc.StatusInfo,
			Data:    doc.StatusData,
			Updated: time.Unix(0, doc.Updated),
		}
	}
	delete(e.statusHistory, globalKey)
	return result
}

func (e *exporter) constraintsArgs(globalKey string) (description.ConstraintsArgs, error) {
	doc, found := e.constraints[globalKey]
	if !found {
		// No constraints for this key.
		e.logger.Tracef("no constraints found for key %q", globalKey)
		return description.ConstraintsArgs{}, nil
	}
	// We capture any type error using a closure to avoid having to return
	// multiple values from the optional functions. This does mean that we will
	// only report on the last one, but that is fine as there shouldn't be any.
	var optionalErr error
	optionalString := func(name string) string {
		switch value := doc[name].(type) {
		case nil:
		case string:
			return value
		default:
			optionalErr = errors.Errorf("expected string for %s, got %T", name, value)
		}
		return ""
	}
	optionalInt := func(name string) uint64 {
		switch value := doc[name].(type) {
		case nil:
		case uint64:
			return value
		case int64:
			return uint64(value)
		default:
			optionalErr = errors.Errorf("expected uint64 for %s, got %T", name, value)
		}
		return 0
	}
	optionalStringSlice := func(name string) []string {
		switch value := doc[name].(type) {
		case nil:
		case []string:
			return value
		case []interface{}:
			var result []string
			for _, val := range value {
				sval, ok := val.(string)
				if !ok {
					optionalErr = errors.Errorf("expected string slice for %s, got %T value", name, val)
					return nil
				}
				result = append(result, sval)
			}
			return result
		default:
			optionalErr = errors.Errorf("expected []string for %s, got %T", name, value)
		}
		return nil
	}
	result := description.ConstraintsArgs{
		Architecture:   optionalString("arch"),
		Container:      optionalString("container"),
		CpuCores:       optionalInt("cpucores"),
		CpuPower:       optionalInt("cpupower"),
		InstanceType:   optionalString("instancetype"),
		Memory:         optionalInt("mem"),
		RootDisk:       optionalInt("rootdisk"),
		RootDiskSource: optionalString("rootdisksource"),
		Spaces:         optionalStringSlice("spaces"),
		Tags:           optionalStringSlice("tags"),
		VirtType:       optionalString("virttype"),
		Zones:          optionalStringSlice("zones"),
	}
	if optionalErr != nil {
		return description.ConstraintsArgs{}, errors.Trace(optionalErr)
	}
	return result, nil
}

func (e *exporter) checkUnexportedValues() error {
	var missing []string

	// As annotations are saved into the model, they are removed from the
	// exporter's map. If there are any left at the end, we are missing
	// things.
	for key, doc := range e.annotations {
		missing = append(missing, fmt.Sprintf("unexported annotations for %s, %s", doc.Tag, key))
	}

	for key := range e.modelSettings {
		missing = append(missing, fmt.Sprintf("unexported settings for %s", key))
	}

	for key := range e.status {
		if !e.cfg.SkipInstanceData && !strings.HasSuffix(key, "#instance") {
			missing = append(missing, fmt.Sprintf("unexported status for %s", key))
		}
	}

	for key := range e.statusHistory {
		if !e.cfg.SkipInstanceData && !(strings.HasSuffix(key, "#instance") || strings.HasSuffix(key, "#modification")) {
			missing = append(missing, fmt.Sprintf("unexported status history for %s", key))
		}
	}

	if len(missing) > 0 {
		content := strings.Join(missing, "\n  ")
		return errors.Errorf("migration missed some docs:\n  %s", content)
	}
	return nil
}

func (e *exporter) remoteApplications() error {
	remoteApps, err := e.st.AllRemoteApplications()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d remote applications", len(remoteApps))
	for _, remoteApp := range remoteApps {
		err := e.addRemoteApplication(remoteApp)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *exporter) addRemoteApplication(app *RemoteApplication) error {
	url, _ := app.URL()
	args := description.RemoteApplicationArgs{
		Tag:             app.Tag().(names.ApplicationTag),
		OfferUUID:       app.OfferUUID(),
		URL:             url,
		SourceModel:     app.SourceModel(),
		IsConsumerProxy: app.IsConsumerProxy(),
		Bindings:        app.Bindings(),
	}
	descApp := e.model.AddRemoteApplication(args)
	status, err := e.statusArgs(app.globalKey())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	// Not all remote applications have status.
	if err == nil {
		descApp.SetStatus(status)
	}
	endpoints, err := app.Endpoints()
	if err != nil {
		return errors.Trace(err)
	}
	for _, ep := range endpoints {
		descApp.AddEndpoint(description.RemoteEndpointArgs{
			Name:      ep.Name,
			Role:      string(ep.Role),
			Interface: ep.Interface,
		})
	}
	for _, space := range app.Spaces() {
		e.addRemoteSpace(descApp, space)
	}
	return nil
}

func (e *exporter) addRemoteSpace(descApp description.RemoteApplication, space RemoteSpace) {
	descSpace := descApp.AddSpace(description.RemoteSpaceArgs{
		CloudType:          space.CloudType,
		Name:               space.Name,
		ProviderId:         space.ProviderId,
		ProviderAttributes: space.ProviderAttributes,
	})
	for _, subnet := range space.Subnets {
		descSpace.AddSubnet(description.SubnetArgs{
			CIDR:              subnet.CIDR,
			ProviderId:        subnet.ProviderId,
			VLANTag:           subnet.VLANTag,
			AvailabilityZones: subnet.AvailabilityZones,
			ProviderSpaceId:   subnet.ProviderSpaceId,
			ProviderNetworkId: subnet.ProviderNetworkId,
		})
	}
}

func (e *exporter) storage() error {
	if err := e.volumes(); err != nil {
		return errors.Trace(err)
	}
	if err := e.filesystems(); err != nil {
		return errors.Trace(err)
	}
	if err := e.storageInstances(); err != nil {
		return errors.Trace(err)
	}
	if err := e.storagePools(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (e *exporter) volumes() error {
	coll, closer := e.st.db().GetCollection(volumesC)
	defer closer()

	attachments, err := e.readVolumeAttachments()
	if err != nil {
		return errors.Trace(err)
	}

	attachmentPlans, err := e.readVolumeAttachmentPlans()
	if err != nil {
		return errors.Trace(err)
	}

	var doc volumeDoc
	iter := coll.Find(nil).Sort("_id").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		vol := &volume{e.st, doc}
		plan := attachmentPlans[doc.Name]
		if err := e.addVolume(vol, attachments[doc.Name], plan); err != nil {
			return errors.Trace(err)
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Annotate(err, "failed to read volumes")
	}
	return nil
}

func (e *exporter) addVolume(vol *volume, volAttachments []volumeAttachmentDoc, attachmentPlans []volumeAttachmentPlanDoc) error {
	args := description.VolumeArgs{
		Tag: vol.VolumeTag(),
	}
	if tag, err := vol.StorageInstance(); err == nil {
		// only returns an error when no storage tag.
		args.Storage = tag
	} else {
		if !errors.IsNotAssigned(err) {
			// This is an unexpected error.
			return errors.Trace(err)
		}
	}
	logger.Debugf("addVolume: %#v", vol.doc)
	if info, err := vol.Info(); err == nil {
		logger.Debugf("  info %#v", info)
		args.Provisioned = true
		args.Size = info.Size
		args.Pool = info.Pool
		args.HardwareID = info.HardwareId
		args.WWN = info.WWN
		args.VolumeID = info.VolumeId
		args.Persistent = info.Persistent
	} else {
		params, _ := vol.Params()
		logger.Debugf("  params %#v", params)
		args.Size = params.Size
		args.Pool = params.Pool
	}

	globalKey := vol.globalKey()
	statusArgs, err := e.statusArgs(globalKey)
	if err != nil {
		return errors.Annotatef(err, "status for volume %s", vol.doc.Name)
	}

	exVolume := e.model.AddVolume(args)
	exVolume.SetStatus(statusArgs)
	exVolume.SetStatusHistory(e.statusHistoryArgs(globalKey))
	if count := len(volAttachments); count != vol.doc.AttachmentCount {
		return errors.Errorf("volume attachment count mismatch, have %d, expected %d",
			count, vol.doc.AttachmentCount)
	}
	for _, doc := range volAttachments {
		va := volumeAttachment{doc}
		logger.Debugf("  attachment %#v", doc)
		args := description.VolumeAttachmentArgs{
			Host: va.Host(),
		}
		if info, err := va.Info(); err == nil {
			logger.Debugf("    info %#v", info)
			args.Provisioned = true
			args.ReadOnly = info.ReadOnly
			args.DeviceName = info.DeviceName
			args.DeviceLink = info.DeviceLink
			args.BusAddress = info.BusAddress
			if info.PlanInfo != nil {
				args.DeviceType = string(info.PlanInfo.DeviceType)
				args.DeviceAttributes = info.PlanInfo.DeviceAttributes
			}
		} else {
			params, _ := va.Params()
			logger.Debugf("    params %#v", params)
			args.ReadOnly = params.ReadOnly
		}
		exVolume.AddAttachment(args)
	}

	for _, doc := range attachmentPlans {
		va := volumeAttachmentPlan{doc}
		logger.Debugf("  attachment plan %#v", doc)
		args := description.VolumeAttachmentPlanArgs{
			Machine: va.Machine(),
		}
		if info, err := va.PlanInfo(); err == nil {
			logger.Debugf("    plan info %#v", info)
			args.DeviceType = string(info.DeviceType)
			args.DeviceAttributes = info.DeviceAttributes
		} else if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if info, err := va.BlockDeviceInfo(); err == nil {
			logger.Debugf("    block device info %#v", info)
			args.DeviceName = info.DeviceName
			args.DeviceLinks = info.DeviceLinks
			args.Label = info.Label
			args.UUID = info.UUID
			args.HardwareId = info.HardwareId
			args.WWN = info.WWN
			args.BusAddress = info.BusAddress
			args.Size = info.Size
			args.FilesystemType = info.FilesystemType
			args.InUse = info.InUse
			args.MountPoint = info.MountPoint
		} else if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		exVolume.AddAttachmentPlan(args)
	}
	return nil
}

func (e *exporter) readVolumeAttachments() (map[string][]volumeAttachmentDoc, error) {
	coll, closer := e.st.db().GetCollection(volumeAttachmentsC)
	defer closer()

	result := make(map[string][]volumeAttachmentDoc)
	var doc volumeAttachmentDoc
	var count int
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		result[doc.Volume] = append(result[doc.Volume], doc)
		count++
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "failed to read volumes attachments")
	}
	e.logger.Debugf("read %d volume attachment documents", count)
	return result, nil
}

func (e *exporter) readVolumeAttachmentPlans() (map[string][]volumeAttachmentPlanDoc, error) {
	coll, closer := e.st.db().GetCollection(volumeAttachmentPlanC)
	defer closer()

	result := make(map[string][]volumeAttachmentPlanDoc)
	var doc volumeAttachmentPlanDoc
	var count int
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		result[doc.Volume] = append(result[doc.Volume], doc)
		count++
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "failed to read volume attachment plans")
	}
	e.logger.Debugf("read %d volume attachment plan documents", count)
	return result, nil
}

func (e *exporter) filesystems() error {
	coll, closer := e.st.db().GetCollection(filesystemsC)
	defer closer()

	attachments, err := e.readFilesystemAttachments()
	if err != nil {
		return errors.Trace(err)
	}
	var doc filesystemDoc
	iter := coll.Find(nil).Sort("_id").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		fs := &filesystem{e.st, doc}
		if err := e.addFilesystem(fs, attachments[doc.FilesystemId]); err != nil {
			return errors.Trace(err)
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Annotate(err, "failed to read filesystems")
	}
	return nil
}

func (e *exporter) addFilesystem(fs *filesystem, fsAttachments []filesystemAttachmentDoc) error {
	// Here we don't care about the cases where the filesystem is not assigned to storage instances
	// nor no backing volues. In both those situations we have empty tags.
	storage, _ := fs.Storage()
	volume, _ := fs.Volume()
	args := description.FilesystemArgs{
		Tag:     fs.FilesystemTag(),
		Storage: storage,
		Volume:  volume,
	}
	logger.Debugf("addFilesystem: %#v", fs.doc)
	if info, err := fs.Info(); err == nil {
		logger.Debugf("  info %#v", info)
		args.Provisioned = true
		args.Size = info.Size
		args.Pool = info.Pool
		args.FilesystemID = info.FilesystemId
	} else {
		params, _ := fs.Params()
		logger.Debugf("  params %#v", params)
		args.Size = params.Size
		args.Pool = params.Pool
	}

	globalKey := fs.globalKey()
	statusArgs, err := e.statusArgs(globalKey)
	if err != nil {
		return errors.Annotatef(err, "status for filesystem %s", fs.doc.FilesystemId)
	}

	exFilesystem := e.model.AddFilesystem(args)
	exFilesystem.SetStatus(statusArgs)
	exFilesystem.SetStatusHistory(e.statusHistoryArgs(globalKey))
	if count := len(fsAttachments); count != fs.doc.AttachmentCount {
		return errors.Errorf("filesystem attachment count mismatch, have %d, expected %d",
			count, fs.doc.AttachmentCount)
	}
	for _, doc := range fsAttachments {
		va := filesystemAttachment{doc}
		logger.Debugf("  attachment %#v", doc)
		args := description.FilesystemAttachmentArgs{
			Host: va.Host(),
		}
		if info, err := va.Info(); err == nil {
			logger.Debugf("    info %#v", info)
			args.Provisioned = true
			args.ReadOnly = info.ReadOnly
			args.MountPoint = info.MountPoint
		} else {
			params, _ := va.Params()
			logger.Debugf("    params %#v", params)
			args.ReadOnly = params.ReadOnly
			args.MountPoint = params.Location
		}
		exFilesystem.AddAttachment(args)
	}
	return nil
}

func (e *exporter) readFilesystemAttachments() (map[string][]filesystemAttachmentDoc, error) {
	coll, closer := e.st.db().GetCollection(filesystemAttachmentsC)
	defer closer()

	result := make(map[string][]filesystemAttachmentDoc)
	var doc filesystemAttachmentDoc
	var count int
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		result[doc.Filesystem] = append(result[doc.Filesystem], doc)
		count++
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "failed to read filesystem attachments")
	}
	e.logger.Debugf("read %d filesystem attachment documents", count)
	return result, nil
}

func (e *exporter) storageInstances() error {
	sb, err := NewStorageBackend(e.st)
	if err != nil {
		return errors.Trace(err)
	}
	coll, closer := e.st.db().GetCollection(storageInstancesC)
	defer closer()

	attachments, err := e.readStorageAttachments()
	if err != nil {
		return errors.Trace(err)
	}
	var doc storageInstanceDoc
	iter := coll.Find(nil).Sort("_id").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		instance := &storageInstance{sb, doc}
		if err := e.addStorage(instance, attachments[doc.Id]); err != nil {
			return errors.Trace(err)
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Annotate(err, "failed to read storage instances")
	}
	return nil
}

func (e *exporter) addStorage(instance *storageInstance, attachments []names.UnitTag) error {
	owner, ok := instance.Owner()
	if !ok {
		owner = nil
	}
	cons := description.StorageInstanceConstraints(instance.doc.Constraints)
	args := description.StorageArgs{
		Tag:         instance.StorageTag(),
		Kind:        instance.Kind().String(),
		Owner:       owner,
		Name:        instance.StorageName(),
		Attachments: attachments,
		Constraints: &cons,
	}
	e.model.AddStorage(args)
	return nil
}

func (e *exporter) readStorageAttachments() (map[string][]names.UnitTag, error) {
	coll, closer := e.st.db().GetCollection(storageAttachmentsC)
	defer closer()

	result := make(map[string][]names.UnitTag)
	var doc storageAttachmentDoc
	var count int
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		unit := names.NewUnitTag(doc.Unit)
		result[doc.StorageInstance] = append(result[doc.StorageInstance], unit)
		count++
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "failed to read storage attachments")
	}
	e.logger.Debugf("read %d storage attachment documents", count)
	return result, nil
}

func (e *exporter) storagePools() error {
	registry, err := e.st.storageProviderRegistry()
	if err != nil {
		return errors.Annotate(err, "getting provider registry")
	}
	pm := poolmanager.New(storagePoolSettingsManager{e: e}, registry)
	poolConfigs, err := pm.List()
	if err != nil {
		return errors.Annotate(err, "listing pools")
	}
	for _, cfg := range poolConfigs {
		e.model.AddStoragePool(description.StoragePoolArgs{
			Name:       cfg.Name(),
			Provider:   string(cfg.Provider()),
			Attributes: cfg.Attrs(),
		})
	}
	return nil
}

func (e *exporter) groupOffersByApplicationName() (map[string][]*crossmodel.ApplicationOffer, error) {
	if e.cfg.SkipApplicationOffers {
		return nil, nil
	}

	offerList, err := NewApplicationOffers(e.st).AllApplicationOffers()
	if err != nil {
		return nil, errors.Annotate(err, "listing offers")
	}

	if len(offerList) == 0 {
		return nil, nil
	}

	appMap := make(map[string][]*crossmodel.ApplicationOffer)
	for _, offer := range offerList {
		appMap[offer.ApplicationName] = append(appMap[offer.ApplicationName], offer)
	}
	return appMap, nil
}

type storagePoolSettingsManager struct {
	poolmanager.SettingsManager
	e *exporter
}

func (m storagePoolSettingsManager) ListSettings(keyPrefix string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})
	for key, doc := range m.e.modelSettings {
		if strings.HasPrefix(key, keyPrefix) {
			result[key] = doc.Settings
			delete(m.e.modelSettings, key)
		}
	}
	return result, nil
}
