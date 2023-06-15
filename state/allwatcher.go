// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/watcher"
)

var allWatcherLogger = logger.Child("allwatcher")

// allWatcherBacking implements AllWatcherBacking by fetching entities
// for all models from the State.
type allWatcherBacking struct {
	watcher          watcher.BaseWatcher
	stPool           *StatePool
	collections      []string
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
func makeAllWatcherCollectionInfo(collNames []string) map[string]allWatcherStateCollection {
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
			// TODO: this should be a subsidiary too.
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
		case permissionsC:
			// Permissions are attached to the Model that they are for.
			collection.docType = reflect.TypeOf(backingPermission{})
			collection.subsidiary = true
		case podSpecsC:
			collection.docType = reflect.TypeOf(backingPodSpec{})
			collection.subsidiary = true
		default:
			allWatcherLogger.Criticalf("programming error: unknown collection %q", collName)
		}

		docType := collection.docType
		if _, ok := seenTypes[docType]; ok {
			allWatcherLogger.Criticalf("programming error: duplicate collection type %s", docType)
		} else {
			seenTypes[docType] = struct{}{}
		}

		if _, ok := collectionByName[collName]; ok {
			allWatcherLogger.Criticalf("programming error: duplicate collection name %q", collName)
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

func (e *backingModel) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`model "%s" updated`, ctx.id)

	// Update the context with the model type.
	ctx.modelType_ = e.Type
	info := &multiwatcher.ModelInfo{
		ModelUUID:       e.UUID,
		Type:            model.ModelType(e.Type),
		Name:            e.Name,
		Life:            life.Value(e.Life.String()),
		Owner:           e.Owner,
		ControllerUUID:  e.ControllerUUID,
		IsController:    ctx.state.IsController(),
		Cloud:           e.Cloud,
		CloudRegion:     e.CloudRegion,
		CloudCredential: e.CloudCredential,
		SLA: multiwatcher.ModelSLAInfo{
			Level: e.SLA.Level.String(),
			Owner: e.SLA.Owner,
		},
	}

	oldInfo := ctx.store.Get(info.EntityID())
	if oldInfo == nil {
		settings, err := ctx.getSettings(modelGlobalKey)
		if e.isNotFoundAndModelDead(err) {
			// Since we know this isn't in the store, stop looking for new
			// things.
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		cfg, err := config.New(config.NoDefaults, settings)
		if err != nil {
			return errors.Trace(err)
		}

		info.Config = cfg.AllAttrs()

		// Annotations are optional, so may not be there.
		info.Annotations = ctx.getAnnotations(modelGlobalKey)

		c, err := ctx.readConstraints(modelGlobalKey)
		if e.isNotFoundAndModelDead(err) {
			// Since we know this isn't in the store, stop looking for new
			// things.
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		info.Constraints = c

		info.Status, err = ctx.getStatus(modelGlobalKey, "model")
		if e.isNotFoundAndModelDead(err) {
			// Since we know this isn't in the store, stop looking for new
			// things.
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}

		permissions, err := ctx.permissionsForModel(e.UUID)
		if err != nil {
			return errors.Trace(err)
		}

		info.UserPermissions = permissions
	} else {
		oldInfo := oldInfo.(*multiwatcher.ModelInfo)
		info.Annotations = oldInfo.Annotations
		info.Config = oldInfo.Config
		info.Constraints = oldInfo.Constraints
		info.Status = oldInfo.Status
		info.UserPermissions = oldInfo.UserPermissions
	}

	ctx.store.Update(info)
	return nil
}

func (e *backingModel) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`model "%s" removed`, ctx.id)
	ctx.removeFromStore(multiwatcher.ModelKind)
	return nil
}

func (e *backingModel) mongoID() string {
	return e.UUID
}

type backingPermission permissionDoc

func (e *backingPermission) modelAndUser(id string) (string, string, bool) {
	parts := strings.Split(id, "#")

	if len(parts) < 4 {
		// Not valid for as far as we care about.
		return "", "", false
	}

	// At this stage, we are only dealing with model user permissions.
	if parts[0] != modelGlobalKey || parts[2] != userGlobalKeyPrefix {
		return "", "", false
	}
	return parts[1], parts[3], true
}

func (e *backingPermission) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`permission "%s" updated`, ctx.id)

	modelUUID, user, ok := e.modelAndUser(ctx.id)
	if !ok {
		// Not valid for as far as we care about.
		return nil
	}

	info := e.getModelInfo(ctx, modelUUID)
	if info == nil {
		return nil
	}

	// Set the access for the user in the permission map of the model.
	info.UserPermissions[user] = permission.Access(e.Access)

	ctx.store.Update(info)
	return nil
}

func (e *backingPermission) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`permission "%s" removed`, ctx.id)

	modelUUID, user, ok := e.modelAndUser(ctx.id)
	if !ok {
		// Not valid for as far as we care about.
		return nil
	}

	info := e.getModelInfo(ctx, modelUUID)
	if info == nil {
		return nil
	}

	delete(info.UserPermissions, user)

	ctx.store.Update(info)
	return nil
}

func (e *backingPermission) getModelInfo(ctx *allWatcherContext, modelUUID string) *multiwatcher.ModelInfo {
	// NOTE: we can't use the modelUUID from the ctx here because it is the
	// modelUUID of the system state.
	storeKey := &multiwatcher.ModelInfo{
		ModelUUID: modelUUID,
	}
	info0 := ctx.store.Get(storeKey.EntityID())
	switch info := info0.(type) {
	case *multiwatcher.ModelInfo:
		return info
	}
	// In all other cases, which really should be never, return nil.
	return nil
}

func (e *backingPermission) mongoID() string {
	allWatcherLogger.Criticalf("programming error: attempting to get mongoID from permissions document")
	return ""
}

type backingMachine machineDoc

func (m *backingMachine) updateAgentVersion(info *multiwatcher.MachineInfo) {
	if m.Tools != nil {
		info.AgentStatus.Version = m.Tools.Version.Number.String()
	}
}

func (m *backingMachine) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`machine "%s:%s" updated`, ctx.modelUUID, ctx.id)
	wantsVote := false
	hasVote := false
	if ctx.state.IsController() {
		// We can handle an extra query here as long as it is only for controller
		// machines. Could potentially optimize further if necessary for initial load.
		node, err := ctx.state.ControllerNode(m.Id)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		wantsVote = err == nil && node.WantsVote()
		hasVote = err == nil && node.HasVote()
	}
	modelID, _, _ := ctx.entityIDForGlobalKey(modelGlobalKey)
	modelEntity := ctx.store.Get(modelID)
	var providerType string
	if modelEntity != nil {
		modelInfo := modelEntity.(*multiwatcher.ModelInfo)
		providerType, _ = modelInfo.Config["type"].(string)

	}
	isManual := isManualMachine(m.Id, m.Nonce, providerType)
	base, err := series.ParseBase(m.Base.OS, m.Base.Channel)
	if err != nil {
		return errors.Trace(err)
	}
	info := &multiwatcher.MachineInfo{
		ModelUUID:                m.ModelUUID,
		ID:                       m.Id,
		Life:                     life.Value(m.Life.String()),
		Base:                     base.DisplayString(),
		ContainerType:            m.ContainerType,
		IsManual:                 isManual,
		Jobs:                     paramsJobsFromJobs(m.Jobs),
		SupportedContainers:      m.SupportedContainers,
		SupportedContainersKnown: m.SupportedContainersKnown,
		HasVote:                  hasVote,
		WantsVote:                wantsVote,
		PreferredPublicAddress:   m.PreferredPublicAddress.networkAddress(),
		PreferredPrivateAddress:  m.PreferredPrivateAddress.networkAddress(),
		Hostname:                 m.Hostname,
	}
	addresses := network.MergedAddresses(networkAddresses(m.MachineAddresses), networkAddresses(m.Addresses))
	for _, addr := range addresses {
		mAddr := network.ProviderAddress{
			MachineAddress: addr.MachineAddress,
		}

		spaceID := addr.SpaceID
		if spaceID != network.AlphaSpaceId && spaceID != "" {
			// TODO: cache spaces
			space, err := ctx.state.Space(spaceID)
			if err != nil {
				return errors.Annotatef(err, "retrieving space for ID %q", spaceID)
			}
			mAddr.SpaceName = network.SpaceName(space.Name())
			mAddr.ProviderSpaceID = space.ProviderId()
		}

		info.Addresses = append(info.Addresses, mAddr)
	}

	oldInfo := ctx.store.Get(info.EntityID())
	if oldInfo == nil {
		key := machineGlobalKey(m.Id)
		agentStatus, err := ctx.getStatus(key, "machine agent")
		if err != nil {
			return errors.Annotatef(err, "reading machine agent for key %s", key)
		}
		info.AgentStatus = agentStatus

		key = machineGlobalInstanceKey(m.Id)
		instanceStatus, err := ctx.getStatus(key, "machine instance")
		if err != nil {
			return errors.Annotatef(err, "reading machine instance for key %s", key)
		}
		info.InstanceStatus = instanceStatus

		// Annotations are optional, so may not be there.
		info.Annotations = ctx.getAnnotations(key)
	} else {
		// The entry already exists, so preserve the current status and
		// instance data. These will be updated as necessary as the status and instance data
		// updates come through.
		oldInfo := oldInfo.(*multiwatcher.MachineInfo)
		info.AgentStatus = oldInfo.AgentStatus
		info.Annotations = oldInfo.Annotations
		info.InstanceStatus = oldInfo.InstanceStatus
		info.InstanceID = oldInfo.InstanceID
		info.HardwareCharacteristics = oldInfo.HardwareCharacteristics
	}
	m.updateAgentVersion(info)

	// If the machine is been provisioned, fetch the instance id as required,
	// and set instance id and hardware characteristics.
	instanceData, err := ctx.getInstanceData(m.Id)
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

	ctx.store.Update(info)
	return nil
}

func (m *backingMachine) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`machine "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.MachineKind)
	return nil
}

func (m *backingMachine) mongoID() string {
	return m.Id
}

type backingInstanceData instanceData

func (i *backingInstanceData) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`instance data "%s:%s" updated`, ctx.modelUUID, ctx.id)
	parentID, _, ok := ctx.entityIDForGlobalKey(machineGlobalKey(ctx.id))
	if !ok {
		return nil
	}

	info0 := ctx.store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.MachineInfo:
		newInfo := *info
		var instanceData *instanceData = (*instanceData)(i)
		newInfo.HardwareCharacteristics = hardwareCharacteristics(*instanceData)
		newInfo.CharmProfiles = instanceData.CharmProfiles
		info0 = &newInfo
	default:
		return errors.Errorf("instanceData for unexpected entity with id %q; type %T", ctx.id, info)
	}
	ctx.store.Update(info0)
	return nil
}

func (i *backingInstanceData) removed(ctx *allWatcherContext) error {
	// If the instanceData is removed, the machine will follow not long
	// after so do nothing.
	return nil
}

func (i *backingInstanceData) mongoID() string {
	// This is a subsidiary collection, we shouldn't be calling mongoID.
	return i.MachineId
}

type backingUnit unitDoc

func (u *backingUnit) unitAndAgentStatus(ctx *allWatcherContext, info *multiwatcher.UnitInfo) error {
	unitStatusResult, err := ctx.getStatus(unitGlobalKey(u.Name), "unit")
	if err != nil {
		return errors.Trace(err)
	}

	agentStatusResult, err := ctx.getStatus(unitAgentGlobalKey(u.Name), "unit")
	if err != nil {
		return errors.Trace(err)
	}

	// NOTE: c.f. *Unit.Status(), we need to deal with the error state.
	if agentStatusResult.Current == status.Error {
		since := agentStatusResult.Since
		unitStatusResult = agentStatusResult
		agentStatusResult = multiwatcher.StatusInfo{
			Current: status.Idle,
			Data:    normaliseStatusData(nil),
			Since:   since,
		}
	}

	// Unit and workload status.
	info.WorkloadStatus = unitStatusResult
	info.AgentStatus = agentStatusResult
	return nil
}

func (u *backingUnit) updateAgentVersion(info *multiwatcher.UnitInfo) {
	if u.Tools != nil {
		info.AgentStatus.Version = u.Tools.Version.Number.String()
	}
}

func (u *backingUnit) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`unit "%s:%s" updated`, ctx.modelUUID, ctx.id)
	base, err := series.ParseBase(u.Base.OS, u.Base.Channel)
	if err != nil {
		return errors.Trace(err)
	}
	info := &multiwatcher.UnitInfo{
		ModelUUID:                u.ModelUUID,
		Name:                     u.Name,
		Application:              u.Application,
		Base:                     base.DisplayString(),
		Life:                     life.Value(u.Life.String()),
		MachineID:                u.MachineId,
		Principal:                u.Principal,
		Subordinate:              u.Principal != "",
		OpenPortRangesByEndpoint: make(network.GroupedPortRanges),
	}
	if u.CharmURL != nil {
		info.CharmURL = *u.CharmURL
	}

	// Construct a unit for the purpose of retrieving other fields as necessary.
	modelType, err := ctx.modelType()
	if err != nil {
		return errors.Annotatef(err, "get model type for %q", ctx.modelUUID)
	}
	var unitDoc unitDoc = unitDoc(*u)
	unit := newUnit(ctx.state, modelType, &unitDoc)

	oldInfo := ctx.store.Get(info.EntityID())
	if oldInfo == nil {
		allWatcherLogger.Debugf("new unit %q added to backing state", u.Name)

		// Annotations are optional, so may not be there.
		info.Annotations = ctx.getAnnotations(unitGlobalKey(u.Name))

		// We're adding the entry for the first time,
		// so fetch the associated unit status and opened ports.
		err := u.unitAndAgentStatus(ctx, info)
		if err != nil {
			return errors.Annotatef(err, "retrieve unit and agent status for %q", u.Name)
		}
		unitPortRangesByEndpoint, err := ctx.getUnitPortRangesByEndpoint(unit)
		if err != nil {
			return errors.Trace(err)
		}
		if len(unitPortRangesByEndpoint) > 0 {
			info.OpenPortRangesByEndpoint = unitPortRangesByEndpoint.Clone()
		}
		if modelType == ModelTypeCAAS {
			containerStatus, err := ctx.getStatus(globalCloudContainerKey(u.Name), "cloud container")
			if err == nil {
				info.ContainerStatus = containerStatus
			}
		}
	} else {
		// The entry already exists, so preserve the current status and ports.
		oldInfo := oldInfo.(*multiwatcher.UnitInfo)
		info.Annotations = oldInfo.Annotations
		// Unit and workload status.
		info.AgentStatus = oldInfo.AgentStatus
		info.WorkloadStatus = oldInfo.WorkloadStatus
		info.ContainerStatus = oldInfo.ContainerStatus
		info.OpenPortRangesByEndpoint = oldInfo.OpenPortRangesByEndpoint
	}

	u.updateAgentVersion(info)

	// This is horrible as we are loading the machine twice for every unit.
	// Can't optimize this yet.
	// TODO: deprecate this ASAP and remove ASAP. It is only there for backwards
	// compatibility to 1.18.
	publicAddress, privateAddress, err := ctx.getUnitAddresses(unit)
	if err != nil {
		return errors.Annotatef(err, "get addresses for %q", u.Name)
	}
	info.PublicAddress = publicAddress
	info.PrivateAddress = privateAddress
	ctx.store.Update(info)
	return nil
}

// getUnitAddresses returns the public and private addresses on a given unit.
// As of 1.18, the addresses are stored on the assigned machine but we retain
// this approach for backwards compatibility.
func (ctx *allWatcherContext) getUnitAddresses(u *Unit) (string, string, error) {
	// If we are dealing with a CAAS unit, use the unit methods, they
	// are complicated and not yet mirrored in the allwatcher. Also there
	// are entities in CAAS models that should probably be exposed up to the
	// model cache, but haven't yet.
	modelType, err := ctx.modelType()
	if err != nil {
		return "", "", errors.Annotatef(err, "get model type for %q", ctx.modelUUID)
	}
	if modelType == ModelTypeCAAS {
		publicAddress, err := u.PublicAddress()
		if err != nil {
			allWatcherLogger.Tracef("getting a public address for unit %q failed: %q", u.Name(), err)
		}
		privateAddress, err := u.PrivateAddress()
		if err != nil {
			allWatcherLogger.Tracef("getting a private address for unit %q failed: %q", u.Name(), err)
		}
		return publicAddress.Value, privateAddress.Value, nil
	}

	machineID, _ := u.AssignedMachineId()
	if machineID == "" {
		return "", "", nil
	}
	// Get the machine out of the store and use the preferred public and
	// preferred private addresses out of that.
	machineInfo := ctx.getMachineInfo(machineID)
	if machineInfo == nil {
		// We know that the machines are processed before the units, so they
		// will always be there when we are looking. Except for the case where
		// we are in the process of deleting the machine or units as they are
		// being destroyed. If this is the case, we don't really care about
		// the addresses, so returning empty values is fine.
		return "", "", nil
	}
	return machineInfo.PreferredPublicAddress.Value, machineInfo.PreferredPrivateAddress.Value, nil
}

func (u *backingUnit) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`unit "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.UnitKind)
	return nil
}

func (u *backingUnit) mongoID() string {
	return u.Name
}

type backingApplication applicationDoc

func (app *backingApplication) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`application "%s:%s" updated`, ctx.modelUUID, ctx.id)
	if app.CharmURL == nil {
		return errors.Errorf("charm url is nil")
	}
	info := &multiwatcher.ApplicationInfo{
		ModelUUID:   app.ModelUUID,
		Name:        app.Name,
		Exposed:     app.Exposed,
		CharmURL:    *app.CharmURL,
		Life:        life.Value(app.Life.String()),
		MinUnits:    app.MinUnits,
		Subordinate: app.Subordinate,
	}
	oldInfo := ctx.store.Get(info.EntityID())
	needConfig := false
	if oldInfo == nil {
		allWatcherLogger.Debugf("new application %q added to backing state", app.Name)
		key := applicationGlobalKey(app.Name)
		// Annotations are optional, so may not be there.
		info.Annotations = ctx.getAnnotations(key)
		// We're adding the entry for the first time,
		// so fetch the associated child documents.
		c, err := ctx.readConstraints(key)
		if err != nil {
			return errors.Trace(err)
		}
		info.Constraints = c
		needConfig = true
		applicationStatus, err := ctx.getStatus(key, "application")
		if err != nil {
			return errors.Annotatef(err, "reading application status for key %s", key)
		}
		info.Status = applicationStatus
		// OperatorStatus is only available for CAAS applications.
		// So if we don't find it, don't worry.
		modelType, err := ctx.modelType()
		if err != nil {
			return errors.Annotatef(err, "get model type for %q", ctx.modelUUID)
		}
		if modelType == ModelTypeCAAS {
			// Look for the PodSpec for this application.
			var doc backingPodSpec
			if err := readPodInfo(ctx.state.db(), app.Name, &doc); err != nil {
				if errors.IsNotFound(err) {
					// This is expected in some situations, there just hasn't
					// been a call to set the pod spec.
				} else {
					return errors.Annotatef(err, "get podSpec for %s", app.Name)
				}
			} else {
				info.PodSpec = doc.asPodSpec()
			}
			key = applicationGlobalOperatorKey(app.Name)
			operatorStatus, err := ctx.getStatus(key, "application operator")
			if err == nil {
				info.OperatorStatus = operatorStatus
			}
		}
	} else {
		// The entry already exists, so preserve the current status.
		appInfo := oldInfo.(*multiwatcher.ApplicationInfo)
		info.Annotations = appInfo.Annotations
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
		info.OperatorStatus = appInfo.OperatorStatus
		info.PodSpec = appInfo.PodSpec
	}
	if needConfig {
		config, err := ctx.getSettings(applicationCharmConfigKey(app.Name, app.CharmURL))
		if err != nil {
			return errors.Annotatef(err, "application %q", app.Name)
		}
		info.Config = config
	}
	ctx.store.Update(info)
	return nil
}

func (app *backingApplication) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`application "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.ApplicationKind)
	return nil
}

func (app *backingApplication) mongoID() string {
	return app.Name
}

type backingPodSpec containerSpecDoc

func (ps *backingPodSpec) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`podspec "%s:%s" updated`, ctx.modelUUID, ctx.id)

	// The id of the podspec is the application global key.
	parentID, _, ok := ctx.entityIDForGlobalKey(ctx.id)
	if !ok {
		return nil
	}
	info0 := ctx.store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore until it does.
		return nil
	case *multiwatcher.ApplicationInfo:
		newInfo := *info
		newInfo.PodSpec = ps.asPodSpec()
		info0 = &newInfo
	default:
		allWatcherLogger.Warningf("unexpected podspec type: %T", info)
		return nil
	}
	ctx.store.Update(info0)
	return nil
}

func (ps *backingPodSpec) asPodSpec() *multiwatcher.PodSpec {
	podSpec := &multiwatcher.PodSpec{
		Spec:    ps.Spec,
		Counter: ps.UpgradeCounter,
	}
	if len(podSpec.Spec) == 0 && len(ps.RawSpec) > 0 {
		podSpec.Spec = ps.RawSpec
		podSpec.Raw = true
	}
	return podSpec
}

func (ps *backingPodSpec) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`podspec "%s:%s" removed`, ctx.modelUUID, ctx.id)
	// The podSpec is only removed when the application is removed, so we don't care.
	return nil
}

func (ps *backingPodSpec) mongoID() string {
	allWatcherLogger.Criticalf("programming error: attempting to get mongoID from podspec document")
	return ""
}

type backingCharm charmDoc

func (ch *backingCharm) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`charm "%s:%s" updated`, ctx.modelUUID, ctx.id)
	info := &multiwatcher.CharmInfo{
		ModelUUID:    ch.ModelUUID,
		CharmURL:     *ch.URL,
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

	ctx.store.Update(info)
	return nil
}

func (ch *backingCharm) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`charm "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.CharmKind)
	return nil
}

func (ch *backingCharm) mongoID() string {
	_, id, ok := splitDocID(ch.DocID)
	if !ok {
		allWatcherLogger.Criticalf("charm ID not valid: %v", ch.DocID)
	}
	return id
}

func toMultiwatcherProfile(profile *LXDProfile) *multiwatcher.Profile {
	unescapedProfile := unescapeLXDProfile(profile)
	return &multiwatcher.Profile{
		Config:      unescapedProfile.Config,
		Description: unescapedProfile.Description,
		Devices:     unescapedProfile.Devices,
	}
}

type backingRemoteApplication remoteApplicationDoc

func (app *backingRemoteApplication) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`remote application "%s:%s" updated`, ctx.modelUUID, ctx.id)
	if app.Name == "" {
		return errors.Errorf("saas application name is not set")
	}
	if app.IsConsumerProxy {
		// Since this is a consumer proxy, we update the offer
		// info in this (the offering) model.
		return app.updateOfferInfo(ctx)
	}
	info := &multiwatcher.RemoteApplicationUpdate{
		ModelUUID: ctx.modelUUID, // ModelUUID not part of the remoteApplicationDoc
		Name:      app.Name,
		OfferURL:  app.URL,
		Life:      life.Value(app.Life.String()),
	}
	oldInfo := ctx.store.Get(info.EntityID())
	if oldInfo == nil {
		allWatcherLogger.Debugf("new remote application %q added to backing state", app.Name)
		// Fetch the status.
		key := remoteApplicationGlobalKey(app.Name)
		appStatus, err := ctx.getStatus(key, "saas application")
		if err != nil {
			return errors.Annotatef(err, "reading remote application status for key %s", key)
		}
		info.Status = appStatus
		allWatcherLogger.Debugf("saas application status %#v", info.Status)
	} else {
		allWatcherLogger.Debugf("use status from existing app")
		switch t := oldInfo.(type) {
		case *multiwatcher.RemoteApplicationUpdate:
			info.Status = t.Status
		default:
			allWatcherLogger.Debugf("unexpected type %t", t)
		}
	}
	ctx.store.Update(info)
	return nil
}

func (app *backingRemoteApplication) updateOfferInfo(ctx *allWatcherContext) error {
	entities := ctx.store.All()
	for _, e := range entities {
		var (
			offerInfo *multiwatcher.ApplicationOfferInfo
			ok        bool
		)
		if offerInfo, ok = e.(*multiwatcher.ApplicationOfferInfo); !ok {
			continue
		}
		if offerInfo.ModelUUID != ctx.modelUUID {
			continue
		}
		// TODO: be smarter about reading status.
		// We should only read offers relevant to the remote app that has been updated,
		// but there would be more db reads to do the filtering than just reading the
		// connection info for each offer, even if it might not have changed.
		remoteConnection, err := ctx.state.RemoteConnectionStatus(offerInfo.OfferUUID)
		if err != nil {
			return errors.Trace(err)
		}
		offerInfo.TotalConnectedCount = remoteConnection.TotalConnectionCount()
		offerInfo.ActiveConnectedCount = remoteConnection.ActiveConnectionCount()
		ctx.store.Update(offerInfo)
	}
	return nil
}

func (app *backingRemoteApplication) removed(ctx *allWatcherContext) (err error) {
	allWatcherLogger.Tracef(`remote application "%s:%s" removed`, ctx.modelUUID, ctx.id)
	// TODO: see if we need the check of consumer proxy like in the change
	if app.IsConsumerProxy {
		err = app.updateOfferInfo(ctx)
	}
	if err != nil {
		// We log the error but don't prevent the remote app removal.
		allWatcherLogger.Errorf("updating application offer info: %v", err)
	}
	ctx.removeFromStore(multiwatcher.RemoteApplicationKind)
	return err
}

func (app *backingRemoteApplication) mongoID() string {
	return app.Name
}

type backingApplicationOffer applicationOfferDoc

func (b *backingApplicationOffer) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`application offer "%s:%s" updated`, ctx.modelUUID, ctx.id)
	info := &multiwatcher.ApplicationOfferInfo{
		ModelUUID:       ctx.modelUUID, // ModelUUID not on applicationOfferDoc
		OfferName:       b.OfferName,
		OfferUUID:       b.OfferUUID,
		ApplicationName: b.ApplicationName,
	}

	// UGH, this abstraction means we are likely doing needless queries.
	offers := NewApplicationOffers(ctx.state)
	offer, err := offers.ApplicationOfferForUUID(info.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	localApp, err := ctx.state.Application(offer.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	info.ApplicationName = offer.ApplicationName
	cURL, _ := localApp.CharmURL()
	curl, err := charm.ParseURL(*cURL)
	if err != nil {
		return errors.Trace(err)
	}
	info.CharmName = curl.Name

	remoteConnection, err := ctx.state.RemoteConnectionStatus(info.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	info.TotalConnectedCount = remoteConnection.TotalConnectionCount()
	info.ActiveConnectedCount = remoteConnection.ActiveConnectionCount()

	ctx.store.Update(info)
	return nil
}

func (b *backingApplicationOffer) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`application offer "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.ApplicationOfferKind)
	return nil
}

func (b *backingApplicationOffer) mongoID() string {
	return b.OfferName
}

type backingAction actionDoc

func (a *backingAction) mongoID() string {
	_, id, ok := splitDocID(a.DocId)
	if !ok {
		allWatcherLogger.Criticalf("action ID not valid: %v", a.DocId)
	}
	return id
}

func (a *backingAction) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`action "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.ActionKind)
	return nil
}

func (a *backingAction) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`action "%s:%s" updated`, ctx.modelUUID, ctx.id)
	info := &multiwatcher.ActionInfo{
		ModelUUID:      a.ModelUUID,
		ID:             ctx.id, // local ID isn't available on the action doc
		Receiver:       a.Receiver,
		Name:           a.Name,
		Parameters:     a.Parameters,
		Parallel:       a.Parallel,
		ExecutionGroup: a.ExecutionGroup,
		Status:         string(a.Status),
		Message:        a.Message,
		Results:        a.Results,
		Enqueued:       a.Enqueued,
		Started:        a.Started,
		Completed:      a.Completed,
	}
	ctx.store.Update(info)
	return nil
}

type backingRelation relationDoc

func (r *backingRelation) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`relation "%s:%s" updated`, ctx.modelUUID, ctx.id)
	eps := make([]multiwatcher.Endpoint, len(r.Endpoints))
	for i, ep := range r.Endpoints {
		eps[i] = multiwatcher.Endpoint{
			ApplicationName: ep.ApplicationName,
			Relation:        newCharmRelation(ep.Relation),
		}
	}
	info := &multiwatcher.RelationInfo{
		ModelUUID: r.ModelUUID,
		Key:       r.Key,
		ID:        r.Id,
		Endpoints: eps,
	}
	ctx.store.Update(info)
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

func (r *backingRelation) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`relation "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.RelationKind)
	return nil
}

func (r *backingRelation) mongoID() string {
	return r.Key
}

type backingAnnotation annotatorDoc

func (a *backingAnnotation) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`annotation "%s:%s" updated`, ctx.modelUUID, ctx.id)
	info := &multiwatcher.AnnotationInfo{
		ModelUUID:   a.ModelUUID,
		Tag:         a.Tag,
		Annotations: a.Annotations,
	}
	ctx.store.Update(info)
	// Also update the annotations on the associated type.
	// When we can kill the old Watch API where annotations are separate
	// entries, we'd only update the associated type.
	parentID, _, ok := ctx.entityIDForGlobalKey(ctx.id)
	if !ok {
		return nil
	}
	info0 := ctx.store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the annotation until it does.
		return nil
	case *multiwatcher.UnitInfo:
		newInfo := *info
		newInfo.Annotations = a.Annotations
		info0 = &newInfo
	case *multiwatcher.ModelInfo:
		newInfo := *info
		newInfo.Annotations = a.Annotations
		info0 = &newInfo
	case *multiwatcher.ApplicationInfo:
		newInfo := *info
		newInfo.Annotations = a.Annotations
		info0 = &newInfo
	case *multiwatcher.MachineInfo:
		newInfo := *info
		newInfo.Annotations = a.Annotations
		info0 = &newInfo
	default:
		// We really don't care about this type yet.
		return nil
	}
	ctx.store.Update(info0)
	return nil
}

func (a *backingAnnotation) removed(ctx *allWatcherContext) error {
	// Annotations are only removed when the entity is removed.
	// So no work is needed for the assocated entity type.
	allWatcherLogger.Tracef(`annotation "%s:%s" removed`, ctx.modelUUID, ctx.id)
	// UGH, TODO, use the global key as the entity id.
	tag, ok := tagForGlobalKey(ctx.id)
	if !ok {
		return errors.Errorf("could not parse global key: %q", ctx.id)
	}
	ctx.store.Remove(multiwatcher.EntityID{
		Kind:      multiwatcher.AnnotationKind,
		ModelUUID: ctx.modelUUID,
		ID:        tag,
	})
	return nil
}

func (a *backingAnnotation) mongoID() string {
	return a.GlobalKey
}

type backingBlock blockDoc

func (a *backingBlock) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`block "%s:%s" updated`, ctx.modelUUID, ctx.id)
	info := &multiwatcher.BlockInfo{
		ModelUUID: a.ModelUUID,
		ID:        ctx.id, // ID not in the blockDoc
		Tag:       a.Tag,
		Type:      a.Type.ToParams(),
		Message:   a.Message,
	}
	ctx.store.Update(info)
	return nil
}

func (a *backingBlock) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`block "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.BlockKind)
	return nil
}

func (a *backingBlock) mongoID() string {
	_, id, ok := splitDocID(a.DocID)
	if !ok {
		allWatcherLogger.Criticalf("block ID not valid: %v", a.DocID)
	}
	return id
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

func (s *backingStatus) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`status "%s:%s" updated`, ctx.modelUUID, ctx.id)
	parentID, suffix, ok := ctx.entityIDForGlobalKey(ctx.id)
	if !ok {
		return nil
	}
	info0 := ctx.store.Get(parentID)
	// NOTE: for both the machine and the unit, where the version
	// is set in the agent status, we need to copy across the version from
	// the existing info.
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.UnitInfo:
		newInfo := *info
		switch suffix {
		case "":
			if err := s.updatedUnitAgentStatus(ctx, &newInfo); err != nil {
				return err
			}
		case "#charm":
			s.updatedUnitWorkloadStatus(ctx, &newInfo)
		case "#charm#container":
			newInfo.ContainerStatus = s.toStatusInfo()
		// I have no idea what the original author was thinking about when
		// they added "sat" as part of the key.
		case "#charm#sat#workload-version":
			// Unit workload status is only shown currently implemented on the
			// application, and the last set unit workload version is considered
			// the one that matters.
			s.updateApplicationWorkload(ctx, info)
			// No need to touch the unit for now, so we can exit the function here.
			return nil
		default:
			allWatcherLogger.Tracef("charm status suffix %q unhandled", suffix)
			return nil
		}
		info0 = &newInfo
	case *multiwatcher.ModelInfo:
		newInfo := *info
		newInfo.Status = s.toStatusInfo()
		info0 = &newInfo
	case *multiwatcher.ApplicationInfo:
		newInfo := *info
		switch suffix {
		case "#operator":
			newInfo.OperatorStatus = s.toStatusInfo()
		case "":
			newInfo.Status = s.toStatusInfo()
		default:
			allWatcherLogger.Tracef("application status suffix %q unhandled", suffix)
			return nil
		}
		info0 = &newInfo
	case *multiwatcher.RemoteApplicationUpdate:
		newInfo := *info
		newInfo.Status = s.toStatusInfo()
		info0 = &newInfo
	case *multiwatcher.MachineInfo:
		newInfo := *info
		switch suffix {
		case "#instance":
			newInfo.InstanceStatus = s.toStatusInfo()
		case "":
			// Preserve the agent version that is set on the agent status.
			agentVersion := newInfo.AgentStatus.Version
			newInfo.AgentStatus = s.toStatusInfo()
			newInfo.AgentStatus.Version = agentVersion
		default:
			allWatcherLogger.Tracef("machine status suffix %q unhandled", suffix)
			return nil
		}
		info0 = &newInfo
	default:
		return errors.Errorf("status for unexpected entity with id %q; type %T", ctx.id, info)
	}
	ctx.store.Update(info0)
	return nil
}

func (s *backingStatus) updateApplicationWorkload(ctx *allWatcherContext, unit *multiwatcher.UnitInfo) {
	// If the workload version is blank, do nothing.
	if s.StatusInfo == "" {
		return
	}
	// Lookup the application for the unit.
	appInfo := &multiwatcher.ApplicationInfo{
		ModelUUID: ctx.modelUUID,
		Name:      unit.Application,
	}
	info0 := ctx.store.Get(appInfo.EntityID())
	if info0 == nil {
		// The parent info doesn't exist. Ignore the workload until it does.
		return
	}
	appInfo = info0.(*multiwatcher.ApplicationInfo)
	updated := *appInfo
	updated.WorkloadVersion = s.StatusInfo
	ctx.store.Update(&updated)
}

func (s *backingStatus) updatedUnitAgentStatus(ctx *allWatcherContext, unitInfo *multiwatcher.UnitInfo) error {
	// Preserve the agent version that is set on the agent status.
	agentVersion := unitInfo.AgentStatus.Version

	// If the agent status used to be "error", then the units workload status will be error.
	if unitInfo.WorkloadStatus.Current == status.Error {
		// If we were in an error state, and continue in an error state, just update the workload
		// status with the new error info.
		if s.Status == status.Error {
			unitInfo.WorkloadStatus = s.toStatusInfo()
			return nil
		}
		// Agent is coming out of error status, so we need to read the workload status.
		workloadStatus, err := ctx.getStatus(unitGlobalKey(unitInfo.Name), "unit")
		if err != nil {
			return err
		}
		unitInfo.WorkloadStatus = workloadStatus
		unitInfo.AgentStatus = s.toStatusInfo()
		unitInfo.AgentStatus.Version = agentVersion
		return nil
	}
	// We weren't in an error state before.
	if s.Status != status.Error {
		// We aren't entering an error state, so just update the value.
		unitInfo.AgentStatus = s.toStatusInfo()
		unitInfo.AgentStatus.Version = agentVersion
		return nil
	}
	// Otherwise we need to do the annoying dance with errors.
	errorState := s.toStatusInfo()
	unitInfo.WorkloadStatus = errorState
	unitInfo.AgentStatus = multiwatcher.StatusInfo{
		Current: status.Idle,
		Data:    normaliseStatusData(nil),
		Since:   errorState.Since,
		Version: agentVersion,
	}
	return nil
}

func (s *backingStatus) updatedUnitWorkloadStatus(ctx *allWatcherContext, unitInfo *multiwatcher.UnitInfo) {
	// If we aren't in an error state, update the workload.
	if unitInfo.WorkloadStatus.Current != status.Error {
		unitInfo.WorkloadStatus = s.toStatusInfo()
	}
	// We don't need to look up the current value of the agent status
	// because any change to that will cause the unit to be updated again,
	// and it will read the workload status if needed.
}

func (s *backingStatus) removed(ctx *allWatcherContext) error {
	// If the status is removed, the parent will follow not long after,
	// so do nothing.
	return nil
}

func (s *backingStatus) mongoID() string {
	allWatcherLogger.Criticalf("programming error: attempting to get mongoID from status document")
	return ""
}

type backingConstraints constraintsDoc

func (c *backingConstraints) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`constraints "%s:%s" updated`, ctx.modelUUID, ctx.id)
	parentID, _, ok := ctx.entityIDForGlobalKey(ctx.id)
	if !ok {
		return nil
	}
	info0 := ctx.store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the constraints until it does.
		return nil
	case *multiwatcher.UnitInfo, *multiwatcher.MachineInfo:
		// We don't (yet) publish unit or machine constraints.
		return nil
	case *multiwatcher.ModelInfo:
		newInfo := *info
		newInfo.Constraints = constraintsDoc(*c).value()
		info0 = &newInfo
	case *multiwatcher.ApplicationInfo:
		newInfo := *info
		newInfo.Constraints = constraintsDoc(*c).value()
		info0 = &newInfo
	default:
		return errors.Errorf("constraints for unexpected entity with id %q; type %T", ctx.id, info)
	}
	ctx.store.Update(info0)
	return nil
}

func (c *backingConstraints) removed(ctx *allWatcherContext) error {
	return nil
}

func (c *backingConstraints) mongoID() string {
	allWatcherLogger.Criticalf("programming error: attempting to get mongoID from constraints document")
	return ""
}

type backingSettings settingsDoc

func (s *backingSettings) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`settings "%s:%s" updated`, ctx.modelUUID, ctx.id)
	parentID, url, ok := ctx.entityIDForSettingsKey(ctx.id)
	if !ok {
		return nil
	}
	info0 := ctx.store.Get(parentID)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *multiwatcher.ModelInfo:
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
	ctx.store.Update(info0)
	return nil
}

func (s *backingSettings) removed(ctx *allWatcherContext) error {
	// Settings docs are only removed when the principal doc is removed. Nothing to do here.
	return nil
}

func (s *backingSettings) mongoID() string {
	allWatcherLogger.Criticalf("programming error: attempting to get mongoID from settings document")
	return ""
}

type backingOpenedPorts map[string]interface{}

func (p *backingOpenedPorts) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`opened ports "%s:%s" updated`, ctx.modelUUID, ctx.id)
	parentID, ok := ctx.entityIDForOpenedPortsKey(ctx.id)
	if !ok {
		return nil
	}
	switch info := ctx.store.Get(parentID).(type) {
	case nil:
		// The parent info doesn't exist. This is unexpected because the port
		// always refers to a machine. Anyway, ignore the ports for now.
		return nil
	case *multiwatcher.MachineInfo:
		// Retrieve the units placed in the machine.
		units, err := ctx.state.UnitsFor(info.ID)
		if err != nil {
			return errors.Trace(err)
		}
		// Update the ports on all units assigned to the machine.
		for _, u := range units {
			if err := updateUnitPorts(ctx, u); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (p *backingOpenedPorts) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`opened ports "%s:%s" removed`, ctx.modelUUID, ctx.id)
	// This magic is needed as an open ports doc may be removed if all
	// open ports on the subnet are removed.
	parentID, ok := ctx.entityIDForOpenedPortsKey(ctx.id)
	if !ok {
		return nil
	}
	switch info := ctx.store.Get(parentID).(type) {
	case nil:
		// The parent info doesn't exist. This is unexpected because the port
		// always refers to a machine. Anyway, ignore the ports for now.
		return nil
	case *multiwatcher.MachineInfo:
		// Retrieve the units placed in the machine.
		units, err := ctx.state.UnitsFor(info.ID)
		if err != nil {
			// An error isn't returned here because the watcher is
			// always acting a little behind reality. It is reasonable
			// that entities have been deleted from State but we're
			// still seeing events related to them from the watcher.
			allWatcherLogger.Errorf("cannot retrieve units for %q: %v", info.ID, err)
			return nil
		}
		// Update the ports on all units assigned to the machine.
		for _, u := range units {
			if err := updateUnitPorts(ctx, u); err != nil {
				allWatcherLogger.Errorf("cannot update unit ports for %q: %v", u.Name(), err)
			}
		}
	}
	return nil
}

func (p *backingOpenedPorts) mongoID() string {
	allWatcherLogger.Criticalf("programming error: attempting to get mongoID from openedPorts document")
	return ""
}

// updateUnitPorts updates the PortRanges info of the given unit.
func updateUnitPorts(ctx *allWatcherContext, u *Unit) error {
	eid, _, ok := ctx.entityIDForGlobalKey(u.globalKey())
	if !ok {
		// This should never happen.
		return errors.New("cannot retrieve entity id for unit")
	}
	switch oldInfo := ctx.store.Get(eid).(type) {
	case nil:
		// The unit info doesn't exist. This is unlikely to happen, but ignore
		// the status until a unitInfo is included in the store.
		return nil
	case *multiwatcher.UnitInfo:
		unitPortRangesByEndpoint, err := ctx.getUnitPortRangesByEndpoint(u)
		if err != nil {
			return errors.Trace(err)
		}
		unitInfo := *oldInfo
		unitInfo.OpenPortRangesByEndpoint = unitPortRangesByEndpoint.Clone()
		ctx.store.Update(&unitInfo)
	default:
		return nil
	}
	return nil
}

type backingGeneration generationDoc

func (g *backingGeneration) updated(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`generation "%s:%s" updated`, ctx.modelUUID, ctx.id)
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
		ModelUUID:     g.ModelUUID,
		ID:            ctx.id, // Id not stored on the doc.
		Name:          g.Name,
		AssignedUnits: assigned,
		Config:        cfg,
		Created:       g.Created,
		CreatedBy:     g.CreatedBy,
		Completed:     g.Completed,
		CompletedBy:   g.CompletedBy,
		GenerationID:  g.GenerationId,
	}
	ctx.store.Update(info)
	return nil

}

func (g *backingGeneration) removed(ctx *allWatcherContext) error {
	allWatcherLogger.Tracef(`branch "%s:%s" removed`, ctx.modelUUID, ctx.id)
	ctx.removeFromStore(multiwatcher.BranchKind)
	return nil
}

func (g *backingGeneration) mongoID() string {
	_, id, ok := splitDocID(g.DocId)
	if !ok {
		allWatcherLogger.Criticalf("charm ID not valid: %v", g.DocId)
	}
	return id
}

// backingEntityDoc is implemented by the documents in
// collections that the allWatcherStateBacking watches.
type backingEntityDoc interface {
	// updated is called when the document has changed.
	// The mongo _id value of the document is provided in id.
	updated(ctx *allWatcherContext) error

	// removed is called when the document has changed.
	// The receiving instance will not contain any data.
	//
	// The mongo _id value of the document is provided in id.
	//
	// In some cases st may be nil. If the implementation requires st
	// then it should do nothing.
	removed(ctx *allWatcherContext) error

	// mongoID returns the localID of the document.
	// It is currently never called for subsidiary documents.
	mongoID() string
}

// AllWatcherBacking is the interface required by the multiwatcher to access the
// underlying state.
type AllWatcherBacking interface {
	// GetAll retrieves information about all information
	// known to the Backing and stashes it in the Store.
	GetAll(multiwatcher.Store) error

	// Changed informs the backing about a change received
	// from a watcher channel.  The backing is responsible for
	// updating the Store to reflect the change.
	Changed(multiwatcher.Store, watcher.Change) error

	// Watch watches for any changes and sends them
	// on the given channel.
	Watch(chan<- watcher.Change)

	// Unwatch stops watching for changes on the
	// given channel.
	Unwatch(chan<- watcher.Change)
}

// NewAllWatcherBacking creates a backing object that watches
// all the models in the controller for changes that are fed through
// the multiwatcher infrastructure.
func NewAllWatcherBacking(pool *StatePool) (AllWatcherBacking, error) {
	collectionNames := []string{
		// The ordering here matters. We want to load machines, then
		// applications, then units. The others don't matter so much.
		modelsC,
		machinesC,
		applicationsC,
		unitsC,
		// The rest don't really matter.
		actionsC,
		annotationsC,
		applicationOffersC,
		blocksC,
		charmsC,
		constraintsC,
		generationsC,
		instanceDataC,
		openedPortsC,
		permissionsC,
		relationsC,
		remoteApplicationsC,
		statusesC,
		settingsC,
		// And for CAAS we need to watch these...
		podSpecsC,
	}
	collectionMap := makeAllWatcherCollectionInfo(collectionNames)
	controllerState, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &allWatcherBacking{
		watcher:          controllerState.workers.txnLogWatcher(),
		stPool:           pool,
		collections:      collectionNames,
		collectionByName: collectionMap,
	}, nil
}

// Watch watches all the collections.
func (b *allWatcherBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.watcher.WatchCollection(c.name, in)
	}
}

// Unwatch unwatches all the collections.
func (b *allWatcherBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.watcher.UnwatchCollection(c.name, in)
	}
}

// GetAll fetches all items that we want to watch from the state.
func (b *allWatcherBacking) GetAll(store multiwatcher.Store) error {
	systemState, err := b.stPool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	modelUUIDs, err := systemState.AllModelUUIDs()
	if err != nil {
		return errors.Annotate(err, "error loading models")
	}
	for _, modelUUID := range modelUUIDs {
		if err := b.loadAllWatcherEntitiesForModel(modelUUID, store); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (b *allWatcherBacking) loadAllWatcherEntitiesForModel(modelUUID string, store multiwatcher.Store) error {
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

	err = loadAllWatcherEntities(st.State, b.collections, b.collectionByName, store)
	if err != nil {
		return errors.Annotatef(err, "error loading entities for model %v", modelUUID)
	}
	return nil
}

// Changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allWatcherBacking) Changed(store multiwatcher.Store, change watcher.Change) error {
	c, ok := b.collectionByName[change.C]
	if !ok {
		return errors.Errorf("unknown collection %q in fetch request", change.C)
	}

	modelUUID, id, err := b.idForChange(change)
	if err != nil {
		return errors.Trace(err)
	}

	doc := reflect.New(c.docType).Interface().(backingEntityDoc)

	systemState, err := b.stPool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	ctx := &allWatcherContext{
		// In order to have a valid state instance, use the controller model initially.
		state:     systemState,
		store:     store,
		modelUUID: modelUUID,
		id:        id,
	}

	st, err := b.getState(modelUUID)
	if err != nil {
		// The state pool will return a not found error if the model is
		// in the process of being removed.
		if errors.IsNotFound(err) {
			// The entity's model is gone so remove the entity from the store.
			_ = doc.removed(ctx)
			return nil
		}
		return errors.Trace(err) // prioritise getState error
	}
	defer st.Release()
	// Update the state in the context to be the valid one from the state pool.
	ctx.state = st.State

	col, closer := st.db().GetCollection(c.name)
	defer closer()

	err = col.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		err := doc.removed(ctx)
		return errors.Trace(err)
	}
	if err != nil {
		return err
	}
	return doc.updated(ctx)
}

func (b *allWatcherBacking) idForChange(change watcher.Change) (string, string, error) {
	if change.C == modelsC {
		modelUUID := change.Id.(string)
		return modelUUID, modelUUID, nil
	} else if change.C == permissionsC {
		// All permissions can just load using the system state.
		systemState, err := b.stPool.SystemState()
		if err != nil {
			return "", "", errors.Trace(err)
		}
		modelUUID := systemState.ModelUUID()
		return modelUUID, change.Id.(string), nil
	}

	modelUUID, id, ok := splitDocID(change.Id.(string))
	if !ok {
		return "", "", errors.Errorf("unknown id format: %v", change.Id.(string))
	}
	return modelUUID, id, nil
}

func (b *allWatcherBacking) getState(modelUUID string) (*PooledState, error) {
	st, err := b.stPool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func loadAllWatcherEntities(st *State, loadOrder []string, collectionByName map[string]allWatcherStateCollection, store multiwatcher.Store) error {
	// Use a single new MongoDB connection for all the work here.
	db, closer := st.newDB()
	defer closer()
	start := st.clock().Now()
	defer func() {
		elapsed := st.clock().Now().Sub(start)
		allWatcherLogger.Infof("allwatcher loaded for model %q in %s", st.ModelUUID(), elapsed)
	}()

	ctx := &allWatcherContext{
		state:     st,
		store:     store,
		modelUUID: st.ModelUUID(),
	}
	// TODO(thumper): make it multimodel aware
	if err := ctx.loadSubsidiaryCollections(); err != nil {
		return errors.Annotate(err, "loading subsidiary collections")
	}

	for _, name := range loadOrder {
		c, found := collectionByName[name]
		if !found {
			allWatcherLogger.Criticalf("programming error, collection %q not found in map", name)
			continue
		}
		if c.subsidiary {
			continue
		}
		if err := loadOneWatcherEntity(ctx, db, st.ModelUUID(), c.docType, c.name); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func loadOneWatcherEntity(ctx *allWatcherContext, db Database, modelUUID string, docType reflect.Type, name string) error {
	col, closer := db.GetCollection(name)
	defer closer()
	infoSlicePtr := reflect.New(reflect.SliceOf(docType))

	// models is a global collection so need to filter on UUID.
	var filter bson.M
	if name == modelsC {
		filter = bson.M{"_id": modelUUID}
	}
	query := col.Find(filter)
	// Units are ordered so we load the subordinates first.
	if name == unitsC {
		// Subordinates have a principal, so will sort after the
		// empty string, which is what principal units have.
		query = query.Sort("principal")
	}
	if err := query.All(infoSlicePtr.Interface()); err != nil {
		return errors.Errorf("cannot get all %s: %v", name, err)
	}
	infos := infoSlicePtr.Elem()
	for i := 0; i < infos.Len(); i++ {
		info := infos.Index(i).Addr().Interface().(backingEntityDoc)
		ctx.id = info.mongoID()
		err := info.updated(ctx)
		if err != nil {
			return errors.Annotatef(err, "failed to initialise backing for %s:%v", name, ctx.id)
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

type allWatcherContext struct {
	state     *State
	store     multiwatcher.Store
	modelUUID string
	id        string

	modelType_ ModelType

	settings    map[string]*settingsDoc
	annotations map[string]map[string]string
	constraints map[string]constraints.Value
	statuses    map[string]status.StatusInfo
	instances   map[string]instanceData
	// A map of the existing MachinePortRanges where the keys are machine IDs.
	openPortRangesForMachine map[string]MachinePortRanges
	// A map of the existing ApplicationPortRanges where the keys are application names.
	openPortRangesForApplication map[string]ApplicationPortRanges
	userAccess                   map[string]map[string]permission.Access
}

func (ctx *allWatcherContext) loadSubsidiaryCollections() error {
	if err := ctx.loadSettings(); err != nil {
		return errors.Annotatef(err, "cache settings")
	}
	if err := ctx.loadAnnotations(); err != nil {
		return errors.Annotatef(err, "cache annotations")
	}
	if err := ctx.loadConstraints(); err != nil {
		return errors.Annotatef(err, "cache constraints")
	}
	if err := ctx.loadStatuses(); err != nil {
		return errors.Annotatef(err, "cache statuses")
	}
	if err := ctx.loadInstanceData(); err != nil {
		return errors.Annotatef(err, "cache instance data")
	}
	if err := ctx.loadOpenedPortRanges(); err != nil {
		return errors.Annotatef(err, "cache opened ports")
	}
	if err := ctx.loadPermissions(); err != nil {
		return errors.Annotatef(err, "permissions")
	}
	return nil
}

func (ctx *allWatcherContext) loadSettings() error {
	col, closer := ctx.state.db().GetCollection(settingsC)
	defer closer()

	var docs []settingsDoc
	if err := col.Find(nil).All(&docs); err != nil {
		return errors.Annotate(err, "cannot read all settings")
	}

	ctx.settings = make(map[string]*settingsDoc)
	for _, doc := range docs {
		docCopy := doc
		ctx.settings[doc.DocID] = &docCopy
	}

	return nil
}

func (ctx *allWatcherContext) loadAnnotations() error {
	col, closer := ctx.state.db().GetCollection(annotationsC)
	defer closer()

	var docs []annotatorDoc
	if err := col.Find(nil).All(&docs); err != nil {
		return errors.Annotate(err, "cannot read all annotations")
	}

	ctx.annotations = make(map[string]map[string]string)
	for _, doc := range docs {
		key := ensureModelUUID(doc.ModelUUID, doc.GlobalKey)
		ctx.annotations[key] = doc.Annotations
	}

	return nil
}

func (ctx *allWatcherContext) loadStatuses() error {
	col, closer := ctx.state.db().GetCollection(statusesC)
	defer closer()

	var docs []statusDocWithID
	if err := col.Find(nil).All(&docs); err != nil {
		return errors.Annotate(err, "cannot read all statuses")
	}

	ctx.statuses = make(map[string]status.StatusInfo)
	for _, doc := range docs {
		ctx.statuses[doc.ID] = doc.asStatusInfo()
	}

	return nil
}

func (ctx *allWatcherContext) loadInstanceData() error {
	col, closer := ctx.state.db().GetCollection(instanceDataC)
	defer closer()

	var docs []instanceData
	if err := col.Find(nil).All(&docs); err != nil {
		return errors.Annotate(err, "cannot read all instance data")
	}

	ctx.instances = make(map[string]instanceData)
	for _, doc := range docs {
		docCopy := doc
		ctx.instances[doc.DocID] = docCopy
	}

	return nil
}

func (ctx *allWatcherContext) loadOpenedPortRanges() error {
	openedMachineRanges, err := getOpenedPortRangesForAllMachines(ctx.state)
	if err != nil {
		return errors.Annotate(err, "cannot read all opened port ranges")
	}
	ctx.openPortRangesForMachine = make(map[string]MachinePortRanges)
	for _, mpr := range openedMachineRanges {
		ctx.openPortRangesForMachine[mpr.MachineID()] = mpr
	}

	openedApplicationRanges, err := getOpenedApplicationPortRangesForAllApplications(ctx.state)
	if err != nil {
		return errors.Annotate(err, "cannot read all opened port ranges")
	}
	ctx.openPortRangesForApplication = make(map[string]ApplicationPortRanges)
	for _, mpr := range openedApplicationRanges {
		ctx.openPortRangesForApplication[mpr.ApplicationName()] = mpr
	}

	return nil
}

func (ctx *allWatcherContext) loadPermissions() error {
	col, closer := ctx.state.db().GetCollection(permissionsC)
	defer closer()

	var docs []backingPermission
	if err := col.Find(nil).All(&docs); err != nil {
		return errors.Annotate(err, "cannot read all permissions")
	}

	ctx.userAccess = make(map[string]map[string]permission.Access)
	for _, doc := range docs {
		modelUUID, user, ok := doc.modelAndUser(doc.ID)
		if !ok {
			continue
		}
		modelPermissions := ctx.userAccess[modelUUID]
		if modelPermissions == nil {
			modelPermissions = make(map[string]permission.Access)
			ctx.userAccess[modelUUID] = modelPermissions
		}
		modelPermissions[user] = permission.Access(doc.Access)
	}

	return nil
}

func (ctx *allWatcherContext) loadConstraints() error {
	col, closer := ctx.state.db().GetCollection(constraintsC)
	defer closer()

	var docs []constraintsDoc
	if err := col.Find(nil).All(&docs); err != nil {
		return errors.Errorf("cannot read all constraints")
	}

	ctx.constraints = make(map[string]constraints.Value)
	for _, doc := range docs {
		ctx.constraints[doc.DocID] = doc.value()
	}

	return nil
}

func (ctx *allWatcherContext) removeFromStore(kind string) {
	ctx.store.Remove(multiwatcher.EntityID{
		Kind:      kind,
		ModelUUID: ctx.modelUUID,
		ID:        ctx.id,
	})
}

func (ctx *allWatcherContext) getAnnotations(key string) map[string]string {
	gKey := ensureModelUUID(ctx.modelUUID, key)
	if ctx.annotations != nil {
		// It is entirely possible and fine for there to be no annotations.
		return ctx.annotations[gKey]
	}

	annotations, closer := ctx.state.db().GetCollection(annotationsC)
	defer closer()

	var doc annotatorDoc
	err := annotations.FindId(gKey).One(&doc)
	if err != nil {
		// We really don't care what the error is. Anything substantial
		// will be caught by other queries.
		return nil
	}
	return doc.Annotations
}

func (ctx *allWatcherContext) getSettings(key string) (map[string]interface{}, error) {
	var doc *settingsDoc
	var err error
	if ctx.settings != nil {
		gKey := ensureModelUUID(ctx.modelUUID, key)
		cDoc, found := ctx.settings[gKey]
		if !found {
			return nil, errors.NotFoundf("settings doc %q", gKey)
		}
		doc = cDoc
	} else {
		doc, err = readSettingsDoc(ctx.state.db(), settingsC, key)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	// The copyMap does the key translation for dots and dollars.
	settings := copyMap(doc.Settings, nil)
	return settings, nil
}

func (ctx *allWatcherContext) readConstraints(key string) (constraints.Value, error) {
	if ctx.constraints != nil {
		gKey := ensureModelUUID(ctx.modelUUID, key)
		value, found := ctx.constraints[gKey]
		if !found {
			return constraints.Value{}, errors.NotFoundf("constraints %q", gKey)
		}
		return value, nil
	}
	value, err := readConstraints(ctx.state, key)
	return value, err
}

func (ctx *allWatcherContext) getStatus(key, badge string) (multiwatcher.StatusInfo, error) {
	var modelStatus status.StatusInfo
	var err error
	if ctx.statuses != nil {
		gKey := ensureModelUUID(ctx.modelUUID, key)
		cached, found := ctx.statuses[gKey]
		if found {
			modelStatus = cached
		} else {
			err = errors.NotFoundf("status doc %q", gKey)
		}
	} else {
		modelStatus, err = getStatus(ctx.state.db(), key, badge)
	}
	if err != nil {
		return multiwatcher.StatusInfo{}, errors.Trace(err)
	}
	return multiwatcher.StatusInfo{
		Current: modelStatus.Status,
		Message: modelStatus.Message,
		Data:    normaliseStatusData(modelStatus.Data),
		Since:   modelStatus.Since,
	}, nil
}

func (ctx *allWatcherContext) getInstanceData(id string) (instanceData, error) {
	if ctx.instances != nil {
		gKey := ensureModelUUID(ctx.modelUUID, id)
		cached, found := ctx.instances[gKey]
		if found {
			return cached, nil
		} else {
			return instanceData{}, errors.NotFoundf("instance data for machine %v", id)
		}
	}
	return getInstanceData(ctx.state, id)
}

func (ctx *allWatcherContext) permissionsForModel(uuid string) (map[string]permission.Access, error) {
	if ctx.userAccess != nil {
		return ctx.userAccess[uuid], nil
	}
	permissions, err := ctx.state.usersPermissions(modelKey(uuid))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]permission.Access)
	for _, perm := range permissions {
		user := userIDFromGlobalKey(perm.doc.SubjectGlobalKey)
		if user == perm.doc.SubjectGlobalKey {
			// Not a user subject
			continue
		}
		result[user] = perm.access()
	}
	return result, nil
}

func (ctx *allWatcherContext) getUnitPortRangesByEndpoint(unit *Unit) (network.GroupedPortRanges, error) {
	if unit.ShouldBeAssigned() {
		machineID, err := unit.AssignedMachineId()
		if err != nil {
			if errors.IsNotAssigned(err) {
				// Not assigned, so there won't be any ports opened.
				// Return an empty port map (see Bug #1425435).
				return make(network.GroupedPortRanges), nil
			}
			return nil, errors.Trace(err)
		}
		// No cached port ranges available; make a direct DB lookup instead.
		if ctx.openPortRangesForMachine == nil || ctx.openPortRangesForMachine[machineID] == nil {
			unitPortRanges, err := unit.OpenedPortRanges()
			return unitPortRanges.ByEndpoint(), err
		}
		return ctx.openPortRangesForMachine[machineID].ForUnit(unit.Name()).ByEndpoint(), nil
	}

	appName := unit.ApplicationName()
	if ctx.openPortRangesForApplication == nil || ctx.openPortRangesForApplication[appName] == nil {
		unitPortRanges, err := unit.OpenedPortRanges()
		if err != nil {
			if errors.Is(err, errors.NotSupported) {
				// We support open/close port for CAAS sidecar applications but not operator applications.
				return make(network.GroupedPortRanges), nil
			}
			return nil, errors.Trace(err)
		}
		return unitPortRanges.ByEndpoint(), nil
	}
	return ctx.openPortRangesForApplication[appName].ForUnit(unit.Name()).ByEndpoint(), nil
}

// entityIDForGlobalKey returns the entity id for the given global key.
// It returns false if the key is not recognized.
func (ctx *allWatcherContext) entityIDForGlobalKey(key string) (multiwatcher.EntityID, string, bool) {
	var result multiwatcher.EntityInfo
	if key == modelGlobalKey {
		result = &multiwatcher.ModelInfo{
			ModelUUID: ctx.modelUUID,
		}
		return result.EntityID(), "", true
	} else if len(key) < 3 || key[1] != '#' {
		return multiwatcher.EntityID{}, "", false
	}
	// NOTE: we should probably have a single place where we have all the
	// global key functions, so we can check coverage both ways.
	parts := strings.Split(key, "#")
	id := parts[1]
	suffix := strings.Join(parts[2:], "#")
	if len(suffix) > 0 {
		suffix = "#" + suffix
	}
	switch parts[0] {
	case "m":
		result = &multiwatcher.MachineInfo{
			ModelUUID: ctx.modelUUID,
			ID:        id,
		}
	case "u":
		result = &multiwatcher.UnitInfo{
			ModelUUID: ctx.modelUUID,
			Name:      id,
		}
	case "a":
		result = &multiwatcher.ApplicationInfo{
			ModelUUID: ctx.modelUUID,
			Name:      id,
		}
	case "c":
		result = &multiwatcher.RemoteApplicationUpdate{
			ModelUUID: ctx.modelUUID,
			Name:      id,
		}
	default:
		return multiwatcher.EntityID{}, "", false
	}
	return result.EntityID(), suffix, true
}

func (ctx *allWatcherContext) modelType() (ModelType, error) {
	if ctx.modelType_ != modelTypeNone {
		return ctx.modelType_, nil
	}
	model, err := ctx.state.Model()
	if err != nil {
		return modelTypeNone, errors.Trace(err)
	}
	ctx.modelType_ = model.Type()
	return ctx.modelType_, nil
}

// entityIDForSettingsKey returns the entity id for the given
// settings key. Any extra information in the key is returned in
// extra.
func (ctx *allWatcherContext) entityIDForSettingsKey(key string) (multiwatcher.EntityID, string, bool) {
	if !strings.HasPrefix(key, "a#") {
		eid, _, ok := ctx.entityIDForGlobalKey(key)
		return eid, "", ok
	}
	key = key[2:]
	i := strings.Index(key, "#")
	if i == -1 {
		return multiwatcher.EntityID{}, "", false
	}
	info := &multiwatcher.ApplicationInfo{
		ModelUUID: ctx.modelUUID,
		Name:      key[0:i],
	}
	extra := key[i+1:]
	return info.EntityID(), extra, true
}

// entityIDForOpenedPortsKey returns the entity id for the given
// openedPorts key. Any extra information in the key is discarded.
func (ctx *allWatcherContext) entityIDForOpenedPortsKey(key string) (multiwatcher.EntityID, bool) {
	id, _, known := ctx.entityIDForGlobalKey(machineGlobalKey(key))
	return id, known
}

func (ctx *allWatcherContext) getMachineInfo(machineID string) *multiwatcher.MachineInfo {
	storeKey := &multiwatcher.MachineInfo{
		ModelUUID: ctx.modelUUID,
		ID:        machineID,
	}
	info0 := ctx.store.Get(storeKey.EntityID())
	switch info := info0.(type) {
	case *multiwatcher.MachineInfo:
		return info
	}
	// In all other cases, which really should be never, return nil.
	return nil
}
