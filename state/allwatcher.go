// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/watcher"
)

// allWatcherStateBacking implements Backing by fetching entities for
// a single model from the State.
type allWatcherStateBacking struct {
	st               *State
	watcher          watcher.BaseWatcher
	collectionByName map[string]allWatcherStateCollection
}

// allModelWatcherStateBacking implements Backing by fetching entities
// for all models from the State.
type allModelWatcherStateBacking struct {
	st               *State
	watcher          watcher.BaseWatcher
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
		case instanceDataC:
			collection.docType = reflect.TypeOf(backingInstanceData{})
			collection.subsidiary = true
		case unitsC:
			collection.docType = reflect.TypeOf(backingUnit{})
		case applicationsC:
			collection.docType = reflect.TypeOf(backingApplication{})
		case charmsC:
			collection.docType = reflect.TypeOf(backingCharm{})
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
		case remoteApplicationsC:
			collection.docType = reflect.TypeOf(backingRemoteApplication{})
		case applicationOffersC:
			collection.docType = reflect.TypeOf(backingApplicationOffer{})
		case generationsC:
			collection.docType = reflect.TypeOf(backingGeneration{})
		default:
			logger.Criticalf("programming error: unknown collection %q", collName)
		}

		docType := collection.docType
		if _, ok := seenTypes[docType]; ok {
			logger.Criticalf("programming error: duplicate collection type %s", docType)
		} else {
			seenTypes[docType] = struct{}{}
		}

		if _, ok := collectionByName[collName]; ok {
			logger.Criticalf("programming error: duplicate collection name %q", collName)
		} else {
			collectionByName[collName] = collection
		}
	}

	return collectionByName
}

type backingModel modelDoc

func (e *backingModel) isNotFoundAndModelDead(err error) bool {
	// Return true if the error is not found and the model is dead.
	// This will be the case if the model has been marked dead, pending cleanup.
	return errors.IsNotFound(err) && e.Life == Dead
}

func (e *backingModel) updated(st *State, store *multiwatcherStore, id string) error {
	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	cfg, err := m.ModelConfig()
	if e.isNotFoundAndModelDead(err) {
		// Treat it as if the model is removed.
		return e.removed(store, e.UUID, e.UUID, st)
	}
	if err != nil {
		return errors.Trace(err)
	}
	info := &multiwatcher.ModelUpdate{
		ModelUUID:      e.UUID,
		Name:           e.Name,
		Life:           life.Value(e.Life.String()),
		Owner:          e.Owner,
		ControllerUUID: e.ControllerUUID,
		IsController:   st.IsController(),
		Config:         cfg.AllAttrs(),
		SLA: multiwatcher.ModelSLAInfo{
			Level: e.SLA.Level.String(),
			Owner: e.SLA.Owner,
		},
	}
	c, err := readConstraints(st, modelGlobalKey)
	// Treat it as if the model is removed.
	if e.isNotFoundAndModelDead(err) {
		return e.removed(store, e.UUID, e.UUID, st)
	}
	if err != nil {
		return errors.Trace(err)
	}
	info.Constraints = c
	modelStatus, err := getStatus(st.db(), modelGlobalKey, "model")
	if e.isNotFoundAndModelDead(err) {
		// Treat it as if the model is removed.
		return e.removed(store, e.UUID, e.UUID, st)
	}
	if err != nil {
		return errors.Trace(err)
	}
	info.Status = multiwatcher.StatusInfo{
		Current: modelStatus.Status,
		Message: modelStatus.Message,
		Data:    normaliseStatusData(modelStatus.Data),
		Since:   modelStatus.Since,
	}
	store.Update(info)
	return nil
}

func (e *backingModel) removed(store *multiwatcherStore, modelUUID, _ string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.ModelKind,
		ModelUUID: modelUUID,
		ID:        modelUUID,
	})
	return nil
}

func (e *backingModel) mongoID() string {
	return e.UUID
}

type backingMachine machineDoc

func (m *backingMachine) updateAgentVersion(entity Entity, info *multiwatcher.MachineInfo) error {
	if agentTooler, ok := entity.(AgentTooler); ok {
		t, err := agentTooler.AgentTools()
		if err != nil && !errors.IsNotFound(err) {
			return errors.Annotatef(err, "retrieving agent tools for machine %q", m.Id)
		}
		if t != nil {
			info.AgentStatus.Version = t.Version.Number.String()
		}
	}
	return nil
}

func (m *backingMachine) machineAndAgentStatus(entity Entity, info *multiwatcher.MachineInfo) error {
	machine, ok := entity.(status.StatusGetter)
	if !ok {
		return errors.Errorf("the given entity does not support Status %v", entity)
	}

	agentStatus, err := machine.Status()
	if err != nil {
		return errors.Annotatef(err, "retrieving agent status for machine %q", m.Id)
	}
	info.AgentStatus = multiwatcher.NewStatusInfo(agentStatus, nil)

	inst, ok := machine.(status.InstanceStatusGetter)
	if !ok {
		return errors.Errorf("the given entity does not support InstanceStatus %v", entity)
	}
	instanceStatusResult, err := inst.InstanceStatus()
	if err != nil {
		return errors.Annotatef(err, "retrieving instance status for machine %q", m.Id)
	}
	info.InstanceStatus = multiwatcher.NewStatusInfo(instanceStatusResult, nil)
	return nil
}

func (m *backingMachine) updated(st *State, store *multiwatcherStore, id string) error {
	node, err := st.ControllerNode(m.Id)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	wantsVote := err == nil && node.WantsVote()
	hasVote := err == nil && node.HasVote()
	info := &multiwatcher.MachineInfo{
		ModelUUID:                st.ModelUUID(),
		ID:                       m.Id,
		Life:                     life.Value(m.Life.String()),
		Series:                   m.Series,
		ContainerType:            m.ContainerType,
		Jobs:                     paramsJobsFromJobs(m.Jobs),
		SupportedContainers:      m.SupportedContainers,
		SupportedContainersKnown: m.SupportedContainersKnown,
		HasVote:                  hasVote,
		WantsVote:                wantsVote,
	}
	addresses := network.MergedAddresses(networkAddresses(m.MachineAddresses), networkAddresses(m.Addresses))
	for _, addr := range addresses {
		mAddr := network.ProviderAddress{
			MachineAddress: addr.MachineAddress,
		}

		spaceID := addr.SpaceID
		if spaceID != network.AlphaSpaceId && spaceID != "" {
			space, err := st.Space(spaceID)
			if err != nil {
				return errors.Annotatef(err, "retrieving space for ID %q", spaceID)
			}
			mAddr.SpaceName = network.SpaceName(space.Name())
			mAddr.ProviderSpaceID = network.Id(space.ProviderId())
		}

		info.Addresses = append(info.Addresses, mAddr)
	}
	// fetch the associated machine.
	entity, err := st.FindEntity(names.NewMachineTag(m.Id))
	if err != nil {
		return errors.Annotatef(err, "retrieving machine %q", m.Id)
	}
	oldInfo := store.Get(info.EntityID())
	if oldInfo == nil {
		err := m.machineAndAgentStatus(entity, info)
		if err != nil {
			return errors.Annotatef(err, "retrieve machine and agent status for %q", m.Id)
		}
	} else {
		// The entry already exists, so preserve the current status and
		// instance data.
		oldInfo := oldInfo.(*multiwatcher.MachineInfo)
		info.AgentStatus = oldInfo.AgentStatus
		info.InstanceStatus = oldInfo.InstanceStatus
		info.InstanceID = oldInfo.InstanceID
		info.HardwareCharacteristics = oldInfo.HardwareCharacteristics
	}
	// try to update agent version
	err = m.updateAgentVersion(entity, info)
	if err != nil {
		return errors.Annotatef(err, "retrieve agent version for machine %q", m.Id)
	}

	// If the machine is been provisioned, fetch the instance id as required,
	// and set instance id and hardware characteristics.
	instanceData, err := getInstanceData(st, m.Id)
	if err == nil {
		if m.Nonce != "" && info.InstanceID == "" {
			info.InstanceID = string(instanceData.InstanceId)
			info.HardwareCharacteristics = hardwareCharacteristics(instanceData)
		}
		// InstanceMutater needs the liveliness of the instanceData.CharmProfiles
		// as this changes with charm-upgrades
		info.CharmProfiles = instanceData.CharmProfiles
	} else if !errors.IsNotFound(err) {
		return err
	}

	store.Update(info)
	return nil
}

func (m *backingMachine) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.MachineKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (m *backingMachine) mongoID() string {
	return m.DocID
}

type backingInstanceData instanceData

func (i *backingInstanceData) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIDForGlobalKey(st.ModelUUID(), machineGlobalKey(id))
	if !ok {
		return nil
	}

	info0 := store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.MachineInfo:
		newInfo := *info
		instanceData, err := getInstanceData(st, newInfo.ID)
		if err != nil {
			return err
		}
		newInfo.HardwareCharacteristics = hardwareCharacteristics(instanceData)
		newInfo.CharmProfiles = instanceData.CharmProfiles
		info0 = &newInfo
	default:
		return errors.Errorf("instanceData for unexpected entity with id %q; type %T", id, info)
	}
	store.Update(info0)
	return nil
}

func (i *backingInstanceData) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	// If the instanceData is removed, the machine will follow not long
	// after so do nothing.
	return nil
}

func (i *backingInstanceData) mongoID() string {
	return i.DocID
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

func (u *backingUnit) unitAndAgentStatus(unit *Unit, info *multiwatcher.UnitInfo) error {
	unitStatusResult, err := unit.Status()
	if err != nil {
		return errors.Trace(err)
	}
	agentStatusResult, err := unit.AgentStatus()
	if err != nil {
		return errors.Trace(err)
	}
	// Unit and workload status.
	info.WorkloadStatus = multiwatcher.NewStatusInfo(unitStatusResult, nil)
	info.AgentStatus = multiwatcher.NewStatusInfo(agentStatusResult, nil)
	return nil
}

func (u *backingUnit) updateAgentVersion(unit *Unit, info *multiwatcher.UnitInfo) error {
	t, err := unit.AgentTools()
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotatef(err, "retrieving agent tools for unit %q", u.Name)
	}
	if t != nil {
		info.AgentStatus.Version = t.Version.Number.String()
	}
	return nil
}

func (u *backingUnit) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.UnitInfo{
		ModelUUID:   st.ModelUUID(),
		Name:        u.Name,
		Application: u.Application,
		Series:      u.Series,
		Life:        life.Value(u.Life.String()),
		MachineID:   u.MachineId,
		Principal:   u.Principal,
		Subordinate: u.Principal != "",
	}
	if u.CharmURL != nil {
		info.CharmURL = u.CharmURL.String()
	}

	// fetch the associated unit to get possible updated status.
	unit, err := st.Unit(u.Name)
	if err != nil {
		return errors.Annotatef(err, "get unit %q", u.Name)
	}

	oldInfo := store.Get(info.EntityID())
	if oldInfo == nil {
		logger.Debugf("new unit %q added to backing state", u.Name)
		// We're adding the entry for the first time,
		// so fetch the associated unit status and opened ports.
		err := u.unitAndAgentStatus(unit, info)
		if err != nil {
			return errors.Annotatef(err, "retrieve unit and agent status for %q", u.Name)
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
		// Unit and workload status.
		info.AgentStatus = oldInfo.AgentStatus
		info.WorkloadStatus = oldInfo.WorkloadStatus
		info.Ports = oldInfo.Ports
		info.PortRanges = oldInfo.PortRanges
	}

	// try to update agent version
	err = u.updateAgentVersion(unit, info)
	if err != nil {
		return errors.Annotatef(err, "retrieve agent version for unit %q", u.Name)
	}

	publicAddress, privateAddress, err := getUnitAddresses(unit)
	if err != nil {
		return errors.Annotatef(err, "get addresses for %q", u.Name)
	}
	info.PublicAddress = publicAddress
	info.PrivateAddress = privateAddress
	store.Update(info)
	return nil
}

// getUnitAddresses returns the public and private addresses on a given unit.
// As of 1.18, the addresses are stored on the assigned machine but we retain
// this approach for backwards compatibility.
func getUnitAddresses(u *Unit) (string, string, error) {
	publicAddress, err := u.PublicAddress()
	if err != nil {
		logger.Infof("getting a public address for unit %q failed: %q", u.Name(), err)
	}
	privateAddress, err := u.PrivateAddress()
	if err != nil {
		logger.Infof("getting a private address for unit %q failed: %q", u.Name(), err)
	}
	return publicAddress.Value, privateAddress.Value, nil
}

func (u *backingUnit) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.UnitKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (u *backingUnit) mongoID() string {
	return u.DocID
}

type backingApplication applicationDoc

func (app *backingApplication) updated(st *State, store *multiwatcherStore, id string) error {
	if app.CharmURL == nil {
		return errors.Errorf("charm url is nil")
	}
	info := &multiwatcher.ApplicationInfo{
		ModelUUID:   st.ModelUUID(),
		Name:        app.Name,
		Exposed:     app.Exposed,
		CharmURL:    app.CharmURL.String(),
		Life:        life.Value(app.Life.String()),
		MinUnits:    app.MinUnits,
		Subordinate: app.Subordinate,
	}
	oldInfo := store.Get(info.EntityID())
	needConfig := false
	if oldInfo == nil {
		logger.Debugf("new application %q added to backing state", app.Name)
		key := applicationGlobalKey(app.Name)
		// We're adding the entry for the first time,
		// so fetch the associated child documents.
		c, err := readConstraints(st, key)
		if err != nil {
			return errors.Trace(err)
		}
		info.Constraints = c
		needConfig = true
		applicationStatus, err := getStatus(st.db(), key, "application")
		if err != nil {
			return errors.Annotatef(err, "reading application status for key %s", key)
		}

		info.Status = multiwatcher.StatusInfo{
			Current: applicationStatus.Status,
			Message: applicationStatus.Message,
			Data:    normaliseStatusData(applicationStatus.Data),
			Since:   applicationStatus.Since,
		}
	} else {
		// The entry already exists, so preserve the current status.
		appInfo := oldInfo.(*multiwatcher.ApplicationInfo)
		info.Constraints = appInfo.Constraints
		info.WorkloadVersion = appInfo.WorkloadVersion
		if info.CharmURL == appInfo.CharmURL {
			// The charm URL remains the same - we can continue to
			// use the same config settings.
			info.Config = appInfo.Config
		} else {
			// The charm URL has changed - we need to fetch the
			// settings from the new charm's settings doc.
			needConfig = true
		}
		info.Status = appInfo.Status
	}
	if needConfig {
		doc, err := readSettingsDoc(st.db(), settingsC, applicationCharmConfigKey(app.Name, app.CharmURL))
		if err != nil {
			return errors.Annotatef(err, "application %q", app.Name)
		}
		info.Config = doc.Settings
	}
	store.Update(info)
	return nil
}

func (app *backingApplication) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.ApplicationKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (app *backingApplication) mongoID() string {
	return app.DocID
}

type backingCharm charmDoc

func (ch *backingCharm) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.CharmInfo{
		ModelUUID:    st.ModelUUID(),
		CharmURL:     ch.URL.String(),
		CharmVersion: ch.CharmVersion,
		Life:         life.Value(ch.Life.String()),
	}

	if ch.LXDProfile != nil && !ch.LXDProfile.Empty() {
		info.LXDProfile = toMultiwatcherProfile(ch.LXDProfile)
	}

	if ch.Config != nil {
		if ds := ch.Config.DefaultSettings(); len(ds) > 0 {
			info.DefaultConfig = ds
		}
	}

	store.Update(info)
	return nil
}

func (ch *backingCharm) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.CharmKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (ch *backingCharm) mongoID() string {
	return ch.DocID
}

func toMultiwatcherProfile(profile *charm.LXDProfile) *multiwatcher.Profile {
	unescapedProfile := unescapeLXDProfile(profile)
	return &multiwatcher.Profile{
		Config:      unescapedProfile.Config,
		Description: unescapedProfile.Description,
		Devices:     unescapedProfile.Devices,
	}
}

type backingRemoteApplication remoteApplicationDoc

func (app *backingRemoteApplication) updated(st *State, store *multiwatcherStore, id string) error {
	if app.Name == "" {
		return errors.Errorf("remote application name is not set")
	}
	if app.IsConsumerProxy {
		// Since this is a consumer proxy, we update the offer
		// info in this (the offering) model.
		return app.updateOfferInfo(st, store)
	}
	info := &multiwatcher.RemoteApplicationUpdate{
		ModelUUID: st.ModelUUID(),
		Name:      app.Name,
		OfferUUID: app.OfferUUID,
		OfferURL:  app.URL,
		Life:      life.Value(app.Life.String()),
	}
	oldInfo := store.Get(info.EntityID())
	if oldInfo == nil {
		logger.Debugf("new remote application %q added to backing state", app.Name)
		// Fetch the status.
		key := remoteApplicationGlobalKey(app.Name)
		appStatus, err := getStatus(st.db(), key, "remote application")
		if err != nil {
			return errors.Annotatef(err, "reading remote application status for key %s", key)
		}
		info.Status = multiwatcher.StatusInfo{
			Current: appStatus.Status,
			Message: appStatus.Message,
			Data:    normaliseStatusData(appStatus.Data),
			Since:   appStatus.Since,
		}
		logger.Debugf("remote application status %#v", info.Status)
	} else {
		logger.Debugf("use status from existing app")
		switch t := oldInfo.(type) {
		case *multiwatcher.RemoteApplicationUpdate:
			info.Status = t.Status
		default:
			logger.Debugf("unexpected type %t", t)
		}
	}
	store.Update(info)
	return nil
}

func (app *backingRemoteApplication) updateOfferInfo(st *State, store *multiwatcherStore) error {
	// Remote Applications reference an offer using the offer UUID.
	// Offers in the store use offer name as the id key, so we need
	// to look through the store entities to find any matching offer.
	var offerInfo *multiwatcher.ApplicationOfferInfo
	entities := store.All()
	for _, e := range entities {
		var ok bool
		if offerInfo, ok = e.(*multiwatcher.ApplicationOfferInfo); ok {
			if offerInfo.OfferUUID != app.OfferUUID {
				offerInfo = nil
				continue
			}
			break
		}
	}
	// If we have an existing remote application,
	// adjust any offer info also.
	if offerInfo == nil {
		return nil
	}
	remoteConnection, err := st.RemoteConnectionStatus(offerInfo.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	offerInfo.TotalConnectedCount = remoteConnection.TotalConnectionCount()
	offerInfo.ActiveConnectedCount = remoteConnection.ActiveConnectionCount()
	store.Update(offerInfo)
	return nil
}

func (app *backingRemoteApplication) removed(store *multiwatcherStore, modelUUID, id string, st *State) (err error) {
	err = app.updateOfferInfo(st, store)
	if err != nil {
		// We log the error but don't prevent the remote app removal.
		logger.Errorf("updating application offer info: %v", err)
	}
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.RemoteApplicationKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return err
}

func (app *backingRemoteApplication) mongoID() string {
	return app.DocID
}

type backingApplicationOffer applicationOfferDoc

func (offer *backingApplicationOffer) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.ApplicationOfferInfo{
		ModelUUID:       st.ModelUUID(),
		OfferName:       offer.OfferName,
		OfferUUID:       offer.OfferUUID,
		ApplicationName: offer.ApplicationName,
	}
	err := updateOfferInfo(st, info)
	if err != nil {
		return errors.Annotatef(err, "reading application offer details for %s", offer.OfferName)
	}
	store.Update(info)
	return nil
}

func updateOfferInfo(st *State, offerInfo *multiwatcher.ApplicationOfferInfo) error {
	offers := NewApplicationOffers(st)
	offer, err := offers.ApplicationOfferForUUID(offerInfo.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	localApp, err := st.Application(offer.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	curl, _ := localApp.CharmURL()
	offerInfo.ApplicationName = offer.ApplicationName
	offerInfo.CharmName = curl.Name

	remoteConnection, err := st.RemoteConnectionStatus(offerInfo.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	offerInfo.TotalConnectedCount = remoteConnection.TotalConnectionCount()
	offerInfo.ActiveConnectedCount = remoteConnection.ActiveConnectionCount()
	return nil
}

func (offer *backingApplicationOffer) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.ApplicationOfferKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (offer *backingApplicationOffer) mongoID() string {
	return offer.DocID
}

type backingAction actionDoc

func (a *backingAction) mongoID() string {
	return a.DocId
}

func (a *backingAction) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.ActionKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (a *backingAction) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.ActionInfo{
		ModelUUID:  st.ModelUUID(),
		ID:         id,
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
			ApplicationName: ep.ApplicationName,
			Relation:        newCharmRelation(ep.Relation),
		}
	}
	info := &multiwatcher.RelationInfo{
		ModelUUID: st.ModelUUID(),
		Key:       r.Key,
		ID:        r.Id,
		Endpoints: eps,
	}
	store.Update(info)
	return nil
}

// newCharmRelation creates a new local CharmRelation structure from the
// charm.Relation structure. NOTE: when we update the database to not store a
// charm.Relation directly in the database, this method should take the state
// structure type.
func newCharmRelation(cr charm.Relation) multiwatcher.CharmRelation {
	return multiwatcher.CharmRelation{
		Name:      cr.Name,
		Role:      string(cr.Role),
		Interface: cr.Interface,
		Optional:  cr.Optional,
		Limit:     cr.Limit,
		Scope:     string(cr.Scope),
	}
}

func (r *backingRelation) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.RelationKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (r *backingRelation) mongoID() string {
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
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.AnnotationKind,
		ModelUUID: modelUUID,
		ID:        tag,
	})
	return nil
}

func (a *backingAnnotation) mongoID() string {
	return a.GlobalKey
}

type backingBlock blockDoc

func (a *backingBlock) updated(st *State, store *multiwatcherStore, id string) error {
	info := &multiwatcher.BlockInfo{
		ModelUUID: st.ModelUUID(),
		ID:        id,
		Tag:       a.Tag,
		Type:      a.Type.ToParams(),
		Message:   a.Message,
	}
	store.Update(info)
	return nil
}

func (a *backingBlock) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.BlockKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (a *backingBlock) mongoID() string {
	return a.DocID
}

type backingStatus statusDoc

func (s *backingStatus) toStatusInfo() multiwatcher.StatusInfo {
	return multiwatcher.StatusInfo{
		Current: s.Status,
		Message: s.StatusInfo,
		Data:    s.StatusData,
		Since:   unixNanoToTime(s.Updated),
	}
}

func (s *backingStatus) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIDForGlobalKey(st.ModelUUID(), id)
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
		statusInfo, err := getStatus(st.db(), unitGlobalKey(newInfo.Name), "unit")
		if err != nil {
			return err
		}
		if err := s.updatedUnitStatus(st, store, id, statusInfo, &newInfo); err != nil {
			return err
		}
		info0 = &newInfo
	case *multiwatcher.ModelUpdate:
		newInfo := *info
		newInfo.Status = s.toStatusInfo()
		info0 = &newInfo
	case *multiwatcher.ApplicationInfo:
		newInfo := *info
		newInfo.Status = s.toStatusInfo()
		info0 = &newInfo
	case *multiwatcher.RemoteApplicationUpdate:
		newInfo := *info
		newInfo.Status = s.toStatusInfo()
		info0 = &newInfo
	case *multiwatcher.MachineInfo:
		newInfo := *info
		// lets disambiguate between juju machine agent and provider instance statuses.
		if strings.HasSuffix(id, "#instance") {
			newInfo.InstanceStatus = s.toStatusInfo()
		} else {
			newInfo.AgentStatus = s.toStatusInfo()
		}
		info0 = &newInfo
	default:
		return errors.Errorf("status for unexpected entity with id %q; type %T", id, info)
	}
	store.Update(info0)
	return nil
}

func (s *backingStatus) updatedUnitStatus(st *State, store *multiwatcherStore, id string, unitStatus status.StatusInfo, newInfo *multiwatcher.UnitInfo) error {
	// Unit or workload status - display the agent status or any error.
	// NOTE: thumper 2016-06-27, this is truly horrible, and we are lying to our users.
	// however, this is explicitly what has been asked for as much as we dislike it.
	if strings.HasSuffix(id, "#charm") || s.Status == status.Error {
		newInfo.WorkloadStatus = s.toStatusInfo()
	} else {
		newInfo.AgentStatus = s.toStatusInfo()
		// If the unit was in error and now it's not, we need to reset its
		// status back to what was previously recorded.
		if newInfo.WorkloadStatus.Current == status.Error {
			newInfo.WorkloadStatus.Current = unitStatus.Status
			newInfo.WorkloadStatus.Message = unitStatus.Message
			newInfo.WorkloadStatus.Data = unitStatus.Data
			newInfo.WorkloadStatus.Since = unixNanoToTime(s.Updated)
		}
	}

	// Retrieve the unit.
	unit, err := st.Unit(newInfo.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			// It is possible that the event processing is happening slower
			// than reality and a missing unit isn't that terrible.
			logger.Debugf("unit %q not in DB", newInfo.Name)
			return nil
		}
		return errors.Annotatef(err, "cannot retrieve unit %q", newInfo.Name)
	}
	// A change in a unit's status might also affect its application.
	application, err := unit.Application()
	if err != nil {
		if errors.IsNotFound(err) {
			// It is possible that the event processing is happening slower
			// than reality and a missing application isn't that terrible.
			logger.Debugf("application for unit %q not in DB", newInfo.Name)
			return nil
		}
		return errors.Trace(err)
	}
	applicationId, ok := backingEntityIDForGlobalKey(st.ModelUUID(), application.globalKey())
	if !ok {
		return nil
	}
	applicationInfo := store.Get(applicationId)
	if applicationInfo == nil {
		return nil
	}
	status, err := application.Status()
	if err != nil {
		return errors.Trace(err)
	}
	newApplicationInfo := *applicationInfo.(*multiwatcher.ApplicationInfo)
	newApplicationInfo.Status = multiwatcher.NewStatusInfo(status, nil)
	workloadVersion, err := unit.WorkloadVersion()
	if err != nil {
		return errors.Annotatef(err, "cannot retrieve workload version for %q", unit.Name())
	} else if workloadVersion != "" {
		newApplicationInfo.WorkloadVersion = workloadVersion
	}
	store.Update(&newApplicationInfo)
	return nil
}

func (s *backingStatus) removed(*multiwatcherStore, string, string, *State) error {
	// If the status is removed, the parent will follow not long after,
	// so do nothing.
	return nil
}

func (s *backingStatus) mongoID() string {
	panic("cannot find mongo id from status document")
}

type backingConstraints constraintsDoc

func (c *backingConstraints) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIDForGlobalKey(st.ModelUUID(), id)
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
	case *multiwatcher.ModelUpdate:
		newInfo := *info
		newInfo.Constraints = constraintsDoc(*c).value()
		info0 = &newInfo
	case *multiwatcher.ApplicationInfo:
		newInfo := *info
		newInfo.Constraints = constraintsDoc(*c).value()
		info0 = &newInfo
	default:
		return errors.Errorf("constraints for unexpected entity with id %q; type %T", id, info)
	}
	store.Update(info0)
	return nil
}

func (c *backingConstraints) removed(*multiwatcherStore, string, string, *State) error {
	return nil
}

func (c *backingConstraints) mongoID() string {
	panic("cannot find mongo id from constraints document")
}

type backingSettings settingsDoc

func (s *backingSettings) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, url, ok := backingEntityIDForSettingsKey(st.ModelUUID(), id)
	if !ok {
		return nil
	}
	info0 := store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.ModelUpdate:
		// We need to construct a model config so that coercion
		// of raw settings values occurs.
		cfg, err := config.New(config.NoDefaults, s.Settings)
		if err != nil {
			return errors.Trace(err)
		}
		newInfo := *info
		newInfo.Config = cfg.AllAttrs()
		info0 = &newInfo
	case *multiwatcher.ApplicationInfo:
		// If we're seeing settings for the application with a different
		// charm URL, we ignore them - we will fetch
		// them again when the application charm changes.
		// By doing this we make sure that the settings in the
		// ApplicationInfo are always consistent with the charm URL.
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
	parentID, url, ok := backingEntityIDForSettingsKey(modelUUID, id)
	if !ok {
		// Application is already gone along with its settings.
		return nil
	}
	parent := store.Get(parentID)
	if info, ok := parent.(*multiwatcher.ApplicationInfo); ok {
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

func (s *backingSettings) mongoID() string {
	panic("cannot find mongo id from settings document")
}

// backingEntityIDForSettingsKey returns the entity id for the given
// settings key. Any extra information in the key is returned in
// extra.
func backingEntityIDForSettingsKey(modelUUID, key string) (eid multiwatcher.EntityID, extra string, ok bool) {
	if !strings.HasPrefix(key, "a#") {
		eid, ok = backingEntityIDForGlobalKey(modelUUID, key)
		return
	}
	key = key[2:]
	i := strings.Index(key, "#")
	if i == -1 {
		return multiwatcher.EntityID{}, "", false
	}
	eid = (&multiwatcher.ApplicationInfo{
		ModelUUID: modelUUID,
		Name:      key[0:i],
	}).EntityID()
	extra = key[i+1:]
	ok = true
	return
}

type backingOpenedPorts map[string]interface{}

func (p *backingOpenedPorts) updated(st *State, store *multiwatcherStore, id string) error {
	parentID, ok := backingEntityIDForOpenedPortsKey(st.ModelUUID(), id)
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
		units, err := st.UnitsFor(info.ID)
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
	parentID, ok := backingEntityIDForOpenedPortsKey(st.ModelUUID(), id)
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
		units, err := st.UnitsFor(info.ID)
		if err != nil {
			// An error isn't returned here because the watcher is
			// always acting a little behind reality. It is reasonable
			// that entities have been deleted from State but we're
			// still seeing events related to them from the watcher.
			logger.Errorf("cannot retrieve units for %q: %v", info.ID, err)
			return nil
		}
		// Update the ports on all units assigned to the machine.
		for _, u := range units {
			if err := updateUnitPorts(st, store, u); err != nil {
				logger.Errorf("cannot update unit ports for %q: %v", u.Name(), err)
			}
		}
	}
	return nil
}

func (p *backingOpenedPorts) mongoID() string {
	logger.Criticalf("programming error: attempting to get mongoID from openedPorts document")
	return ""
}

// updateUnitPorts updates the Ports and PortRanges info of the given unit.
func updateUnitPorts(st *State, store *multiwatcherStore, u *Unit) error {
	eid, ok := backingEntityIDForGlobalKey(st.ModelUUID(), u.globalKey())
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

// backingEntityIDForOpenedPortsKey returns the entity id for the given
// openedPorts key. Any extra information in the key is discarded.
func backingEntityIDForOpenedPortsKey(modelUUID, key string) (multiwatcher.EntityID, bool) {
	parts, err := extractPortsIDParts(key)
	if err != nil {
		logger.Debugf("cannot parse ports key %q: %v", key, err)
		return multiwatcher.EntityID{}, false
	}
	return backingEntityIDForGlobalKey(modelUUID, machineGlobalKey(parts[1]))
}

// backingEntityIDForGlobalKey returns the entity id for the given global key.
// It returns false if the key is not recognized.
func backingEntityIDForGlobalKey(modelUUID, key string) (multiwatcher.EntityID, bool) {
	if key == modelGlobalKey {
		return (&multiwatcher.ModelUpdate{
			ModelUUID: modelUUID,
		}).EntityID(), true
	} else if len(key) < 3 || key[1] != '#' {
		return multiwatcher.EntityID{}, false
	}
	id := key[2:]
	switch key[0] {
	case 'm':
		id = strings.TrimSuffix(id, "#instance")
		return (&multiwatcher.MachineInfo{
			ModelUUID: modelUUID,
			ID:        id,
		}).EntityID(), true
	case 'u':
		id = strings.TrimSuffix(id, "#charm")
		return (&multiwatcher.UnitInfo{
			ModelUUID: modelUUID,
			Name:      id,
		}).EntityID(), true
	case 'a':
		return (&multiwatcher.ApplicationInfo{
			ModelUUID: modelUUID,
			Name:      id,
		}).EntityID(), true
	case 'c':
		return (&multiwatcher.RemoteApplicationUpdate{
			ModelUUID: modelUUID,
			Name:      id,
		}).EntityID(), true
	default:
		return multiwatcher.EntityID{}, false
	}
}

type backingGeneration generationDoc

func (g *backingGeneration) updated(st *State, store *multiwatcherStore, id string) error {
	// Convert the state representation of config deltas
	// to the multiwatcher representation.
	var cfg map[string][]multiwatcher.ItemChange
	if len(g.Config) > 0 {
		cfg = make(map[string][]multiwatcher.ItemChange, len(g.Config))
		for app, deltas := range g.Config {
			d := make([]multiwatcher.ItemChange, len(deltas))
			for i, delta := range deltas {
				d[i] = multiwatcher.ItemChange{
					Type:     delta.Type,
					Key:      delta.Key,
					OldValue: delta.OldValue,
					NewValue: delta.NewValue,
				}
			}
			cfg[app] = d
		}
	}

	// Make a copy of the AssignedUnits map.
	assigned := make(map[string][]string, len(g.AssignedUnits))
	for k, v := range g.AssignedUnits {
		units := make([]string, len(v))
		copy(units, v)
		assigned[k] = units
	}

	info := &multiwatcher.BranchInfo{
		ModelUUID:     st.ModelUUID(),
		ID:            st.localID(id),
		Name:          g.Name,
		AssignedUnits: assigned,
		Config:        cfg,
		Created:       g.Created,
		CreatedBy:     g.CreatedBy,
		Completed:     g.Completed,
		CompletedBy:   g.CompletedBy,
		GenerationID:  g.GenerationId,
	}
	store.Update(info)
	return nil

}

func (g *backingGeneration) removed(store *multiwatcherStore, modelUUID, id string, _ *State) error {
	store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.BranchKind,
		ModelUUID: modelUUID,
		ID:        id,
	})
	return nil
}

func (g *backingGeneration) mongoID() string {
	return g.DocId
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

	// mongoID returns the mongo _id field of the document.
	// It is currently never called for subsidiary documents.
	mongoID() string
}

func newAllWatcherStateBacking(st *State, params WatchParams) Backing {
	collectionNames := []string{
		actionsC,
		annotationsC,
		applicationsC,
		blocksC,
		charmsC,
		constraintsC,
		generationsC,
		instanceDataC,
		machinesC,
		openedPortsC,
		relationsC,
		remoteApplicationsC,
		statusesC,
		settingsC,
		unitsC,
	}
	if params.IncludeOffers {
		collectionNames = append(collectionNames, applicationOffersC)
	}
	collections := makeAllWatcherCollectionInfo(collectionNames...)
	return &allWatcherStateBacking{
		st:               st,
		watcher:          st.workers.txnLogWatcher(),
		collectionByName: collections,
	}
}

func (b *allWatcherStateBacking) filterModel(docID interface{}) bool {
	_, err := b.st.strictLocalID(docID.(string))
	return err == nil
}

// Watch watches all the collections.
func (b *allWatcherStateBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.watcher.WatchCollectionWithFilter(c.name, in, b.filterModel)
	}
}

// Unwatch unwatches all the collections.
func (b *allWatcherStateBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.watcher.UnwatchCollection(c.name, in)
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
	col, closer := b.st.db().GetCollection(c.name)
	defer closer()
	doc := reflect.New(c.docType).Interface().(backingEntityDoc)

	id := b.st.localID(change.Id.(string))

	// TODO(rog) investigate ways that this can be made more efficient
	// than simply fetching each entity in turn.
	// TODO(rog) avoid fetching documents that we have no interest
	// in, such as settings changes to entities we don't care about.
	err := col.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		logger.Criticalf("%s in %s not found", id, c.name)
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

func NewAllModelWatcherStateBacking(st *State, pool *StatePool) Backing {
	collections := makeAllWatcherCollectionInfo(
		annotationsC,
		applicationsC,
		charmsC,
		constraintsC,
		generationsC,
		instanceDataC,
		modelsC,
		machinesC,
		openedPortsC,
		relationsC,
		remoteApplicationsC,
		statusesC,
		settingsC,
		unitsC,
	)
	return &allModelWatcherStateBacking{
		st:               st,
		watcher:          st.workers.txnLogWatcher(),
		stPool:           pool,
		collectionByName: collections,
	}
}

// Watch watches all the collections.
func (b *allModelWatcherStateBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.watcher.WatchCollection(c.name, in)
	}
}

// Unwatch unwatches all the collections.
func (b *allModelWatcherStateBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.watcher.UnwatchCollection(c.name, in)
	}
}

// GetAll fetches all items that we want to watch from the state.
func (b *allModelWatcherStateBacking) GetAll(all *multiwatcherStore) error {
	modelUUIDs, err := b.st.AllModelUUIDs()
	if err != nil {
		return errors.Annotate(err, "error loading models")
	}
	for _, modelUUID := range modelUUIDs {
		if err := b.loadAllWatcherEntitiesForModel(modelUUID, all); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (b *allModelWatcherStateBacking) loadAllWatcherEntitiesForModel(modelUUID string, all *multiwatcherStore) error {
	st, err := b.stPool.Get(modelUUID)
	if err != nil {
		if errors.IsNotFound(err) {
			// This can occur if the model has been destroyed since
			// the moment when model uuid has been retrieved.
			// If we cannot find the model in the above call,
			// we do not want to err out and we do not want to proceed
			// with this call - just leave.
			return nil
		}
		return errors.Trace(err)
	}
	defer st.Release()

	err = loadAllWatcherEntities(st.State, b.collectionByName, all)
	if err != nil {
		return errors.Annotatef(err, "error loading entities for model %v", modelUUID)
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

	st, err := b.getState(modelUUID)
	if err != nil {
		// The state pool will return a not found error if the model is
		// in the process of being removed.
		if errors.IsNotFound(err) {
			// The entity's model is gone so remove the entity from the store.
			_ = doc.removed(all, modelUUID, id, nil)
			return nil
		}
		return errors.Trace(err) // prioritise getState error
	}
	defer st.Release()

	col, closer := st.db().GetCollection(c.name)
	defer closer()

	// TODO - see TODOs in allWatcherStateBacking.Changed()
	err = col.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		err := doc.removed(all, modelUUID, id, st.State)
		return errors.Trace(err)
	}
	if err != nil {
		return err
	}
	return doc.updated(st.State, all, id)
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

func (b *allModelWatcherStateBacking) getState(modelUUID string) (*PooledState, error) {
	st, err := b.stPool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// Release implements the Backing interface.
func (b *allModelWatcherStateBacking) Release() error {
	// Nothing to release.
	return nil
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

		// models is a global collection so need to filter on UUID.
		var filter bson.M
		if c.name == modelsC {
			filter = bson.M{"_id": st.ModelUUID()}
		}

		if err := col.Find(filter).All(infoSlicePtr.Interface()); err != nil {
			return errors.Errorf("cannot get all %s: %v", c.name, err)
		}
		infos := infoSlicePtr.Elem()
		for i := 0; i < infos.Len(); i++ {
			info := infos.Index(i).Addr().Interface().(backingEntityDoc)
			id := info.mongoID()
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
