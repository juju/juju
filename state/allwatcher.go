// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

// allWatcherStateBacking implements Backing by fetching entities for
// a single model from the State.
type allWatcherStateBacking struct {
	st               *State
	collectionByName map[string]allWatcherStateCollection
}

// allModelWatcherStateBacking implements Backing by fetching entities
// for all models from the State.
type allModelWatcherStateBacking struct {
	st               *State
	stPool           *StatePool
	collectionByName map[string]allWatcherStateCollection
}

// allWatcherStateCollection holds information about a
// collection watched by an allWatcher and the
// type of value we use to store entity information
// for that collection.
type allWatcherStateCollection struct {
	// name stores the name of the collection.
	name string

	// docType stores the type of document
	// that we use for this collection.
	docType reflect.Type

	// subsidiary is true if the collection is used only
	// to modify a primary entity.
	subsidiary bool
}

// makeAllWatcherCollectionInfo returns a name indexed map of
// allWatcherStateCollection instances for the collections specified.
func makeAllWatcherCollectionInfo(collNames ...string) map[string]allWatcherStateCollection {
	seenTypes := make(map[reflect.Type]struct{})
	collectionByName := make(map[string]allWatcherStateCollection)

	for _, collName := range collNames {
		collection := allWatcherStateCollection{name: collName}
		switch collName {
		case modelsC:
			collection.docType = reflect.TypeOf(backingModel{})
		case machinesC:
			collection.docType = reflect.TypeOf(backingMachine{})
		case unitsC:
			collection.docType = reflect.TypeOf(backingUnit{})
		case servicesC:
			collection.docType = reflect.TypeOf(backingService{})
		case actionsC:
			collection.docType = reflect.TypeOf(backingAction{})
		case relationsC:
			collection.docType = reflect.TypeOf(backingRelation{})
		case annotationsC:
			collection.docType = reflect.TypeOf(backingAnnotation{})
		case blocksC:
			collection.docType = reflect.TypeOf(backingBlock{})
		case statusesC:
			collection.docType = reflect.TypeOf(backingStatus{})
			collection.subsidiary = true
		case constraintsC:
			collection.docType = reflect.TypeOf(backingConstraints{})
			collection.subsidiary = true
		case settingsC:
			collection.docType = reflect.TypeOf(backingSettings{})
			collection.subsidiary = true
		case openedPortsC:
			collection.docType = reflect.TypeOf(backingOpenedPorts{})
			collection.subsidiary = true
		default:
			panic(errors.Errorf("unknown collection %q", collName))
		}

		docType := collection.docType
		if _, ok := seenTypes[docType]; ok {
			panic(errors.Errorf("duplicate collection type %s", docType))
		}
		seenTypes[docType] = struct{}{}

		if _, ok := collectionByName[collName]; ok {
			panic(errors.Errorf("duplicate collection name %q", collName))
		}
		collectionByName[collName] = collection
	}

	return collectionByName
}

type backingModel modelDoc

func (e *backingModel) updated(st *State, store *multiwatcherStore, id string) error {
	store.Update(&multiwatcher.ModelInfo{
		ModelUUID:  e.UUID,
		Name:       e.Name,
		Life:       multiwatcher.Life(e.Life.String()),
		Owner:      e.Owner,
		ServerUUID: e.ServerUUID,
	})
	return nil
}

func (e *backingModel) removed(store *multiwatcherStore, modelUUID, _ string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "model",
		ModelUUID: modelUUID,
		Id:        modelUUID,
	})
	return nil
}

func (e *backingModel) mongoId() string {
	return e.UUID
}

type backingMachine machineDoc

func (m *backingMachine) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.MachineInfo{
		ModelUUID:                st.ModelUUID(),
		Id:                       m.Id,
		Life:                     multiwatcher.Life(m.Life.String()),
		Series:                   m.Series,
		Jobs:                     paramsJobsFromJobs(m.Jobs),
		Addresses:                network.MergedAddresses(networkAddresses(m.MachineAddresses), networkAddresses(m.Addresses)),
		SupportedContainers:      m.SupportedContainers,
		SupportedContainersKnown: m.SupportedContainersKnown,
		HasVote:                  m.HasVote,
		WantsVote:                wantsVote(m.Jobs, m.NoVote),
		StatusData:               make(map[string]interface{}),
	}

	oldInfo := store.Get(info.EntityId())
	if oldInfo == nil {
		// We're adding the entry for the first time,
		// so fetch the associated machine status.
		statusInfo, err := getStatus(st, machineGlobalKey(m.Id), "machine")
		if err != nil {
			return err
		}
		info.Status = multiwatcher.Status(statusInfo.Status)
		info.StatusInfo = statusInfo.Message
	} else {
		// The entry already exists, so preserve the current status and
		// instance data.
		oldInfo := oldInfo.(*multiwatcher.MachineInfo)
		info.Status = oldInfo.Status
		info.StatusInfo = oldInfo.StatusInfo
		info.InstanceId = oldInfo.InstanceId
		info.HardwareCharacteristics = oldInfo.HardwareCharacteristics
	}
	// If the machine is been provisioned, fetch the instance id as required,
	// and set instance id and hardware characteristics.
	if m.Nonce != "" && info.InstanceId == "" {
		instanceData, err := getInstanceData(st, m.Id)
		if err == nil {
			info.InstanceId = string(instanceData.InstanceId)
			info.HardwareCharacteristics = hardwareCharacteristics(instanceData)
		} else if !errors.IsNotFound(err) {
			return err
		}
	}
	store.Update(info)
	return nil
}

func (m *backingMachine) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "machine",
		ModelUUID: modelUUID,
		Id:        id,
	})
	return nil
}

func (m *backingMachine) mongoId() string {
	return m.DocID
}

type backingUnit unitDoc

func getUnitPortRangesAndPorts(st *State, unitName string) ([]network.PortRange, []network.Port, error) {
	// Get opened port ranges for the unit and convert them to ports,
	// as older clients/servers do not know about ranges). See bug
	// http://pad.lv/1418344 for more info.
	unit, err := st.Unit(unitName)
	if errors.IsNotFound(err) {
		// Empty slices ensure backwards compatibility with older clients.
		// See Bug #1425435.
		return []network.PortRange{}, []network.Port{}, nil
	} else if err != nil {
		return nil, nil, errors.Annotatef(err, "failed to get unit %q", unitName)
	}
	portRanges, err := unit.OpenedPorts()
	// Since the port ranges are associated with the unit's machine,
	// we need to check for NotAssignedError.
	if errors.IsNotAssigned(err) {
		// Not assigned, so there won't be any ports opened.
		// Empty slices ensure backwards compatibility with older clients.
		// See Bug #1425435.
		return []network.PortRange{}, []network.Port{}, nil
	} else if err != nil {
		return nil, nil, errors.Annotate(err, "failed to get unit port ranges")
	}
	// For backward compatibility, if there are no ports opened, return an
	// empty slice rather than a nil slice. Use a len(portRanges) capacity to
	// avoid unnecessary allocations, since most of the times only specific
	// ports are opened by charms.
	compatiblePorts := make([]network.Port, 0, len(portRanges))
	for _, portRange := range portRanges {
		for j := portRange.FromPort; j <= portRange.ToPort; j++ {
			compatiblePorts = append(compatiblePorts, network.Port{
				Number:   j,
				Protocol: portRange.Protocol,
			})
		}
	}
	return portRanges, compatiblePorts, nil
}

func unitAndAgentStatus(st *State, name string) (unitStatus, agentStatus *StatusInfo, err error) {
	unit, err := st.Unit(name)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	unitStatusResult, err := unit.Status()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	agentStatusResult, err := unit.AgentStatus()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return &unitStatusResult, &agentStatusResult, nil
}

func (u *backingUnit) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.UnitInfo{
		ModelUUID:   st.ModelUUID(),
		Name:        u.Name,
		Service:     u.Service,
		Series:      u.Series,
		MachineId:   u.MachineId,
		Subordinate: u.Principal != "",
		StatusData:  make(map[string]interface{}),
	}
	if u.CharmURL != nil {
		info.CharmURL = u.CharmURL.String()
	}
	oldInfo := store.Get(info.EntityId())
	if oldInfo == nil {
		logger.Debugf("new unit %q added to backing state", u.Name)
		// We're adding the entry for the first time,
		// so fetch the associated unit status and opened ports.
		unitStatus, agentStatus, err := unitAndAgentStatus(st, u.Name)
		if err != nil {
			return errors.Annotatef(err, "reading unit and agent status for %q", u.Name)
		}
		// Unit and workload status.
		info.WorkloadStatus = multiwatcher.StatusInfo{
			Current: multiwatcher.Status(unitStatus.Status),
			Message: unitStatus.Message,
			Data:    normaliseStatusData(unitStatus.Data),
			Since:   unitStatus.Since,
		}
		if u.Tools != nil {
			info.AgentStatus.Version = u.Tools.Version.Number.String()
		}
		info.AgentStatus = multiwatcher.StatusInfo{
			Current: multiwatcher.Status(agentStatus.Status),
			Message: agentStatus.Message,
			Data:    normaliseStatusData(agentStatus.Data),
			Since:   agentStatus.Since,
		}
		// Legacy status info.
		if unitStatus.Status == StatusError {
			info.Status = multiwatcher.Status(unitStatus.Status)
			info.StatusInfo = unitStatus.Message
			info.StatusData = normaliseStatusData(unitStatus.Data)
		} else {
			legacyStatus, ok := TranslateToLegacyAgentState(agentStatus.Status, unitStatus.Status, unitStatus.Message)
			if !ok {
				logger.Warningf(
					"translate to legacy status encounted unexpected workload status %q and agent status %q",
					unitStatus.Status, agentStatus.Status)
			}
			info.Status = multiwatcher.Status(legacyStatus)
			info.StatusInfo = agentStatus.Message
			info.StatusData = normaliseStatusData(agentStatus.Data)
		}

		portRanges, compatiblePorts, err := getUnitPortRangesAndPorts(st, u.Name)
		if err != nil {
			return errors.Trace(err)
		}
		info.PortRanges = portRanges
		info.Ports = compatiblePorts

	} else {
		// The entry already exists, so preserve the current status and ports.
		oldInfo := oldInfo.(*multiwatcher.UnitInfo)
		// Legacy status.
		info.Status = oldInfo.Status
		info.StatusInfo = oldInfo.StatusInfo
		// Unit and workload status.
		info.AgentStatus = oldInfo.AgentStatus
		info.WorkloadStatus = oldInfo.WorkloadStatus
		info.Ports = oldInfo.Ports
		info.PortRanges = oldInfo.PortRanges
	}
	publicAddress, privateAddress, err := getUnitAddresses(st, u.Name)
	if err != nil {
		return err
	}
	info.PublicAddress = publicAddress
	info.PrivateAddress = privateAddress
	store.Update(info)
	return nil
}

// getUnitAddresses returns the public and private addresses on a given unit.
// As of 1.18, the addresses are stored on the assigned machine but we retain
// this approach for backwards compatibility.
func getUnitAddresses(st *State, unitName string) (string, string, error) {
	u, err := st.Unit(unitName)
	if errors.IsNotFound(err) {
		// Not found, so there won't be any addresses.
		return "", "", nil
	} else if err != nil {
		return "", "", err
	}
	publicAddress, err := u.PublicAddress()
	if err != nil {
		logger.Warningf("getting a public address for unit %q failed: %q", u.Name(), err)
	}
	privateAddress, err := u.PrivateAddress()
	if err != nil {
		logger.Warningf("getting a private address for unit %q failed: %q", u.Name(), err)
	}
	return publicAddress.Value, privateAddress.Value, nil
}

func (u *backingUnit) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "unit",
		ModelUUID: modelUUID,
		Id:        id,
	})
	return nil
}

func (u *backingUnit) mongoId() string {
	return u.DocID
}

type backingService serviceDoc

func (svc *backingService) updated(st *State, store *multiwatcherStore, id string) error {
	if svc.CharmURL == nil {
		return errors.Errorf("charm url is nil")
	}
	env, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	info := &multiwatcher.ServiceInfo{
		ModelUUID:   st.ModelUUID(),
		Name:        svc.Name,
		Exposed:     svc.Exposed,
		CharmURL:    svc.CharmURL.String(),
		OwnerTag:    svc.fixOwnerTag(env),
		Life:        multiwatcher.Life(svc.Life.String()),
		MinUnits:    svc.MinUnits,
		Subordinate: svc.Subordinate,
	}
	oldInfo := store.Get(info.EntityId())
	needConfig := false
	if oldInfo == nil {
		logger.Debugf("new service %q added to backing state", svc.Name)
		key := serviceGlobalKey(svc.Name)
		// We're adding the entry for the first time,
		// so fetch the associated child documents.
		c, err := readConstraints(st, key)
		if err != nil {
			return errors.Trace(err)
		}
		info.Constraints = c
		needConfig = true
		// Fetch the status.
		service, err := st.Service(svc.Name)
		if err != nil {
			return errors.Trace(err)
		}
		serviceStatus, err := service.Status()
		if err != nil {
			logger.Warningf("reading service status for key %s: %v", key, err)
		}
		if err != nil && !errors.IsNotFound(err) {
			return errors.Annotatef(err, "reading service status for key %s", key)
		}
		if err == nil {
			info.Status = multiwatcher.StatusInfo{
				Current: multiwatcher.Status(serviceStatus.Status),
				Message: serviceStatus.Message,
				Data:    normaliseStatusData(serviceStatus.Data),
				Since:   serviceStatus.Since,
			}
		} else {
			// TODO(wallyworld) - bug http://pad.lv/1451283
			// return an error here once we figure out what's happening
			// Not sure how status can even return NotFound as it is created
			// with the service initially. For now, we'll log the error as per
			// the above and return Unknown.
			now := time.Now()
			info.Status = multiwatcher.StatusInfo{
				Current: multiwatcher.Status(StatusUnknown),
				Since:   &now,
				Data:    normaliseStatusData(nil),
			}
		}
	} else {
		// The entry already exists, so preserve the current status.
		oldInfo := oldInfo.(*multiwatcher.ServiceInfo)
		info.Constraints = oldInfo.Constraints
		if info.CharmURL == oldInfo.CharmURL {
			// The charm URL remains the same - we can continue to
			// use the same config settings.
			info.Config = oldInfo.Config
		} else {
			// The charm URL has changed - we need to fetch the
			// settings from the new charm's settings doc.
			needConfig = true
		}
	}
	if needConfig {
		doc, err := readSettingsDoc(st, serviceSettingsKey(svc.Name, svc.CharmURL))
		if err != nil {
			return errors.Trace(err)
		}
		info.Config = doc.Settings
	}
	store.Update(info)
	return nil
}

func (svc *backingService) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "service",
		ModelUUID: modelUUID,
		Id:        id,
	})
	return nil
}

// SCHEMACHANGE
// TODO(mattyw) remove when schema upgrades are possible
func (svc *backingService) fixOwnerTag(env *Model) string {
	if svc.OwnerTag != "" {
		return svc.OwnerTag
	}
	return env.Owner().String()
}

func (svc *backingService) mongoId() string {
	return svc.DocID
}

type backingAction actionDoc

func (a *backingAction) mongoId() string {
	return a.DocId
}

func (a *backingAction) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "action",
		ModelUUID: modelUUID,
		Id:        id,
	})
	return nil
}

func (a *backingAction) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.ActionInfo{
		ModelUUID:  st.ModelUUID(),
		Id:         id,
		Receiver:   a.Receiver,
		Name:       a.Name,
		Parameters: a.Parameters,
		Status:     string(a.Status),
		Message:    a.Message,
		Results:    a.Results,
		Enqueued:   a.Enqueued,
		Started:    a.Started,
		Completed:  a.Completed,
	}
	store.Update(info)
	return nil
}

type backingRelation relationDoc

func (r *backingRelation) updated(st *State, store *multiwatcherStore, id string) error {
	eps := make([]multiwatcher.Endpoint, len(r.Endpoints))
	for i, ep := range r.Endpoints {
		eps[i] = multiwatcher.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		}
	}
	info := &multiwatcher.RelationInfo{
		ModelUUID: st.ModelUUID(),
		Key:       r.Key,
		Id:        r.Id,
		Endpoints: eps,
	}
	store.Update(info)
	return nil
}

func (r *backingRelation) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "relation",
		ModelUUID: modelUUID,
		Id:        id,
	})
	return nil
}

func (r *backingRelation) mongoId() string {
	return r.DocID
}

type backingAnnotation annotatorDoc

func (a *backingAnnotation) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.AnnotationInfo{
		ModelUUID:   st.ModelUUID(),
		Tag:         a.Tag,
		Annotations: a.Annotations,
	}
	store.Update(info)
	return nil
}

func (a *backingAnnotation) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	tag, ok := tagForGlobalKey(id)
	if !ok {
		return errors.Errorf("could not parse global key: %q", id)
	}
	store.Remove(multiwatcher.EntityId{
		Kind:      "annotation",
		ModelUUID: modelUUID,
		Id:        tag,
	})
	return nil
}

func (a *backingAnnotation) mongoId() string {
	return a.GlobalKey
}

type backingBlock blockDoc

func (a *backingBlock) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.BlockInfo{
		ModelUUID: st.ModelUUID(),
		Id:        id,
		Tag:       a.Tag,
		Type:      a.Type.ToParams(),
		Message:   a.Message,
	}
	store.Update(info)
	return nil
}

func (a *backingBlock) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityId{
		Kind:      "block",
		ModelUUID: modelUUID,
		Id:        id,
	})
	return nil
}

func (a *backingBlock) mongoId() string {
	return a.DocID
}

type backingStatus statusDoc

func (s *backingStatus) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIdForGlobalKey(st.ModelUUID(), id)
	if !ok {
		return nil
	}
	info0 := store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.UnitInfo:
		newInfo := *info
		// Get the unit's current recorded status from state.
		// It's needed to reset the unit status when a unit comes off error.
		statusInfo, err := getStatus(st, unitGlobalKey(newInfo.Name), "unit")
		if err != nil {
			return err
		}
		if err := s.updatedUnitStatus(st, store, id, statusInfo, &newInfo); err != nil {
			return err
		}
		info0 = &newInfo
	case *multiwatcher.ServiceInfo:
		newInfo := *info
		newInfo.Status.Current = multiwatcher.Status(s.Status)
		newInfo.Status.Message = s.StatusInfo
		newInfo.Status.Data = normaliseStatusData(s.StatusData)
		newInfo.Status.Since = unixNanoToTime(s.Updated)
		info0 = &newInfo
	case *multiwatcher.MachineInfo:
		newInfo := *info
		newInfo.Status = multiwatcher.Status(s.Status)
		newInfo.StatusInfo = s.StatusInfo
		newInfo.StatusData = normaliseStatusData(s.StatusData)
		info0 = &newInfo
	default:
		return errors.Errorf("status for unexpected entity with id %q; type %T", id, info)
	}
	store.Update(info0)
	return nil
}

func (s *backingStatus) updatedUnitStatus(st *State, store *multiwatcherStore, id string, unitStatus StatusInfo, newInfo *multiwatcher.UnitInfo) error {
	// Unit or workload status - display the agent status or any error.
	if strings.HasSuffix(id, "#charm") || s.Status == StatusError {
		newInfo.WorkloadStatus.Current = multiwatcher.Status(s.Status)
		newInfo.WorkloadStatus.Message = s.StatusInfo
		newInfo.WorkloadStatus.Data = normaliseStatusData(s.StatusData)
		newInfo.WorkloadStatus.Since = unixNanoToTime(s.Updated)
	} else {
		newInfo.AgentStatus.Current = multiwatcher.Status(s.Status)
		newInfo.AgentStatus.Message = s.StatusInfo
		newInfo.AgentStatus.Data = normaliseStatusData(s.StatusData)
		newInfo.AgentStatus.Since = unixNanoToTime(s.Updated)
		// If the unit was in error and now it's not, we need to reset its
		// status back to what was previously recorded.
		if newInfo.WorkloadStatus.Current == multiwatcher.Status(StatusError) {
			newInfo.WorkloadStatus.Current = multiwatcher.Status(unitStatus.Status)
			newInfo.WorkloadStatus.Message = unitStatus.Message
			newInfo.WorkloadStatus.Data = normaliseStatusData(unitStatus.Data)
			newInfo.WorkloadStatus.Since = unixNanoToTime(s.Updated)
		}
	}

	// Legacy status info - it is an aggregated value between workload and agent statuses.
	legacyStatus, ok := TranslateToLegacyAgentState(
		Status(newInfo.AgentStatus.Current),
		Status(newInfo.WorkloadStatus.Current),
		newInfo.WorkloadStatus.Message,
	)
	if !ok {
		logger.Warningf(
			"translate to legacy status encounted unexpected workload status %q and agent status %q",
			newInfo.WorkloadStatus.Current, newInfo.AgentStatus.Current)
	}
	newInfo.Status = multiwatcher.Status(legacyStatus)
	if newInfo.Status == multiwatcher.Status(StatusError) {
		newInfo.StatusInfo = newInfo.WorkloadStatus.Message
		newInfo.StatusData = normaliseStatusData(newInfo.WorkloadStatus.Data)
	} else {
		newInfo.StatusInfo = newInfo.AgentStatus.Message
		newInfo.StatusData = normaliseStatusData(newInfo.AgentStatus.Data)
	}

	// A change in a unit's status might also affect it's service.
	service, err := st.Service(newInfo.Service)
	if err != nil {
		return errors.Trace(err)
	}
	serviceId, ok := backingEntityIdForGlobalKey(st.ModelUUID(), service.globalKey())
	if !ok {
		return nil
	}
	serviceInfo := store.Get(serviceId)
	if serviceInfo == nil {
		return nil
	}
	status, err := service.Status()
	if err != nil {
		return errors.Trace(err)
	}
	newServiceInfo := *serviceInfo.(*multiwatcher.ServiceInfo)
	newServiceInfo.Status.Current = multiwatcher.Status(status.Status)
	newServiceInfo.Status.Message = status.Message
	newServiceInfo.Status.Data = normaliseStatusData(status.Data)
	newServiceInfo.Status.Since = status.Since
	store.Update(&newServiceInfo)
	return nil
}

func (s *backingStatus) removed(*multiwatcherStore, string, string, *State) error {
	// If the status is removed, the parent will follow not long after,
	// so do nothing.
	return nil
}

func (s *backingStatus) mongoId() string {
	panic("cannot find mongo id from status document")
}

type backingConstraints constraintsDoc

func (c *backingConstraints) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIdForGlobalKey(st.ModelUUID(), id)
	if !ok {
		return nil
	}
	info0 := store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.UnitInfo, *multiwatcher.MachineInfo:
		// We don't (yet) publish unit or machine constraints.
		return nil
	case *multiwatcher.ServiceInfo:
		newInfo := *info
		newInfo.Constraints = constraintsDoc(*c).value()
		info0 = &newInfo
	default:
		return errors.Errorf("status for unexpected entity with id %q; type %T", id, info)
	}
	store.Update(info0)
	return nil
}

func (c *backingConstraints) removed(*multiwatcherStore, string, string, *State) error {
	return nil
}

func (c *backingConstraints) mongoId() string {
	panic("cannot find mongo id from constraints document")
}

type backingSettings settingsDoc

func (s *backingSettings) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, url, ok := backingEntityIdForSettingsKey(st.ModelUUID(), id)
	if !ok {
		return nil
	}
	info0 := store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.ServiceInfo:
		// If we're seeing settings for the service with a different
		// charm URL, we ignore them - we will fetch
		// them again when the service charm changes.
		// By doing this we make sure that the settings in the
		// ServiceInfo are always consistent with the charm URL.
		if info.CharmURL != url {
			break
		}
		newInfo := *info
		newInfo.Config = s.Settings
		info0 = &newInfo
	default:
		return nil
	}
	store.Update(info0)
	return nil
}

func (s *backingSettings) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	parentID, url, ok := backingEntityIdForSettingsKey(modelUUID, id)
	if !ok {
		// Service is already gone along with its settings.
		return nil
	}
	parent := store.Get(parentID)
	if info, ok := parent.(*multiwatcher.ServiceInfo); ok {
		if info.CharmURL != url {
			return nil
		}
		newInfo := *info
		newInfo.Config = s.Settings
		parent = &newInfo
		store.Update(parent)
	}
	return nil
}

func (s *backingSettings) mongoId() string {
	panic("cannot find mongo id from settings document")
}

// backingEntityIdForSettingsKey returns the entity id for the given
// settings key. Any extra information in the key is returned in
// extra.
func backingEntityIdForSettingsKey(modelUUID, key string) (eid multiwatcher.EntityId, extra string, ok bool) {
	if !strings.HasPrefix(key, "s#") {
		eid, ok = backingEntityIdForGlobalKey(modelUUID, key)
		return
	}
	key = key[2:]
	i := strings.Index(key, "#")
	if i == -1 {
		return multiwatcher.EntityId{}, "", false
	}
	eid = (&multiwatcher.ServiceInfo{
		ModelUUID: modelUUID,
		Name:      key[0:i],
	}).EntityId()
	extra = key[i+1:]
	ok = true
	return
}

type backingOpenedPorts map[string]interface{}

func (p *backingOpenedPorts) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIdForOpenedPortsKey(st.ModelUUID(), id)
	if !ok {
		return nil
	}
	switch info := store.Get(parentID).(type) {
	case nil:
		// The parent info doesn't exist. This is unexpected because the port
		// always refers to a machine. Anyway, ignore the ports for now.
		return nil
	case *multiwatcher.MachineInfo:
		// Retrieve the units placed in the machine.
		units, err := st.UnitsFor(info.Id)
		if err != nil {
			return errors.Trace(err)
		}
		// Update the ports on all units assigned to the machine.
		for _, u := range units {
			if err := updateUnitPorts(st, store, u); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (p *backingOpenedPorts) removed(store *multiwatcherStore, modelUUID, id string, st *State) error {
	if st == nil {
		return nil
	}
	parentID, ok := backingEntityIdForOpenedPortsKey(st.ModelUUID(), id)
	if !ok {
		return nil
	}
	switch info := store.Get(parentID).(type) {
	case nil:
		// The parent info doesn't exist. This is unexpected because the port
		// always refers to a machine. Anyway, ignore the ports for now.
		return nil
	case *multiwatcher.MachineInfo:
		// Retrieve the units placed in the machine.
		units, err := st.UnitsFor(info.Id)
		if err != nil {
			// An error isn't returned here because the watcher is
			// always acting a little behind reality. It is reasonable
			// that entities have been deleted from State but we're
			// still seeing events related to them from the watcher.
			logger.Warningf("cannot retrieve units for %q: %v", info.Id, err)
			return nil
		}
		// Update the ports on all units assigned to the machine.
		for _, u := range units {
			if err := updateUnitPorts(st, store, u); err != nil {
				logger.Warningf("cannot update unit ports for %q: %v", u.Name(), err)
			}
		}
	}
	return nil
}

func (p *backingOpenedPorts) mongoId() string {
	panic("cannot find mongo id from openedPorts document")
}

// updateUnitPorts updates the Ports and PortRanges info of the given unit.
func updateUnitPorts(st *State, store *multiwatcherStore, u *Unit) error {
	eid, ok := backingEntityIdForGlobalKey(st.ModelUUID(), u.globalKey())
	if !ok {
		// This should never happen.
		return errors.New("cannot retrieve entity id for unit")
	}
	switch oldInfo := store.Get(eid).(type) {
	case nil:
		// The unit info doesn't exist. This is unlikely to happen, but ignore
		// the status until a unitInfo is included in the store.
		return nil
	case *multiwatcher.UnitInfo:
		portRanges, compatiblePorts, err := getUnitPortRangesAndPorts(st, oldInfo.Name)
		if err != nil {
			return errors.Trace(err)
		}
		unitInfo := *oldInfo
		unitInfo.PortRanges = portRanges
		unitInfo.Ports = compatiblePorts
		store.Update(&unitInfo)
	default:
		return nil
	}
	return nil
}

// backingEntityIdForOpenedPortsKey returns the entity id for the given
// openedPorts key. Any extra information in the key is discarded.
func backingEntityIdForOpenedPortsKey(modelUUID, key string) (multiwatcher.EntityId, bool) {
	parts, err := extractPortsIdParts(key)
	if err != nil {
		logger.Debugf("cannot parse ports key %q: %v", key, err)
		return multiwatcher.EntityId{}, false
	}
	return backingEntityIdForGlobalKey(modelUUID, machineGlobalKey(parts[1]))
}

// backingEntityIdForGlobalKey returns the entity id for the given global key.
// It returns false if the key is not recognized.
func backingEntityIdForGlobalKey(modelUUID, key string) (multiwatcher.EntityId, bool) {
	if len(key) < 3 || key[1] != '#' {
		return multiwatcher.EntityId{}, false
	}
	id := key[2:]
	switch key[0] {
	case 'm':
		return (&multiwatcher.MachineInfo{
			ModelUUID: modelUUID,
			Id:        id,
		}).EntityId(), true
	case 'u':
		id = strings.TrimSuffix(id, "#charm")
		return (&multiwatcher.UnitInfo{
			ModelUUID: modelUUID,
			Name:      id,
		}).EntityId(), true
	case 's':
		return (&multiwatcher.ServiceInfo{
			ModelUUID: modelUUID,
			Name:      id,
		}).EntityId(), true
	default:
		return multiwatcher.EntityId{}, false
	}
}

// backingEntityDoc is implemented by the documents in
// collections that the allWatcherStateBacking watches.
type backingEntityDoc interface {
	// updated is called when the document has changed.
	// The mongo _id value of the document is provided in id.
	updated(st *State, store *multiwatcherStore, id string) error

	// removed is called when the document has changed.
	// The receiving instance will not contain any data.
	//
	// The mongo _id value of the document is provided in id.
	//
	// In some cases st may be nil. If the implementation requires st
	// then it should do nothing.
	removed(store *multiwatcherStore, modelUUID, id string, st *State) error

	// mongoId returns the mongo _id field of the document.
	// It is currently never called for subsidiary documents.
	mongoId() string
}

func newAllWatcherStateBacking(st *State) Backing {
	collections := makeAllWatcherCollectionInfo(
		machinesC,
		unitsC,
		servicesC,
		relationsC,
		annotationsC,
		statusesC,
		constraintsC,
		settingsC,
		openedPortsC,
		actionsC,
		blocksC,
	)
	return &allWatcherStateBacking{
		st:               st,
		collectionByName: collections,
	}
}

func (b *allWatcherStateBacking) filterEnv(docID interface{}) bool {
	_, err := b.st.strictLocalID(docID.(string))
	return err == nil
}

// Watch watches all the collections.
func (b *allWatcherStateBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.WatchCollectionWithFilter(c.name, in, b.filterEnv)
	}
}

// Unwatch unwatches all the collections.
func (b *allWatcherStateBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.UnwatchCollection(c.name, in)
	}
}

// GetAll fetches all items that we want to watch from the state.
func (b *allWatcherStateBacking) GetAll(all *multiwatcherStore) error {
	err := loadAllWatcherEntities(b.st, b.collectionByName, all)
	return errors.Trace(err)
}

// Changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allWatcherStateBacking) Changed(all *multiwatcherStore, change watcher.Change) error {
	c, ok := b.collectionByName[change.C]
	if !ok {
		return errors.Errorf("unknown collection %q in fetch request", change.C)
	}
	col, closer := b.st.getCollection(c.name)
	defer closer()
	doc := reflect.New(c.docType).Interface().(backingEntityDoc)

	id := b.st.localID(change.Id.(string))

	// TODO(rog) investigate ways that this can be made more efficient
	// than simply fetching each entity in turn.
	// TODO(rog) avoid fetching documents that we have no interest
	// in, such as settings changes to entities we don't care about.
	err := col.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		err := doc.removed(all, b.st.ModelUUID(), id, b.st)
		return errors.Trace(err)
	}
	if err != nil {
		return err
	}
	return doc.updated(b.st, all, id)
}

// Release implements the Backing interface.
func (b *allWatcherStateBacking) Release() error {
	// allWatcherStateBacking doesn't need to release anything.
	return nil
}

func NewAllModelWatcherStateBacking(st *State) Backing {
	collections := makeAllWatcherCollectionInfo(
		modelsC,
		machinesC,
		unitsC,
		servicesC,
		relationsC,
		annotationsC,
		statusesC,
		constraintsC,
		settingsC,
		openedPortsC,
	)
	return &allModelWatcherStateBacking{
		st:               st,
		stPool:           NewStatePool(st),
		collectionByName: collections,
	}
}

// Watch watches all the collections.
func (b *allModelWatcherStateBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.WatchCollection(c.name, in)
	}
}

// Unwatch unwatches all the collections.
func (b *allModelWatcherStateBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.UnwatchCollection(c.name, in)
	}
}

// GetAll fetches all items that we want to watch from the state.
func (b *allModelWatcherStateBacking) GetAll(all *multiwatcherStore) error {
	envs, err := b.st.AllModels()
	if err != nil {
		return errors.Annotate(err, "error loading models")
	}
	for _, env := range envs {
		st, err := b.st.ForModel(env.ModelTag())
		if err != nil {
			return errors.Trace(err)
		}
		defer st.Close()

		err = loadAllWatcherEntities(st, b.collectionByName, all)
		if err != nil {
			return errors.Annotatef(err, "error loading entities for model %v", env.UUID())
		}
	}
	return nil
}

// Changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allModelWatcherStateBacking) Changed(all *multiwatcherStore, change watcher.Change) error {
	c, ok := b.collectionByName[change.C]
	if !ok {
		return errors.Errorf("unknown collection %q in fetch request", change.C)
	}

	modelUUID, id, err := b.idForChange(change)
	if err != nil {
		return errors.Trace(err)
	}

	doc := reflect.New(c.docType).Interface().(backingEntityDoc)

	st, err := b.getState(change.C, modelUUID)
	if err != nil {
		_, envErr := b.st.GetModel(names.NewModelTag(modelUUID))
		if errors.IsNotFound(envErr) {
			// The entity's model is gone so remove the entity
			// from the store.
			doc.removed(all, modelUUID, id, nil)
			return nil
		}
		return errors.Trace(err)
	}

	col, closer := st.getCollection(c.name)
	defer closer()

	// TODO - see TODOs in allWatcherStateBacking.Changed()
	err = col.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		err := doc.removed(all, modelUUID, id, st)
		return errors.Trace(err)
	}
	if err != nil {
		return err
	}
	return doc.updated(st, all, id)
}

func (b *allModelWatcherStateBacking) idForChange(change watcher.Change) (string, string, error) {
	if change.C == modelsC {
		modelUUID := change.Id.(string)
		return modelUUID, modelUUID, nil
	}

	modelUUID, id, ok := splitDocID(change.Id.(string))
	if !ok {
		return "", "", errors.Errorf("unknown id format: %v", change.Id.(string))
	}
	return modelUUID, id, nil
}

func (b *allModelWatcherStateBacking) getState(collName, modelUUID string) (*State, error) {
	if collName == modelsC {
		return b.st, nil
	}

	st, err := b.stPool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// Release implements the Backing interface.
func (b *allModelWatcherStateBacking) Release() error {
	err := b.stPool.Close()
	return errors.Trace(err)
}

func loadAllWatcherEntities(st *State, collectionByName map[string]allWatcherStateCollection, all *multiwatcherStore) error {
	// Use a single new MongoDB connection for all the work here.
	db, closer := st.newDB()
	defer closer()

	// TODO(rog) fetch collections concurrently?
	for _, c := range collectionByName {
		if c.subsidiary {
			continue
		}
		col, closer := db.GetCollection(c.name)
		defer closer()
		infoSlicePtr := reflect.New(reflect.SliceOf(c.docType))
		if err := col.Find(nil).All(infoSlicePtr.Interface()); err != nil {
			return errors.Errorf("cannot get all %s: %v", c.name, err)
		}
		infos := infoSlicePtr.Elem()
		for i := 0; i < infos.Len(); i++ {
			info := infos.Index(i).Addr().Interface().(backingEntityDoc)
			id := info.mongoId()
			err := info.updated(st, all, id)
			if err != nil {
				return errors.Annotatef(err, "failed to initialise backing for %s:%v", c.name, id)
			}
		}
	}

	return nil
}

func normaliseStatusData(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return make(map[string]interface{})
	}
	return data
}
