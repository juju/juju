// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/state/migration"
)

// Export the current environment for the State. If a different environment
// is required, the caller is expected to use st.ForEnviron(...) and close
// the session as required.
func (st *State) Export() (migration.Description, error) {
	environment, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}

	export := exporter{
		st:          st,
		environment: environment,
		logger:      loggo.GetLogger("juju.state.export-model"),
	}

	settings, err := export.readAllSettings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	envConfig, found := settings[environGlobalKey]
	if !found {
		return nil, errors.New("missing environ config")
	}

	args := migration.ModelArgs{
		Owner:              environment.Owner(),
		Config:             envConfig.Settings,
		LatestToolsVersion: environment.LatestToolsVersion(),
	}
	result := migration.NewDescription(args)

	export.model = result.Model()
	if err := export.environmentUsers(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.machines(); err != nil {
		return nil, errors.Trace(err)
	}

	return result, nil
}

type exporter struct {
	st          *State
	environment *Environment
	model       migration.Model
	logger      loggo.Logger
}

func (e *exporter) environmentUsers() error {
	users, err := e.environment.Users()
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

	instanceDataCollection, closer := e.st.getCollection(instanceDataC)
	defer closer()

	var instData []instanceData
	instances := make(map[string]instanceData)
	if err := instanceDataCollection.FindId(nil).All(&instData); err != nil {
		return errors.Annotate(err, "instance data")
	}
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
	//SupportedContainers *[]string
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
		return nil, errors.NotValidf("missing instance data for machine %s", machine.doc.Id)
	}
	exMachine.SetInstance(e.newCloudInstanceArgs(instData))

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

func (e *exporter) readLastConnectionTimes() (map[string]time.Time, error) {
	lastConnections, closer := e.st.getCollection(envUserLastConnectionC)
	defer closer()

	var docs []envUserLastConnectionDoc
	if err := lastConnections.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]time.Time)
	for _, doc := range docs {
		result[doc.UserName] = doc.LastConnection.UTC()
	}
	return result, nil
}

func (e *exporter) readAllSettings() (map[string]settingsDoc, error) {
	settings, closer := e.st.getCollection(settingsC)
	defer closer()

	var docs []settingsDoc
	if err := settings.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]settingsDoc)
	for _, doc := range docs {
		key := e.st.localID(doc.DocID)
		result[key] = doc
	}
	return result, nil
}
