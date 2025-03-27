// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"

	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/relation"
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

	// Create the model.
	cfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	args := ModelArgs{
		Type:           modelType,
		CloudName:      model.Cloud(),
		CloudRegion:    model.CloudRegion(),
		Config:         cfg,
		Owner:          names.NewUserTag(model.Owner()),
		MigrationMode:  MigrationModeImporting,
		EnvironVersion: model.EnvironVersion(),
		PasswordHash:   model.PasswordHash(),
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
	if err := restore.applications(controllerConfig); err != nil {
		return nil, nil, errors.Annotate(err, "applications")
	}
	if err := restore.relations(); err != nil {
		return nil, nil, errors.Annotate(err, "relations")
	}
	if err := restore.linklayerdevices(); err != nil {
		return nil, nil, errors.Annotate(err, "linklayerdevices")
	}
	if err := restore.ipAddresses(); err != nil {
		return nil, nil, errors.Annotate(err, "ipAddresses")
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
	// applicationUnits is populated at the end of loading the applications, and is a
	// map of application name to the units of that application.
	applicationUnits map[string]map[string]*Unit
	charmOrigins     map[string]*CharmOrigin
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
	jobs, err := i.makeMachineJobs(m.Jobs())
	if err != nil {
		return nil, errors.Trace(err)
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
		Nonce:                    m.Nonce(),
		Base:                     macBase.Normalise(),
		ContainerType:            m.ContainerType(),
		Principals:               nil, // Set during unit import.
		Life:                     Alive,
		Tools:                    agentTools,
		Jobs:                     jobs,
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

func (i *importer) applications(controllerConfig controller.Config) error {
	i.logger.Debugf(context.TODO(), "importing applications")

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

	i.charmOrigins = make(map[string]*CharmOrigin, len(principals)+len(subordinates))

	for _, s := range append(principals, subordinates...) {
		if err := i.application(s, controllerConfig); err != nil {
			i.logger.Errorf(context.TODO(), "error importing application %s: %s", s.Name(), err)
			return errors.Annotate(err, s.Name())
		}
	}

	if err := i.loadUnits(); err != nil {
		return errors.Annotate(err, "loading new units from db")
	}
	i.logger.Debugf(context.TODO(), "importing applications succeeded")
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

	result := make(map[string]map[string]*Unit)
	for _, doc := range docs {
		units, found := result[doc.Application]
		if !found {
			units = make(map[string]*Unit)
			result[doc.Application] = units
		}
		units[doc.Name] = newUnit(i.st, i.dbModel.Type(), &doc)
	}
	i.applicationUnits = result
	return nil

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

func (i *importer) application(a description.Application, ctrlCfg controller.Config) error {
	// Import this application, then its units.
	i.logger.Debugf(context.TODO(), "importing application %s", a.Name())

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
	appStatusDoc := i.makeStatusDoc(status)

	// When creating the settings, we ignore nils.  In other circumstances, nil
	// means to delete the value (reset to default), so creating with nil should
	// mean to use the default, i.e. don't set the value.
	// There may have existed some applications with settings that contained
	// nil values, see lp#1667199. When importing, we want these stripped.
	removeNils(a.CharmConfig())
	removeNils(a.ApplicationConfig())

	var operatorStatusDoc *statusDoc
	if i.dbModel.Type() == ModelTypeCAAS {
		operatorStatus := i.makeStatusDoc(a.OperatorStatus())
		operatorStatusDoc = &operatorStatus
	}
	ops, err := addApplicationOps(i.st, app, addApplicationOpsArgs{
		applicationDoc:    appDoc,
		statusDoc:         appStatusDoc,
		constraints:       i.constraints(a.Constraints()),
		storage:           i.storageConstraints(a.StorageDirectives()),
		charmConfig:       a.CharmConfig(),
		applicationConfig: a.ApplicationConfig(),
		operatorStatus:    operatorStatusDoc,
	})
	if err != nil {
		return errors.Trace(err)
	}

	bindings, err := i.parseBindings(a.EndpointBindings())
	if err != nil {
		return errors.Trace(err)
	}

	ops = append(ops, txn.Op{
		C:      endpointBindingsC,
		Id:     app.globalKey(),
		Assert: txn.DocMissing,
		Insert: endpointBindingsDoc{
			Bindings: bindings.Map(),
		},
	})

	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	if cs := a.CloudService(); cs != nil {
		app, err := i.st.Application(a.Name())
		if err != nil {
			return errors.Trace(err)
		}
		addr := i.makeAddresses(cs.Addresses())
		if err := app.UpdateCloudService(cs.ProviderId(), networkAddresses(addr)); err != nil {
			return errors.Trace(err)
		}
	}

	for _, unit := range a.Units() {
		if err := i.unit(a, unit, ctrlCfg); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// parseBindings converts a bindings map from a 2.6.x or 2.7+ migration export
// into a Bindings object.
//
// When migrating from a 2.6.x controller, the bindings in the description
// output are encoded as {endpoint name => space name} with "" representing
// the "default" (now called alpha) space. The empty spaces must be remapped
// to the correct default space name for the new controller.
//
// On the other hand, migration exports from 2.7+ are using space IDs instead
// of space names as the map values and can safely be passed to the NewBindings
// c-tor.
func (i *importer) parseBindings(bindingsMap map[string]string) (*Bindings, error) {
	defaultMappingsAreIds := true
	for epName, spNameOrID := range bindingsMap {
		if spNameOrID == "" {
			defaultMappingsAreIds = false
			bindingsMap[epName] = network.AlphaSpaceName
		}
	}

	// 2.6 controllers only populate the default space key if set to the
	// non-default space whereas 2.7 controllers always set it.
	// The application implementation in the description package has
	// `omitempty` for bindings, so we need to create it if nil.
	// There's an added complication. Coming from a 2.6 controller, the
	// mapping is endpoint -> spaceName. If the 2.6 controller has been
	// upgraded to 2.7.x (x < 7), the mapping is endpoint -> spaceID.
	// We need to ensure that a consistent mapping value is used.
	if bindingsMap == nil {
		bindingsMap = make(map[string]string, 1)
	}
	if _, exists := bindingsMap[defaultEndpointName]; !exists {
		if defaultMappingsAreIds {
			bindingsMap[defaultEndpointName] = network.AlphaSpaceId
		} else {
			bindingsMap[defaultEndpointName] = network.AlphaSpaceName
		}
	}

	return NewBindings(i.st, bindingsMap)
}

func (i *importer) storageConstraints(cons map[string]description.StorageDirective) map[string]StorageConstraints {
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

func (i *importer) unit(s description.Application, u description.Unit, ctrlCfg controller.Config) error {
	i.logger.Debugf(context.TODO(), "importing unit %s", u.Name())

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

	var cloudContainer *cloudContainerDoc
	if cc := u.CloudContainer(); cc != nil {
		cloudContainer = &cloudContainerDoc{
			Id:         unitGlobalKey(u.Name()),
			ProviderId: cc.ProviderId(),
			Ports:      cc.Ports(),
		}
		if cc.Address() != nil {
			addr := i.makeAddress(cc.Address())
			cloudContainer.Address = &addr
		}
	}

	ops, err := addUnitOps(i.st, addUnitOpsArgs{
		unitDoc:            udoc,
		agentStatusDoc:     agentStatusDoc,
		workloadStatusDoc:  &workloadStatusDoc,
		workloadVersionDoc: &workloadVersionDoc,
		containerDoc:       cloudContainer,
	})
	if err != nil {
		return errors.Trace(err)
	}

	if i.dbModel.Type() == ModelTypeIAAS && u.Principal() == "" {
		// If the unit is a principal, add it to its machine.
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     u.Machine(),
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
		i.logger.Debugf(context.TODO(), "failed ops: %#v", ops)
		return errors.Trace(err)
	}

	model, err := i.st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	// The assertion logic in unit.SetState assumes that the DocID is
	// present.  Since the txn for creating the unit doc has completed
	// without an error, we can safely populate the doc's model UUID and
	// DocID.
	udoc.ModelUUID = model.UUID()
	udoc.DocID = ensureModelUUID(udoc.ModelUUID, udoc.Name)

	unit := newUnit(i.st, model.Type(), udoc)
	if err := i.importUnitState(unit, u, ctrlCfg); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (i *importer) importUnitState(unit *Unit, u description.Unit, ctrlCfg controller.Config) error {
	us := NewUnitState()

	if charmState := u.CharmState(); len(charmState) != 0 {
		us.SetCharmState(charmState)
	}
	if relationState := u.RelationState(); len(relationState) != 0 {
		us.SetRelationState(relationState)
	}
	if uniterState := u.UniterState(); uniterState != "" {
		us.SetUniterState(uniterState)
	}
	if storageState := u.StorageState(); storageState != "" {
		us.SetStorageState(storageState)
	}

	// No state to persist.
	if !us.Modified() {
		return nil
	}

	return unit.SetState(us, UnitStateSizeLimits{
		MaxCharmStateSize: ctrlCfg.MaxCharmStateSize(),
		MaxAgentStateSize: ctrlCfg.MaxAgentStateSize(),
	})
}

func (i *importer) makeApplicationDoc(a description.Application) (*applicationDoc, error) {
	units := a.Units()

	origin, err := i.makeCharmOrigin(a)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var exposedEndpoints map[string]ExposedEndpoint
	if expEps := a.ExposedEndpoints(); len(expEps) > 0 {
		exposedEndpoints = make(map[string]ExposedEndpoint, len(expEps))
		for epName, details := range expEps {
			exposedEndpoints[epName] = ExposedEndpoint{
				ExposeToSpaceIDs: details.ExposeToSpaceIDs(),
				ExposeToCIDRs:    details.ExposeToCIDRs(),
			}
		}
	}

	agentTools, err := i.makeTools(a.Tools())
	if err != nil {
		return nil, errors.Trace(err)
	}
	cURLStr := a.CharmURL()

	appDoc := &applicationDoc{
		Name:                 a.Name(),
		Subordinate:          a.Subordinate(),
		CharmURL:             &cURLStr,
		CharmModifiedVersion: a.CharmModifiedVersion(),
		CharmOrigin:          *origin,
		ForceCharm:           a.ForceCharm(),
		Life:                 Alive,
		UnitCount:            len(units),
		RelationCount:        i.relationCount(a.Name()),
		Exposed:              a.Exposed(),
		ExposedEndpoints:     exposedEndpoints,
		Tools:                agentTools,
		Placement:            a.Placement(),
		HasResources:         a.HasResources(),
	}

	return appDoc, nil
}

// makeCharmOrigin returns the charm origin for an application
//
// Previous versions of the Juju server and clients have treated applications charm
// origins very loosely, particularly during `refresh --switch`s. The server performed
// no validation on origins received from the client, and client often mutated them
// incorrectly. For instance, when switching from a ch charm to local, pylibjuju simply
// sent back a copy of the ch charm origin, whereas the CLI only set the source to local.
// Both resulted in incorrect/invalidate origins.
//
// Calculate the origin Source and Revision from the charm url. Ensure ID, Hash and Channel
// are dropped from local charm. Keep ID, Hash and Channel (for ch charms) and Platform (always)
// we get from the origin. We can trust these since supported clients cannot break these
//
// This was fixed in pylibjuju 3.2.3.0 and juju 3.3.0. As of writing, no versions of the
// server validate new charm origins on calls to SetCharm. Ideally, the client shouldn't
// handle charm origins at all, being an implementation detail. But this will probably have
// to wait until the api re-write
//
// https://bugs.launchpad.net/juju/+bug/2039267
// https://github.com/juju/python-libjuju/issues/962
//
// Due to LP:1986547: where the track is missing from the effective channel it implicitly
// resolves to 'latest' if the charm does not have a default channel defined. So if the
// received channel has no track, we can be confident it should be 'latest'
//
// TODO: Once we have confidence in charm origins, do not parse charm url and simplify
// into a translation layer
func (i *importer) makeCharmOrigin(a description.Application) (*CharmOrigin, error) {
	sourceOrigin := a.CharmOrigin()
	curl, err := charm.ParseURL(a.CharmURL())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Fix bad datasets from LP 1999060 during migration.
	// ID and Hash missing from N-1 of N applications'
	// charm origins when deployed using the same charm.
	if foundOrigin, ok := i.charmOrigins[curl.String()]; ok {
		return foundOrigin, nil
	}

	var channel *Channel
	serialized := sourceOrigin.Channel()
	if serialized != "" && charm.CharmHub.Matches(curl.Schema) {
		c, err := charm.ParseChannelNormalize(serialized)
		if err != nil {
			return nil, errors.Trace(err)
		}
		track := c.Track
		if track == "" {
			track = "latest"
		}
		channel = &Channel{
			Track:  track,
			Risk:   string(c.Risk),
			Branch: c.Branch,
		}
	}

	p, err := corecharm.ParsePlatformNormalize(sourceOrigin.Platform())
	if err != nil {
		return nil, errors.Trace(err)
	}
	platform := &Platform{
		Architecture: p.Architecture,
		OS:           p.OS,
		Channel:      p.Channel,
	}

	// We can hardcode type to charm as we never store bundles in state.
	var origin *CharmOrigin
	if charm.Local.Matches(curl.Schema) {
		origin = &CharmOrigin{
			Source:   corecharm.Local.String(),
			Type:     "charm",
			Revision: &curl.Revision,
			Platform: platform,
		}
	} else if charm.CharmHub.Matches(curl.Schema) {
		origin = &CharmOrigin{
			Source:   corecharm.CharmHub.String(),
			Type:     "charm",
			Revision: &curl.Revision,
			ID:       sourceOrigin.ID(),
			Hash:     sourceOrigin.Hash(),
			Channel:  channel,
			Platform: platform,
		}
	} else {
		return nil, errors.Errorf("Unrecognised charm url schema %q", curl.Schema)
	}

	if !reflect.DeepEqual(sourceOrigin, origin) {
		i.logger.Warningf(context.TODO(), "Source origin for application %q is invalid. Normalising", a.Name())
	}

	i.charmOrigins[curl.String()] = origin
	return origin, nil
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

func (i *importer) getPrincipalMachineID(principal string) string {
	// We know this is a valid unit name, so we don't care about the error.
	appName, _ := names.UnitApplication(principal)
	for _, app := range i.model.Applications() {
		if app.Name() == appName {
			for _, unit := range app.Units() {
				if unit.Name() == principal {
					return unit.Machine()
				}
			}
		}
	}
	// We should never get here, but if we do, just return an empty
	// machine ID.
	i.logger.Warningf(context.TODO(), "unable to find principal %q", principal)
	return ""
}

func (i *importer) makeUnitDoc(s description.Application, u description.Unit) (*unitDoc, error) {
	// NOTE: if we want to support units having different charms deployed
	// than the application recommends and migrate that, then we should serialize
	// the charm url for each unit rather than grabbing the applications charm url.
	// Currently the units charm url matching the application is a precondiation
	// to migration.
	charmURL := s.CharmURL()

	var subordinates []string
	if subs := u.Subordinates(); len(subs) > 0 {
		for _, s := range subs {
			subordinates = append(subordinates, s)
		}
	}

	machineID := u.Machine()
	if s.Subordinate() && machineID == "" && i.dbModel.Type() != ModelTypeCAAS {
		// If we don't have a machine ID and we should, go get the
		// machine ID from the principal.
		machineID = i.getPrincipalMachineID(u.Principal())
	}

	agentTools, err := i.makeTools(u.Tools())
	if err != nil {
		return nil, errors.Trace(err)
	}

	p, err := corecharm.ParsePlatformNormalize(s.CharmOrigin().Platform())
	if err != nil {
		return nil, errors.Trace(err)
	}
	base := Base{OS: p.OS, Channel: p.Channel}.Normalise()
	return &unitDoc{
		Name:                   u.Name(),
		Application:            s.Name(),
		Base:                   base,
		CharmURL:               &charmURL,
		Principal:              u.Principal(),
		Subordinates:           subordinates,
		StorageAttachmentCount: i.unitStorageAttachmentCount(u.Name()),
		MachineId:              machineID,
		Tools:                  agentTools,
		Life:                   Alive,
		PasswordHash:           u.PasswordHash(),
	}, nil
}

func (i *importer) unitStorageAttachmentCount(unitName string) int {
	count := 0
	for _, storage := range i.model.Storages() {
		for _, host := range storage.Attachments() {
			if host == unitName {
				count++
			}
		}
	}
	return count
}

func (i *importer) relations() error {
	i.logger.Debugf(context.TODO(), "importing relations")
	for _, r := range i.model.Relations() {
		if err := i.relation(r); err != nil {
			i.logger.Errorf(context.TODO(), "error importing relation %s: %s", r.Key(), err)
			return errors.Annotate(err, r.Key())
		}
	}

	i.logger.Debugf(context.TODO(), "importing relations succeeded")
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

	var relStatusDoc statusDoc
	relStatus := rel.Status()
	if relStatus != nil {
		relStatusDoc = i.makeStatusDoc(relStatus)
	} else {
		// Relations are marked as either
		// joining or joined, depending on
		// whether there are any units in scope.
		relStatusDoc = statusDoc{
			Status:  status.Joining,
			Updated: time.Now().UnixNano(),
		}
		if relationDoc.UnitCount > 0 {
			relStatusDoc.Status = status.Joined
		}
	}
	ops = append(ops, createStatusOp(i.st, relationGlobalScope(rel.Id()), relStatusDoc))

	dbRelation := newRelation(i.st, relationDoc)
	// Add an op that adds the relation scope document for each
	// unit of the application, and an op that adds the relation settings
	// for each unit.
	for _, endpoint := range rel.Endpoints() {
		appKey := relationApplicationSettingsKey(dbRelation.Id(), endpoint.ApplicationName())
		appSettings := endpoint.ApplicationSettings()
		ops = append(ops, createSettingsOp(settingsC, appKey, appSettings))

		units := i.applicationUnits[endpoint.ApplicationName()]
		for unitName, settings := range endpoint.AllSettings() {
			var ru *RelationUnit
			var err error

			if unit, ok := units[unitName]; ok {
				ru, err = dbRelation.Unit(unit)
				if err != nil {
					return errors.Trace(err)
				}
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
				createSettingsOp(settingsC, ruKey, settings),
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
		Endpoints: make([]relation.Endpoint, len(endpoints)),
		Life:      Alive,
	}
	for i, ep := range endpoints {
		doc.Endpoints[i] = relation.Endpoint{
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

func (i *importer) linklayerdevices() error {
	i.logger.Debugf(context.TODO(), "importing linklayerdevices")
	for _, device := range i.model.LinkLayerDevices() {
		err := i.addLinkLayerDevice(device)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing ip device %v: %s", device, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing linklayerdevices succeeded")
	return nil
}

func (i *importer) addLinkLayerDevice(device description.LinkLayerDevice) error {
	providerID := device.ProviderID()
	modelUUID := i.st.ModelUUID()
	localID := linkLayerDeviceGlobalKey(device.MachineID(), device.Name())
	linkLayerDeviceDocID := i.st.docID(localID)
	newDoc := &linkLayerDeviceDoc{
		ModelUUID:       modelUUID,
		DocID:           linkLayerDeviceDocID,
		MachineID:       device.MachineID(),
		ProviderID:      providerID,
		Name:            device.Name(),
		MTU:             device.MTU(),
		Type:            network.LinkLayerDeviceType(device.Type()),
		MACAddress:      device.MACAddress(),
		IsAutoStart:     device.IsAutoStart(),
		IsUp:            device.IsUp(),
		ParentName:      device.ParentName(),
		VirtualPortType: network.VirtualPortType(device.VirtualPortType()),
	}

	ops := []txn.Op{{
		C:      linkLayerDevicesC,
		Id:     newDoc.DocID,
		Insert: newDoc,
	}}
	if providerID != "" {
		id := network.Id(providerID)
		ops = append(ops, i.st.networkEntityGlobalKeyOp("linklayerdevice", id))
	}
	if err := i.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (i *importer) ipAddresses() error {
	i.logger.Debugf(context.TODO(), "importing IP addresses")
	for _, addr := range i.model.IPAddresses() {
		err := i.addIPAddress(addr)
		if err != nil {
			i.logger.Errorf(context.TODO(), "error importing IP address %v: %s", addr, err)
			return errors.Trace(err)
		}
	}
	i.logger.Debugf(context.TODO(), "importing IP addresses succeeded")
	return nil
}

func (i *importer) addIPAddress(addr description.IPAddress) error {
	addressValue := addr.Value()
	subnetCIDR := addr.SubnetCIDR()

	globalKey := ipAddressGlobalKey(addr.MachineID(), addr.DeviceName(), addressValue)
	ipAddressDocID := i.st.docID(globalKey)
	providerID := addr.ProviderID()

	modelUUID := i.st.ModelUUID()

	// Compatibility shim for deployments prior to 2.9.1.
	configType := addr.ConfigMethod()
	if configType == "dynamic" {
		configType = string(network.ConfigDHCP)
	}

	newDoc := &ipAddressDoc{
		DocID:             ipAddressDocID,
		ModelUUID:         modelUUID,
		ProviderID:        providerID,
		DeviceName:        addr.DeviceName(),
		MachineID:         addr.MachineID(),
		SubnetCIDR:        subnetCIDR,
		ConfigMethod:      network.AddressConfigType(configType),
		Value:             addressValue,
		DNSServers:        addr.DNSServers(),
		DNSSearchDomains:  addr.DNSSearchDomains(),
		GatewayAddress:    addr.GatewayAddress(),
		IsDefaultGateway:  addr.IsDefaultGateway(),
		ProviderNetworkID: addr.ProviderNetworkID(),
		ProviderSubnetID:  addr.ProviderSubnetID(),
		Origin:            network.Origin(addr.Origin()),
		IsShadow:          addr.IsShadow(),
		IsSecondary:       addr.IsSecondary(),
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
