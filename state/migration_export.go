// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/migration"
)

// Export the current model for the State.
func (st *State) Export() (migration.Model, error) {
	dbModel, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	export := exporter{
		st:      st,
		dbModel: dbModel,
		logger:  loggo.GetLogger("juju.state.export-model"),
	}
	if err := export.readAllStatuses(); err != nil {
		return nil, errors.Annotate(err, "reading statuses")
	}
	if err := export.readAllSettings(); err != nil {
		return nil, errors.Trace(err)
	}

	envConfig, found := export.settings[modelGlobalKey]
	if !found {
		return nil, errors.New("missing environ config")
	}

	args := migration.ModelArgs{
		Owner:              dbModel.Owner(),
		Config:             envConfig.Settings,
		LatestToolsVersion: dbModel.LatestToolsVersion(),
	}
	export.model = migration.NewModel(args)
	if err := export.modelUsers(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.machines(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.services(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.model.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return export.model, nil
}

type exporter struct {
	st       *State
	dbModel  *Model
	model    migration.Model
	logger   loggo.Logger
	settings map[string]settingsDoc
	status   map[string]bson.M
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
		lastConn := lastConnections[strings.ToLower(user.UserName())]
		arg := migration.UserArgs{
			Name:           user.UserTag(),
			DisplayName:    user.DisplayName(),
			CreatedBy:      names.NewUserTag(user.CreatedBy()),
			DateCreated:    user.DateCreated(),
			LastConnection: lastConn,
			ReadOnly:       user.ReadOnly(),
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

	instanceDataCollection, closer := e.st.getCollection(instanceDataC)
	defer closer()

	var instData []instanceData
	instances := make(map[string]instanceData)
	if err := instanceDataCollection.Find(nil).All(&instData); err != nil {
		return errors.Annotate(err, "instance data")
	}
	e.logger.Debugf("found %d instanceData", len(instData))
	for _, data := range instData {
		instances[data.MachineId] = data
	}
	// We are iterating through a flat list of machines, but the migration
	// model stores the nesting. The AllMachines method assures us that the
	// machines are returned in an order so the parent will always before
	// any children.
	machineMap := make(map[string]migration.Machine)

	for _, machine := range machines {
		e.logger.Debugf("export machine %s", machine.Id())

		var exParent migration.Machine
		if parentId := ParentId(machine.Id()); parentId != "" {
			var found bool
			exParent, found = machineMap[parentId]
			if !found {
				return errors.Errorf("machine %s missing parent", machine.Id())
			}
		}

		exMachine, err := e.newMachine(exParent, machine, instances)
		if err != nil {
			return errors.Trace(err)
		}
		machineMap[machine.Id()] = exMachine

		// TODO: status and constraints for machines.
	}

	return nil
}

func (e *exporter) newMachine(exParent migration.Machine, machine *Machine, instances map[string]instanceData) (migration.Machine, error) {
	args := migration.MachineArgs{
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
	var exMachine migration.Machine
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
	instData, found := instances[machine.doc.Id]
	if !found {
		return nil, errors.NotValidf("missing instance data for machine %s", machine.Id())
	}
	exMachine.SetInstance(e.newCloudInstanceArgs(instData))

	// Find the current machine status.
	statusArgs, err := e.statusArgs(machine.globalKey())
	if err != nil {
		return nil, errors.Annotatef(err, "status for machine %s", machine.Id())
	}
	exMachine.SetStatus(statusArgs)

	tools, err := machine.AgentTools()
	if err != nil {
		// This means the tools aren't set, but they should be.
		return nil, errors.Trace(err)
	}

	exMachine.SetTools(migration.AgentToolsArgs{
		Version: tools.Version,
		URL:     tools.URL,
		SHA256:  tools.SHA256,
		Size:    tools.Size,
	})

	return exMachine, nil
}

func (e *exporter) newAddressArgsSlice(a []address) []migration.AddressArgs {
	result := []migration.AddressArgs{}
	for _, addr := range a {
		result = append(result, e.newAddressArgs(addr))
	}
	return result
}

func (e *exporter) newAddressArgs(a address) migration.AddressArgs {
	return migration.AddressArgs{
		Value:       a.Value,
		Type:        a.AddressType,
		NetworkName: a.NetworkName,
		Scope:       a.Scope,
		Origin:      a.Origin,
	}
}

func (e *exporter) newCloudInstanceArgs(data instanceData) migration.CloudInstanceArgs {
	inst := migration.CloudInstanceArgs{
		InstanceId: string(data.InstanceId),
		Status:     data.Status,
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
	return inst
}

func (e *exporter) services() error {
	services, err := e.st.AllServices()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("found %d services", len(services))

	refcounts, err := e.readAllSettingsRefCounts()
	if err != nil {
		return errors.Trace(err)
	}

	for _, service := range services {
		if err := e.addService(service, refcounts); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *exporter) addService(service *Service, refcounts map[string]int) error {
	settingsKey := service.settingsKey()
	leadershipKey := leadershipSettingsKey(service.Name())

	serviceSettingsDoc, found := e.settings[settingsKey]
	if !found {
		return errors.Errorf("missing settings for service %q", service.Name())
	}
	refCount, found := refcounts[settingsKey]
	if !found {
		return errors.Errorf("missing settings refcount for service %q", service.Name())
	}
	leadershipSettingsDoc, found := e.settings[leadershipKey]
	if !found {
		return errors.Errorf("missing leadership settings for service %q", service.Name())
	}

	args := migration.ServiceArgs{
		Tag:                service.ServiceTag(),
		Series:             service.doc.Series,
		Subordinate:        service.doc.Subordinate,
		CharmURL:           service.doc.CharmURL.String(),
		ForceCharm:         service.doc.ForceCharm,
		Exposed:            service.doc.Exposed,
		MinUnits:           service.doc.MinUnits,
		Settings:           serviceSettingsDoc.Settings,
		SettingsRefCount:   refCount,
		LeadershipSettings: leadershipSettingsDoc.Settings,
	}
	exService := e.model.AddService(args)
	// Find the current service status.
	statusArgs, err := e.statusArgs(service.globalKey())
	if err != nil {
		return errors.Annotatef(err, "status for service %s", service.Name())
	}
	exService.SetStatus(statusArgs)
	return nil
}

func (e *exporter) readLastConnectionTimes() (map[string]time.Time, error) {
	lastConnections, closer := e.st.getCollection(modelUserLastConnectionC)
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

func (e *exporter) readAllSettings() error {
	settings, closer := e.st.getCollection(settingsC)
	defer closer()

	var docs []settingsDoc
	if err := settings.Find(nil).All(&docs); err != nil {
		return errors.Trace(err)
	}

	e.settings = make(map[string]settingsDoc)
	for _, doc := range docs {
		key := e.st.localID(doc.DocID)
		e.settings[key] = doc
	}
	return nil
}

func (e *exporter) readAllStatuses() error {
	statuses, closer := e.st.getCollection(statusesC)
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

func (e *exporter) statusArgs(globalKey string) (migration.StatusArgs, error) {
	result := migration.StatusArgs{}
	statusDoc, found := e.status[globalKey]
	if !found {
		return result, errors.NotFoundf("status data for %s", globalKey)
	}

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

func (e *exporter) readAllSettingsRefCounts() (map[string]int, error) {
	refCounts, closer := e.st.getCollection(settingsrefsC)
	defer closer()

	var docs []bson.M
	err := refCounts.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read settings refcount collection")
	}

	e.logger.Debugf("read %d settings refcount documents", len(docs))
	result := make(map[string]int)
	for _, doc := range docs {
		docId, ok := doc["_id"].(string)
		if !ok {
			return nil, errors.Errorf("expected string, got %s (%T)", doc["_id"], doc["_id"])
		}
		id := e.st.localID(docId)
		count, ok := doc["refcount"].(int)
		if !ok {
			return nil, errors.Errorf("expected int, got %s (%T)", doc["refcount"], doc["refcount"])
		}
		result[id] = count
	}

	return result, nil
}
