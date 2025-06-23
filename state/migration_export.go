// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/feature"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state/migrations"
	"github.com/juju/juju/storage/poolmanager"
)

// The following exporter type is being refactored. This is to better model the
// dependencies for creating the exported yaml and to correctly provide us to
// unit tests at the right level of work. Rather than create integration tests
// at the "unit" level.
//
// All exporting migrations have been currently moved to `state/migrations`.
// Each provide their own type that allows them to execute a migration step
// before return if successful or not via an error. The step resembles the
// visitor pattern for good reason, as it allows us to safely model what is
// required at a type level and type safety level. Everything is typed all the
// way down. We can then create mocks for each one independently from other
// migration steps (see examples).
//
// As this is in its infancy, there are intermediary steps. Each export type
// creates its own StateExportMigration. In the future, there will be only
// one and each migration step will add itself to that and Run for completion.
//
// Whilst we're creating these steps, it is expected to create the unit tests
// and supplement all of these tests with existing tests, to ensure that no
// gaps are missing. In the future the integration tests should be replaced with
// the new shell tests to ensure a full end to end test is performed.

const maxStatusHistoryEntries = 20

// ExportConfig allows certain aspects of the model to be skipped
// during the export. The intent of this is to be able to get a partial
// export to support other API calls, like status.
type ExportConfig struct {
	IgnoreIncompleteModel    bool
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
	SkipOfferConnections     bool
	SkipExternalControllers  bool
	SkipSecrets              bool
	SkipVirtualHostKeys      bool
}

// ExportPartial the current model for the State optionally skipping
// aspects as defined by the ExportConfig.
func (st *State) ExportPartial(cfg ExportConfig) (description.Model, error) {
	return st.exportImpl(cfg, map[string]string{})
}

// Export the current model for the State.
func (st *State) Export(leaders map[string]string) (description.Model, error) {
	return st.exportImpl(ExportConfig{}, leaders)
}

func (st *State) exportImpl(cfg ExportConfig, leaders map[string]string) (description.Model, error) {
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
		Cloud:              dbModel.CloudName(),
		CloudRegion:        dbModel.CloudRegion(),
		Owner:              dbModel.Owner(),
		Config:             modelConfig.Settings,
		PasswordHash:       dbModel.doc.PasswordHash,
		LatestToolsVersion: dbModel.LatestToolsVersion(),
		EnvironVersion:     dbModel.EnvironVersion(),
		Blocks:             blocks,
	}
	if args.SecretBackendID, err = export.secretBackendID(); err != nil {
		return nil, errors.Trace(err)
	}
	export.model = description.NewModel(args)
	if credsTag, credsSet := dbModel.CloudCredentialTag(); credsSet && !cfg.SkipCredentials {
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
	if err := export.applications(leaders); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.remoteApplications(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.relations(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.remoteEntities(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.offerConnections(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.relationNetworks(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.spaces(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.subnets(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.ipAddresses(); err != nil {
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
	if err := export.operations(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.cloudimagemetadata(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.storage(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.externalControllers(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.secrets(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.remoteSecrets(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.virtualHostKeys(); err != nil {
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

// ExportStateMigration defines a migration for exporting various entities into
// a destination description model from the source state.
// It accumulates a series of migrations to run at a later time.
// Running the state migration visits all the migrations and exits upon seeing
// the first error from the migration.
type ExportStateMigration struct {
	src        *State
	dst        description.Model
	exporter   *exporter
	migrations []func() error
}

// Add adds a migration to execute at a later time
// Return error from the addition will cause the Run to terminate early.
func (m *ExportStateMigration) Add(f func() error) {
	m.migrations = append(m.migrations, f)
}

// Run executes all the migrations required to be run.
func (m *ExportStateMigration) Run() error {
	for _, f := range m.migrations {
		if err := f(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
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
	openedPorts, err := e.loadOpenedPortRangesForMachine()
	if err != nil {
		return errors.Trace(err)
	}

	// We are iterating through a flat list of machines, but the migration
	// model stores the nesting. The AllMachines method assures us that the
	// machines are returned in an order so the parent will always before
	// any children.
	machineMap := make(map[string]description.Machine)

	for _, machine := range machines {
		e.logger.Debugf("export machine %s", machine.Id())

		var exParent description.Machine
		if parentId := container.ParentId(machine.Id()); parentId != "" {
			var found bool
			exParent, found = machineMap[parentId]
			if !found {
				return errors.Errorf("machine %s missing parent", machine.Id())
			}
		}

		exMachine, err := e.newMachine(exParent, machine, instances, openedPorts, blockDevices)
		if err != nil {
			return errors.Trace(err)
		}
		machineMap[machine.Id()] = exMachine
	}

	return nil
}

func (e *exporter) loadOpenedPortRangesForMachine() (map[string]*machinePortRanges, error) {
	mprs, err := getOpenedPortRangesForAllMachines(e.st)
	if err != nil {
		return nil, errors.Annotate(err, "opened port ranges")
	}

	openedPortsByMachine := make(map[string]*machinePortRanges)
	for _, mpr := range mprs {
		openedPortsByMachine[mpr.MachineID()] = mpr
	}

	e.logger.Debugf("found %d openedPorts docs", len(openedPortsByMachine))
	return openedPortsByMachine, nil
}

func (e *exporter) loadOpenedPortRangesForApplication() (map[string]*applicationPortRanges, error) {
	mprs, err := getOpenedApplicationPortRangesForAllApplications(e.st)
	if err != nil {
		return nil, errors.Annotate(err, "opened port ranges")
	}

	openedPortsByApplication := make(map[string]*applicationPortRanges)
	for _, mpr := range mprs {
		openedPortsByApplication[mpr.ApplicationName()] = mpr
	}

	e.logger.Debugf("found %d openedPorts docs", len(openedPortsByApplication))
	return openedPortsByApplication, nil
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

func (e *exporter) newMachine(exParent description.Machine, machine *Machine, instances map[string]instanceData, portsData map[string]*machinePortRanges, blockDevices map[string][]BlockDeviceInfo) (description.Machine, error) {
	args := description.MachineArgs{
		Id:            machine.MachineTag(),
		Nonce:         machine.doc.Nonce,
		PasswordHash:  machine.doc.PasswordHash,
		Placement:     machine.doc.Placement,
		Base:          machine.doc.Base.String(),
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
		if !found && !e.cfg.IgnoreIncompleteModel {
			return nil, errors.NotValidf("missing instance data for machine %s", machine.Id())
		}
		if found {
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
		if err != nil && !e.cfg.IgnoreIncompleteModel {
			// This means the tools aren't set, but they should be.
			return nil, errors.Trace(err)
		}
		if err == nil {
			exMachine.SetTools(description.AgentToolsArgs{
				Version: tools.Version,
				URL:     tools.URL,
				SHA256:  tools.SHA256,
				Size:    tools.Size,
			})
		}
	}

	for _, args := range e.openedPortRangesArgsForMachine(machine.Id(), portsData) {
		exMachine.AddOpenedPortRange(args)
	}

	exMachine.SetAnnotations(e.getAnnotations(globalKey))

	constraintsArgs, err := e.constraintsArgs(globalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	exMachine.SetConstraints(constraintsArgs)

	return exMachine, nil
}

func (e *exporter) openedPortRangesArgsForMachine(machineID string, portsData map[string]*machinePortRanges) []description.OpenedPortRangeArgs {
	if portsData[machineID] == nil {
		return nil
	}

	var result []description.OpenedPortRangeArgs
	for unitName, unitPorts := range portsData[machineID].ByUnit() {
		for endpointName, portRanges := range unitPorts.ByEndpoint() {
			for _, pr := range portRanges {
				result = append(result, description.OpenedPortRangeArgs{
					UnitName:     unitName,
					EndpointName: endpointName,
					FromPort:     pr.FromPort,
					ToPort:       pr.ToPort,
					Protocol:     pr.Protocol,
				})
			}
		}
	}
	return result
}

func (e *exporter) newAddressArgsSlice(a []address) []description.AddressArgs {
	result := make([]description.AddressArgs, len(a))
	for i, addr := range a {
		result[i] = e.newAddressArgs(addr)
	}
	return result
}

func (e *exporter) newAddressArgs(a address) description.AddressArgs {
	return description.AddressArgs{
		Value:   a.Value,
		Type:    a.AddressType,
		Scope:   a.Scope,
		Origin:  a.Origin,
		SpaceID: a.SpaceID,
		// CIDR is not supported in juju/description@v5,
		// but it has been added in DB to fix the bug https://bugs.launchpad.net/juju/+bug/2073986
		// In this use case, CIDR are always fetched from machine before using them anyway, so not migrating them
		// is not harmful.
		// CIDR:    a.CIDR,
	}
}

func (e *exporter) newCloudInstanceArgs(data instanceData) description.CloudInstanceArgs {
	inst := description.CloudInstanceArgs{
		InstanceId:  string(data.InstanceId),
		DisplayName: data.DisplayName,
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
	if data.VirtType != nil {
		inst.VirtType = *data.VirtType
	}
	if len(data.CharmProfiles) > 0 {
		inst.CharmProfiles = data.CharmProfiles
	}
	return inst
}

func (e *exporter) applications(leaders map[string]string) error {
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

	resourcesSt := e.st.Resources()

	appOfferMap, err := e.groupOffersByApplicationName()
	if err != nil {
		return errors.Trace(err)
	}

	openedPorts, err := e.loadOpenedPortRangesForApplication()
	if err != nil {
		return errors.Trace(err)
	}

	for _, application := range applications {
		applicationUnits := e.units[application.Name()]
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
			payloads:         payloads,
			resources:        resources,
			endpoingBindings: bindings,
			leader:           leaders[application.Name()],
			portsData:        openedPorts,
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
	defer func() { _ = iter.Close() }()
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

func (e *exporter) storageConstraints(doc storageConstraintsDoc) map[string]description.StorageDirectiveArgs {
	result := make(map[string]description.StorageDirectiveArgs)
	for key, value := range doc.Constraints {
		result[key] = description.StorageDirectiveArgs{
			Pool:  value.Pool,
			Size:  value.Size,
			Count: value.Count,
		}
	}
	return result
}

func (e *exporter) readAllPayloads() (map[string][]payloads.FullPayloadInfo, error) {
	result := make(map[string][]payloads.FullPayloadInfo)
	all, err := ModelPayloads{db: e.st.database}.ListAll()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, pl := range all {
		result[pl.Unit] = append(result[pl.Unit], pl)
	}
	return result, nil
}

type addApplicationContext struct {
	application      *Application
	units            []*Unit
	meterStatus      map[string]*meterStatusDoc
	leader           string
	payloads         map[string][]payloads.FullPayloadInfo
	resources        resources.ApplicationResources
	endpoingBindings map[string]bindingsMap
	portsData        map[string]*applicationPortRanges

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

	var charmConfig map[string]interface{}
	applicationCharmSettingsDoc, found := e.modelSettings[charmConfigKey]
	if !found && !e.cfg.SkipSettings && !e.cfg.IgnoreIncompleteModel {
		return errors.Errorf("missing charm settings for application %q", appName)
	}
	if found {
		charmConfig = applicationCharmSettingsDoc.Settings
	}
	delete(e.modelSettings, charmConfigKey)

	var applicationConfig map[string]interface{}
	applicationConfigDoc, found := e.modelSettings[appConfigKey]
	if !found && !e.cfg.SkipSettings && !e.cfg.IgnoreIncompleteModel {
		return errors.Errorf("missing config for application %q", appName)
	}
	if found {
		applicationConfig = applicationConfigDoc.Settings
	}
	delete(e.modelSettings, appConfigKey)

	var leadershipSettings map[string]interface{}
	leadershipSettingsDoc, found := e.modelSettings[leadershipKey]
	if !found && !e.cfg.SkipSettings && !e.cfg.IgnoreIncompleteModel {
		return errors.Errorf("missing leadership settings for application %q", appName)
	}
	if found {
		leadershipSettings = leadershipSettingsDoc.Settings
	}
	delete(e.modelSettings, leadershipKey)

	charmURL := application.doc.CharmURL
	if charmURL == nil {
		return errors.Errorf("missing charm URL for application %q", appName)
	}

	args := description.ApplicationArgs{
		Tag:                  application.ApplicationTag(),
		Type:                 e.model.Type(),
		Subordinate:          application.doc.Subordinate,
		CharmURL:             *charmURL,
		CharmModifiedVersion: application.doc.CharmModifiedVersion,
		ForceCharm:           application.doc.ForceCharm,
		Exposed:              application.doc.Exposed,
		PasswordHash:         application.doc.PasswordHash,
		Placement:            application.doc.Placement,
		HasResources:         application.doc.HasResources,
		DesiredScale:         application.doc.DesiredScale,
		MinUnits:             application.doc.MinUnits,
		EndpointBindings:     map[string]string(ctx.endpoingBindings[globalKey]),
		ApplicationConfig:    applicationConfig,
		CharmConfig:          charmConfig,
		Leader:               ctx.leader,
		LeadershipSettings:   leadershipSettings,
		MetricsCredentials:   application.doc.MetricCredentials,
		PodSpec:              ctx.podSpecs[application.globalKey()],
	}

	if cloudService, found := ctx.cloudServices[application.globalKey()]; found {
		args.CloudService = e.cloudService(cloudService)
	}
	if constraints, found := e.modelStorageConstraints[storageConstraintsKey]; found {
		args.StorageDirectives = e.storageConstraints(constraints)
	}

	if ps := application.ProvisioningState(); ps != nil {
		args.ProvisioningState = &description.ProvisioningStateArgs{
			Scaling:     ps.Scaling,
			ScaleTarget: ps.ScaleTarget,
		}
	}

	// Include exposed endpoint details
	if len(application.doc.ExposedEndpoints) > 0 {
		args.ExposedEndpoints = make(map[string]description.ExposedEndpointArgs)
		for epName, details := range application.doc.ExposedEndpoints {
			args.ExposedEndpoints[epName] = description.ExposedEndpointArgs{
				ExposeToSpaceIDs: details.ExposeToSpaceIDs,
				ExposeToCIDRs:    details.ExposeToCIDRs,
			}
		}
	}

	exApplication := e.model.AddApplication(args)

	// Populate offer list
	for _, offer := range ctx.offers {
		endpoints := make(map[string]string, len(offer.Endpoints))
		for k, ep := range offer.Endpoints {
			endpoints[k] = ep.Name
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
			OfferUUID:              offer.OfferUUID,
			OfferName:              offer.OfferName,
			Endpoints:              endpoints,
			ACL:                    acl,
			ApplicationName:        offer.ApplicationName,
			ApplicationDescription: offer.ApplicationDescription,
		})
	}

	// Find the current charm.
	charmData, err := e.charmData(*charmURL)
	if err != nil {
		return errors.Annotatef(err, "charm metadata for application %s", appName)
	}

	exApplication.SetCharmMetadata(charmData.Metadata)
	exApplication.SetCharmManifest(charmData.Manifest)
	exApplication.SetCharmActions(charmData.Actions)
	exApplication.SetCharmConfigs(charmData.Config)

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

	defaultArch := constraintsArgs.Architecture
	if defaultArch == "" {
		defaultArch = arch.DefaultArchitecture
	}
	charmOriginArgs, err := e.getCharmOrigin(application.doc, defaultArch)
	if err != nil {
		return errors.Annotatef(err, "charm origin")
	}
	exApplication.SetCharmOrigin(charmOriginArgs)

	if err := e.setResources(exApplication, ctx.resources); err != nil {
		return errors.Trace(err)
	}

	for _, args := range e.openedPortRangesArgsForApplication(appName, ctx.portsData) {
		exApplication.AddOpenedPortRange(args)
	}

	// Set Tools for application - this is only for CAAS models.
	isSidecar, err := ctx.application.IsSidecar()
	if err != nil {
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

		// Export charm and agent state stored to the controller.
		unitState, err := unit.State()
		if err != nil {
			return errors.Trace(err)
		}
		if charmState, found := unitState.CharmState(); found {
			args.CharmState = charmState
		}
		if relationState, found := unitState.RelationState(); found {
			args.RelationState = relationState
		}
		if uniterState, found := unitState.UniterState(); found {
			args.UniterState = uniterState
		}
		if storageState, found := unitState.StorageState(); found {
			args.StorageState = storageState
		}
		if meterStatusState, found := unitState.MeterStatusState(); found {
			args.MeterStatusState = meterStatusState
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

		if (e.dbModel.Type() != ModelTypeCAAS && !e.cfg.SkipUnitAgentBinaries) || isSidecar {
			tools, err := unit.AgentTools()
			if err != nil && !e.cfg.IgnoreIncompleteModel {
				// This means the tools aren't set, but they should be.
				return errors.Trace(err)
			}
			if err == nil {
				exUnit.SetTools(description.AgentToolsArgs{
					Version: tools.Version,
					URL:     tools.URL,
					SHA256:  tools.SHA256,
					Size:    tools.Size,
				})
			}
		}
		if e.dbModel.Type() == ModelTypeCAAS {
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

	if e.dbModel.Type() == ModelTypeCAAS && !isSidecar {
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

func (e *exporter) openedPortRangesArgsForApplication(appName string, portsData map[string]*applicationPortRanges) []description.OpenedPortRangeArgs {
	if portsData[appName] == nil {
		return nil
	}

	var result []description.OpenedPortRangeArgs
	for unitName, unitPorts := range portsData[appName].ByUnit() {
		for endpointName, portRanges := range unitPorts.ByEndpoint() {
			for _, pr := range portRanges {
				result = append(result, description.OpenedPortRangeArgs{
					UnitName:     unitName,
					EndpointName: endpointName,
					FromPort:     pr.FromPort,
					ToPort:       pr.ToPort,
					Protocol:     pr.Protocol,
				})
			}
		}
	}
	return result
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

func (e *exporter) setResources(exApp description.Application, resources resources.ApplicationResources) error {
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

func (e *exporter) setUnitResources(exUnit description.Unit, allResources []resources.UnitResources) {
	for _, res := range findUnitResources(exUnit.Name(), allResources) {
		exUnit.AddResource(description.UnitResourceArgs{
			Name: res.Name,
			RevisionArgs: description.ResourceRevisionArgs{
				Revision:       res.Revision,
				Type:           res.Type.String(),
				Path:           res.Path,
				Description:    res.Description,
				Origin:         res.Origin.String(),
				FingerprintHex: res.Fingerprint.Hex(),
				Size:           res.Size,
				Timestamp:      res.Timestamp,
				Username:       res.Username,
			},
		})
	}
}

func findUnitResources(unitName string, allResources []resources.UnitResources) []resources.Resource {
	for _, unitResources := range allResources {
		if unitResources.Tag.Id() == unitName {
			return unitResources.Resources
		}
	}
	return nil
}

func (e *exporter) setUnitPayloads(exUnit description.Unit, payloads []payloads.FullPayloadInfo) error {
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

		for _, ep := range relation.Endpoints() {
			if err := e.relationEndpoint(relation, exRelation, ep, relationScopes); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (e *exporter) relationEndpoint(
	relation *Relation,
	exRelation description.Relation,
	ep Endpoint,
	relationScopes set.Strings,
) error {
	exEndPoint := exRelation.AddEndpoint(description.EndpointArgs{
		ApplicationName: ep.ApplicationName,
		Name:            ep.Name,
		Role:            string(ep.Role),
		Interface:       ep.Interface,
		Optional:        ep.Optional,
		Limit:           ep.Limit,
		Scope:           string(ep.Scope),
	})

	key := relationApplicationSettingsKey(relation.Id(), ep.ApplicationName)
	appSettingsDoc, found := e.modelSettings[key]
	if !found && !e.cfg.SkipSettings && !e.cfg.SkipRelationData {
		return errors.Errorf("missing application settings for %q application %q", relation, ep.ApplicationName)
	}
	delete(e.modelSettings, key)
	exEndPoint.SetApplicationSettings(appSettingsDoc.Settings)

	// We expect a relationScope and settings for each of
	// the units of the specified application.
	// We need to check both local and remote applications
	// in case we are dealing with a CMR.
	if units, ok := e.units[ep.ApplicationName]; ok {
		for _, unit := range units {
			ru, err := relation.Unit(unit)
			if err != nil {
				return errors.Trace(err)
			}

			if err := e.relationUnit(exEndPoint, ru, unit.Name(), relationScopes); err != nil {
				return errors.Annotatef(err, "processing relation unit in %s", relation)
			}
		}
	} else {
		remotes, err := relation.AllRemoteUnits(ep.ApplicationName)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				// If there are no local or remote units for this application,
				// then there are none in scope. We are done.
				return nil
			}
			return errors.Annotatef(err, "retrieving remote units for %s", relation)
		}

		for _, ru := range remotes {
			if err := e.relationUnit(exEndPoint, ru, ru.unitName, relationScopes); err != nil {
				return errors.Annotatef(err, "processing relation unit in %s", relation)
			}
		}
	}

	return nil
}

func (e *exporter) relationUnit(
	exEndPoint description.Endpoint,
	ru *RelationUnit,
	unitName string,
	relationScopes set.Strings,
) error {
	valid, err := ru.Valid()
	if err != nil {
		return errors.Trace(err)
	}
	if !valid {
		// It doesn't make sense for this application to have a
		// relations scope for this endpoint. For example the
		// situation where we have a subordinate charm related to
		// two different principals.
		return nil
	}

	key := ru.key()
	if !e.cfg.SkipRelationData && !relationScopes.Contains(key) && !e.cfg.IgnoreIncompleteModel {
		return errors.Errorf("missing relation scope for %s", unitName)
	}
	settingsDoc, found := e.modelSettings[key]
	if !found && !e.cfg.SkipSettings && !e.cfg.SkipRelationData && !e.cfg.IgnoreIncompleteModel {
		return errors.Errorf("missing relation settings for %s", unitName)
	}
	delete(e.modelSettings, key)
	exEndPoint.SetUnitSettings(unitName, settingsDoc.Settings)

	return nil
}

func (e *exporter) remoteEntities() error {
	e.logger.Debugf("reading remote entities")
	migration := &ExportStateMigration{
		src: e.st,
		dst: e.model,
	}
	migration.Add(func() error {
		m := migrations.ExportRemoteEntities{}
		return m.Execute(remoteEntitiesShim{
			st: migration.src,
		}, migration.dst)
	})
	return migration.Run()
}

// offerConnectionsShim provides a way to model our dependencies by providing
// a shim layer to manage the covariance of the state package to the migration
// package.
type offerConnectionsShim struct {
	st *State
}

// AllOfferConnections returns all offer connections in the model.
// The offer connection shim converts a state.OfferConnection to a
// migrations.MigrationOfferConnection.
func (s offerConnectionsShim) AllOfferConnections() ([]migrations.MigrationOfferConnection, error) {
	conns, err := s.st.AllOfferConnections()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]migrations.MigrationOfferConnection, len(conns))
	for k, v := range conns {
		result[k] = v
	}
	return result, nil
}

func (e *exporter) offerConnections() error {
	if e.cfg.SkipOfferConnections {
		return nil
	}

	e.logger.Debugf("reading offer connections")
	migration := &ExportStateMigration{
		src: e.st,
		dst: e.model,
	}
	migration.Add(func() error {
		m := migrations.ExportOfferConnections{}
		return m.Execute(offerConnectionsShim{st: migration.src}, migration.dst)
	})
	return migration.Run()
}

// externalControllersShim is to handle the fact that go doesn't handle
// covariance and the tight abstraction around the new migration export work
// ensures that we handle our dependencies up front.
type externalControllerShim struct {
	st *State
}

// externalControllerInfoShim is used to align to an interface with in the
// migrations package.
type externalControllerInfoShim struct {
	info externalControllerDoc
}

// ID holds the controller ID from the external controller
func (e externalControllerInfoShim) ID() string {
	return e.info.Id
}

// Alias holds an alias (human friendly) name for the controller.
func (e externalControllerInfoShim) Alias() string {
	return e.info.Alias
}

// Addrs holds the host:port values for the external
// controller's API server.
func (e externalControllerInfoShim) Addrs() []string {
	return e.info.Addrs
}

// CACert holds the certificate to validate the external
// controller's target API server's TLS certificate.
func (e externalControllerInfoShim) CACert() string {
	return e.info.CACert
}

// Models holds model UUIDs hosted on this controller.
func (e externalControllerInfoShim) Models() []string {
	return e.info.Models
}

func (s externalControllerShim) ControllerForModel(uuid string) (migrations.MigrationExternalController, error) {
	entity, err := s.st.ExternalControllerForModel(uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return externalControllerInfoShim{
		info: entity.doc,
	}, nil
}

// AllRemoteApplications returns all remote applications in the model.
func (s externalControllerShim) AllRemoteApplications() ([]migrations.MigrationRemoteApplication, error) {
	remoteApps, err := s.st.AllRemoteApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]migrations.MigrationRemoteApplication, len(remoteApps))
	for k, v := range remoteApps {
		result[k] = remoteApplicationShim{RemoteApplication: v}
	}
	return result, nil
}

func (e *exporter) externalControllers() error {
	if e.cfg.SkipExternalControllers {
		return nil
	}
	e.logger.Debugf("reading external controllers")
	migration := &ExportStateMigration{
		src: e.st,
		dst: e.model,
	}
	migration.Add(func() error {
		m := migrations.ExportExternalControllers{}
		return m.Execute(externalControllerShim{st: migration.src}, migration.dst)
	})
	return migration.Run()
}

// remoteEntitiesShim is to handle the fact that go doesn't handle covariance
// and the tight abstraction around the new migration export work ensures that
// we handle our dependencies up front.
type remoteEntitiesShim struct {
	st *State
}

// AllRemoteEntities returns all remote entities in the model.
func (s remoteEntitiesShim) AllRemoteEntities() ([]migrations.MigrationRemoteEntity, error) {
	entities, err := s.st.AllRemoteEntities()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]migrations.MigrationRemoteEntity, len(entities))
	for k, v := range entities {
		result[k] = v
	}
	return result, nil
}

func (e *exporter) relationNetworks() error {
	e.logger.Debugf("reading relation networks")
	migration := &ExportStateMigration{
		src: e.st,
		dst: e.model,
	}
	migration.Add(func() error {
		m := migrations.ExportRelationNetworks{}
		return m.Execute(relationNetworksShim{st: migration.src}, migration.dst)
	})
	return migration.Run()
}

// relationNetworksShim is to handle the fact that go doesn't handle covariance
// and the tight abstraction around the new migration export work ensures that
// we handle our dependencies up front.
type relationNetworksShim struct {
	st *State
}

func (s relationNetworksShim) AllRelationNetworks() ([]migrations.MigrationRelationNetworks, error) {
	entities, err := NewRelationNetworks(s.st).AllRelationNetworks()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]migrations.MigrationRelationNetworks, len(entities))
	for k, v := range entities {
		result[k] = v
	}
	return result, nil
}

func (e *exporter) spaces() error {
	spaces, err := e.st.AllSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d spaces", len(spaces))

	for _, space := range spaces {
		// We do not export the alpha space because it is created by default
		// with the new model. This is OK, because it is immutable.
		// Any subnets added to the space will still be exported.
		if space.Id() == network.AlphaSpaceId {
			continue
		}

		e.model.AddSpace(description.SpaceArgs{
			Id:         space.Id(),
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
			ProviderID:      string(device.ProviderID()),
			MachineID:       device.MachineID(),
			Name:            device.Name(),
			MTU:             device.MTU(),
			Type:            string(device.Type()),
			MACAddress:      device.MACAddress(),
			IsAutoStart:     device.IsAutoStart(),
			IsUp:            device.IsUp(),
			ParentName:      device.ParentName(),
			VirtualPortType: string(device.VirtualPortType()),
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
			ID:                subnet.ID(),
			CIDR:              subnet.CIDR(),
			ProviderId:        string(subnet.ProviderId()),
			ProviderNetworkId: string(subnet.ProviderNetworkId()),
			VLANTag:           subnet.VLANTag(),
			SpaceID:           subnet.SpaceID(),
			AvailabilityZones: subnet.AvailabilityZones(),
			FanLocalUnderlay:  subnet.FanLocalUnderlay(),
			FanOverlay:        subnet.FanOverlay(),
			IsPublic:          subnet.IsPublic(),
		}
		e.model.AddSubnet(args)
	}
	return nil
}

func (e *exporter) ipAddresses() error {
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
			ProviderID:        string(addr.ProviderID()),
			DeviceName:        addr.DeviceName(),
			MachineID:         addr.MachineID(),
			SubnetCIDR:        addr.SubnetCIDR(),
			ConfigMethod:      string(addr.ConfigMethod()),
			Value:             addr.Value(),
			DNSServers:        addr.DNSServers(),
			DNSSearchDomains:  addr.DNSSearchDomains(),
			GatewayAddress:    addr.GatewayAddress(),
			ProviderNetworkID: addr.ProviderNetworkID().String(),
			ProviderSubnetID:  addr.ProviderSubnetID().String(),
			Origin:            string(addr.Origin()),
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
	for _, a := range actions {
		results, message := a.Results()
		arg := description.ActionArgs{
			Receiver:       a.Receiver(),
			Name:           a.Name(),
			Operation:      a.(*action).doc.Operation,
			Parameters:     a.Parameters(),
			Enqueued:       a.Enqueued(),
			Started:        a.Started(),
			Completed:      a.Completed(),
			Status:         string(a.Status()),
			Results:        results,
			Message:        message,
			Id:             a.Id(),
			Parallel:       a.Parallel(),
			ExecutionGroup: a.ExecutionGroup(),
		}
		messages := a.Messages()
		arg.Messages = make([]description.ActionMessage, len(messages))
		for i, m := range messages {
			arg.Messages[i] = m
		}
		e.model.AddAction(arg)
	}
	return nil
}

func (e *exporter) operations() error {
	if e.cfg.SkipActions {
		return nil
	}

	m, err := e.st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	operations, err := m.AllOperations()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d operations", len(operations))
	for _, op := range operations {
		opDetails, ok := op.(*operation)
		if !ok {
			return errors.Errorf("operation must be of type operation")
		}
		arg := description.OperationArgs{
			Summary:           op.Summary(),
			Fail:              op.Fail(),
			Enqueued:          op.Enqueued(),
			Started:           op.Started(),
			Completed:         op.Completed(),
			Status:            string(op.Status()),
			CompleteTaskCount: opDetails.doc.CompleteTaskCount,
			SpawnedTaskCount:  opDetails.doc.SpawnedTaskCount,
			Id:                op.Id(),
		}
		e.model.AddOperation(arg)
	}
	return nil
}

func (e *exporter) secretBackendID() (string, error) {
	mCfg, err := e.dbModel.Config()
	if err != nil {
		return "", errors.Trace(err)
	}
	backendName := mCfg.SecretBackend()
	if backendName == "" || backendName == secretsprovider.Auto || backendName == secretsprovider.Internal {
		return "", nil
	}
	store := NewSecretBackends(e.st)
	backend, err := store.GetSecretBackend(backendName)
	if err != nil {
		return "", errors.Trace(err)
	}
	return backend.ID, nil
}

func (e *exporter) secrets() error {
	if e.cfg.SkipSecrets {
		return nil
	}
	store := NewSecrets(e.st)

	allSecrets, err := store.ListSecrets(SecretsFilter{})
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d secrets", len(allSecrets))
	allRevisions, err := store.allSecretRevisions()
	if err != nil {
		return errors.Trace(err)
	}
	revisionArgsByID := make(map[string][]description.SecretRevisionArgs)
	for _, rev := range allRevisions {
		id, _ := splitSecretRevision(e.st.localID(rev.DocID))
		revArg := description.SecretRevisionArgs{
			Number:        rev.Revision,
			Created:       rev.CreateTime,
			Updated:       rev.UpdateTime,
			ExpireTime:    rev.ExpireTime,
			Obsolete:      rev.Obsolete,
			PendingDelete: rev.PendingDelete,
		}
		if len(rev.Data) > 0 {
			revArg.Content = make(secrets.SecretData)
			for k, v := range rev.Data {
				revArg.Content[k] = fmt.Sprintf("%v", v)
			}
		}
		if rev.ValueRef != nil {
			revArg.ValueRef = &description.SecretValueRefArgs{
				BackendID:  rev.ValueRef.BackendID,
				RevisionID: rev.ValueRef.RevisionID,
			}
		}
		revisionArgsByID[id] = append(revisionArgsByID[id], revArg)
	}
	allPermissions, err := store.allSecretPermissions()
	if err != nil {
		return errors.Trace(err)
	}
	accessArgsByID := make(map[string]map[string]description.SecretAccessArgs)
	for _, perm := range allPermissions {
		id := strings.Split(e.st.localID(perm.DocID), "#")[0]
		accessArg := description.SecretAccessArgs{
			Scope: perm.Scope,
			Role:  perm.Role,
		}
		access, ok := accessArgsByID[id]
		if !ok {
			access = make(map[string]description.SecretAccessArgs)
			accessArgsByID[id] = access
		}
		access[perm.Subject] = accessArg
	}
	allConsumers, err := store.allLocalSecretConsumers()
	if err != nil {
		return errors.Trace(err)
	}
	consumersByID := make(map[string][]description.SecretConsumerArgs)
	for _, info := range allConsumers {
		consumer, err := names.ParseTag(info.ConsumerTag)
		if err != nil {
			return errors.Trace(err)
		}
		id := strings.Split(e.st.localID(info.DocID), "#")[0]
		consumerArg := description.SecretConsumerArgs{
			Consumer:        consumer,
			Label:           info.Label,
			CurrentRevision: info.CurrentRevision,
		}
		consumersByID[id] = append(consumersByID[id], consumerArg)
	}

	allRemoteConsumers, err := store.allSecretRemoteConsumers()
	if err != nil {
		return errors.Trace(err)
	}
	remoteConsumersByID := make(map[string][]description.SecretRemoteConsumerArgs)
	for _, info := range allRemoteConsumers {
		consumer, err := names.ParseTag(info.ConsumerTag)
		if err != nil {
			return errors.Trace(err)
		}
		id := strings.Split(e.st.localID(info.DocID), "#")[0]
		remoteConsumerArg := description.SecretRemoteConsumerArgs{
			Consumer:        consumer,
			CurrentRevision: info.CurrentRevision,
		}
		remoteConsumersByID[id] = append(remoteConsumersByID[id], remoteConsumerArg)
	}

	for _, md := range allSecrets {
		owner, err := names.ParseTag(md.OwnerTag)
		if err != nil {
			return errors.Trace(err)
		}
		arg := description.SecretArgs{
			ID:                     md.URI.ID,
			Version:                md.Version,
			Description:            md.Description,
			Label:                  md.Label,
			RotatePolicy:           md.RotatePolicy.String(),
			AutoPrune:              md.AutoPrune,
			Owner:                  owner,
			Created:                md.CreateTime,
			Updated:                md.UpdateTime,
			NextRotateTime:         md.NextRotateTime,
			LatestRevisionChecksum: md.LatestRevisionChecksum,
			Revisions:              revisionArgsByID[md.URI.ID],
			ACL:                    accessArgsByID[md.URI.ID],
			Consumers:              consumersByID[md.URI.ID],
			RemoteConsumers:        remoteConsumersByID[md.URI.ID],
		}
		e.model.AddSecret(arg)
	}
	return nil
}

func (e *exporter) remoteSecrets() error {
	if e.cfg.SkipSecrets {
		return nil
	}
	store := NewSecrets(e.st)

	allConsumers, err := store.allRemoteSecretConsumers()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d remote secret consumers", len(allConsumers))
	for _, info := range allConsumers {
		consumer, err := names.ParseTag(info.ConsumerTag)
		if err != nil {
			return errors.Trace(err)
		}
		id := strings.Split(e.st.localID(info.DocID), "#")[0]
		uri, err := secrets.ParseURI(id)
		if err != nil {
			return errors.Trace(err)
		}
		arg := description.RemoteSecretArgs{
			ID:              uri.ID,
			SourceUUID:      uri.SourceUUID,
			Consumer:        consumer,
			Label:           info.Label,
			CurrentRevision: info.CurrentRevision,
			LatestRevision:  info.LatestRevision,
		}
		e.model.AddRemoteSecret(arg)
	}
	return nil
}

func (e *exporter) virtualHostKeys() error {
	if e.cfg.SkipVirtualHostKeys {
		return nil
	}
	virtualHostKeys, err := e.st.AllVirtualHostKeys()
	if err != nil {
		return errors.Trace(err)
	}
	for _, virtualHostKey := range virtualHostKeys {
		e.model.AddVirtualHostKey(description.VirtualHostKeyArgs{
			ID:      e.st.localID(virtualHostKey.doc.DocId),
			HostKey: virtualHostKey.doc.HostKey,
		})
	}
	return nil
}

func (e *exporter) readAllRelationScopes() (set.Strings, error) {
	relationScopes, closer := e.st.db().GetCollection(relationScopesC)
	defer closer()

	var docs []relationScopeDoc
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

	var docs []unitDoc
	err := unitsCollection.Find(nil).Sort("name").All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all units")
	}
	e.logger.Debugf("found %d unit docs", len(docs))
	result := make(map[string][]*Unit)
	for _, doc := range docs {
		units := result[doc.Application]
		result[doc.Application] = append(units, newUnit(e.st, e.dbModel.Type(), &doc))
	}
	return result, nil
}

func (e *exporter) readAllEndpointBindings() (map[string]bindingsMap, error) {
	bindings, closer := e.st.db().GetCollection(endpointBindingsC)
	defer closer()

	var docs []endpointBindingsDoc
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

	var docs []meterStatusDoc
	err := meterStatuses.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all meter status docs")
	}
	e.logger.Debugf("found %d meter status docs", len(docs))
	result := make(map[string]*meterStatusDoc)
	for _, v := range docs {
		doc := v
		result[e.st.localID(doc.DocID)] = &doc
	}
	return result, nil
}

func (e *exporter) readAllPodSpecs() (map[string]string, error) {
	specs, closer := e.st.db().GetCollection(podSpecsC)
	defer closer()

	var docs []containerSpecDoc
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

	var docs []cloudServiceDoc
	err := cloudServices.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all cloud service docs")
	}
	e.logger.Debugf("found %d cloud service docs", len(docs))
	result := make(map[string]*cloudServiceDoc)
	for _, v := range docs {
		doc := v
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

	var docs []cloudContainerDoc
	err := cloudContainers.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all cloud container docs")
	}
	e.logger.Debugf("found %d cloud container docs", len(docs))
	result := make(map[string]*cloudContainerDoc)
	for _, v := range docs {
		doc := v
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

func (e *exporter) getCharmOrigin(doc applicationDoc, defaultArch string) (description.CharmOriginArgs, error) {
	// Everything should be migrated, but in the case that it's not, handle
	// that case.
	origin := doc.CharmOrigin

	// If the channel is empty, then we fall back to the Revision.
	// Set default revision to -1. This is because a revision of 0 is
	// a valid revision for local charms which we need to be able to
	// from. On import, in the -1 case we grab the revision by parsing
	// the charm url.
	revision := -1
	if rev := origin.Revision; rev != nil {
		revision = *rev
	}

	var channel charm.Channel
	if origin.Channel != nil {
		channel = charm.MakePermissiveChannel(origin.Channel.Track, origin.Channel.Risk, origin.Channel.Branch)
	}
	// Platform is now mandatory moving forward, so we need to ensure that
	// the architecture is set in the platform if it's not set. This
	// shouldn't happen that often, but handles clients sending bad requests
	// when deploying.
	pArch := origin.Platform.Architecture
	if pArch == "" {
		e.logger.Debugf("using default architecture (%q) for doc[%q]", defaultArch, doc.DocID)
		pArch = defaultArch
	}
	platform := corecharm.Platform{
		Architecture: pArch,
		OS:           origin.Platform.OS,
		Channel:      origin.Platform.Channel,
	}

	return description.CharmOriginArgs{
		Source:   origin.Source,
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: revision,
		Channel:  channel.String(),
		Platform: platform.String(),
	}, nil
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
	defer func() { _ = iter.Close() }()
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

	result.Value = status
	result.Message = info
	result.Data = dataMap
	result.Updated = time.Unix(0, updated)
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
	optionalBool := func(name string) bool {
		switch value := doc[name].(type) {
		case nil:
		case bool:
			return value
		default:
			optionalErr = errors.Errorf("expected bool for %s, got %T", name, value)
		}
		return false
	}
	result := description.ConstraintsArgs{
		AllocatePublicIP: optionalBool("allocatepublicip"),
		Architecture:     optionalString("arch"),
		Container:        optionalString("container"),
		CpuCores:         optionalInt("cpucores"),
		CpuPower:         optionalInt("cpupower"),
		ImageID:          optionalString("imageid"),
		InstanceType:     optionalString("instancetype"),
		Memory:           optionalInt("mem"),
		RootDisk:         optionalInt("rootdisk"),
		RootDiskSource:   optionalString("rootdisksource"),
		Spaces:           optionalStringSlice("spaces"),
		Tags:             optionalStringSlice("tags"),
		VirtType:         optionalString("virttype"),
		Zones:            optionalStringSlice("zones"),
	}
	if optionalErr != nil {
		return description.ConstraintsArgs{}, errors.Trace(optionalErr)
	}
	return result, nil
}

func (e *exporter) checkUnexportedValues() error {
	if e.cfg.IgnoreIncompleteModel {
		return nil
	}

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
	e.logger.Debugf("read remote applications")
	migration := &ExportStateMigration{
		src:      e.st,
		dst:      e.model,
		exporter: e,
	}
	migration.Add(func() error {
		m := migrations.ExportRemoteApplications{}
		return m.Execute(remoteApplicationsShim{
			st:       migration.src,
			exporter: e,
		}, migration.dst)
	})
	return migration.Run()
}

// remoteApplicationsShim is to handle the fact that go doesn't handle covariance
// and the tight abstraction around the new migration export work ensures that
// we handle our dependencies up front.
type remoteApplicationsShim struct {
	st       *State
	exporter *exporter
}

// AllRemoteApplications returns all remote applications in the model.
func (s remoteApplicationsShim) AllRemoteApplications() ([]migrations.MigrationRemoteApplication, error) {
	remoteApps, err := s.st.AllRemoteApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]migrations.MigrationRemoteApplication, len(remoteApps))
	for k, v := range remoteApps {
		result[k] = remoteApplicationShim{RemoteApplication: v}
	}
	return result, nil
}

func (s remoteApplicationsShim) StatusArgs(key string) (description.StatusArgs, error) {
	return s.exporter.statusArgs(key)
}

type remoteApplicationShim struct {
	*RemoteApplication
}

func (s remoteApplicationShim) Endpoints() ([]migrations.MigrationRemoteEndpoint, error) {
	endpoints, err := s.RemoteApplication.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]migrations.MigrationRemoteEndpoint, len(endpoints))
	for k, v := range endpoints {
		result[k] = migrations.MigrationRemoteEndpoint{
			Name:      v.Name,
			Role:      v.Role,
			Interface: v.Interface,
		}
	}
	return result, nil
}

func (s remoteApplicationShim) Spaces() []migrations.MigrationRemoteSpace {
	spaces := s.RemoteApplication.Spaces()
	result := make([]migrations.MigrationRemoteSpace, len(spaces))
	for k, v := range spaces {
		subnets := make([]migrations.MigrationRemoteSubnet, len(v.Subnets))
		for k, v := range v.Subnets {
			subnets[k] = migrations.MigrationRemoteSubnet{
				CIDR:              v.CIDR,
				ProviderId:        v.ProviderId,
				VLANTag:           v.VLANTag,
				AvailabilityZones: v.AvailabilityZones,
				ProviderSpaceId:   v.ProviderSpaceId,
				ProviderNetworkId: v.ProviderNetworkId,
			}
		}
		result[k] = migrations.MigrationRemoteSpace{
			CloudType:          v.CloudType,
			Name:               v.Name,
			ProviderId:         v.ProviderId,
			ProviderAttributes: v.ProviderAttributes,
			Subnets:            subnets,
		}
	}
	return result
}

func (s remoteApplicationShim) GlobalKey() string {
	return s.RemoteApplication.globalKey()
}

// Macaroon returns the encoded macaroon JSON.
func (s remoteApplicationShim) Macaroon() string {
	return s.RemoteApplication.doc.Macaroon
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
	defer func() { _ = iter.Close() }()
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
	defer func() { _ = iter.Close() }()
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
	defer func() { _ = iter.Close() }()
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
	defer func() { _ = iter.Close() }()
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
	defer func() { _ = iter.Close() }()
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
	defer func() { _ = iter.Close() }()
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
	defer func() { _ = iter.Close() }()
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

type charmData struct {
	Metadata description.CharmMetadataArgs
	Manifest description.CharmManifestArgs
	Actions  description.CharmActionsArgs
	Config   description.CharmConfigsArgs
}

func (e *exporter) charmData(charmURL string) (charmData, error) {
	ch, err := e.st.Charm(charmURL)
	if err != nil {
		return charmData{}, errors.Annotatef(err, "getting charm %q", charmURL)
	}

	metadata, err := e.charmMetadata(ch)
	if err != nil {
		return charmData{}, errors.Annotatef(err, "getting metadata for charm %q", charmURL)
	}

	manifest, err := e.charmManifest(ch)
	if err != nil {
		return charmData{}, errors.Annotatef(err, "getting manifest for charm %q", charmURL)
	}

	actions, err := e.charmActions(ch)
	if err != nil {
		return charmData{}, errors.Annotatef(err, "getting actions for charm %q", charmURL)
	}

	config, err := e.charmConfig(ch)
	if err != nil {
		return charmData{}, errors.Annotatef(err, "getting config for charm %q", charmURL)
	}

	return charmData{
		Metadata: metadata,
		Manifest: manifest,
		Actions:  actions,
		Config:   config,
	}, nil
}

func (e *exporter) charmMetadata(ch *Charm) (description.CharmMetadataArgs, error) {
	meta := ch.Meta()
	if meta == nil {
		return description.CharmMetadataArgs{}, errors.Errorf("missing metadata")
	}

	assumes, err := json.Marshal(meta.Assumes)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Annotate(err, "marshalling assumes")
	}

	var lxdProfile []byte
	if charmProfile := ch.LXDProfile(); charmProfile != nil {
		if lxdProfile, err = json.Marshal(charmProfile); err != nil {
			return description.CharmMetadataArgs{}, errors.Annotate(err, "marshalling lxd profile")
		}
	}

	return description.CharmMetadataArgs{
		Name:           meta.Name,
		Summary:        meta.Summary,
		Description:    meta.Description,
		Subordinate:    meta.Subordinate,
		MinJujuVersion: meta.MinJujuVersion.String(),
		RunAs:          string(meta.CharmUser),
		Assumes:        string(assumes),
		Tags:           meta.Tags,
		Categories:     meta.Categories,
		Terms:          meta.Terms,
		Provides:       e.charmRelations(meta.Provides),
		Requires:       e.charmRelations(meta.Requires),
		Peers:          e.charmRelations(meta.Peers),
		ExtraBindings:  e.charmExtraBindings(meta.ExtraBindings),
		Storage:        e.charmStorage(meta.Storage),
		Devices:        e.charmDevices(meta.Devices),
		Payloads:       e.charmPayloads(meta.PayloadClasses),
		Resources:      e.charmResources(meta.Resources),
		Containers:     e.charmContainers(meta.Containers),
		LXDProfile:     string(lxdProfile),
	}, nil
}

func (e *exporter) charmManifest(ch *Charm) (description.CharmManifestArgs, error) {
	manifest := ch.Manifest()
	if manifest == nil {
		return description.CharmManifestArgs{}, nil
	}

	bases := make([]description.CharmManifestBase, len(manifest.Bases))

	for i, base := range manifest.Bases {
		bases[i] = charmManifestBase{
			name:          base.Name,
			channel:       base.Channel.String(),
			architectures: base.Architectures,
		}
	}

	return description.CharmManifestArgs{
		Bases: bases,
	}, nil
}

func (e *exporter) charmActions(ch *Charm) (description.CharmActionsArgs, error) {
	actions := ch.Actions()
	if actions == nil {
		return description.CharmActionsArgs{}, nil
	}

	result := make(map[string]description.CharmAction)
	for name, action := range actions.ActionSpecs {
		result[name] = charmAction{
			description:    action.Description,
			parallel:       action.Parallel,
			executionGroup: action.ExecutionGroup,
			parameters:     action.Params,
		}
	}

	return description.CharmActionsArgs{
		Actions: result,
	}, nil
}

func (e *exporter) charmConfig(ch *Charm) (description.CharmConfigsArgs, error) {
	config := ch.Config()
	if config == nil {
		return description.CharmConfigsArgs{}, nil
	}

	result := make(map[string]description.CharmConfig)
	for name, cfg := range config.Options {
		result[name] = charmConfig{
			description:  cfg.Description,
			typ:          cfg.Type,
			defaultValue: cfg.Default,
		}
	}

	return description.CharmConfigsArgs{
		Configs: result,
	}, nil
}

func (e *exporter) charmRelations(relations map[string]charm.Relation) map[string]description.CharmMetadataRelation {
	result := make(map[string]description.CharmMetadataRelation)
	for name, rel := range relations {
		result[name] = charmMetadataRelation{
			name:     rel.Name,
			role:     string(rel.Role),
			iface:    rel.Interface,
			scope:    string(rel.Scope),
			optional: rel.Optional,
			limit:    rel.Limit,
		}
	}
	return result
}

func (e *exporter) charmExtraBindings(bindings map[string]charm.ExtraBinding) map[string]string {
	result := make(map[string]string)
	for name, binding := range bindings {
		result[name] = binding.Name
	}
	return result
}

func (e *exporter) charmStorage(storages map[string]charm.Storage) map[string]description.CharmMetadataStorage {
	result := make(map[string]description.CharmMetadataStorage)
	for name, storage := range storages {
		result[name] = charmMetadataStorage{
			name:        storage.Name,
			description: storage.Description,
			typ:         string(storage.Type),
			shared:      storage.Shared,
			readonly:    storage.ReadOnly,
			countMin:    storage.CountMin,
			countMax:    storage.CountMax,
			minimumSize: int(storage.MinimumSize),
			location:    storage.Location,
			properties:  storage.Properties,
		}
	}
	return result
}

func (e *exporter) charmDevices(devices map[string]charm.Device) map[string]description.CharmMetadataDevice {
	result := make(map[string]description.CharmMetadataDevice)
	for name, device := range devices {
		result[name] = charmMetadataDevice{
			name:        device.Name,
			description: device.Description,
			typ:         string(device.Type),
			countMin:    int(device.CountMin),
			countMax:    int(device.CountMax),
		}
	}
	return result
}

func (e *exporter) charmPayloads(payloads map[string]charm.PayloadClass) map[string]description.CharmMetadataPayload {
	result := make(map[string]description.CharmMetadataPayload)
	for name, payload := range payloads {
		result[name] = charmMetadataPayload{
			name: name,
			typ:  payload.Type,
		}
	}
	return result
}

func (e *exporter) charmResources(resources map[string]resource.Meta) map[string]description.CharmMetadataResource {
	result := make(map[string]description.CharmMetadataResource)
	for name, resource := range resources {
		result[name] = charmMetadataResource{
			name:        resource.Name,
			typ:         resource.Type.String(),
			path:        resource.Path,
			description: resource.Description,
		}
	}
	return result
}

func (e *exporter) charmContainers(containers map[string]charm.Container) map[string]description.CharmMetadataContainer {
	result := make(map[string]description.CharmMetadataContainer)
	for name, container := range containers {
		mounts := make([]charmMetadataContainerMount, len(container.Mounts))
		for i, mount := range container.Mounts {
			mounts[i] = charmMetadataContainerMount{
				storage:  mount.Storage,
				location: mount.Location,
			}
		}
		result[name] = charmMetadataContainer{
			resource: container.Resource,
			mounts:   mounts,
			uid:      container.Uid,
			gid:      container.Gid,
		}
	}
	return result
}

type charmMetadataRelation struct {
	name     string
	role     string
	iface    string
	optional bool
	limit    int
	scope    string
}

// Name returns the name of the relation.
func (r charmMetadataRelation) Name() string {
	return r.name
}

// Role returns the role of the relation.
func (r charmMetadataRelation) Role() string {
	return r.role
}

// Interface returns the interface of the relation.
func (r charmMetadataRelation) Interface() string {
	return r.iface
}

// Optional returns whether the relation is optional.
func (r charmMetadataRelation) Optional() bool {
	return r.optional
}

// Limit returns the limit of the relation.
func (r charmMetadataRelation) Limit() int {
	return r.limit
}

// Scope returns the scope of the relation.
func (r charmMetadataRelation) Scope() string {
	return r.scope
}

type charmMetadataStorage struct {
	name        string
	description string
	typ         string
	shared      bool
	readonly    bool
	countMin    int
	countMax    int
	minimumSize int
	location    string
	properties  []string
}

// Name returns the name of the storage.
func (s charmMetadataStorage) Name() string {
	return s.name
}

// Description returns the description of the storage.
func (s charmMetadataStorage) Description() string {
	return s.description
}

// Type returns the type of the storage.
func (s charmMetadataStorage) Type() string {
	return s.typ
}

// Shared returns whether the storage is shared.
func (s charmMetadataStorage) Shared() bool {
	return s.shared
}

// Readonly returns whether the storage is readonly.
func (s charmMetadataStorage) Readonly() bool {
	return s.readonly
}

// CountMin returns the minimum count of the storage.
func (s charmMetadataStorage) CountMin() int {
	return s.countMin
}

// CountMax returns the maximum count of the storage.
func (s charmMetadataStorage) CountMax() int {
	return s.countMax
}

// MinimumSize returns the minimum size of the storage.
func (s charmMetadataStorage) MinimumSize() int {
	return s.minimumSize
}

// Location returns the location of the storage.
func (s charmMetadataStorage) Location() string {
	return s.location
}

// Properties returns the properties of the storage.
func (s charmMetadataStorage) Properties() []string {
	return s.properties
}

type charmMetadataDevice struct {
	name        string
	description string
	typ         string
	countMin    int
	countMax    int
}

// Name returns the name of the device.
func (d charmMetadataDevice) Name() string {
	return d.name
}

// Description returns the description of the device.
func (d charmMetadataDevice) Description() string {
	return d.description
}

// Type returns the type of the device.
func (d charmMetadataDevice) Type() string {
	return d.typ
}

// CountMin returns the minimum count of the device.
func (d charmMetadataDevice) CountMin() int {
	return d.countMin
}

// CountMax returns the maximum count of the device.
func (d charmMetadataDevice) CountMax() int {
	return d.countMax
}

type charmMetadataPayload struct {
	name string
	typ  string
}

// Name returns the name of the payload.
func (p charmMetadataPayload) Name() string {
	return p.name
}

// Type returns the type of the payload.
func (p charmMetadataPayload) Type() string {
	return p.typ
}

type charmMetadataResource struct {
	name        string
	typ         string
	path        string
	description string
}

// Name returns the name of the resource.
func (r charmMetadataResource) Name() string {
	return r.name
}

// Type returns the type of the resource.
func (r charmMetadataResource) Type() string {
	return r.typ
}

// Path returns the path of the resource.
func (r charmMetadataResource) Path() string {
	return r.path
}

// Description returns the description of the resource.
func (r charmMetadataResource) Description() string {
	return r.description
}

type charmMetadataContainer struct {
	resource string
	mounts   []charmMetadataContainerMount
	uid      *int
	gid      *int
}

// Resource returns the resource of the container.
func (c charmMetadataContainer) Resource() string {
	return c.resource
}

// Mounts returns the mounts of the container.
func (c charmMetadataContainer) Mounts() []description.CharmMetadataContainerMount {
	mounts := make([]description.CharmMetadataContainerMount, len(c.mounts))
	for i, m := range c.mounts {
		mounts[i] = m
	}
	return mounts
}

// Uid returns the uid of the container.
func (c charmMetadataContainer) Uid() *int {
	return c.uid
}

// Gid returns the gid of the container.
func (c charmMetadataContainer) Gid() *int {
	return c.gid
}

type charmMetadataContainerMount struct {
	storage  string
	location string
}

// Storage returns the storage of the mount.
func (m charmMetadataContainerMount) Storage() string {
	return m.storage
}

// Location returns the location of the mount.
func (m charmMetadataContainerMount) Location() string {
	return m.location
}

type charmManifestBase struct {
	name          string
	channel       string
	architectures []string
}

// Name returns the name of the base.
func (r charmManifestBase) Name() string {
	return r.name
}

// Channel returns the channel of the base.
func (r charmManifestBase) Channel() string {
	return r.channel
}

// Architectures returns the architectures of the base.
func (r charmManifestBase) Architectures() []string {
	return r.architectures
}

type charmAction struct {
	description    string
	parallel       bool
	executionGroup string
	parameters     map[string]interface{}
}

// Description returns the description of the action.
func (a charmAction) Description() string {
	return a.description
}

// Parallel returns whether the action can be run in parallel.
func (a charmAction) Parallel() bool {
	return a.parallel
}

// ExecutionGroup returns the execution group of the action.
func (a charmAction) ExecutionGroup() string {
	return a.executionGroup
}

// Parameters returns the parameters of the action.
func (a charmAction) Parameters() map[string]interface{} {
	return a.parameters
}

type charmConfig struct {
	typ          string
	defaultValue interface{}
	description  string
}

// Type returns the type of the config.
func (c charmConfig) Type() string {
	return c.typ
}

// Default returns the default value of the config.
func (c charmConfig) Default() interface{} {
	return c.defaultValue
}

// Description returns the description of the config.
func (c charmConfig) Description() string {
	return c.description
}
