// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"time"

	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/tools"
)

// When we import a new model, we need to give the leaders some time to
// settle. We don't want to have leader switches just because we migrated an
// environment, so this time needs to be long enough to make sure we cover
// the time taken to migration a reasonable sized environment. We don't yet
// know how long this is going to be, but we need something.
var initialLeaderClaimTime = time.Minute

// Import the database agnostic model representation into the database.
func (st *State) Import(model description.Model) (_ *Model, _ *State, err error) {
	logger := loggo.GetLogger("juju.state.import-model")
	logger.Debugf("import starting for model %s", model.Tag().Id())
	// At this stage, attempting to import a model with the same
	// UUID as an existing model will error.
	tag := model.Tag()
	_, err = st.GetModel(tag)
	if err == nil {
		// We have an existing matching model.
		return nil, nil, errors.AlreadyExistsf("model with UUID %s", tag.Id())
	} else if !errors.IsNotFound(err) {
		return nil, nil, errors.Trace(err)
	}

	if len(model.RemoteApplications()) != 0 {
		// Cross-model relations are currently limited to models on
		// the same controller, while migration is for getting the
		// model to a new controller.
		return nil, nil, errors.New("can't import models with remote applications")
	}

	// Create the model.
	cfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	args := ModelArgs{
		CloudName:     model.Cloud(),
		CloudRegion:   model.CloudRegion(),
		Config:        cfg,
		Owner:         model.Owner(),
		MigrationMode: MigrationModeImporting,

		// NOTE(axw) we create the model without any storage
		// pools. We'll need to import the storage pools from
		// the model description before adding any volumes,
		// filesystems or storage instances.
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	}
	if creds := model.CloudCredential(); creds != nil {
		// Need to add credential or make sure an existing credential
		// matches.
		// TODO: there really should be a way to create a cloud credential
		// tag in the names package from the cloud, owner and name.
		credID := fmt.Sprintf("%s/%s/%s", creds.Cloud(), creds.Owner(), creds.Name())
		if !names.IsValidCloudCredential(credID) {
			return nil, nil, errors.Errorf("model credentails id not valid: %q", credID)
		}
		credTag := names.NewCloudCredentialTag(credID)

		existingCreds, err := st.CloudCredential(credTag)

		if errors.IsNotFound(err) {
			credential := cloud.NewCredential(
				cloud.AuthType(creds.AuthType()),
				creds.Attributes())
			if err := st.UpdateCloudCredential(credTag, credential); err != nil {
				return nil, nil, errors.Trace(err)
			}
		} else if err != nil {
			return nil, nil, errors.Trace(err)
		} else {
			// ensure existing creds match
			if string(existingCreds.AuthType()) != creds.AuthType() {
				return nil, nil, errors.Errorf("credential auth type mismatch: %q != %q", existingCreds.AuthType(), creds.AuthType())
			}
			if !reflect.DeepEqual(existingCreds.Attributes(), creds.Attributes()) {
				return nil, nil, errors.Errorf("credential attribute mismatch: %v != %v", existingCreds.Attributes(), creds.Attributes())
			}
			if existingCreds.Revoked {
				return nil, nil, errors.Errorf("credential %q is revoked", credID)
			}
		}

		args.CloudCredential = credTag
	}
	dbModel, newSt, err := st.NewModel(args)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logger.Debugf("model created %s/%s", dbModel.Owner().Id(), dbModel.Name())
	defer func() {
		if err != nil {
			newSt.Close()
		}
	}()
	// We don't actually care what the old model status was, because we are
	// going to set it to busy, with a message of migrating.
	if err := dbModel.SetStatus(status.StatusInfo{
		Status:  status.Busy,
		Message: "importing",
	}); err != nil {
		return nil, nil, errors.Trace(err)
	}

	// I would have loved to use import, but that is a reserved word.
	restore := importer{
		st:      newSt,
		dbModel: dbModel,
		model:   model,
		logger:  logger,
	}
	if err := restore.sequences(); err != nil {
		return nil, nil, errors.Annotate(err, "sequences")
	}
	// We need to import the sequences first as we may add blocks
	// in the modelExtras which will touch the block sequence.
	if err := restore.modelExtras(); err != nil {
		return nil, nil, errors.Annotate(err, "base model aspects")
	}
	if err := newSt.SetModelConstraints(restore.constraints(model.Constraints())); err != nil {
		return nil, nil, errors.Annotate(err, "model constraints")
	}
	if err := restore.sshHostKeys(); err != nil {
		return nil, nil, errors.Annotate(err, "sshHostKeys")
	}
	if err := restore.cloudimagemetadata(); err != nil {
		return nil, nil, errors.Annotate(err, "cloudimagemetadata")
	}
	if err := restore.actions(); err != nil {
		return nil, nil, errors.Annotate(err, "actions")
	}

	if err := restore.modelUsers(); err != nil {
		return nil, nil, errors.Annotate(err, "modelUsers")
	}
	if err := restore.machines(); err != nil {
		return nil, nil, errors.Annotate(err, "machines")
	}
	if err := restore.applications(); err != nil {
		return nil, nil, errors.Annotate(err, "applications")
	}
	if err := restore.relations(); err != nil {
		return nil, nil, errors.Annotate(err, "relations")
	}
	if err := restore.spaces(); err != nil {
		return nil, nil, errors.Annotate(err, "spaces")
	}
	if err := restore.linklayerdevices(); err != nil {
		return nil, nil, errors.Annotate(err, "linklayerdevices")
	}
	if err := restore.subnets(); err != nil {
		return nil, nil, errors.Annotate(err, "subnets")
	}
	if err := restore.ipaddresses(); err != nil {
		return nil, nil, errors.Annotate(err, "ipaddresses")
	}

	if err := restore.storage(); err != nil {
		return nil, nil, errors.Annotate(err, "storage")
	}

	// NOTE: at the end of the import make sure that the mode of the model
	// is set to "imported" not "active" (or whatever we call it). This way
	// we don't start model workers for it before the migration process
	// is complete.

	// Update the sequences to match that the source.

	if err := dbModel.SetSLA(
		model.SLA().Level(),
		model.SLA().Owner(),
		[]byte(model.SLA().Credentials()),
	); err != nil {
		return nil, nil, errors.Trace(err)
	}

	if MeterStatusFromString(model.MeterStatus().Code()).String() != MeterNotAvailable.String() {
		if err := dbModel.SetMeterStatus(model.MeterStatus().Code(), model.MeterStatus().Info()); err != nil {
			return nil, nil, errors.Trace(err)
		}
	}

	logger.Debugf("import success")
	return dbModel, newSt, nil
}

type importer struct {
	st      *State
	dbModel *Model
	model   description.Model
	logger  loggo.Logger
	// applicationUnits is populated at the end of loading the applications, and is a
	// map of application name to units of that application.
	applicationUnits map[string][]*Unit
}

func (i *importer) modelExtras() error {
	if latest := i.model.LatestToolsVersion(); latest != version.Zero {
		if err := i.dbModel.UpdateLatestToolsVersion(latest); err != nil {
			return errors.Trace(err)
		}
	}

	if annotations := i.model.Annotations(); len(annotations) > 0 {
		if err := i.st.SetAnnotations(i.dbModel, annotations); err != nil {
			return errors.Trace(err)
		}
	}

	blockType := map[string]BlockType{
		"destroy-model": DestroyBlock,
		"remove-object": RemoveBlock,
		"all-changes":   ChangeBlock,
	}

	for blockName, message := range i.model.Blocks() {
		block, ok := blockType[blockName]
		if !ok {
			return errors.Errorf("unknown block type: %q", blockName)
		}
		i.st.SwitchBlockOn(block, message)
	}

	if err := i.importStatusHistory(modelGlobalKey, i.model.StatusHistory()); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) sequences() error {
	sequenceValues := i.model.Sequences()
	docs := make([]interface{}, 0, len(sequenceValues))
	for key, value := range sequenceValues {
		// The sequences which track charm revisions aren't imported
		// here because they get set when charm binaries are imported
		// later. Importing them here means the wrong values get used.
		if !isCharmRevSeqName(key) {
			docs = append(docs, sequenceDoc{
				DocID:   key,
				Name:    key,
				Counter: value,
			})
		}
	}

	// In reality, we will almost always have sequences to migrate.
	// However, in tests, sometimes we don't.
	if len(docs) == 0 {
		return nil
	}

	sequences, closer := i.st.db().GetCollection(sequenceC)
	defer closer()

	if err := sequences.Writeable().Insert(docs...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) modelUsers() error {
	i.logger.Debugf("importing users")

	// The user that was auto-added when we created the model will have
	// the wrong DateCreated, so we remove it, and add in all the users we
	// know about. It is also possible that the owner of the model no
	// longer has access to the model due to changes over time.
	if err := i.st.RemoveUserAccess(i.dbModel.Owner(), i.dbModel.ModelTag()); err != nil {
		return errors.Trace(err)
	}

	users := i.model.Users()
	modelUUID := i.dbModel.UUID()
	var ops []txn.Op
	for _, user := range users {
		ops = append(ops, createModelUserOps(
			modelUUID,
			user.Name(),
			user.CreatedBy(),
			user.DisplayName(),
			user.DateCreated(),
			permission.Access(user.Access()))...,
		)
	}
	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	// Now set their last connection times.
	for _, user := range users {
		i.logger.Debugf("user %s", user.Name())
		lastConnection := user.LastConnection()
		if lastConnection.IsZero() {
			continue
		}
		err := i.st.updateLastModelConnection(user.Name(), lastConnection)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (i *importer) machines() error {
	i.logger.Debugf("importing machines")
	for _, m := range i.model.Machines() {
		if err := i.machine(m); err != nil {
			i.logger.Errorf("error importing machine: %s", err)
			return errors.Annotate(err, m.Id())
		}
	}

	i.logger.Debugf("importing machines succeeded")
	return nil
}

func (i *importer) machine(m description.Machine) error {
	// Import this machine, then import its containers.
	i.logger.Debugf("importing machine %s", m.Id())

	// 1. construct a machineDoc
	mdoc, err := i.makeMachineDoc(m)
	if err != nil {
		return errors.Annotatef(err, "machine %s", m.Id())
	}
	// 2. construct enough MachineTemplate to pass into 'insertNewMachineOps'
	//    - adds constraints doc
	//    - adds status doc
	//    - adds machine block devices doc

	mStatus := m.Status()
	if mStatus == nil {
		return errors.NotValidf("missing status")
	}
	machineStatusDoc := statusDoc{
		ModelUUID:  i.st.ModelUUID(),
		Status:     status.Status(mStatus.Value()),
		StatusInfo: mStatus.Message(),
		StatusData: mStatus.Data(),
		Updated:    mStatus.Updated().UnixNano(),
	}
	// A machine isn't valid if it doesn't have an instance.
	instance := m.Instance()
	instStatus := instance.Status()
	instanceStatusDoc := statusDoc{
		ModelUUID:  i.st.ModelUUID(),
		Status:     status.Status(instStatus.Value()),
		StatusInfo: instStatus.Message(),
		StatusData: instStatus.Data(),
		Updated:    instStatus.Updated().UnixNano(),
	}
	cons := i.constraints(m.Constraints())
	prereqOps, machineOp := i.st.baseNewMachineOps(
		mdoc,
		machineStatusDoc,
		instanceStatusDoc,
		cons,
	)

	// 3. create op for adding in instance data
	prereqOps = append(prereqOps, i.machineInstanceOp(mdoc, instance))

	if parentId := ParentId(mdoc.Id); parentId != "" {
		prereqOps = append(prereqOps,
			// Update containers record for host machine.
			addChildToContainerRefOp(i.st, parentId, mdoc.Id),
		)
	}
	// insertNewContainerRefOp adds an empty doc into the containerRefsC
	// collection for the machine being added.
	prereqOps = append(prereqOps, insertNewContainerRefOp(i.st, mdoc.Id))

	// 4. gather prereqs and machine op, run ops.
	ops := append(prereqOps, machineOp)

	// 5. add any ops that we may need to add the opened ports information.
	ops = append(ops, i.machinePortsOps(m)...)

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	machine := newMachine(i.st, mdoc)
	if annotations := m.Annotations(); len(annotations) > 0 {
		if err := i.st.SetAnnotations(machine, annotations); err != nil {
			return errors.Trace(err)
		}
	}
	if err := i.importStatusHistory(machine.globalKey(), m.StatusHistory()); err != nil {
		return errors.Trace(err)
	}
	if err := i.importStatusHistory(machine.globalInstanceKey(), instance.StatusHistory()); err != nil {
		return errors.Trace(err)
	}
	if err := i.importMachineBlockDevices(machine, m); err != nil {
		return errors.Trace(err)
	}

	// Now that this machine exists in the database, process each of the
	// containers in this machine.
	for _, container := range m.Containers() {
		if err := i.machine(container); err != nil {
			return errors.Annotate(err, container.Id())
		}
	}
	return nil
}

func (i *importer) importMachineBlockDevices(machine *Machine, m description.Machine) error {
	var devices []BlockDeviceInfo
	for _, device := range m.BlockDevices() {
		devices = append(devices, BlockDeviceInfo{
			DeviceName:     device.Name(),
			DeviceLinks:    device.Links(),
			Label:          device.Label(),
			UUID:           device.UUID(),
			HardwareId:     device.HardwareID(),
			WWN:            device.WWN(),
			BusAddress:     device.BusAddress(),
			Size:           device.Size(),
			FilesystemType: device.FilesystemType(),
			InUse:          device.InUse(),
			MountPoint:     device.MountPoint(),
		})
	}

	if err := machine.SetMachineBlockDevices(devices...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) machinePortsOps(m description.Machine) []txn.Op {
	var result []txn.Op
	machineID := m.Id()

	for _, ports := range m.OpenedPorts() {
		subnetID := ports.SubnetID()
		doc := &portsDoc{
			MachineID: machineID,
			SubnetID:  subnetID,
		}
		for _, opened := range ports.OpenPorts() {
			doc.Ports = append(doc.Ports, PortRange{
				UnitName: opened.UnitName(),
				FromPort: opened.FromPort(),
				ToPort:   opened.ToPort(),
				Protocol: opened.Protocol(),
			})
		}
		result = append(result, txn.Op{
			C:      openedPortsC,
			Id:     portsGlobalKey(machineID, subnetID),
			Assert: txn.DocMissing,
			Insert: doc,
		})
	}

	return result
}

func (i *importer) machineInstanceOp(mdoc *machineDoc, inst description.CloudInstance) txn.Op {
	doc := &instanceData{
		DocID:      mdoc.DocID,
		MachineId:  mdoc.Id,
		InstanceId: instance.Id(inst.InstanceId()),
		ModelUUID:  mdoc.ModelUUID,
	}

	if arch := inst.Architecture(); arch != "" {
		doc.Arch = &arch
	}
	if mem := inst.Memory(); mem != 0 {
		doc.Mem = &mem
	}
	if rootDisk := inst.RootDisk(); rootDisk != 0 {
		doc.RootDisk = &rootDisk
	}
	if cores := inst.CpuCores(); cores != 0 {
		doc.CpuCores = &cores
	}
	if power := inst.CpuPower(); power != 0 {
		doc.CpuPower = &power
	}
	if tags := inst.Tags(); len(tags) > 0 {
		doc.Tags = &tags
	}
	if az := inst.AvailabilityZone(); az != "" {
		doc.AvailZone = &az
	}

	return txn.Op{
		C:      instanceDataC,
		Id:     mdoc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (i *importer) makeMachineDoc(m description.Machine) (*machineDoc, error) {
	id := m.Id()
	supported, supportedSet := m.SupportedContainers()
	supportedContainers := make([]instance.ContainerType, len(supported))
	for j, c := range supported {
		supportedContainers[j] = instance.ContainerType(c)
	}
	jobs, err := i.makeMachineJobs(m.Jobs())
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTag := m.Tag()
	return &machineDoc{
		DocID:                    i.st.docID(id),
		Id:                       id,
		ModelUUID:                i.st.ModelUUID(),
		Nonce:                    m.Nonce(),
		Series:                   m.Series(),
		ContainerType:            m.ContainerType(),
		Principals:               nil, // Set during unit import.
		Life:                     Alive,
		Tools:                    i.makeTools(m.Tools()),
		Jobs:                     jobs,
		NoVote:                   true,  // State servers can't be migrated yet.
		HasVote:                  false, // State servers can't be migrated yet.
		PasswordHash:             m.PasswordHash(),
		Clean:                    !i.machineHasUnits(machineTag),
		Volumes:                  i.machineVolumes(machineTag),
		Filesystems:              i.machineFilesystems(machineTag),
		Addresses:                i.makeAddresses(m.ProviderAddresses()),
		MachineAddresses:         i.makeAddresses(m.MachineAddresses()),
		PreferredPrivateAddress:  i.makeAddress(m.PreferredPrivateAddress()),
		PreferredPublicAddress:   i.makeAddress(m.PreferredPublicAddress()),
		SupportedContainersKnown: supportedSet,
		SupportedContainers:      supportedContainers,
		Placement:                m.Placement(),
	}, nil
}

func (i *importer) machineHasUnits(tag names.MachineTag) bool {
	for _, app := range i.model.Applications() {
		for _, unit := range app.Units() {
			if unit.Machine() == tag {
				return true
			}
		}
	}
	return false
}

func (i *importer) machineVolumes(tag names.MachineTag) []string {
	var result []string
	for _, volume := range i.model.Volumes() {
		for _, attachment := range volume.Attachments() {
			if attachment.Machine() == tag {
				result = append(result, volume.Tag().Id())
			}
		}
	}
	return result
}

func (i *importer) machineFilesystems(tag names.MachineTag) []string {
	var result []string
	for _, filesystem := range i.model.Filesystems() {
		for _, attachment := range filesystem.Attachments() {
			if attachment.Machine() == tag {
				result = append(result, filesystem.Tag().Id())
			}
		}
	}
	return result
}

func (i *importer) makeMachineJobs(jobs []string) ([]MachineJob, error) {
	// At time of writing, there are three valid jobs. If any jobs gets
	// deprecated or changed in the future, older models that specify those
	// jobs need to be handled here.
	result := make([]MachineJob, 0, len(jobs))
	for _, job := range jobs {
		switch job {
		case "host-units":
			result = append(result, JobHostUnits)
		case "api-server":
			result = append(result, JobManageModel)
		default:
			return nil, errors.Errorf("unknown machine job: %q", job)
		}
	}
	return result, nil
}

func (i *importer) makeTools(t description.AgentTools) *tools.Tools {
	if t == nil {
		return nil
	}
	return &tools.Tools{
		Version: t.Version(),
		URL:     t.URL(),
		SHA256:  t.SHA256(),
		Size:    t.Size(),
	}
}

func (i *importer) makeAddress(addr description.Address) address {
	if addr == nil {
		return address{}
	}
	return address{
		Value:       addr.Value(),
		AddressType: addr.Type(),
		Scope:       addr.Scope(),
		Origin:      addr.Origin(),
	}
}

func (i *importer) makeAddresses(addrs []description.Address) []address {
	result := make([]address, len(addrs))
	for j, addr := range addrs {
		result[j] = i.makeAddress(addr)
	}
	return result
}

func (i *importer) applications() error {
	i.logger.Debugf("importing applications")

	// Ensure we import principal applications first, so that
	// subordinate units can refer to the principal ones.
	var principals, subordinates []description.Application
	for _, app := range i.model.Applications() {
		if app.Subordinate() {
			subordinates = append(subordinates, app)
		} else {
			principals = append(principals, app)
		}
	}

	for _, s := range append(principals, subordinates...) {
		if err := i.application(s); err != nil {
			i.logger.Errorf("error importing application %s: %s", s.Name(), err)
			return errors.Annotate(err, s.Name())
		}
	}

	if err := i.loadUnits(); err != nil {
		return errors.Annotate(err, "loading new units from db")
	}
	i.logger.Debugf("importing applications succeeded")
	return nil
}

func (i *importer) loadUnits() error {
	unitsCollection, closer := i.st.db().GetCollection(unitsC)
	defer closer()

	docs := []unitDoc{}
	err := unitsCollection.Find(nil).All(&docs)
	if err != nil {
		return errors.Annotate(err, "cannot get all units")
	}

	result := make(map[string][]*Unit)
	for _, doc := range docs {
		units := result[doc.Application]
		result[doc.Application] = append(units, newUnit(i.st, &doc))
	}
	i.applicationUnits = result
	return nil

}

// makeStatusDoc assumes status is non-nil.
func (i *importer) makeStatusDoc(statusVal description.Status) statusDoc {
	return statusDoc{
		Status:     status.Status(statusVal.Value()),
		StatusInfo: statusVal.Message(),
		StatusData: statusVal.Data(),
		Updated:    statusVal.Updated().UnixNano(),
	}
}

func (i *importer) application(a description.Application) error {
	// Import this application, then its units.
	i.logger.Debugf("importing application %s", a.Name())

	// 1. construct an applicationDoc
	appDoc, err := i.makeApplicationDoc(a)
	if err != nil {
		return errors.Trace(err)
	}
	app := newApplication(i.st, appDoc)

	// 2. construct a statusDoc
	status := a.Status()
	if status == nil {
		return errors.NotValidf("missing status")
	}
	statusDoc := i.makeStatusDoc(status)
	// TODO: update never set malarky... maybe...

	// When creating the settings, we ignore nils.  In other circumstances, nil
	// means to delete the value (reset to default), so creating with nil should
	// mean to use the default, i.e. don't set the value.
	// There may have existed some applications with settings that contained
	// nil values, see lp#1667199. When importing, we want these stripped.
	removeNils(a.Settings())

	ops, err := addApplicationOps(i.st, app, addApplicationOpsArgs{
		applicationDoc:     appDoc,
		statusDoc:          statusDoc,
		constraints:        i.constraints(a.Constraints()),
		storage:            i.storageConstraints(a.StorageConstraints()),
		settings:           a.Settings(),
		leadershipSettings: a.LeadershipSettings(),
	})
	if err != nil {
		return errors.Trace(err)
	}

	ops = append(ops, txn.Op{
		C:      endpointBindingsC,
		Id:     app.globalKey(),
		Assert: txn.DocMissing,
		Insert: endpointBindingsDoc{
			Bindings: bindingsMap(a.EndpointBindings()),
		},
	})

	ops = append(ops, i.appResourceOps(a)...)

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	if annotations := a.Annotations(); len(annotations) > 0 {
		if err := i.st.SetAnnotations(app, annotations); err != nil {
			return errors.Trace(err)
		}
	}
	if err := i.importStatusHistory(app.globalKey(), a.StatusHistory()); err != nil {
		return errors.Trace(err)
	}

	for _, unit := range a.Units() {
		if err := i.unit(a, unit); err != nil {
			return errors.Trace(err)
		}
	}

	if a.Leader() != "" {
		if err := i.st.LeadershipClaimer().ClaimLeadership(
			a.Name(),
			a.Leader(),
			initialLeaderClaimTime); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (i *importer) appResourceOps(app description.Application) []txn.Op {
	// Add a placeholder record for each resource that is a placeholder.
	// Resources define placeholders as resources where the timestamp is Zero.
	var result []txn.Op
	appName := app.Name()

	var makeResourceDoc = func(id, name string, rev description.ResourceRevision) resourceDoc {
		fingerprint, _ := hex.DecodeString(rev.FingerprintHex())
		return resourceDoc{
			ID:            id,
			ApplicationID: appName,
			Name:          name,
			Type:          rev.Type(),
			Path:          rev.Path(),
			Description:   rev.Description(),
			Origin:        rev.Origin(),
			Revision:      rev.Revision(),
			Fingerprint:   fingerprint,
			Size:          rev.Size(),
			Username:      rev.Username(),
		}
	}

	for _, r := range app.Resources() {
		// I cannot for the life of me find the function where the underlying
		// resource id is defined to be the appname/resname but that is what
		// ends up in the DB.
		resName := r.Name()
		resID := appName + "/" + resName
		// Check both the app and charmstore
		if appRev := r.ApplicationRevision(); appRev.Timestamp().IsZero() {
			result = append(result, txn.Op{
				C:      resourcesC,
				Id:     applicationResourceID(resID),
				Assert: txn.DocMissing,
				Insert: makeResourceDoc(resID, resName, appRev),
			})
		}
		if storeRev := r.CharmStoreRevision(); storeRev.Timestamp().IsZero() {
			doc := makeResourceDoc(resID, resName, storeRev)
			// Now the resource code is particularly stupid and instead of using
			// the ID, or encoding the type somewhere, it uses the fact that the
			// LastPolled time to indicate it is the charm store version.
			doc.LastPolled = time.Now()
			result = append(result, txn.Op{
				C:      resourcesC,
				Id:     charmStoreResourceID(resID),
				Assert: txn.DocMissing,
				Insert: doc,
			})
		}
	}
	return result
}

func (i *importer) storageConstraints(cons map[string]description.StorageConstraint) map[string]StorageConstraints {
	if len(cons) == 0 {
		return nil
	}
	result := make(map[string]StorageConstraints)
	for key, value := range cons {
		result[key] = StorageConstraints{
			Pool:  value.Pool(),
			Size:  value.Size(),
			Count: value.Count(),
		}
	}
	return result
}

func (i *importer) unit(s description.Application, u description.Unit) error {
	i.logger.Debugf("importing unit %s", u.Name())

	// 1. construct a unitDoc
	udoc, err := i.makeUnitDoc(s, u)
	if err != nil {
		return errors.Trace(err)
	}

	// 2. construct a statusDoc for the workload status and agent status
	agentStatus := u.AgentStatus()
	if agentStatus == nil {
		return errors.NotValidf("missing agent status")
	}
	agentStatusDoc := i.makeStatusDoc(agentStatus)

	workloadStatus := u.WorkloadStatus()
	if workloadStatus == nil {
		return errors.NotValidf("missing workload status")
	}
	workloadStatusDoc := i.makeStatusDoc(workloadStatus)

	workloadVersion := u.WorkloadVersion()
	versionStatus := status.Active
	if workloadVersion == "" {
		versionStatus = status.Unknown
	}
	workloadVersionDoc := statusDoc{
		Status:     versionStatus,
		StatusInfo: workloadVersion,
	}

	ops, err := addUnitOps(i.st, addUnitOpsArgs{
		unitDoc:            udoc,
		agentStatusDoc:     agentStatusDoc,
		workloadStatusDoc:  workloadStatusDoc,
		workloadVersionDoc: workloadVersionDoc,
		meterStatusDoc: &meterStatusDoc{
			Code: u.MeterStatusCode(),
			Info: u.MeterStatusInfo(),
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	// If the unit is a principal, add it to its machine.
	if u.Principal().Id() == "" {
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     u.Machine().Id(),
			Assert: txn.DocExists,
			Update: bson.M{"$addToSet": bson.M{"principals": u.Name()}},
		})
	}

	// We should only have constraints for principal agents.
	// We don't encode that business logic here, if there are constraints
	// in the imported model, we put them in the database.
	if cons := u.Constraints(); cons != nil {
		agentGlobalKey := unitAgentGlobalKey(u.Name())
		ops = append(ops, createConstraintsOp(agentGlobalKey, i.constraints(cons)))
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		i.logger.Debugf("failed ops: %#v", ops)
		return errors.Trace(err)
	}

	unit := newUnit(i.st, udoc)
	if annotations := u.Annotations(); len(annotations) > 0 {
		if err := i.st.SetAnnotations(unit, annotations); err != nil {
			return errors.Trace(err)
		}
	}
	if err := i.importStatusHistory(unit.globalKey(), u.WorkloadStatusHistory()); err != nil {
		return errors.Trace(err)
	}
	if err := i.importStatusHistory(unit.globalAgentKey(), u.AgentStatusHistory()); err != nil {
		return errors.Trace(err)
	}
	if err := i.importStatusHistory(unit.globalWorkloadVersionKey(), u.WorkloadVersionHistory()); err != nil {
		return errors.Trace(err)
	}
	if err := i.importUnitPayloads(unit, u.Payloads()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (i *importer) importUnitPayloads(unit *Unit, payloads []description.Payload) error {
	up, err := i.st.UnitPayloads(unit)
	if err != nil {
		return errors.Trace(err)
	}

	for _, p := range payloads {
		if err := up.Track(payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: p.Name(),
				Type: p.Type(),
			},
			ID:     p.RawID(),
			Status: p.State(),
			Labels: p.Labels(),
		}); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (i *importer) makeApplicationDoc(s description.Application) (*applicationDoc, error) {
	charmURL, err := charm.ParseURL(s.CharmURL())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &applicationDoc{
		Name:                 s.Name(),
		Series:               s.Series(),
		Subordinate:          s.Subordinate(),
		CharmURL:             charmURL,
		Channel:              s.Channel(),
		CharmModifiedVersion: s.CharmModifiedVersion(),
		ForceCharm:           s.ForceCharm(),
		Life:                 Alive,
		UnitCount:            len(s.Units()),
		RelationCount:        i.relationCount(s.Name()),
		Exposed:              s.Exposed(),
		MinUnits:             s.MinUnits(),
		MetricCredentials:    s.MetricsCredentials(),
	}, nil
}

func (i *importer) relationCount(application string) int {
	count := 0

	for _, rel := range i.model.Relations() {
		for _, ep := range rel.Endpoints() {
			if ep.ApplicationName() == application {
				count++
			}
		}
	}

	return count
}

func (i *importer) makeUnitDoc(s description.Application, u description.Unit) (*unitDoc, error) {
	// NOTE: if we want to support units having different charms deployed
	// than the application recomments and migrate that, then we should serialize
	// the charm url for each unit rather than grabbing the applications charm url.
	// Currently the units charm url matching the application is a precondiation
	// to migration.
	charmURL, err := charm.ParseURL(s.CharmURL())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var subordinates []string
	if subs := u.Subordinates(); len(subs) > 0 {
		for _, s := range subs {
			subordinates = append(subordinates, s.Id())
		}
	}

	return &unitDoc{
		Name:                   u.Name(),
		Application:            s.Name(),
		Series:                 s.Series(),
		CharmURL:               charmURL,
		Principal:              u.Principal().Id(),
		Subordinates:           subordinates,
		StorageAttachmentCount: i.unitStorageAttachmentCount(u.Tag()),
		MachineId:              u.Machine().Id(),
		Tools:                  i.makeTools(u.Tools()),
		Life:                   Alive,
		PasswordHash:           u.PasswordHash(),
	}, nil
}

func (i *importer) unitStorageAttachmentCount(unit names.UnitTag) int {
	count := 0
	for _, storage := range i.model.Storages() {
		for _, tag := range storage.Attachments() {
			if tag == unit {
				count++
			}
		}
	}
	return count
}

func (i *importer) relations() error {
	i.logger.Debugf("importing relations")
	for _, r := range i.model.Relations() {
		if err := i.relation(r); err != nil {
			i.logger.Errorf("error importing relation %s: %s", r.Key(), err)
			return errors.Annotate(err, r.Key())
		}
	}

	i.logger.Debugf("importing relations succeeded")
	return nil
}

func (i *importer) relation(rel description.Relation) error {
	relationDoc := i.makeRelationDoc(rel)
	ops := []txn.Op{
		{
			C:      relationsC,
			Id:     relationDoc.Key,
			Assert: txn.DocMissing,
			Insert: relationDoc,
		},
	}

	dbRelation := newRelation(i.st, relationDoc)
	// Add an op that adds the relation scope document for each
	// unit of the application, and an op that adds the relation settings
	// for each unit.
	for _, endpoint := range rel.Endpoints() {
		units := i.applicationUnits[endpoint.ApplicationName()]
		for _, unit := range units {
			ru, err := dbRelation.Unit(unit)
			if err != nil {
				return errors.Trace(err)
			}
			ruKey := ru.key()
			ops = append(ops, txn.Op{
				C:      relationScopesC,
				Id:     ruKey,
				Assert: txn.DocMissing,
				Insert: relationScopeDoc{
					Key: ruKey,
				},
			},
				createSettingsOp(settingsC, ruKey, endpoint.Settings(unit.Name())),
			)
		}
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (i *importer) makeRelationDoc(rel description.Relation) *relationDoc {
	endpoints := rel.Endpoints()
	doc := &relationDoc{
		Key:       rel.Key(),
		Id:        rel.Id(),
		Endpoints: make([]Endpoint, len(endpoints)),
		Life:      Alive,
	}
	for i, ep := range endpoints {
		doc.Endpoints[i] = Endpoint{
			ApplicationName: ep.ApplicationName(),
			Relation: charm.Relation{
				Name:      ep.Name(),
				Role:      charm.RelationRole(ep.Role()),
				Interface: ep.Interface(),
				Optional:  ep.Optional(),
				Limit:     ep.Limit(),
				Scope:     charm.RelationScope(ep.Scope()),
			},
		}
		doc.UnitCount += ep.UnitCount()
	}
	return doc
}

func (i *importer) spaces() error {
	i.logger.Debugf("importing spaces")
	for _, s := range i.model.Spaces() {
		// The subnets are added after the spaces.
		_, err := i.st.AddSpace(s.Name(), network.Id(s.ProviderID()), nil, s.Public())
		if err != nil {
			i.logger.Errorf("error importing space %s: %s", s.Name(), err)
			return errors.Annotate(err, s.Name())
		}
	}

	i.logger.Debugf("importing spaces succeeded")
	return nil
}

func (i *importer) linklayerdevices() error {
	i.logger.Debugf("importing linklayerdevices")
	for _, device := range i.model.LinkLayerDevices() {
		err := i.addLinkLayerDevice(device)
		if err != nil {
			i.logger.Errorf("error importing ip device %v: %s", device, err)
			return errors.Trace(err)
		}
	}
	// Loop a second time so we can ensure that all devices have had their
	// parent created.
	ops := []txn.Op{}
	for _, device := range i.model.LinkLayerDevices() {
		if device.ParentName() == "" {
			continue
		}
		parentDocID, err := i.parentDocIDFromDevice(device)
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, incrementDeviceNumChildrenOp(parentDocID))

	}
	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	i.logger.Debugf("importing linklayerdevices succeeded")
	return nil
}

func (i *importer) parentDocIDFromDevice(device description.LinkLayerDevice) (string, error) {
	hostMachineID, parentName, err := parseLinkLayerDeviceParentNameAsGlobalKey(device.ParentName())
	if err != nil {
		return "", errors.Trace(err)
	}
	if hostMachineID == "" {
		// ParentName is not a global key, but on the same machine.
		hostMachineID = device.MachineID()
		parentName = device.ParentName()
	}
	return i.st.docID(linkLayerDeviceGlobalKey(hostMachineID, parentName)), nil
}

func (i *importer) addLinkLayerDevice(device description.LinkLayerDevice) error {
	providerID := device.ProviderID()
	modelUUID := i.st.ModelUUID()
	localID := linkLayerDeviceGlobalKey(device.MachineID(), device.Name())
	linkLayerDeviceDocID := i.st.docID(localID)
	newDoc := &linkLayerDeviceDoc{
		ModelUUID:   modelUUID,
		DocID:       linkLayerDeviceDocID,
		MachineID:   device.MachineID(),
		ProviderID:  providerID,
		Name:        device.Name(),
		MTU:         device.MTU(),
		Type:        LinkLayerDeviceType(device.Type()),
		MACAddress:  device.MACAddress(),
		IsAutoStart: device.IsAutoStart(),
		IsUp:        device.IsUp(),
		ParentName:  device.ParentName(),
	}

	ops := []txn.Op{{
		C:      linkLayerDevicesC,
		Id:     newDoc.DocID,
		Insert: newDoc,
	},
		insertLinkLayerDevicesRefsOp(modelUUID, linkLayerDeviceDocID),
	}
	if providerID != "" {
		id := network.Id(providerID)
		ops = append(ops, i.st.networkEntityGlobalKeyOp("linklayerdevice", id))
	}
	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) subnets() error {
	i.logger.Debugf("importing subnets")
	for _, subnet := range i.model.Subnets() {
		info := SubnetInfo{
			CIDR:              subnet.CIDR(),
			ProviderId:        network.Id(subnet.ProviderId()),
			ProviderNetworkId: network.Id(subnet.ProviderNetworkId()),
			VLANTag:           subnet.VLANTag(),
			SpaceName:         subnet.SpaceName(),
		}
		// TODO(babbageclunk): at the moment state.Subnet only stores
		// one AZ.
		zones := subnet.AvailabilityZones()
		if len(zones) > 0 {
			info.AvailabilityZone = zones[0]
		}
		err := i.addSubnet(info)
		if err != nil {
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing subnets succeeded")
	return nil
}

func (i *importer) addSubnet(args SubnetInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		subnet, err := i.st.newSubnetFromArgs(args)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := i.st.addSubnetOps(args)
		if attempt != 0 {
			if _, err = i.st.Subnet(args.CIDR); err == nil {
				return nil, errors.AlreadyExistsf("subnet %q", args.CIDR)
			}
			if err := subnet.Refresh(); err != nil {
				if errors.IsNotFound(err) {
					return nil, errors.Errorf("ProviderId %q not unique", args.ProviderId)
				}
				return nil, errors.Trace(err)
			}
		}
		return ops, nil
	}
	err := i.st.db().Run(buildTxn)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) ipaddresses() error {
	i.logger.Debugf("importing ip addresses")
	for _, addr := range i.model.IPAddresses() {
		err := i.addIPAddress(addr)
		if err != nil {
			i.logger.Errorf("error importing ip address %v: %s", addr, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing ip addresses succeeded")
	return nil
}

func (i *importer) addIPAddress(addr description.IPAddress) error {
	addressValue := addr.Value()
	subnetCIDR := addr.SubnetCIDR()

	globalKey := ipAddressGlobalKey(addr.MachineID(), addr.DeviceName(), addressValue)
	ipAddressDocID := i.st.docID(globalKey)
	providerID := addr.ProviderID()

	modelUUID := i.st.ModelUUID()

	newDoc := &ipAddressDoc{
		DocID:            ipAddressDocID,
		ModelUUID:        modelUUID,
		ProviderID:       providerID,
		DeviceName:       addr.DeviceName(),
		MachineID:        addr.MachineID(),
		SubnetCIDR:       subnetCIDR,
		ConfigMethod:     AddressConfigMethod(addr.ConfigMethod()),
		Value:            addressValue,
		DNSServers:       addr.DNSServers(),
		DNSSearchDomains: addr.DNSSearchDomains(),
		GatewayAddress:   addr.GatewayAddress(),
	}

	ops := []txn.Op{{
		C:      ipAddressesC,
		Id:     newDoc.DocID,
		Insert: newDoc,
	}}

	if providerID != "" {
		id := network.Id(providerID)
		ops = append(ops, i.st.networkEntityGlobalKeyOp("address", id))
	}
	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) sshHostKeys() error {
	i.logger.Debugf("importing ssh host keys")
	for _, key := range i.model.SSHHostKeys() {
		name := names.NewMachineTag(key.MachineID())
		err := i.st.SetSSHHostKeys(name, key.Keys())
		if err != nil {
			i.logger.Errorf("error importing ssh host keys %v: %s", key, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing ssh host keys succeeded")
	return nil
}

func (i *importer) cloudimagemetadata() error {
	i.logger.Debugf("importing cloudimagemetadata")
	images := i.model.CloudImageMetadata()
	metadatas := make([]cloudimagemetadata.Metadata, len(images))
	for index, image := range images {
		metadatas[index] = cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				Source:          image.Source(),
				Stream:          image.Stream(),
				Region:          image.Region(),
				Version:         image.Version(),
				Series:          image.Series(),
				Arch:            image.Arch(),
				RootStorageType: image.RootStorageType(),
				VirtType:        image.VirtType(),
			},
			image.Priority(),
			image.ImageId(),
			image.DateCreated(),
		}
	}
	err := i.st.CloudImageMetadataStorage.SaveMetadata(metadatas)
	if err != nil {
		i.logger.Errorf("error importing cloudimagemetadata %v: %s", images, err)
		return errors.Trace(err)
	}
	i.logger.Debugf("importing cloudimagemetadata succeeded")
	return nil
}

func (i *importer) actions() error {
	i.logger.Debugf("importing actions")
	for _, action := range i.model.Actions() {
		err := i.addAction(action)
		if err != nil {
			i.logger.Errorf("error importing action %v: %s", action, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing actions succeeded")
	return nil
}

func (i *importer) addAction(action description.Action) error {
	modelUUID := i.st.ModelUUID()
	newDoc := &actionDoc{
		DocId:      i.st.docID(action.Id()),
		ModelUUID:  modelUUID,
		Receiver:   action.Receiver(),
		Name:       action.Name(),
		Parameters: action.Parameters(),
		Enqueued:   action.Enqueued(),
		Results:    action.Results(),
		Message:    action.Message(),
		Started:    action.Started(),
		Completed:  action.Completed(),
		Status:     ActionStatus(action.Status()),
	}
	prefix := ensureActionMarker(action.Receiver())
	notificationDoc := &actionNotificationDoc{
		DocId:     i.st.docID(prefix + action.Id()),
		ModelUUID: modelUUID,
		Receiver:  action.Receiver(),
		ActionID:  action.Id(),
	}
	ops := []txn.Op{{
		C:      actionsC,
		Id:     newDoc.DocId,
		Insert: newDoc,
	}, {
		C:      actionNotificationsC,
		Id:     notificationDoc.DocId,
		Insert: notificationDoc,
	}}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) importStatusHistory(globalKey string, history []description.Status) error {
	docs := make([]interface{}, len(history))
	for i, statusVal := range history {
		docs[i] = historicalStatusDoc{
			GlobalKey:  globalKey,
			Status:     status.Status(statusVal.Value()),
			StatusInfo: statusVal.Message(),
			StatusData: statusVal.Data(),
			Updated:    statusVal.Updated().UnixNano(),
		}
	}
	if len(docs) == 0 {
		return nil
	}

	statusHistory, closer := i.st.db().GetCollection(statusesHistoryC)
	defer closer()

	if err := statusHistory.Writeable().Insert(docs...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) constraints(cons description.Constraints) constraints.Value {
	var result constraints.Value
	if cons == nil {
		return result
	}

	if arch := cons.Architecture(); arch != "" {
		result.Arch = &arch
	}
	if container := instance.ContainerType(cons.Container()); container != "" {
		result.Container = &container
	}
	if cores := cons.CpuCores(); cores != 0 {
		result.CpuCores = &cores
	}
	if power := cons.CpuPower(); power != 0 {
		result.CpuPower = &power
	}
	if inst := cons.InstanceType(); inst != "" {
		result.InstanceType = &inst
	}
	if mem := cons.Memory(); mem != 0 {
		result.Mem = &mem
	}
	if disk := cons.RootDisk(); disk != 0 {
		result.RootDisk = &disk
	}
	if spaces := cons.Spaces(); len(spaces) > 0 {
		result.Spaces = &spaces
	}
	if tags := cons.Tags(); len(tags) > 0 {
		result.Tags = &tags
	}
	if virt := cons.VirtType(); virt != "" {
		result.VirtType = &virt
	}
	return result
}

func (i *importer) storage() error {
	if err := i.storagePools(); err != nil {
		return errors.Annotate(err, "storage pools")
	}
	if err := i.storageInstances(); err != nil {
		return errors.Annotate(err, "storage instances")
	}
	if err := i.volumes(); err != nil {
		return errors.Annotate(err, "volumes")
	}
	if err := i.filesystems(); err != nil {
		return errors.Annotate(err, "filesystems")
	}
	return nil
}

func (i *importer) storageInstances() error {
	i.logger.Debugf("importing storage instances")
	for _, storage := range i.model.Storages() {
		err := i.addStorageInstance(storage)
		if err != nil {
			i.logger.Errorf("error importing storage %s: %s", storage.Tag(), err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing storage instances succeeded")
	return nil
}

func (i *importer) addStorageInstance(storage description.Storage) error {
	kind := parseStorageKind(storage.Kind())
	if kind == StorageKindUnknown {
		return errors.Errorf("storage kind %q is unknown", storage.Kind())
	}
	owner, err := storage.Owner()
	if err != nil {
		return errors.Annotate(err, "storage owner")
	}
	var storageOwner string
	if owner != nil {
		storageOwner = owner.String()
	}
	attachments := storage.Attachments()
	tag := storage.Tag()
	var ops []txn.Op
	for _, unit := range attachments {
		ops = append(ops, createStorageAttachmentOp(tag, unit))
	}
	doc := &storageInstanceDoc{
		Id:              storage.Tag().Id(),
		Kind:            kind,
		Owner:           storageOwner,
		StorageName:     storage.Name(),
		AttachmentCount: len(attachments),
		Constraints:     i.storageInstanceConstraints(storage),
	}
	ops = append(ops, txn.Op{
		C:      storageInstancesC,
		Id:     tag.Id(),
		Assert: txn.DocMissing,
		Insert: doc,
	})

	refcounts, closer := i.st.db().GetCollection(refcountsC)
	defer closer()
	storageRefcountKey := entityStorageRefcountKey(owner, storage.Name())
	incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, storageRefcountKey, 1)
	if err != nil {
		return errors.Trace(err)
	}
	ops = append(ops, incRefOp)

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) storageInstanceConstraints(storage description.Storage) storageInstanceConstraints {
	if cons, ok := storage.Constraints(); ok {
		return storageInstanceConstraints(cons)
	}
	// Older versions of Juju did not record storage constraints on the
	// storage instance, so we must do what we do during upgrade steps:
	// reconstitute the constraints from the corresponding volume or
	// filesystem, or else look in the owner's application storage
	// constraints, and if all else fails, apply the defaults.
	var cons storageInstanceConstraints
	var defaultPool string
	switch parseStorageKind(storage.Kind()) {
	case StorageKindBlock:
		defaultPool = string(provider.LoopProviderType)
		for _, volume := range i.model.Volumes() {
			if volume.Storage() == storage.Tag() {
				cons.Pool = volume.Pool()
				cons.Size = volume.Size()
				break
			}
		}
	case StorageKindFilesystem:
		defaultPool = string(provider.RootfsProviderType)
		for _, filesystem := range i.model.Filesystems() {
			if filesystem.Storage() == storage.Tag() {
				cons.Pool = filesystem.Pool()
				cons.Size = filesystem.Size()
				break
			}
		}
	}
	if cons.Pool == "" {
		cons.Pool = defaultPool
		cons.Size = 1024
		if owner, _ := storage.Owner(); owner != nil {
			var appName string
			switch owner := owner.(type) {
			case names.ApplicationTag:
				appName = owner.Id()
			case names.UnitTag:
				appName, _ = names.UnitApplication(owner.Id())
			}
			for _, app := range i.model.Applications() {
				if app.Name() != appName {
					continue
				}
				storageName, _ := names.StorageName(storage.Tag().Id())
				appStorageCons, ok := app.StorageConstraints()[storageName]
				if ok {
					cons.Pool = appStorageCons.Pool()
					cons.Size = appStorageCons.Size()
				}
				break
			}
		}
		logger.Warningf(
			"no volume or filesystem found, using application storage constraints for %s",
			names.ReadableString(storage.Tag()),
		)
	}
	return cons
}

func (i *importer) volumes() error {
	i.logger.Debugf("importing volumes")
	for _, volume := range i.model.Volumes() {
		err := i.addVolume(volume)
		if err != nil {
			i.logger.Errorf("error importing volume %s: %s", volume.Tag(), err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing volumes succeeded")
	return nil
}

func (i *importer) addVolume(volume description.Volume) error {
	im, err := i.st.IAASModel()
	if err != nil {
		return errors.Trace(err)
	}
	attachments := volume.Attachments()
	tag := volume.Tag()
	var params *VolumeParams
	var info *VolumeInfo
	if volume.Provisioned() {
		info = &VolumeInfo{
			HardwareId: volume.HardwareID(),
			WWN:        volume.WWN(),
			Size:       volume.Size(),
			Pool:       volume.Pool(),
			VolumeId:   volume.VolumeID(),
			Persistent: volume.Persistent(),
		}
	} else {
		params = &VolumeParams{
			Size: volume.Size(),
			Pool: volume.Pool(),
		}
	}
	doc := volumeDoc{
		Name:      tag.Id(),
		StorageId: volume.Storage().Id(),
		// Life: ..., // TODO: import life, default is Alive
		Params:          params,
		Info:            info,
		AttachmentCount: len(attachments),
	}
	if detachable, err := isDetachableVolumePool(im, volume.Pool()); err != nil {
		return errors.Trace(err)
	} else if !detachable && len(attachments) == 1 {
		doc.MachineId = attachments[0].Machine().Id()
	}
	status := i.makeStatusDoc(volume.Status())
	ops := im.newVolumeOps(doc, status)

	for _, attachment := range attachments {
		ops = append(ops, i.addVolumeAttachmentOp(tag.Id(), attachment))
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	if err := i.importStatusHistory(volumeGlobalKey(tag.Id()), volume.StatusHistory()); err != nil {
		return errors.Annotate(err, "status history")
	}
	return nil
}

func (i *importer) addVolumeAttachmentOp(volID string, attachment description.VolumeAttachment) txn.Op {
	var info *VolumeAttachmentInfo
	var params *VolumeAttachmentParams
	if attachment.Provisioned() {
		info = &VolumeAttachmentInfo{
			DeviceName: attachment.DeviceName(),
			DeviceLink: attachment.DeviceLink(),
			BusAddress: attachment.BusAddress(),
			ReadOnly:   attachment.ReadOnly(),
		}
	} else {
		params = &VolumeAttachmentParams{
			ReadOnly: attachment.ReadOnly(),
		}
	}

	machineId := attachment.Machine().Id()
	return txn.Op{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(machineId, volID),
		Assert: txn.DocMissing,
		Insert: &volumeAttachmentDoc{
			Volume:  volID,
			Machine: machineId,
			Params:  params,
			Info:    info,
		},
	}
}

func (i *importer) filesystems() error {
	i.logger.Debugf("importing filesystems")
	for _, fs := range i.model.Filesystems() {
		err := i.addFilesystem(fs)
		if err != nil {
			i.logger.Errorf("error importing filesystem %s: %s", fs.Tag(), err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf("importing filesystems succeeded")
	return nil
}

func (i *importer) addFilesystem(filesystem description.Filesystem) error {

	attachments := filesystem.Attachments()
	tag := filesystem.Tag()
	var params *FilesystemParams
	var info *FilesystemInfo
	if filesystem.Provisioned() {
		info = &FilesystemInfo{
			Size:         filesystem.Size(),
			Pool:         filesystem.Pool(),
			FilesystemId: filesystem.FilesystemID(),
		}
	} else {
		params = &FilesystemParams{
			Size: filesystem.Size(),
			Pool: filesystem.Pool(),
		}
	}
	doc := filesystemDoc{
		FilesystemId: tag.Id(),
		StorageId:    filesystem.Storage().Id(),
		VolumeId:     filesystem.Volume().Id(),
		// Life: ..., // TODO: import life, default is Alive
		Params:          params,
		Info:            info,
		AttachmentCount: len(attachments),
	}
	if detachable, err := isDetachableFilesystemPool(i.st, filesystem.Pool()); err != nil {
		return errors.Trace(err)
	} else if !detachable && len(attachments) == 1 {
		doc.MachineId = attachments[0].Machine().Id()
	}
	status := i.makeStatusDoc(filesystem.Status())
	ops := i.st.newFilesystemOps(doc, status)

	for _, attachment := range attachments {
		ops = append(ops, i.addFilesystemAttachmentOp(tag.Id(), attachment))
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	if err := i.importStatusHistory(filesystemGlobalKey(tag.Id()), filesystem.StatusHistory()); err != nil {
		return errors.Annotate(err, "status history")
	}
	return nil
}

func (i *importer) addFilesystemAttachmentOp(fsID string, attachment description.FilesystemAttachment) txn.Op {
	var info *FilesystemAttachmentInfo
	var params *FilesystemAttachmentParams
	if attachment.Provisioned() {
		info = &FilesystemAttachmentInfo{
			MountPoint: attachment.MountPoint(),
			ReadOnly:   attachment.ReadOnly(),
		}
	} else {
		params = &FilesystemAttachmentParams{
			Location: attachment.MountPoint(),
			ReadOnly: attachment.ReadOnly(),
		}
	}

	machineId := attachment.Machine().Id()
	return txn.Op{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(machineId, fsID),
		Assert: txn.DocMissing,
		Insert: &filesystemAttachmentDoc{
			Filesystem: fsID,
			Machine:    machineId,
			// Life: ..., // TODO: import life, default is Alive
			Params: params,
			Info:   info,
		},
	}
}

func (i *importer) storagePools() error {
	registry, err := i.st.storageProviderRegistry()
	if err != nil {
		return errors.Annotate(err, "getting provider registry")
	}
	pm := poolmanager.New(NewStateSettings(i.st), registry)

	for _, pool := range i.model.StoragePools() {
		_, err := pm.Create(pool.Name(), storage.ProviderType(pool.Provider()), pool.Attributes())
		if err != nil {
			return errors.Annotatef(err, "creating pool %q", pool.Name())
		}
	}
	return nil
}
