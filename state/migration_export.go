// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/container"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
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

// ExportConfig allows certain aspects of the model to be skipped
// during the export. The intent of this is to be able to get a partial
// export to support other API calls, like status.
type ExportConfig struct {
	IgnoreIncompleteModel  bool
	SkipActions            bool
	SkipAnnotations        bool
	SkipCloudImageMetadata bool
	SkipCredentials        bool
	SkipIPAddresses        bool
	SkipSettings           bool
	SkipSSHHostKeys        bool
	SkipLinkLayerDevices   bool
	SkipRelationData       bool
	SkipInstanceData       bool
	SkipSecrets            bool
}

// ExportPartial the current model for the State optionally skipping
// aspects as defined by the ExportConfig.
func (st *State) ExportPartial(cfg ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	return st.exportImpl(cfg, store)
}

// Export the current model for the State.
func (st *State) Export(store objectstore.ObjectStore) (description.Model, error) {
	return st.exportImpl(ExportConfig{}, store)
}

func (st *State) exportImpl(cfg ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	dbModel, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	export := exporter{
		st:      st,
		cfg:     cfg,
		dbModel: dbModel,
		store:   store,
		logger:  internallogger.GetLogger("juju.state.export-model"),
	}
	if err := export.readAllStatuses(); err != nil {
		return nil, errors.Annotate(err, "reading statuses")
	}
	if err := export.readAllSettings(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllStorageConstraints(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllConstraints(); err != nil {
		return nil, errors.Trace(err)
	}

	args := description.ModelArgs{
		Type:           string(dbModel.TypeOld()),
		Cloud:          dbModel.CloudNameOld(),
		CloudRegion:    dbModel.CloudRegionOld(),
		Owner:          dbModel.Owner().Id(),
		Config:         make(map[string]interface{}, 0),
		PasswordHash:   dbModel.doc.PasswordHash,
		EnvironVersion: dbModel.EnvironVersion(),
	}
	if dbModel.LatestToolsVersion() != semversion.Zero {
		args.LatestToolsVersion = dbModel.LatestToolsVersion().String()
	}
	export.model = description.NewModel(args)
	// We used to export the model credential here but that is now done
	// using the new domain/credential exporter. We still need to set the
	// credential tag details so the exporter knows the credential to export.
	credTag, exists := dbModel.CloudCredentialTagOld()
	if exists && !cfg.SkipCredentials {
		export.model.SetCloudCredential(description.CloudCredentialArgs{
			Owner: credTag.Owner().Id(),
			Cloud: credTag.Cloud().Id(),
			Name:  credTag.Name(),
		})
	}
	modelKey := dbModel.globalKey()
	if err := export.sequences(); err != nil {
		return nil, errors.Trace(err)
	}
	constraintsArgs, err := export.constraintsArgs(modelKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	export.model.SetConstraints(constraintsArgs)
	if err := export.machines(); err != nil {
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
	if err := export.storage(); err != nil {
		return nil, errors.Trace(err)
	}

	if featureflag.Enabled(featureflag.StrictMigration) {
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
	store   objectstore.ObjectStore
	logger  corelogger.Logger

	constraints             map[string]bson.M
	modelSettings           map[string]settingsDoc
	modelStorageConstraints map[string]storageConstraintsDoc
	status                  map[string]bson.M
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

func (e *exporter) machines() error {
	machines, err := e.st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf(context.TODO(), "found %d machines", len(machines))

	// We are iterating through a flat list of machines, but the migration
	// model stores the nesting. The AllMachines method assures us that the
	// machines are returned in an order so the parent will always before
	// any children.
	machineMap := make(map[string]description.Machine)

	for _, machine := range machines {
		e.logger.Debugf(context.TODO(), "export machine %s", machine.Id())

		var exParent description.Machine
		if parentId := container.ParentId(machine.Id()); parentId != "" {
			var found bool
			exParent, found = machineMap[parentId]
			if !found {
				return errors.Errorf("machine %s missing parent", machine.Id())
			}
		}

		exMachine, err := e.newMachine(exParent, machine, nil)
		if err != nil {
			return errors.Trace(err)
		}
		machineMap[machine.Id()] = exMachine
	}

	return nil
}

func (e *exporter) newMachine(exParent description.Machine, machine *Machine, blockDevices map[string][]BlockDeviceInfo) (description.Machine, error) {
	args := description.MachineArgs{
		Id:            machine.Id(),
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

	constraintsArgs, err := e.constraintsArgs(globalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	exMachine.SetConstraints(constraintsArgs)

	return exMachine, nil
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
	e.logger.Debugf(context.TODO(), "read %d storage constraint documents", len(storageConstraints))
	e.modelStorageConstraints = storageConstraints
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
	e.logger.Debugf(context.TODO(), "read %d ip devices", len(linklayerdevices))
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

func (e *exporter) ipAddresses() error {
	if e.cfg.SkipIPAddresses {
		return nil
	}
	ipaddresses, err := e.st.AllIPAddresses()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf(context.TODO(), "read %d ip addresses", len(ipaddresses))
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
		if errors.Is(err, errors.NotFound) {
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
	e.logger.Debugf(context.TODO(), "read %d actions", len(actions))
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
	e.logger.Debugf(context.TODO(), "read %d operations", len(operations))
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

	e.logger.Debugf(context.TODO(), "read %d constraints docs", len(docs))
	e.constraints = make(map[string]bson.M)
	for _, doc := range docs {
		docId, ok := doc["_id"].(string)
		if !ok {
			return errors.Errorf("expected string, got %s (%T)", doc["_id"], doc["_id"])
		}
		id := e.st.localID(docId)
		e.constraints[id] = doc
		e.logger.Debugf(context.TODO(), "doc[%q] = %#v", id, doc)
	}
	return nil
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

	e.logger.Debugf(context.TODO(), "read %d status documents", len(docs))
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

func (e *exporter) constraintsArgs(globalKey string) (description.ConstraintsArgs, error) {
	doc, found := e.constraints[globalKey]
	if !found {
		// No constraints for this key.
		e.logger.Tracef(context.TODO(), "no constraints found for key %q", globalKey)
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

	for key := range e.modelSettings {
		missing = append(missing, fmt.Sprintf("unexported settings for %s", key))
	}

	for key := range e.status {
		if !e.cfg.SkipInstanceData && !strings.HasSuffix(key, "#instance") {
			missing = append(missing, fmt.Sprintf("unexported status for %s", key))
		}
	}

	if len(missing) > 0 {
		content := strings.Join(missing, "\n  ")
		return errors.Errorf("migration missed some docs:\n  %s", content)
	}
	return nil
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
		ID: vol.VolumeTag().Id(),
	}
	if tag, err := vol.StorageInstance(); err == nil {
		// only returns an error when no storage tag.
		args.Storage = tag.Id()
	} else {
		if !errors.Is(err, errors.NotAssigned) {
			// This is an unexpected error.
			return errors.Trace(err)
		}
	}
	logger.Debugf(context.TODO(), "addVolume: %#v", vol.doc)
	if info, err := vol.Info(); err == nil {
		logger.Debugf(context.TODO(), "  info %#v", info)
		args.Provisioned = true
		args.Size = info.Size
		args.Pool = info.Pool
		args.HardwareID = info.HardwareId
		args.WWN = info.WWN
		args.VolumeID = info.VolumeId
		args.Persistent = info.Persistent
	} else {
		params, _ := vol.Params()
		logger.Debugf(context.TODO(), "  params %#v", params)
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
	if count := len(volAttachments); count != vol.doc.AttachmentCount {
		return errors.Errorf("volume attachment count mismatch, have %d, expected %d",
			count, vol.doc.AttachmentCount)
	}
	for _, doc := range volAttachments {
		va := volumeAttachment{doc}
		logger.Debugf(context.TODO(), "  attachment %#v", doc)
		var (
			hostMachine, hostUnit string
		)
		if va.Host().Kind() == names.UnitTagKind {
			hostUnit = va.Host().Id()
		} else {
			hostMachine = va.Host().Id()
		}
		args := description.VolumeAttachmentArgs{
			HostUnit:    hostUnit,
			HostMachine: hostMachine,
		}
		if info, err := va.Info(); err == nil {
			logger.Debugf(context.TODO(), "    info %#v", info)
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
			logger.Debugf(context.TODO(), "    params %#v", params)
			args.ReadOnly = params.ReadOnly
		}
		exVolume.AddAttachment(args)
	}

	for _, doc := range attachmentPlans {
		va := volumeAttachmentPlan{doc}
		logger.Debugf(context.TODO(), "  attachment plan %#v", doc)
		args := description.VolumeAttachmentPlanArgs{
			Machine: va.Machine().Id(),
		}
		if info, err := va.PlanInfo(); err == nil {
			logger.Debugf(context.TODO(), "    plan info %#v", info)
			args.DeviceType = string(info.DeviceType)
			args.DeviceAttributes = info.DeviceAttributes
		} else if !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		if info, err := va.BlockDeviceInfo(); err == nil {
			logger.Debugf(context.TODO(), "    block device info %#v", info)
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
		} else if !errors.Is(err, errors.NotFound) {
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
	e.logger.Debugf(context.TODO(), "read %d volume attachment documents", count)
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
	e.logger.Debugf(context.TODO(), "read %d volume attachment plan documents", count)
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
		ID:      fs.FilesystemTag().Id(),
		Storage: storage.Id(),
		Volume:  volume.Id(),
	}
	logger.Debugf(context.TODO(), "addFilesystem: %#v", fs.doc)
	if info, err := fs.Info(); err == nil {
		logger.Debugf(context.TODO(), "  info %#v", info)
		args.Provisioned = true
		args.Size = info.Size
		args.Pool = info.Pool
		args.FilesystemID = info.FilesystemId
	} else {
		params, _ := fs.Params()
		logger.Debugf(context.TODO(), "  params %#v", params)
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
	if count := len(fsAttachments); count != fs.doc.AttachmentCount {
		return errors.Errorf("filesystem attachment count mismatch, have %d, expected %d",
			count, fs.doc.AttachmentCount)
	}
	for _, doc := range fsAttachments {
		va := filesystemAttachment{doc}
		logger.Debugf(context.TODO(), "  attachment %#v", doc)
		var (
			hostMachine, hostUnit string
		)
		if va.Host().Kind() == names.UnitTagKind {
			hostUnit = va.Host().Id()
		} else {
			hostMachine = va.Host().Id()
		}
		args := description.FilesystemAttachmentArgs{
			HostUnit:    hostUnit,
			HostMachine: hostMachine,
		}
		if info, err := va.Info(); err == nil {
			logger.Debugf(context.TODO(), "    info %#v", info)
			args.Provisioned = true
			args.ReadOnly = info.ReadOnly
			args.MountPoint = info.MountPoint
		} else {
			params, _ := va.Params()
			logger.Debugf(context.TODO(), "    params %#v", params)
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
	e.logger.Debugf(context.TODO(), "read %d filesystem attachment documents", count)
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

func (e *exporter) addStorage(instance *storageInstance, attachments []string) error {
	owner, ok := instance.Owner()
	if !ok {
		owner = nil
	}
	cons := description.StorageInstanceConstraints(instance.doc.Constraints)
	args := description.StorageArgs{
		ID:          instance.StorageTag().Id(),
		Kind:        instance.Kind().String(),
		UnitOwner:   owner.Id(),
		Name:        instance.StorageName(),
		Attachments: attachments,
		Constraints: &cons,
	}
	e.model.AddStorage(args)
	return nil
}

func (e *exporter) readStorageAttachments() (map[string][]string, error) {
	coll, closer := e.st.db().GetCollection(storageAttachmentsC)
	defer closer()

	result := make(map[string][]string)
	var doc storageAttachmentDoc
	var count int
	iter := coll.Find(nil).Iter()
	defer func() { _ = iter.Close() }()
	for iter.Next(&doc) {
		result[doc.StorageInstance] = append(result[doc.StorageInstance], doc.Unit)
		count++
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "failed to read storage attachments")
	}
	e.logger.Debugf(context.TODO(), "read %d storage attachment documents", count)
	return result, nil
}
