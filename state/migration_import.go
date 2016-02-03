// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/migration"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// Import the database agnostic model representation into the database.
func (st *State) Import(model migration.Model) (_ *Environment, _ *State, err error) {
	logger := loggo.GetLogger("juju.state.import-model")
	logger.Debugf("import starting for model %s", model.Tag().Id())
	// At this stage, attempting to import a model with the same
	// UUID as an existing model will error.
	envTag := model.Tag()
	_, err = st.GetEnvironment(envTag)
	if err == nil {
		// We have an existing matching environment (model).
		return nil, nil, errors.AlreadyExistsf("model with UUID %s", envTag.Id())
	} else if !errors.IsNotFound(err) {
		return nil, nil, errors.Trace(err)
	}

	// Create the environment.
	cfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	env, envSt, err := st.NewEnvironment(cfg, model.Owner())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logger.Debugf("model created %s/%s", env.Owner().Canonical(), env.Name())
	defer func() {
		if err != nil {
			envSt.Close()
		}
	}()

	if latest := model.LatestToolsVersion(); latest != version.Zero {
		if err := env.UpdateLatestToolsVersion(latest); err != nil {
			return nil, nil, errors.Trace(err)
		}
	}

	// I would have loved to use import, but that is a reserved word.
	restore := importer{
		st:          envSt,
		environment: env,
		model:       model,
		logger:      logger,
	}
	if err := restore.environmentUsers(); err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := restore.machines(); err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := restore.services(); err != nil {
		logger.Debugf("services error")
		return nil, nil, errors.Trace(err)
	}

	// NOTE: at the end of the import make sure that the mode of the model
	// is set to "imported" not "active" (or whatever we call it). This way
	// we don't start environment workers for it before the migration process
	// is complete.

	// Update the sequences to match that the source.

	logger.Debugf("import success")
	return env, envSt, nil
}

type importer struct {
	st          *State
	environment *Environment
	model       migration.Model
	logger      loggo.Logger
}

func (i *importer) environmentUsers() error {
	i.logger.Debugf("importing users")

	// The user that was auto-added when we created the environment will have
	// the wrong DateCreated, so we remove it, and add in all the users we
	// know about. It is also possible that the owner of the environment no
	// longer has access to the environment due to changes over time.
	if err := i.st.RemoveEnvironmentUser(i.model.Owner()); err != nil {
		return errors.Trace(err)
	}

	users := i.model.Users()
	envuuid := i.environment.UUID()
	var ops []txn.Op
	for _, user := range users {
		ops = append(ops, createEnvUserOp(
			envuuid,
			user.Name(),
			user.CreatedBy(),
			user.DisplayName(),
			user.DateCreated(),
			user.ReadOnly()))
	}
	if err := i.st.runTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	// Now set their last connection times.
	for _, user := range users {
		i.logger.Debugf("user %s", user.Name())
		lastConnection := user.LastConnection()
		if lastConnection.IsZero() {
			continue
		}
		envUser, err := i.st.EnvironmentUser(user.Name())
		if err != nil {
			return errors.Trace(err)
		}
		err = envUser.updateLastConnection(lastConnection)
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

func (i *importer) machine(m migration.Machine) error {
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
	//    - adds requested network doc
	//    - adds machine block devices doc

	// TODO: consider filesystems and volumes

	// TODO: constraints for machines.
	status := m.Status()
	if status == nil {
		return errors.NotValidf("missing status")
	}
	statusDoc := statusDoc{
		EnvUUID:    i.st.EnvironUUID(),
		Status:     Status(status.Value()),
		StatusInfo: status.Message(),
		StatusData: status.Data(),
		Updated:    status.Updated().UnixNano(),
	}
	cons := constraints.Value{}
	networks := []string{}
	prereqOps, machineOp := i.st.baseNewMachineOps(mdoc, statusDoc, cons, networks)

	// 3. create op for adding in instance data
	if instance := m.Instance(); instance != nil {
		prereqOps = append(prereqOps, i.machineInstanceOp(mdoc, instance))
	}

	if parentId := ParentId(mdoc.Id); parentId != "" {
		prereqOps = append(prereqOps,
			// Update containers record for host machine.
			i.st.addChildToContainerRefOp(parentId, mdoc.Id),
		)
	}
	// insertNewContainerRefOp adds an empty doc into the containerRefsC
	// collection for the machine being added.
	prereqOps = append(prereqOps, i.st.insertNewContainerRefOp(mdoc.Id))

	// 4. gather prereqs and machine op, run ops.
	ops := append(prereqOps, machineOp)
	if err := i.st.runTransaction(ops); err != nil {
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

func (i *importer) machineInstanceOp(mdoc *machineDoc, inst migration.CloudInstance) txn.Op {
	doc := &instanceData{
		DocID:      mdoc.DocID,
		MachineId:  mdoc.Id,
		InstanceId: instance.Id(inst.InstanceId()),
		EnvUUID:    mdoc.EnvUUID,
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

func (i *importer) makeMachineDoc(m migration.Machine) (*machineDoc, error) {
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
	return &machineDoc{
		DocID:                    i.st.docID(id),
		Id:                       id,
		EnvUUID:                  i.st.EnvironUUID(),
		Nonce:                    m.Nonce(),
		Series:                   m.Series(),
		ContainerType:            m.ContainerType(),
		Principals:               nil, // TODO
		Life:                     Alive,
		Tools:                    i.makeTools(m.Tools()),
		Jobs:                     jobs,
		NoVote:                   true,  // State servers can't be migrated yet.
		HasVote:                  false, // State servers can't be migrated yet.
		PasswordHash:             m.PasswordHash(),
		Clean:                    true, // check this later
		Addresses:                i.makeAddresses(m.ProviderAddresses()),
		MachineAddresses:         i.makeAddresses(m.MachineAddresses()),
		PreferredPrivateAddress:  i.makeAddress(m.PreferredPrivateAddress()),
		PreferredPublicAddress:   i.makeAddress(m.PreferredPublicAddress()),
		SupportedContainersKnown: supportedSet,
		SupportedContainers:      supportedContainers,
		Placement:                m.Placement(),
	}, nil
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
			result = append(result, JobManageEnviron)
		case "manage-networking":
			result = append(result, JobManageNetworking)
		default:
			return nil, errors.Errorf("unknown machine job: %q", job)
		}
	}
	return result, nil
}

func (i *importer) makeTools(t migration.AgentTools) *tools.Tools {
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

func (i *importer) makeAddress(addr migration.Address) address {
	if addr == nil {
		return address{}
	}
	return address{
		Value:       addr.Value(),
		AddressType: addr.Type(),
		NetworkName: addr.NetworkName(),
		Scope:       addr.Scope(),
		Origin:      addr.Origin(),
	}
}

func (i *importer) makeAddresses(addrs []migration.Address) []address {
	result := make([]address, len(addrs))
	for j, addr := range addrs {
		result[j] = i.makeAddress(addr)
	}
	return result
}

func (i *importer) services() error {
	i.logger.Debugf("importing services")
	for _, s := range i.model.Services() {
		if err := i.service(s); err != nil {
			i.logger.Errorf("error importing service: %s", err)
			return errors.Annotate(err, s.Name())
		}
	}

	i.logger.Debugf("importing services succeeded")
	return nil
}

// makeStatusDoc assumes status is non-nil.
func (i *importer) makeStatusDoc(status migration.Status) statusDoc {
	return statusDoc{
		Status:     Status(status.Value()),
		StatusInfo: status.Message(),
		StatusData: status.Data(),
		Updated:    status.Updated().UnixNano(),
	}
}

func (i *importer) service(s migration.Service) error {
	// Import this service, then soon, its units.
	i.logger.Debugf("importing service %s", s.Name())

	// 1. construct a serviceDoc
	sdoc, err := i.makeServiceDoc(s)
	if err != nil {
		return errors.Trace(err)
	}

	// 2. construct a statusDoc
	status := s.Status()
	if status == nil {
		return errors.NotValidf("missing status")
	}
	statusDoc := i.makeStatusDoc(status)
	// TODO: update never set malarky... maybe...

	ops := addServiceOps(i.st, addServiceOpsArgs{
		serviceDoc: sdoc,
		statusDoc:  statusDoc,
		// constraints:     TODO,
		// networks         TODO,
		// storage          TODO,
		settings:           s.Settings(),
		settingsRefCount:   s.SettingsRefCount(),
		leadershipSettings: s.LeadershipSettings(),
	})

	if err := i.st.runTransaction(ops); err != nil {
		return errors.Trace(err)
	}

	// TODO: units here.

	return nil
}

func (i *importer) makeServiceDoc(s migration.Service) (*serviceDoc, error) {
	charmUrl, err := charm.ParseURL(s.CharmURL())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &serviceDoc{
		Name:        s.Name(),
		Series:      s.Series(),
		Subordinate: s.Subordinate(),
		CharmURL:    charmUrl,
		ForceCharm:  s.ForceCharm(),
		Life:        Alive,
		// UnitCount:    TODO,
		// RelationCount:  TODO,
		Exposed:  s.Exposed(),
		MinUnits: s.MinUnits(),
		// MetricCredentials: TODO,
	}, nil
}
