// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

// allWatcherStateBacking implements allWatcherBacking by
// fetching entities from the State.
type allWatcherStateBacking struct {
	st *State
	// collections
	collectionByName map[string]allWatcherStateCollection
}

type backingMachine machineDoc

func (m *backingMachine) updated(st *State, store *multiwatcherStore, id interface{}) error {
	info := &multiwatcher.MachineInfo{
		Id:                       m.Id,
		Life:                     multiwatcher.Life(m.Life.String()),
		Series:                   m.Series,
		Jobs:                     paramsJobsFromJobs(m.Jobs),
		Addresses:                mergedAddresses(m.MachineAddresses, m.Addresses),
		SupportedContainers:      m.SupportedContainers,
		SupportedContainersKnown: m.SupportedContainersKnown,
		HasVote:                  m.HasVote,
		WantsVote:                wantsVote(m.Jobs, m.NoVote),
	}

	oldInfo := store.Get(info.EntityId())
	if oldInfo == nil {
		// We're adding the entry for the first time,
		// so fetch the associated machine status.
		sdoc, err := getStatus(st, machineGlobalKey(m.Id))
		if err != nil {
			return err
		}
		info.Status = multiwatcher.Status(sdoc.Status)
		info.StatusInfo = sdoc.StatusInfo
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

func (m *backingMachine) removed(st *State, store *multiwatcherStore, id interface{}) {
	store.Remove(multiwatcher.EntityId{
		Kind: "machine",
		Id:   st.localID(id.(string)),
	})
}

func (m *backingMachine) mongoId() interface{} {
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

func (u *backingUnit) updated(st *State, store *multiwatcherStore, id interface{}) error {
	info := &multiwatcher.UnitInfo{
		Name:        u.Name,
		Service:     u.Service,
		Series:      u.Series,
		MachineId:   u.MachineId,
		Subordinate: u.Principal != "",
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
			Data:    unitStatus.Data,
			Since:   unitStatus.Since,
		}
		if u.Tools != nil {
			info.AgentStatus.Version = u.Tools.Version.Number.String()
		}
		info.AgentStatus = multiwatcher.StatusInfo{
			Current: multiwatcher.Status(agentStatus.Status),
			Message: agentStatus.Message,
			Data:    agentStatus.Data,
			Since:   agentStatus.Since,
		}
		// Legacy status info.
		if unitStatus.Status == StatusError {
			info.Status = multiwatcher.Status(unitStatus.Status)
			info.StatusInfo = unitStatus.Message
			info.StatusData = unitStatus.Data
		} else {
			legacyStatus, ok := TranslateToLegacyAgentState(agentStatus.Status, unitStatus.Status, unitStatus.Message)
			if !ok {
				logger.Warningf(
					"translate to legacy status encounted unexpected workload status %q and agent status %q",
					unitStatus.Status, agentStatus.Status)
			}
			info.Status = multiwatcher.Status(legacyStatus)
			info.StatusInfo = agentStatus.Message
			info.StatusData = agentStatus.Data
		}
		if len(info.StatusData) == 0 {
			info.StatusData = nil
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
func getUnitAddresses(st *State, unitName string) (publicAddress, privateAddress string, err error) {
	u, err := st.Unit(unitName)
	if errors.IsNotFound(err) {
		// Not found, so there won't be any addresses.
		return "", "", nil
	} else if err != nil {
		return "", "", err
	}
	publicAddress, _ = u.PublicAddress()
	privateAddress, _ = u.PrivateAddress()
	return publicAddress, privateAddress, nil
}

func (u *backingUnit) removed(st *State, store *multiwatcherStore, id interface{}) {
	store.Remove(multiwatcher.EntityId{
		Kind: "unit",
		Id:   st.localID(id.(string)),
	})
}

func (u *backingUnit) mongoId() interface{} {
	return u.DocID
}

type backingService serviceDoc

func (svc *backingService) updated(st *State, store *multiwatcherStore, id interface{}) error {
	if svc.CharmURL == nil {
		return errors.Errorf("charm url is nil")
	}
	env, err := st.Environment()
	if err != nil {
		return errors.Trace(err)
	}
	info := &multiwatcher.ServiceInfo{
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
				Data:    serviceStatus.Data,
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
		var err error
		info.Config, _, err = readSettingsDoc(st, serviceSettingsKey(svc.Name, svc.CharmURL))
		if err != nil {
			return errors.Trace(err)
		}
	}
	store.Update(info)
	return nil
}

func (svc *backingService) removed(st *State, store *multiwatcherStore, id interface{}) {
	store.Remove(multiwatcher.EntityId{
		Kind: "service",
		Id:   st.localID(id.(string)),
	})
}

// SCHEMACHANGE
// TODO(mattyw) remove when schema upgrades are possible
func (svc *backingService) fixOwnerTag(env *Environment) string {
	if svc.OwnerTag != "" {
		return svc.OwnerTag
	}
	return env.Owner().String()
}

func (svc *backingService) mongoId() interface{} {
	return svc.DocID
}

type backingAction actionDoc

func (a *backingAction) mongoId() interface{} {
	return a.DocId
}

func (a *backingAction) removed(st *State, store *multiwatcherStore, id interface{}) {
	store.Remove(multiwatcher.EntityId{
		Kind: "action",
		Id:   st.localID(id.(string)),
	})
}

func (a *backingAction) updated(st *State, store *multiwatcherStore, id interface{}) error {
	info := &multiwatcher.ActionInfo{
		Id:         st.localID(a.DocId),
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

func (r *backingRelation) updated(st *State, store *multiwatcherStore, id interface{}) error {
	eps := make([]multiwatcher.Endpoint, len(r.Endpoints))
	for i, ep := range r.Endpoints {
		eps[i] = multiwatcher.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		}
	}
	info := &multiwatcher.RelationInfo{
		Key:       r.Key,
		Id:        r.Id,
		Endpoints: eps,
	}
	store.Update(info)
	return nil
}

func (r *backingRelation) removed(st *State, store *multiwatcherStore, id interface{}) {
	store.Remove(multiwatcher.EntityId{
		Kind: "relation",
		Id:   st.localID(id.(string)),
	})
}

func (r *backingRelation) mongoId() interface{} {
	return r.DocID
}

type backingAnnotation annotatorDoc

func (a *backingAnnotation) updated(st *State, store *multiwatcherStore, id interface{}) error {
	info := &multiwatcher.AnnotationInfo{
		Tag:         a.Tag,
		Annotations: a.Annotations,
	}
	store.Update(info)
	return nil
}

func (a *backingAnnotation) removed(st *State, store *multiwatcherStore, id interface{}) {
	localID := st.localID(id.(string))
	tag, ok := tagForGlobalKey(localID)
	if !ok {
		panic(fmt.Errorf("unknown global key %q in state", localID))
	}
	store.Remove(multiwatcher.EntityId{
		Kind: "annotation",
		Id:   tag,
	})
}

func (a *backingAnnotation) mongoId() interface{} {
	return a.GlobalKey
}

type backingBlock blockDoc

func (a *backingBlock) updated(st *State, store *multiwatcherStore, id interface{}) error {
	info := &multiwatcher.BlockInfo{
		Id:      st.localID(a.DocID),
		Tag:     a.Tag,
		Type:    a.Type.ToParams(),
		Message: a.Message,
	}
	store.Update(info)
	return nil
}

func (a *backingBlock) removed(st *State, store *multiwatcherStore, id interface{}) {
	store.Remove(multiwatcher.EntityId{
		Kind: "block",
		Id:   st.localID(id.(string)),
	})
}

func (a *backingBlock) mongoId() interface{} {
	return a.DocID
}

type backingStatus statusDoc

func (s *backingStatus) updated(st *State, store *multiwatcherStore, id interface{}) error {
	parentID, ok := backingEntityIdForGlobalKey(st.localID(id.(string)))
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
		if err := s.updatedUnitStatus(st, store, id.(string), &newInfo); err != nil {
			return err
		}
		info0 = &newInfo
	case *multiwatcher.ServiceInfo:
		newInfo := *info
		newInfo.Status.Current = multiwatcher.Status(s.Status)
		newInfo.Status.Message = s.StatusInfo
		newInfo.Status.Data = s.StatusData
		newInfo.Status.Since = s.Updated
		info0 = &newInfo
	case *multiwatcher.MachineInfo:
		newInfo := *info
		newInfo.Status = multiwatcher.Status(s.Status)
		newInfo.StatusInfo = s.StatusInfo
		newInfo.StatusData = s.StatusData
		info0 = &newInfo
	default:
		panic(fmt.Errorf("status for unexpected entity with id %q; type %T", id, info))
	}
	store.Update(info0)
	return nil
}

func (s *backingStatus) updatedUnitStatus(st *State, store *multiwatcherStore, id string, newInfo *multiwatcher.UnitInfo) error {
	// Unit or workload status - display the agent status or any error.
	if strings.HasSuffix(id, "#charm") || s.Status == StatusError {
		newInfo.WorkloadStatus.Current = multiwatcher.Status(s.Status)
		newInfo.WorkloadStatus.Message = s.StatusInfo
		newInfo.WorkloadStatus.Data = s.StatusData
		newInfo.WorkloadStatus.Since = s.Updated
	} else {
		newInfo.AgentStatus.Current = multiwatcher.Status(s.Status)
		newInfo.AgentStatus.Message = s.StatusInfo
		newInfo.AgentStatus.Data = s.StatusData
		newInfo.AgentStatus.Since = s.Updated
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
		newInfo.StatusData = newInfo.WorkloadStatus.Data
	} else {
		newInfo.StatusInfo = newInfo.AgentStatus.Message
		newInfo.StatusData = newInfo.AgentStatus.Data
	}

	// A change in a unit's status might also affect it's service.
	service, err := st.Service(newInfo.Service)
	if err != nil {
		return errors.Trace(err)
	}
	serviceId, ok := backingEntityIdForGlobalKey(service.globalKey())
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
	newServiceInfo.Status.Data = status.Data
	newServiceInfo.Status.Since = status.Since
	store.Update(&newServiceInfo)
	return nil
}

func (s *backingStatus) removed(st *State, store *multiwatcherStore, id interface{}) {
	// If the status is removed, the parent will follow not long after,
	// so do nothing.
}

func (s *backingStatus) mongoId() interface{} {
	panic("cannot find mongo id from status document")
}

type backingConstraints constraintsDoc

func (c *backingConstraints) updated(st *State, store *multiwatcherStore, id interface{}) error {
	localID := st.localID(id.(string))
	parentID, ok := backingEntityIdForGlobalKey(localID)
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
		panic(fmt.Errorf("status for unexpected entity with id %q; type %T", localID, info))
	}
	store.Update(info0)
	return nil
}

func (c *backingConstraints) removed(st *State, store *multiwatcherStore, id interface{}) {}

func (c *backingConstraints) mongoId() interface{} {
	panic("cannot find mongo id from constraints document")
}

type backingSettings map[string]interface{}

func (s *backingSettings) updated(st *State, store *multiwatcherStore, id interface{}) error {
	localID := st.localID(id.(string))
	parentID, url, ok := backingEntityIdForSettingsKey(localID)
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
		cleanSettingsMap(*s)
		newInfo.Config = *s
		info0 = &newInfo
	default:
		return nil
	}
	store.Update(info0)
	return nil
}

func (s *backingSettings) removed(st *State, store *multiwatcherStore, id interface{}) {
	localID := st.localID(id.(string))
	parentID, url, ok := backingEntityIdForSettingsKey(localID)
	if !ok {
		// Service is already gone along with its settings.
		return
	}
	parent := store.Get(parentID)
	if info, ok := parent.(*multiwatcher.ServiceInfo); ok {
		if info.CharmURL != url {
			return
		}
		newInfo := *info
		cleanSettingsMap(*s)
		newInfo.Config = *s
		parent = &newInfo
		store.Update(parent)
	}
}

func (s *backingSettings) mongoId() interface{} {
	panic("cannot find mongo id from settings document")
}

// backingEntityIdForSettingsKey returns the entity id for the given
// settings key. Any extra information in the key is returned in
// extra.
func backingEntityIdForSettingsKey(key string) (eid multiwatcher.EntityId, extra string, ok bool) {
	if !strings.HasPrefix(key, "s#") {
		eid, ok = backingEntityIdForGlobalKey(key)
		return
	}
	key = key[2:]
	i := strings.Index(key, "#")
	if i == -1 {
		return multiwatcher.EntityId{}, "", false
	}
	eid = (&multiwatcher.ServiceInfo{Name: key[0:i]}).EntityId()
	extra = key[i+1:]
	ok = true
	return
}

type backingOpenedPorts map[string]interface{}

func (p *backingOpenedPorts) updated(st *State, store *multiwatcherStore, id interface{}) error {
	localID := st.localID(id.(string))
	parentID, ok := backingEntityIdForOpenedPortsKey(localID)
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

func (p *backingOpenedPorts) removed(st *State, store *multiwatcherStore, id interface{}) {
	localID := st.localID(id.(string))
	parentID, ok := backingEntityIdForOpenedPortsKey(localID)
	if !ok {
		return
	}
	switch info := store.Get(parentID).(type) {
	case nil:
		// The parent info doesn't exist. This is unexpected because the port
		// always refers to a machine. Anyway, ignore the ports for now.
		return
	case *multiwatcher.MachineInfo:
		// Retrieve the units placed in the machine.
		units, err := st.UnitsFor(info.Id)
		if err != nil {
			logger.Errorf("cannot retrieve units for %q: %v", info.Id, err)
			return
		}
		// Update the ports on all units assigned to the machine.
		for _, u := range units {
			if err := updateUnitPorts(st, store, u); err != nil {
				logger.Errorf("cannot update unit ports for %q: %v", u.Name(), err)
			}
		}
	}
}

func (p *backingOpenedPorts) mongoId() interface{} {
	panic("cannot find mongo id from openedPorts document")
}

// updateUnitPorts updates the Ports and PortRanges info of the given unit.
func updateUnitPorts(st *State, store *multiwatcherStore, u *Unit) error {
	eid, ok := backingEntityIdForGlobalKey(u.globalKey())
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
func backingEntityIdForOpenedPortsKey(key string) (multiwatcher.EntityId, bool) {
	parts, err := extractPortsIdParts(key)
	if err != nil {
		logger.Debugf("cannot parse ports key %q: %v", key, err)
		return multiwatcher.EntityId{}, false
	}
	return backingEntityIdForGlobalKey(machineGlobalKey(parts[1]))
}

// backingEntityIdForGlobalKey returns the entity id for the given global key.
// It returns false if the key is not recognized.
func backingEntityIdForGlobalKey(key string) (multiwatcher.EntityId, bool) {
	if len(key) < 3 || key[1] != '#' {
		return multiwatcher.EntityId{}, false
	}
	id := key[2:]
	switch key[0] {
	case 'm':
		return (&multiwatcher.MachineInfo{Id: id}).EntityId(), true
	case 'u':
		id = strings.TrimSuffix(id, "#charm")
		return (&multiwatcher.UnitInfo{Name: id}).EntityId(), true
	case 's':
		return (&multiwatcher.ServiceInfo{Name: id}).EntityId(), true
	default:
		return multiwatcher.EntityId{}, false
	}
}

// backingEntityDoc is implemented by the documents in
// collections that the allWatcherStateBacking watches.
type backingEntityDoc interface {
	// updated is called when the document has changed.
	// The mongo _id value of the document is provided in id.
	updated(st *State, store *multiwatcherStore, id interface{}) error

	// removed is called when the document has changed.
	// The receiving instance will not contain any data.
	// The mongo _id value of the document is provided in id.
	removed(st *State, store *multiwatcherStore, id interface{})

	// mongoId returns the mongo _id field of the document.
	// It is currently never called for subsidiary documents.
	mongoId() interface{}
}

// allWatcherStateCollection holds information about a
// collection watched by an allWatcher and the
// type of value we use to store entity information
// for that collection.
type allWatcherStateCollection struct {
	*mgo.Collection

	// infoType stores the type of the info type
	// that we use for this collection.
	infoType reflect.Type
	// subsidiary is true if the collection is used only
	// to modify a primary entity.
	subsidiary bool
}

func newAllWatcherStateBacking(st *State) Backing {
	collectionByType := make(map[reflect.Type]allWatcherStateCollection)
	b := &allWatcherStateBacking{
		st:               st,
		collectionByName: make(map[string]allWatcherStateCollection),
	}

	collections := []allWatcherStateCollection{{
		Collection: st.db.C(machinesC),
		infoType:   reflect.TypeOf(backingMachine{}),
	}, {
		Collection: st.db.C(unitsC),
		infoType:   reflect.TypeOf(backingUnit{}),
	}, {
		Collection: st.db.C(servicesC),
		infoType:   reflect.TypeOf(backingService{}),
	}, {
		Collection: st.db.C(actionsC),
		infoType:   reflect.TypeOf(backingAction{}),
	}, {
		Collection: st.db.C(relationsC),
		infoType:   reflect.TypeOf(backingRelation{}),
	}, {
		Collection: st.db.C(annotationsC),
		infoType:   reflect.TypeOf(backingAnnotation{}),
	}, {
		Collection: st.db.C(blocksC),
		infoType:   reflect.TypeOf(backingBlock{}),
	}, {
		Collection: st.db.C(statusesC),
		infoType:   reflect.TypeOf(backingStatus{}),
		subsidiary: true,
	}, {
		Collection: st.db.C(constraintsC),
		infoType:   reflect.TypeOf(backingConstraints{}),
		subsidiary: true,
	}, {
		Collection: st.db.C(settingsC),
		infoType:   reflect.TypeOf(backingSettings{}),
		subsidiary: true,
	}, {
		Collection: st.db.C(openedPortsC),
		infoType:   reflect.TypeOf(backingOpenedPorts{}),
		subsidiary: true,
	}}
	// Populate the collection maps from the above set of collections.
	for _, c := range collections {
		docType := c.infoType
		if _, ok := collectionByType[docType]; ok {
			panic(fmt.Errorf("duplicate collection type %s", docType))
		}
		collectionByType[docType] = c
		if _, ok := b.collectionByName[c.Name]; ok {
			panic(fmt.Errorf("duplicate collection name %q", c.Name))
		}
		b.collectionByName[c.Name] = c
	}
	return b
}

func (b *allWatcherStateBacking) filterEnv(docID interface{}) bool {
	_, err := b.st.strictLocalID(docID.(string))
	return err == nil
}

// Watch watches all the collections.
func (b *allWatcherStateBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.WatchCollectionWithFilter(c.Name, in, b.filterEnv)
	}
}

// Unwatch unwatches all the collections.
func (b *allWatcherStateBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.UnwatchCollection(c.Name, in)
	}
}

// GetAll fetches all items that we want to watch from the state.
func (b *allWatcherStateBacking) GetAll(all *multiwatcherStore) error {
	db, closer := b.st.newDB()
	defer closer()

	envUUID := b.st.EnvironUUID()

	// TODO(rog) fetch collections concurrently?
	for _, c := range b.collectionByName {
		if c.subsidiary {
			continue
		}
		col := newStateCollection(db.C(c.Name), envUUID)
		infoSlicePtr := reflect.New(reflect.SliceOf(c.infoType))
		if err := col.Find(nil).All(infoSlicePtr.Interface()); err != nil {
			return fmt.Errorf("cannot get all %s: %v", c.Name, err)
		}
		infos := infoSlicePtr.Elem()
		for i := 0; i < infos.Len(); i++ {
			info := infos.Index(i).Addr().Interface().(backingEntityDoc)
			id := info.mongoId()
			err := info.updated(b.st, all, id)
			if err != nil {
				return errors.Annotatef(err, "failed to initialise backing for %s:%v", c.Name, id)
			}
		}
	}
	return nil
}

// Changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allWatcherStateBacking) Changed(all *multiwatcherStore, change watcher.Change) error {
	db, closer := b.st.newDB()
	defer closer()

	c, ok := b.collectionByName[change.C]
	if !ok {
		panic(fmt.Errorf("unknown collection %q in fetch request", change.C))
	}
	col := db.C(c.Name)
	doc := reflect.New(c.infoType).Interface().(backingEntityDoc)

	// TODO(rog) investigate ways that this can be made more efficient
	// than simply fetching each entity in turn.
	// TODO(rog) avoid fetching documents that we have no interest
	// in, such as settings changes to entities we don't care about.
	err := col.FindId(change.Id).One(doc)
	if err == mgo.ErrNotFound {
		doc.removed(b.st, all, change.Id)
		return nil
	}
	if err != nil {
		return err
	}
	return doc.updated(b.st, all, change.Id)
}
