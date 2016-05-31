// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/description"
)

// Export the current model for the State.
func (st *State) Export() (description.Model, error) {
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
	if err := export.readAllStatusHistory(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllSettings(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllAnnotations(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.readAllConstraints(); err != nil {
		return nil, errors.Trace(err)
	}

	envConfig, found := export.settings[modelGlobalKey]
	if !found {
		return nil, errors.New("missing environ config")
	}

	blocks, err := export.readBlocks()
	if err != nil {
		return nil, errors.Trace(err)
	}

	args := description.ModelArgs{
		Owner:              dbModel.Owner(),
		Config:             envConfig.Settings,
		LatestToolsVersion: dbModel.LatestToolsVersion(),
		Blocks:             blocks,
	}
	export.model = description.NewModel(args)
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

	if err := export.modelUsers(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.machines(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.applications(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := export.relations(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := export.model.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	export.logExtras()

	return export.model, nil
}

type exporter struct {
	st      *State
	dbModel *Model
	model   description.Model
	logger  loggo.Logger

	annotations   map[string]annotatorDoc
	constraints   map[string]bson.M
	settings      map[string]settingsDoc
	status        map[string]bson.M
	statusHistory map[string][]historicalStatusDoc
	// Map of application name to units. Populated as part
	// of the services export.
	units map[string][]*Unit
}

func (e *exporter) sequences() error {
	sequences, closer := e.st.getCollection(sequenceC)
	defer closer()

	var docs []sequenceDoc
	if err := sequences.Find(nil).All(&docs); err != nil {
		return errors.Trace(err)
	}

	for _, doc := range docs {
		e.model.SetSequence(doc.Name, doc.Counter)
	}
	return nil
}

func (e *exporter) readBlocks() (map[string]string, error) {
	blocks, closer := e.st.getCollection(blocksC)
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
		arg := description.UserArgs{
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

	// Read all the open ports documents.
	openedPorts, closer := e.st.getCollection(openedPortsC)
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

		exMachine, err := e.newMachine(exParent, machine, instances, portsData)
		if err != nil {
			return errors.Trace(err)
		}
		machineMap[machine.Id()] = exMachine
	}

	return nil
}

func (e *exporter) newMachine(exParent description.Machine, machine *Machine, instances map[string]instanceData, portsData []portsDoc) (description.Machine, error) {
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
	instData, found := instances[machine.doc.Id]
	if !found {
		return nil, errors.NotValidf("missing instance data for machine %s", machine.Id())
	}
	exMachine.SetInstance(e.newCloudInstanceArgs(instData))

	// Find the current machine status.
	globalKey := machine.globalKey()
	statusArgs, err := e.statusArgs(globalKey)
	if err != nil {
		return nil, errors.Annotatef(err, "status for machine %s", machine.Id())
	}
	exMachine.SetStatus(statusArgs)
	exMachine.SetStatusHistory(e.statusHistoryArgs(globalKey))

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

func (e *exporter) applications() error {
	services, err := e.st.AllApplications()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("found %d applications", len(services))

	refcounts, err := e.readAllSettingsRefCounts()
	if err != nil {
		return errors.Trace(err)
	}

	e.units, err = e.readAllUnits()
	if err != nil {
		return errors.Trace(err)
	}

	meterStatus, err := e.readAllMeterStatus()
	if err != nil {
		return errors.Trace(err)
	}

	leaders, err := e.readServiceLeaders()
	if err != nil {
		return errors.Trace(err)
	}

	for _, service := range services {
		serviceUnits := e.units[service.Name()]
		leader := leaders[service.Name()]
		if err := e.addService(service, refcounts, serviceUnits, meterStatus, leader); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *exporter) readServiceLeaders() (map[string]string, error) {
	client, err := e.st.getLeadershipLeaseClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leases := client.Leases()
	result := make(map[string]string, len(leases))
	for key, value := range leases {
		result[key] = value.Holder
	}
	return result, nil
}

func (e *exporter) addService(service *Application, refcounts map[string]int, units []*Unit, meterStatus map[string]*meterStatusDoc, leader string) error {
	settingsKey := service.settingsKey()
	leadershipKey := leadershipSettingsKey(service.Name())

	serviceSettingsDoc, found := e.settings[settingsKey]
	if !found {
		return errors.Errorf("missing settings for application %q", service.Name())
	}
	refCount, found := refcounts[settingsKey]
	if !found {
		return errors.Errorf("missing settings refcount for application %q", service.Name())
	}
	leadershipSettingsDoc, found := e.settings[leadershipKey]
	if !found {
		return errors.Errorf("missing leadership settings for application %q", service.Name())
	}

	args := description.ApplicationArgs{
		Tag:                  service.ApplicationTag(),
		Series:               service.doc.Series,
		Subordinate:          service.doc.Subordinate,
		CharmURL:             service.doc.CharmURL.String(),
		Channel:              service.doc.Channel,
		CharmModifiedVersion: service.doc.CharmModifiedVersion,
		ForceCharm:           service.doc.ForceCharm,
		Exposed:              service.doc.Exposed,
		MinUnits:             service.doc.MinUnits,
		Settings:             serviceSettingsDoc.Settings,
		SettingsRefCount:     refCount,
		Leader:               leader,
		LeadershipSettings:   leadershipSettingsDoc.Settings,
		MetricsCredentials:   service.doc.MetricCredentials,
	}
	exService := e.model.AddApplication(args)
	// Find the current service status.
	globalKey := service.globalKey()
	statusArgs, err := e.statusArgs(globalKey)
	if err != nil {
		return errors.Annotatef(err, "status for application %s", service.Name())
	}
	exService.SetStatus(statusArgs)
	exService.SetStatusHistory(e.statusHistoryArgs(globalKey))
	exService.SetAnnotations(e.getAnnotations(globalKey))

	constraintsArgs, err := e.constraintsArgs(globalKey)
	if err != nil {
		return errors.Trace(err)
	}
	exService.SetConstraints(constraintsArgs)

	for _, unit := range units {
		agentKey := unit.globalAgentKey()
		unitMeterStatus, found := meterStatus[agentKey]
		if !found {
			return errors.Errorf("missing meter status for unit %s", unit.Name())
		}

		args := description.UnitArgs{
			Tag:             unit.UnitTag(),
			Machine:         names.NewMachineTag(unit.doc.MachineId),
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
		exUnit := exService.AddUnit(args)
		// workload uses globalKey, agent uses globalAgentKey.
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
		exUnit.SetAnnotations(e.getAnnotations(globalKey))

		constraintsArgs, err := e.constraintsArgs(agentKey)
		if err != nil {
			return errors.Trace(err)
		}
		exUnit.SetConstraints(constraintsArgs)
	}

	return nil
}

func (e *exporter) relations() error {
	rels, err := e.st.AllRelations()
	if err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d relations", len(rels))

	relationScopes, err := e.readAllRelationScopes()
	if err != nil {
		return errors.Trace(err)
	}

	for _, relation := range rels {
		exRelation := e.model.AddRelation(description.RelationArgs{
			Id:  relation.Id(),
			Key: relation.String(),
		})
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
			// units of the specified service.
			units := e.units[ep.ApplicationName]
			for _, unit := range units {
				ru, err := relation.Unit(unit)
				if err != nil {
					return errors.Trace(err)
				}
				key := ru.key()
				if !relationScopes.Contains(key) {
					return errors.Errorf("missing relation scope for %s and %s", relation, unit.Name())
				}
				settingsDoc, found := e.settings[key]
				if !found {
					return errors.Errorf("missing relation settings for %s and %s", relation, unit.Name())
				}
				exEndPoint.SetUnitSettings(unit.Name(), settingsDoc.Settings)
			}
		}
	}
	return nil
}

func (e *exporter) readAllRelationScopes() (set.Strings, error) {
	relationScopes, closer := e.st.getCollection(relationScopesC)
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
	unitsCollection, closer := e.st.getCollection(unitsC)
	defer closer()

	docs := []unitDoc{}
	err := unitsCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all units")
	}
	e.logger.Debugf("found %d unit docs", len(docs))
	result := make(map[string][]*Unit)
	for _, doc := range docs {
		units := result[doc.Application]
		result[doc.Application] = append(units, newUnit(e.st, &doc))
	}
	return result, nil
}

func (e *exporter) readAllMeterStatus() (map[string]*meterStatusDoc, error) {
	meterStatuses, closer := e.st.getCollection(meterStatusC)
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

func (e *exporter) readAllAnnotations() error {
	annotations, closer := e.st.getCollection(annotationsC)
	defer closer()

	var docs []annotatorDoc
	if err := annotations.Find(nil).All(&docs); err != nil {
		return errors.Trace(err)
	}
	e.logger.Debugf("read %d annotations docs", len(docs))

	e.annotations = make(map[string]annotatorDoc)
	for _, doc := range docs {
		e.annotations[doc.GlobalKey] = doc
	}
	return nil
}

func (e *exporter) readAllConstraints() error {
	constraintsCollection, closer := e.st.getCollection(constraintsC)
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

func (e *exporter) readAllStatusHistory() error {
	statuses, closer := e.st.getCollection(statusesHistoryC)
	defer closer()

	count := 0
	e.statusHistory = make(map[string][]historicalStatusDoc)
	var doc historicalStatusDoc
	iter := statuses.Find(nil).Sort("-updated").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		history := e.statusHistory[doc.GlobalKey]
		e.statusHistory[doc.GlobalKey] = append(history, doc)
		count++
	}

	if err := iter.Err(); err != nil {
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
	result := make([]description.StatusArgs, len(history))
	e.logger.Debugf("found %d status history docs for %s", len(history), globalKey)
	for i, doc := range history {
		result[i] = description.StatusArgs{
			Value:   string(doc.Status),
			Message: doc.StatusInfo,
			Data:    doc.StatusData,
			Updated: time.Unix(0, doc.Updated),
		}
	}

	return result
}

func (e *exporter) constraintsArgs(globalKey string) (description.ConstraintsArgs, error) {
	doc, found := e.constraints[globalKey]
	if !found {
		// No constraints for this key.
		e.logger.Debugf("no constraints found for key %q", globalKey)
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
			optionalErr = errors.Errorf("expected uint64 for %s, got %T", name, value)
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
		default:
			optionalErr = errors.Errorf("expected []string] for %s, got %T", name, value)
		}
		return nil
	}
	result := description.ConstraintsArgs{
		Architecture: optionalString("arch"),
		Container:    optionalString("container"),
		CpuCores:     optionalInt("cpucores"),
		CpuPower:     optionalInt("cpupower"),
		InstanceType: optionalString("instancetype"),
		Memory:       optionalInt("mem"),
		RootDisk:     optionalInt("rootdisk"),
		Spaces:       optionalStringSlice("spaces"),
		Tags:         optionalStringSlice("tags"),
	}
	if optionalErr != nil {
		return description.ConstraintsArgs{}, errors.Trace(optionalErr)
	}
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

func (e *exporter) logExtras() {
	// As annotations are saved into the model, they are removed from the
	// exporter's map. If there are any left at the end, we are missing
	// things. Not an error just now, just a warning that we have missed
	// something. Could potentially be an error at a later date when
	// migrations are complete (but probably not).
	for key, doc := range e.annotations {
		e.logger.Warningf("unexported annotation for %s, %s", doc.Tag, key)
	}
}
