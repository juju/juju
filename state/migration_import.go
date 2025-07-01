// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"

	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/tools"
)

// Import the database agnostic model representation into the database.
func (ctrl *Controller) Import(
	model description.Model, controllerConfig controller.Config,
) (_ *Model, _ *State, err error) {
	st, err := ctrl.pool.SystemState()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	modelUUID := model.UUID()
	logger := internallogger.GetLogger("juju.state.import-model")
	logger.Debugf(context.TODO(), "import starting for model %s", modelUUID)

	// At this stage, attempting to import a model with the same
	// UUID as an existing model will error.
	if modelExists, err := st.ModelExists(modelUUID); err != nil {
		return nil, nil, errors.Trace(err)
	} else if modelExists {
		// We have an existing matching model.
		return nil, nil, errors.AlreadyExistsf("model %s", modelUUID)
	}

	modelType, err := ParseModelType(model.Type())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	args := ModelArgs{
		Name:          model.Config()[config.NameKey].(string),
		UUID:          coremodel.UUID(modelUUID),
		Type:          modelType,
		CloudName:     model.Cloud(),
		CloudRegion:   model.CloudRegion(),
		Owner:         names.NewUserTag(model.Owner()),
		MigrationMode: MigrationModeImporting,
		PasswordHash:  model.PasswordHash(),
	}
	if creds := model.CloudCredential(); creds != nil {
		// Need to add credential or make sure an existing credential
		// matches.
		// TODO: there really should be a way to create a cloud credential
		// tag in the names package from the cloud, owner and name.
		credID := fmt.Sprintf("%s/%s/%s", creds.Cloud(), creds.Owner(), creds.Name())
		if !names.IsValidCloudCredential(credID) {
			return nil, nil, errors.NotValidf("cloud credential ID %q", credID)
		}
		credTag := names.NewCloudCredentialTag(credID)
		// We used to load the credential here to check it but
		// that is now done using the new domain/credential importer.
		args.CloudCredential = credTag
	}
	dbModel, newSt, err := ctrl.NewModel(args)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "model created %s/%s", dbModel.Owner().Id(), dbModel.Name())
	defer func() {
		if err != nil {
			newSt.Close()
		}
	}()

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
	if err := restore.sshHostKeys(); err != nil {
		return nil, nil, errors.Annotate(err, "sshHostKeys")
	}
	if err := restore.actions(); err != nil {
		return nil, nil, errors.Annotate(err, "actions")
	}
	if err := restore.operations(); err != nil {
		return nil, nil, errors.Annotate(err, "operations")
	}
	if err := restore.machines(); err != nil {
		return nil, nil, errors.Annotate(err, "machines")
	}
	if err := restore.storage(); err != nil {
		return nil, nil, errors.Annotate(err, "storage")
	}

	// NOTE: at the end of the import make sure that the mode of the model
	// is set to "imported" not "active" (or whatever we call it). This way
	// we don't start model workers for it before the migration process
	// is complete.

	logger.Debugf(context.TODO(), "import success")
	return dbModel, newSt, nil
}

// ImportStateMigration defines a migration for importing various entities from
// a source description model to the destination state.
// It accumulates a series of migrations to Run at a later time.
// Running the state migration visits all the migrations and exits upon seeing
// the first error from the migration.
type ImportStateMigration struct {
	migrations []func() error
}

// Add adds a migration to execute at a later time
// Return error from the addition will cause the Run to terminate early.
func (m *ImportStateMigration) Add(f func() error) {
	m.migrations = append(m.migrations, f)
}

// Run executes all the migrations required to be run.
func (m *ImportStateMigration) Run() error {
	for _, f := range m.migrations {
		if err := f(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type importer struct {
	st      *State
	dbModel *Model
	model   description.Model
	logger  corelogger.Logger
}

func (i *importer) modelExtras() error {
	if latest := i.model.LatestToolsVersion(); latest != "" {
		if err := i.dbModel.UpdateLatestToolsVersion(latest); err != nil {
			return errors.Trace(err)
		}
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

func (i *importer) machines() error {
	i.logger.Debugf(context.TODO(), "importing machines")
	for _, m := range i.model.Machines() {
		if err := i.machine(m, ""); err != nil {
			i.logger.Errorf(context.TODO(), "error importing machine: %s", err)
			return errors.Annotate(err, m.Id())
		}
	}

	i.logger.Debugf(context.TODO(), "importing machines succeeded")
	return nil
}

func (i *importer) machine(m description.Machine, arch string) error {
	// Import this machine, then import its containers.
	i.logger.Debugf(context.TODO(), "importing machine %s", m.Id())

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
	// importing without a modification-status shouldn't cause a panic, so we
	// should check if it's nil or not.
	var modificationStatusDoc statusDoc
	if modStatus := instance.ModificationStatus(); modStatus != nil {
		modificationStatusDoc = statusDoc{
			ModelUUID:  i.st.ModelUUID(),
			Status:     status.Status(modStatus.Value()),
			StatusInfo: modStatus.Message(),
			StatusData: modStatus.Data(),
			Updated:    modStatus.Updated().UnixNano(),
		}
	}
	cons := i.constraints(m.Constraints())
	prereqOps, machineOp := i.st.baseNewMachineOps(
		mdoc,
		machineStatusDoc,
		instanceStatusDoc,
		modificationStatusDoc,
		cons,
	)

	if parentId := container.ParentId(mdoc.Id); parentId != "" {
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

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	// Now that this machine exists in the database, process each of the
	// containers in this machine.
	for _, container := range m.Containers() {
		// Pass the parent machine's architecture when creating an op to fix
		// a container's data.
		if err := i.machine(container, m.Instance().Architecture()); err != nil {
			return errors.Annotate(err, container.Id())
		}
	}
	return nil
}

func (i *importer) makeMachineDoc(m description.Machine) (*machineDoc, error) {
	id := m.Id()
	supported, supportedSet := m.SupportedContainers()
	supportedContainers := make([]instance.ContainerType, len(supported))
	for j, c := range supported {
		supportedContainers[j] = instance.ContainerType(c)
	}

	agentTools, err := i.makeTools(m.Tools())
	if err != nil {
		return nil, errors.Trace(err)
	}

	base, err := corebase.ParseBaseFromString(m.Base())
	if err != nil {
		return nil, errors.Trace(err)
	}
	macBase := Base{OS: base.OS, Channel: base.Channel.String()}
	return &machineDoc{
		DocID:                    i.st.docID(id),
		Id:                       id,
		ModelUUID:                i.st.ModelUUID(),
		Base:                     macBase.Normalise(),
		ContainerType:            m.ContainerType(),
		Principals:               nil, // Set during unit import.
		Life:                     Alive,
		Tools:                    agentTools,
		PasswordHash:             m.PasswordHash(),
		Clean:                    !i.machineHasUnits(m.Id()),
		Volumes:                  i.machineVolumes(m.Id()),
		Filesystems:              i.machineFilesystems(m.Id()),
		Addresses:                i.makeAddresses(m.ProviderAddresses()),
		MachineAddresses:         i.makeAddresses(m.MachineAddresses()),
		PreferredPrivateAddress:  i.makeAddress(m.PreferredPrivateAddress()),
		PreferredPublicAddress:   i.makeAddress(m.PreferredPublicAddress()),
		SupportedContainersKnown: supportedSet,
		SupportedContainers:      supportedContainers,
		Placement:                m.Placement(),
	}, nil
}

func (i *importer) machineHasUnits(machineId string) bool {
	for _, app := range i.model.Applications() {
		for _, unit := range app.Units() {
			if unit.Machine() == machineId {
				return true
			}
		}
	}
	return false
}

func (i *importer) machineVolumes(machineId string) []string {
	var result []string
	for _, volume := range i.model.Volumes() {
		for _, attachment := range volume.Attachments() {
			hostMachine, ok := attachment.HostMachine()
			if ok && hostMachine == machineId {
				result = append(result, volume.ID())
			}
		}
	}
	return result
}

func (i *importer) machineFilesystems(machineId string) []string {
	var result []string
	for _, filesystem := range i.model.Filesystems() {
		for _, attachment := range filesystem.Attachments() {
			hostMachine, ok := attachment.HostMachine()
			if ok && hostMachine == machineId {
				result = append(result, filesystem.ID())
			}
		}
	}
	return result
}

func (i *importer) makeTools(t description.AgentTools) (*tools.Tools, error) {
	if t == nil {
		return nil, nil
	}
	v, err := semversion.ParseBinary(t.Version())
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &tools.Tools{
		Version: v,
		URL:     t.URL(),
		SHA256:  t.SHA256(),
		Size:    t.Size(),
	}
	return result, nil
}

func (i *importer) makeAddress(addr description.Address) address {
	if addr == nil {
		return address{}
	}

	newAddr := address{
		Value:       addr.Value(),
		AddressType: addr.Type(),
		Scope:       addr.Scope(),
		Origin:      addr.Origin(),
		SpaceID:     addr.SpaceID(),
		// CIDR is not supported in juju/description@v5,
		// but it has been added in DB to fix the bug https://bugs.launchpad.net/juju/+bug/2073986
		// In this use case, CIDR are always fetched from machine before using them anyway, so not migrating them
		// is not harmful.
		// CIDR:    addr.CIDR(),
	}

	// Addresses are placed in the default space if no space ID is set.
	if newAddr.SpaceID == "" {
		newAddr.SpaceID = "0"
	}

	return newAddr
}

func (i *importer) makeAddresses(addrs []description.Address) []address {
	result := make([]address, len(addrs))
	for j, addr := range addrs {
		result[j] = i.makeAddress(addr)
	}
	return result
}

// makeStatusDoc assumes status is non-nil.
func (i *importer) makeStatusDoc(statusVal description.Status) statusDoc {
	doc := statusDoc{
		Status:     status.Status(statusVal.Value()),
		StatusInfo: statusVal.Message(),
		StatusData: statusVal.Data(),
		Updated:    statusVal.Updated().UnixNano(),
	}
	// Older versions of Juju would pass through NeverSet() on the status
	// description for application statuses that hadn't been explicitly
	// set by the lead unit. If that is the case, we make the status what
	// the new code expects.
	if statusVal.NeverSet() {
		doc.Status = status.Unset
		doc.StatusInfo = ""
		doc.StatusData = nil
	}
	return doc
}

func (i *importer) sshHostKeys() error {
	i.logger.Debugf(context.TODO(), "importing ssh host keys")
	for _, key := range i.model.SSHHostKeys() {
		name := names.NewMachineTag(key.MachineID())
		err := i.st.SetSSHHostKeys(name, key.Keys())
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing ssh host keys %v: %s", key, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing ssh host keys succeeded")
	return nil
}

func (i *importer) actions() error {
	i.logger.Debugf(context.TODO(), "importing actions")
	for _, action := range i.model.Actions() {
		err := i.addAction(action)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing action %v: %s", action, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing actions succeeded")
	return nil
}

func (i *importer) addAction(action description.Action) error {
	modelUUID := i.st.ModelUUID()
	newDoc := &actionDoc{
		DocId:          i.st.docID(action.Id()),
		ModelUUID:      modelUUID,
		Receiver:       action.Receiver(),
		Name:           action.Name(),
		Operation:      action.Operation(),
		Parameters:     action.Parameters(),
		Enqueued:       action.Enqueued(),
		Results:        action.Results(),
		Message:        action.Message(),
		Started:        action.Started(),
		Completed:      action.Completed(),
		Status:         ActionStatus(action.Status()),
		Parallel:       action.Parallel(),
		ExecutionGroup: action.ExecutionGroup(),
	}

	ops := []txn.Op{{
		C:      actionsC,
		Id:     newDoc.DocId,
		Insert: newDoc,
	}}

	if activeStatus.Contains(string(newDoc.Status)) {
		prefix := ensureActionMarker(action.Receiver())
		notificationDoc := &actionNotificationDoc{
			DocId:     i.st.docID(prefix + action.Id()),
			ModelUUID: modelUUID,
			Receiver:  action.Receiver(),
			ActionID:  action.Id(),
		}
		ops = append(ops, txn.Op{
			C:      actionNotificationsC,
			Id:     notificationDoc.DocId,
			Insert: notificationDoc,
		})
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// operations takes the imported operations data and writes it to
// the new model.
func (i *importer) operations() error {
	i.logger.Debugf(context.TODO(), "importing operations")
	for _, op := range i.model.Operations() {
		err := i.addOperation(op)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing operation %v: %s", op, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing operations succeeded")
	return nil
}

func (i *importer) addOperation(op description.Operation) error {
	modelUUID := i.st.ModelUUID()
	newDoc := &operationDoc{
		DocId:             i.st.docID(op.Id()),
		ModelUUID:         modelUUID,
		Summary:           op.Summary(),
		Fail:              op.Fail(),
		Enqueued:          op.Enqueued(),
		Started:           op.Started(),
		Completed:         op.Completed(),
		Status:            ActionStatus(op.Status()),
		CompleteTaskCount: op.CompleteTaskCount(),
		SpawnedTaskCount:  i.countActionTasksForOperation(op),
	}
	ops := []txn.Op{{
		C:      operationsC,
		Id:     newDoc.DocId,
		Insert: newDoc,
	}}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) countActionTasksForOperation(op description.Operation) int {
	if op.SpawnedTaskCount() > 0 {
		return op.SpawnedTaskCount()
	}
	opID := op.Id()
	var count int
	for _, action := range i.model.Actions() {
		if action.Operation() == opID {
			count += 1
		}
	}
	return count
}

func (i *importer) constraints(cons description.Constraints) constraints.Value {
	var result constraints.Value
	if cons == nil {
		return result
	}

	if allocate := cons.AllocatePublicIP(); allocate {
		result.AllocatePublicIP = &allocate
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
	if source := cons.RootDiskSource(); source != "" {
		result.RootDiskSource = &source
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
	if zones := cons.Zones(); len(zones) > 0 {
		result.Zones = &zones
	}
	return result
}

func (i *importer) storage() error {
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
	i.logger.Debugf(context.TODO(), "importing storage instances")
	for _, storage := range i.model.Storages() {
		err := i.addStorageInstance(storage)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing storage %s: %s", storage.ID(), err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing storage instances succeeded")
	return nil
}

func (i *importer) addStorageInstance(storage description.Storage) error {
	kind := parseStorageKind(storage.Kind())
	if kind == StorageKindUnknown {
		return errors.Errorf("storage kind %q is unknown", storage.Kind())
	}
	unitOwner, ok := storage.UnitOwner()
	var owner names.Tag
	if ok {
		owner = names.NewUnitTag(unitOwner)
	}
	var storageOwner string
	if owner != nil {
		storageOwner = owner.String()
	}
	attachments := storage.Attachments()
	var ops []txn.Op
	for _, unit := range attachments {
		ops = append(ops, createStorageAttachmentOp(names.NewStorageTag(storage.ID()), names.NewUnitTag(unit)))
	}
	doc := &storageInstanceDoc{
		Id:              storage.ID(),
		Kind:            kind,
		Owner:           storageOwner,
		StorageName:     storage.Name(),
		AttachmentCount: len(attachments),
		Constraints:     i.storageInstanceConstraints(storage),
	}
	ops = append(ops, txn.Op{
		C:      storageInstancesC,
		Id:     storage.ID(),
		Assert: txn.DocMissing,
		Insert: doc,
	})

	if owner != nil {
		refcounts, closer := i.st.db().GetCollection(refcountsC)
		defer closer()
		storageRefcountKey := entityStorageRefcountKey(owner, storage.Name())
		incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, storageRefcountKey, 1)
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, incRefOp)
	}

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
			if volume.Storage() == storage.ID() {
				cons.Pool = volume.Pool()
				cons.Size = volume.Size()
				break
			}
		}
	case StorageKindFilesystem:
		defaultPool = string(provider.RootfsProviderType)
		for _, filesystem := range i.model.Filesystems() {
			if filesystem.Storage() == storage.ID() {
				cons.Pool = filesystem.Pool()
				cons.Size = filesystem.Size()
				break
			}
		}
	}
	if cons.Pool == "" {
		cons.Pool = defaultPool
		cons.Size = 1024
		if owner, ok := storage.UnitOwner(); ok {
			appName, _ := names.UnitApplication(owner)
			for _, app := range i.model.Applications() {
				if app.Name() != appName {
					continue
				}
				storageName, _ := names.StorageName(storage.ID())
				appStorageCons, ok := app.StorageDirectives()[storageName]
				if ok {
					cons.Pool = appStorageCons.Pool()
					cons.Size = appStorageCons.Size()
				}
				break
			}
		}
		logger.Warningf(context.TODO(),
			"no volume or filesystem found, using application storage constraints for %s",
			names.ReadableString(names.NewStorageTag(storage.ID())),
		)
	}
	return cons
}

func (i *importer) volumes() error {
	i.logger.Debugf(context.TODO(), "importing volumes")
	sb, err := NewStorageBackend(i.st)
	if err != nil {
		return errors.Trace(err)
	}
	for _, volume := range i.model.Volumes() {
		err := i.addVolume(volume, sb)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing volume %s: %s", volume.ID(), err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing volumes succeeded")
	return nil
}

func (i *importer) addVolume(volume description.Volume, sb *storageBackend) error {
	attachments := volume.Attachments()
	attachmentPlans := volume.AttachmentPlans()

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
		Name:      volume.ID(),
		StorageId: volume.Storage(),
		// Life: ..., // TODO: import life, default is Alive
		Params:          params,
		Info:            info,
		AttachmentCount: len(attachments),
	}
	if detachable, err := isDetachableVolumePool(sb, volume.Pool()); err != nil {
		return errors.Trace(err)
	} else if !detachable && len(attachments) == 1 {
		host, ok := attachments[0].HostUnit()
		if !ok {
			host, _ = attachments[0].HostMachine()
		}
		doc.HostId = host
	}
	status := i.makeStatusDoc(volume.Status())
	ops := sb.newVolumeOps(doc, status)

	for _, attachment := range attachments {
		ops = append(ops, i.addVolumeAttachmentOp(volume.ID(), attachment, attachment.VolumePlanInfo()))
	}

	if len(attachmentPlans) > 0 {
		for _, val := range attachmentPlans {
			ops = append(ops, i.addVolumeAttachmentPlanOp(volume.ID(), val))
		}
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (i *importer) addVolumeAttachmentPlanOp(volID string, volumePlan description.VolumeAttachmentPlan) txn.Op {
	descriptionPlanInfo := volumePlan.VolumePlanInfo()
	planInfo := &VolumeAttachmentPlanInfo{
		DeviceType:       storage.DeviceType(descriptionPlanInfo.DeviceType()),
		DeviceAttributes: descriptionPlanInfo.DeviceAttributes(),
	}

	descriptionBlockInfo := volumePlan.BlockDevice()
	blockInfo := &BlockDeviceInfo{
		DeviceName:     descriptionBlockInfo.Name(),
		DeviceLinks:    descriptionBlockInfo.Links(),
		Label:          descriptionBlockInfo.Label(),
		UUID:           descriptionBlockInfo.UUID(),
		HardwareId:     descriptionBlockInfo.HardwareID(),
		WWN:            descriptionBlockInfo.WWN(),
		BusAddress:     descriptionBlockInfo.BusAddress(),
		Size:           descriptionBlockInfo.Size(),
		FilesystemType: descriptionBlockInfo.FilesystemType(),
		InUse:          descriptionBlockInfo.InUse(),
		MountPoint:     descriptionBlockInfo.MountPoint(),
	}

	machineId := volumePlan.Machine()
	return txn.Op{
		C:      volumeAttachmentPlanC,
		Id:     volumeAttachmentId(machineId, volID),
		Assert: txn.DocMissing,
		Insert: &volumeAttachmentPlanDoc{
			Volume:      volID,
			Machine:     machineId,
			PlanInfo:    planInfo,
			BlockDevice: blockInfo,
		},
	}
}

func (i *importer) addVolumeAttachmentOp(volID string, attachment description.VolumeAttachment, planInfo description.VolumePlanInfo) txn.Op {
	var info *VolumeAttachmentInfo
	var params *VolumeAttachmentParams

	planInf := &VolumeAttachmentPlanInfo{}

	deviceType := planInfo.DeviceType()
	deviceAttrs := planInfo.DeviceAttributes()
	if deviceType != "" || deviceAttrs != nil {
		if deviceType != "" {
			planInf.DeviceType = storage.DeviceType(deviceType)
		}
		if deviceAttrs != nil {
			planInf.DeviceAttributes = deviceAttrs
		}
	} else {
		planInf = nil
	}

	if attachment.Provisioned() {
		info = &VolumeAttachmentInfo{
			DeviceName: attachment.DeviceName(),
			DeviceLink: attachment.DeviceLink(),
			BusAddress: attachment.BusAddress(),
			ReadOnly:   attachment.ReadOnly(),
			PlanInfo:   planInf,
		}
	} else {
		params = &VolumeAttachmentParams{
			ReadOnly: attachment.ReadOnly(),
		}
	}

	host, ok := attachment.HostUnit()
	if !ok {
		host, _ = attachment.HostMachine()
	}
	return txn.Op{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(host, volID),
		Assert: txn.DocMissing,
		Insert: &volumeAttachmentDoc{
			Volume: volID,
			Host:   host,
			Params: params,
			Info:   info,
		},
	}
}

func (i *importer) filesystems() error {
	i.logger.Debugf(context.TODO(), "importing filesystems")
	sb, err := NewStorageBackend(i.st)
	if err != nil {
		return errors.Trace(err)
	}
	for _, fs := range i.model.Filesystems() {
		err := i.addFilesystem(fs, sb)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing filesystem %s: %s", fs.ID(), err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing filesystems succeeded")
	return nil
}

func (i *importer) addFilesystem(filesystem description.Filesystem, sb *storageBackend) error {

	attachments := filesystem.Attachments()
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
		FilesystemId: filesystem.ID(),
		StorageId:    filesystem.Storage(),
		VolumeId:     filesystem.Volume(),
		// Life: ..., // TODO: import life, default is Alive
		Params:          params,
		Info:            info,
		AttachmentCount: len(attachments),
	}
	if detachable, err := isDetachableFilesystemPool(sb, filesystem.Pool()); err != nil {
		return errors.Trace(err)
	} else if !detachable && len(attachments) == 1 {
		host, ok := attachments[0].HostUnit()
		if !ok {
			host, _ = attachments[0].HostMachine()
		}
		doc.HostId = host
	}
	status := i.makeStatusDoc(filesystem.Status())
	ops := sb.newFilesystemOps(doc, status)

	for _, attachment := range attachments {
		ops = append(ops, i.addFilesystemAttachmentOp(filesystem.ID(), attachment))
	}

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
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

	host, ok := attachment.HostUnit()
	if !ok {
		host, _ = attachment.HostMachine()
	}
	return txn.Op{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(host, fsID),
		Assert: txn.DocMissing,
		Insert: &filesystemAttachmentDoc{
			Filesystem: fsID,
			Host:       host,
			// Life: ..., // TODO: import life, default is Alive
			Params: params,
			Info:   info,
		},
	}
}
