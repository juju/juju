// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	mgoutils "github.com/juju/juju/internal/mongo/utils"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/tools"
	stateerrors "github.com/juju/juju/state/errors"
)

// unitAgentGlobalKey returns the global database key for the named unit.
func unitAgentGlobalKey(name string) string {
	return unitAgentGlobalKeyPrefix + name
}

// unitAgentGlobalKeyPrefix is the string we use to denote unit agent kind.
const unitAgentGlobalKeyPrefix = "u#"

var unitLogger = internallogger.GetLogger("juju.state.unit")

// MachineRef is a reference to a machine, without being a full machine.
// This exists to allow us to use state functions without requiring a
// state.Machine, without having to require a real machine.
type MachineRef interface {
	DocID() string
	Id() string
	MachineTag() names.MachineTag
	Life() Life
	Clean() bool
	ContainerType() instance.ContainerType
	Base() Base
	Jobs() []MachineJob
	AddPrincipal(string)
	FileSystems() []string
}

// unitDoc represents the internal state of a unit in MongoDB.
// Note the correspondence with UnitInfo in core/multiwatcher.
type unitDoc struct {
	DocID                  string `bson:"_id"`
	Name                   string `bson:"name"`
	ModelUUID              string `bson:"model-uuid"`
	Base                   Base   `bson:"base"`
	Application            string
	CharmURL               *string
	Principal              string
	Subordinates           []string
	StorageAttachmentCount int `bson:"storageattachmentcount"`
	MachineId              string
	Tools                  *tools.Tools `bson:",omitempty"`
	Life                   Life
	PasswordHash           string
}

// Unit represents the state of an application unit.
type Unit struct {
	st  *State
	doc unitDoc

	// Cache the model type as it is immutable as is referenced
	// during the lifecycle of the unit.
	modelType ModelType
}

func newUnit(st *State, modelType ModelType, udoc *unitDoc) *Unit {
	unit := &Unit{
		st:        st,
		doc:       *udoc,
		modelType: modelType,
	}
	return unit
}

// ContainerInfo returns information about the containing hosting this unit.
// This is only used for CAAS models.
func (u *Unit) ContainerInfo() (CloudContainer, error) {
	doc, err := u.cloudContainer()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &cloudContainer{doc: *doc, unitName: u.Name()}, nil
}

// ShouldBeAssigned returns whether the unit should be assigned to a machine.
// IAAS models require units to be assigned.
func (u *Unit) ShouldBeAssigned() bool {
	return !u.isCaas()
}

func (u *Unit) isCaas() bool {
	return u.modelType == ModelTypeCAAS
}

// Application returns the application.
func (u *Unit) Application() (*Application, error) {
	return u.st.Application(u.doc.Application)
}

// ConfigSettings returns the complete set of application charm config settings
// available to the unit. Unset values will be replaced with the default
// value for the associated option, and may thus be nil when no default is
// specified.
func (u *Unit) ConfigSettings() (charm.Settings, error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("unit's charm URL must be set before retrieving config")
	}

	s, err := charmSettingsWithDefaults(u.st, u.doc.CharmURL, u.doc.Application)
	if err != nil {
		return nil, errors.Annotatef(err, "charm config for unit %q", u.Name())
	}
	return s, nil
}

// ApplicationName returns the application name.
func (u *Unit) ApplicationName() string {
	return u.doc.Application
}

// Base returns the deployed charm's base.
func (u *Unit) Base() Base {
	return u.doc.Base
}

// String returns the unit as string.
func (u *Unit) String() string {
	return u.doc.Name
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.doc.Name
}

// unitGlobalKey returns the global database key for the named unit.
func unitGlobalKey(name string) string {
	return "u#" + name + "#charm"
}

// unitWorkloadVersionKind returns the unit workload version kind.
func (u *Unit) unitWorkloadVersionKind() string {
	return u.Kind() + "-version"
}

// globalWorkloadVersionKey returns the global database key for the
// workload version status key for this unit.
func globalWorkloadVersionKey(name string) string {
	return unitGlobalKey(name) + "#sat#workload-version"
}

// globalAgentKey returns the global database key for the unit.
func (u *Unit) globalAgentKey() string {
	return unitAgentGlobalKey(u.doc.Name)
}

// globalKey returns the global database key for the unit.
func (u *Unit) globalKey() string {
	return unitGlobalKey(u.doc.Name)
}

// globalWorkloadVersionKey returns the global database key for the unit's
// workload version info.
func (u *Unit) globalWorkloadVersionKey() string {
	return globalWorkloadVersionKey(u.doc.Name)
}

// globalCloudContainerKey returns the global database key for the unit's
// Cloud Container info.
func (u *Unit) globalCloudContainerKey() string {
	return globalCloudContainerKey(u.doc.Name)
}

// Life returns whether the unit is Alive, Dying or Dead.
func (u *Unit) Life() Life {
	return u.doc.Life
}

// WorkloadVersion returns the version of the running workload set by
// the charm (eg, the version of postgresql that is running, as
// opposed to the version of the postgresql charm).
func (u *Unit) WorkloadVersion() (string, error) {
	unitStatus, err := getStatus(u.st.db(), u.globalWorkloadVersionKey(), "workload")
	if errors.Is(err, errors.NotFound) {
		return "", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return unitStatus.Message, nil
}

// SetWorkloadVersion sets the version of the workload that the unit
// is currently running.
func (u *Unit) SetWorkloadVersion(version string) error {
	// Store in status rather than an attribute of the unit doc - we
	// want to avoid everything being an attr of the main docs to
	// stop a swarm of watchers being notified for irrelevant changes.
	now := u.st.clock().Now()
	return setStatus(u.st.db(), setStatusParams{
		badge:      "workload",
		statusKind: u.unitWorkloadVersionKind(),
		statusId:   u.Name(),
		globalKey:  u.globalWorkloadVersionKey(),
		status:     status.Active,
		message:    version,
		updated:    &now,
	})
}

// AgentTools returns the tools that the agent is currently running.
// It an error that satisfies errors.IsNotFound if the tools have not
// yet been set.
func (u *Unit) AgentTools() (*tools.Tools, error) {
	if u.doc.Tools == nil {
		return nil, errors.NotFoundf("agent binaries for unit %q", u)
	}
	result := *u.doc.Tools
	return &result, nil
}

// SetAgentVersion sets the version of juju that the agent is
// currently running.
func (u *Unit) SetAgentVersion(v semversion.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "setting agent version for unit %q", u)
	if err = checkVersionValidity(v); err != nil {
		return err
	}
	versionedTool := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"tools", versionedTool}}}},
	}}
	if err := u.st.db().RunTransaction(ops); err != nil {
		return onAbort(err, stateerrors.ErrDead)
	}
	u.doc.Tools = versionedTool
	return nil
}

// UpdateOperation returns a model operation that will update a unit.
func (u *Unit) UpdateOperation(props UnitUpdateProperties) *UpdateUnitOperation {
	return &UpdateUnitOperation{
		unit:  &Unit{st: u.st, doc: u.doc, modelType: u.modelType},
		props: props,
	}
}

// UpdateUnitOperation is a model operation for updating a unit.
type UpdateUnitOperation struct {
	unit  *Unit
	props UnitUpdateProperties

	setStatusDocs map[string]statusDoc
}

// Build is part of the ModelOperation interface.
func (op *UpdateUnitOperation) Build(_ int) ([]txn.Op, error) {
	op.setStatusDocs = make(map[string]statusDoc)

	containerInfo, err := op.unit.cloudContainer()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if containerInfo == nil {
		containerInfo = &cloudContainerDoc{
			Id: op.unit.globalKey(),
		}
	}
	existingContainerInfo := *containerInfo

	var newProviderId string
	if op.props.ProviderId != nil {
		newProviderId = *op.props.ProviderId
	}
	if containerInfo.ProviderId != "" &&
		newProviderId != "" &&
		containerInfo.ProviderId != newProviderId {
		logger.Debugf(context.TODO(), "unit %q has provider id %q which changed to %q",
			op.unit.Name(), containerInfo.ProviderId, newProviderId)
	}

	if op.props.ProviderId != nil {
		containerInfo.ProviderId = newProviderId
	}
	if op.props.Address != nil {
		networkAddr := network.NewSpaceAddress(*op.props.Address, network.WithScope(network.ScopeMachineLocal))
		addr := fromNetworkAddress(networkAddr, network.OriginProvider)
		containerInfo.Address = &addr
	}
	if op.props.Ports != nil {
		containerInfo.Ports = *op.props.Ports
	}
	// Currently, we only update container attributes but that might change.
	var ops []txn.Op
	if !reflect.DeepEqual(*containerInfo, existingContainerInfo) {
		containerOps, err := op.unit.saveContainerOps(*containerInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, containerOps...)
	}

	updateStatus := func(key, badge string, status *status.StatusInfo) error {
		now := op.unit.st.clock().Now()
		doc := statusDoc{
			Status:     status.Status,
			StatusInfo: status.Message,
			StatusData: mgoutils.EscapeKeys(status.Data),
			Updated:    now.UnixNano(),
		}
		op.setStatusDocs[key] = doc
		// It's possible we're getting a first status update (i.e. cloud container)
		_, err = getStatus(op.unit.st.db(), key, badge)
		if err != nil {
			if !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			statusOps := createStatusOp(op.unit.st, key, doc)
			ops = append(ops, statusOps)
		} else {
			statusOps, err := statusSetOps(op.unit.st.db(), doc, key)
			if err != nil {
				return errors.Trace(err)
			}
			ops = append(ops, statusOps...)
		}
		return nil
	}
	if op.props.AgentStatus != nil {
		if err := updateStatus(op.unit.globalAgentKey(), "agent", op.props.AgentStatus); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var cloudContainerStatus status.StatusInfo
	if op.props.CloudContainerStatus != nil {
		if err := updateStatus(op.unit.globalCloudContainerKey(), "cloud container", op.props.CloudContainerStatus); err != nil {
			return nil, errors.Trace(err)
		}
		cloudContainerStatus = *op.props.CloudContainerStatus
	}
	if cloudContainerStatus.Status != "" {
		// Since we have updated cloud container, that may impact on
		// the perceived unit status. we'll update status history if the
		// unit status is different due to having a cloud container status.
		// This correctly ensures the status history goes from "waiting for
		// container" to <something else>.
		unitStatus, err := getStatus(op.unit.st.db(), op.unit.globalKey(), "unit")
		if err != nil {
			return nil, errors.Trace(err)
		}

		modifiedStatus := status.UnitDisplayStatus(unitStatus, cloudContainerStatus)
		now := op.unit.st.clock().Now()
		doc := statusDoc{
			Status:     modifiedStatus.Status,
			StatusInfo: modifiedStatus.Message,
			StatusData: mgoutils.EscapeKeys(modifiedStatus.Data),
			Updated:    now.UnixNano(),
		}
		op.setStatusDocs[op.unit.globalKey()] = doc
	}
	return ops, nil
}

// Done is part of the ModelOperation interface.
func (op *UpdateUnitOperation) Done(err error) error {
	if err != nil {
		return errors.Annotatef(err, "updating unit %q", op.unit.Name())
	}
	return nil
}

// Destroy, when called on a Alive unit, advances its lifecycle as far as
// possible; it otherwise has no effect. In most situations, the unit's
// life is just set to Dying; but if a principal unit that is not assigned
// to a provisioned machine is Destroyed, it will be removed from state
// directly.
// NB This is only called from tests.
func (u *Unit) Destroy(store objectstore.ObjectStore) error {
	_, errs, err := u.DestroyWithForce(store, false, time.Duration(0))
	if len(errs) != 0 {
		logger.Warningf(context.TODO(), "operational errors destroying unit %v: %v", u.Name(), errs)
	}
	return err
}

// DestroyMaybeRemove destroys a unit and returns if it was also removed.
func (u *Unit) DestroyMaybeRemove(store objectstore.ObjectStore) (bool, error) {
	removed, errs, err := u.DestroyWithForce(store, false, time.Duration(0))
	if len(errs) != 0 {
		logger.Warningf(context.TODO(), "operational errors destroying unit %v: %v", u.Name(), errs)
	}
	return removed, err
}

// DestroyWithForce does the same thing as Destroy() but
// ignores errors.
func (u *Unit) DestroyWithForce(store objectstore.ObjectStore, force bool, maxWait time.Duration) (removed bool, errs []error, err error) {
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			u.doc.Life = Dying
		}
	}()
	op := u.DestroyOperation(store)
	op.Force = force
	op.MaxWait = maxWait
	err = u.st.ApplyOperation(op)
	return op.Removed, op.Errors, err
}

// DestroyOperation returns a model operation that will destroy the unit.
func (u *Unit) DestroyOperation(store objectstore.ObjectStore) *DestroyUnitOperation {
	return &DestroyUnitOperation{
		unit:  &Unit{st: u.st, doc: u.doc, modelType: u.modelType},
		Store: store,
	}
}

// DestroyUnitOperation is a model operation for destroying a unit.
type DestroyUnitOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// unit holds the unit to destroy.
	unit *Unit

	// DestroyStorage controls whether or not storage attached
	// to the unit is destroyed. If this is false, then detachable
	// storage will be detached and left in the model.
	DestroyStorage bool

	// Removed is true if the application is removed during destroy.
	Removed bool

	// Store is the object store to use for blob access.
	Store objectstore.ObjectStore
}

// Build is part of the ModelOperation interface.
func (op *DestroyUnitOperation) Build(attempt int) ([]txn.Op, error) {
	if op.Store == nil {
		return nil, errors.New("no object store provided")
	}
	if attempt > 0 {
		if err := op.unit.Refresh(); errors.Is(err, errors.NotFound) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// When 'force' is set on the operation, this call will return both needed operations
	// as well as all operational errors encountered.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.destroyOps(); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			logger.Warningf(context.TODO(), "forcing unit destruction for %v despite error %v", op.unit.Name(), err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

// Done is part of the ModelOperation interface.
func (op *DestroyUnitOperation) Done(err error) error {
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "cannot destroy unit %q", op.unit)
		}
		op.AddError(errors.Errorf("force destroy unit %q proceeded despite encountering ERROR %v", op.unit, err))
	}
	// Reimplement in dqlite.
	//if err := op.deleteSecrets(); err != nil {
	//	logger.Errorf(context.TODO(), "cannot delete secrets for unit %q: %v", op.unit, err)
	//}
	return nil
}

// destroyOps returns the operations required to destroy the unit. If it
// returns errRefresh, the unit should be refreshed and the destruction
// operations recalculated.
// When 'force' is set on the operation, this call will return both needed operations
// as well as all operational errors encountered.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (op *DestroyUnitOperation) destroyOps() ([]txn.Op, error) {
	if op.unit.doc.Life != Alive {
		if !op.Force {
			return nil, errAlreadyDying
		}
	}

	// Where possible, we'd like to be able to short-circuit unit destruction
	// such that units can be removed directly rather than waiting for their
	// agents to start, observe Dying, set Dead, and shut down; this takes a
	// long time and is vexing to users. This turns out to be possible if and
	// only if the unit agent has not yet set its status; this implies that the
	// most the unit could possibly have done is to run its install hook.
	//
	// There's no harm in removing a unit that's run its install hook only --
	// or, at least, there is no more harm than there is in removing a unit
	// that's run its stop hook, and that's the usual condition.
	//
	// Principals with subordinates are never eligible for this shortcut,
	// because the unit agent must inevitably have set a status before getting
	// to the point where it can actually create its subordinate.
	//
	// Subordinates should be eligible for the shortcut but are not currently
	// considered, on the basis that (1) they were created by active principals
	// and can be expected to be deployed pretty soon afterwards, so we don't
	// lose much time and (2) by maintaining this restriction, I can reduce
	// the number of tests that have to change and defer that improvement to
	// its own CL.

	cleanupOp := newCleanupOp(cleanupDyingUnit, op.unit.doc.Name, op.DestroyStorage, op.Force, op.MaxWait)

	// If we're forcing destruction the assertion shouldn't be that
	// life is alive, but that it's what we think it is now.
	assertion := isAliveDoc
	if op.Force {
		assertion = bson.D{{"life", op.unit.doc.Life}}
	}

	setDyingOp := txn.Op{
		C:      unitsC,
		Id:     op.unit.doc.DocID,
		Assert: assertion,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}
	setDyingOps := func(dyingErr error) ([]txn.Op, error) {
		if !op.Force && dyingErr != nil {
			// If we are not forcing removal, we care about the errors as they will stop removal.
			// Don't return operations.
			return nil, dyingErr
		}
		// If we are forcing, we care about the errors as we want report them to the user.
		// But we also want operations to power through the removal.
		if dyingErr != nil {
			op.AddError(errors.Errorf("force destroying dying unit %v despite error %v", op.unit.Name(), dyingErr))
		}
		ops := []txn.Op{setDyingOp, cleanupOp}
		return ops, nil
	}
	if op.unit.doc.Principal != "" {
		return setDyingOps(nil)
	} else if len(op.unit.doc.Subordinates)+op.unit.doc.StorageAttachmentCount != 0 {
		return setDyingOps(nil)
	}

	// See if the unit agent has started running.
	// If so then we can't set directly to dead.
	isAssigned := op.unit.doc.MachineId != ""
	shouldBeAssigned := op.unit.ShouldBeAssigned()
	agentStatusDocId := op.unit.globalAgentKey()
	agentStatusInfo, agentErr := getStatus(op.unit.st.db(), agentStatusDocId, "agent")
	if errors.Is(agentErr, errors.NotFound) {
		return nil, errAlreadyDying
	} else if agentErr != nil {
		if !op.Force {
			return nil, errors.Trace(agentErr)
		}
	}

	// This has to be a function since we want to delay the evaluation of the value,
	// in case agent erred out.
	isReady := func() (bool, error) {
		// IAAS models need the unit to be assigned.
		if shouldBeAssigned {
			return isAssigned && agentStatusInfo.Status != status.Allocating, nil
		}
		// For CAAS models, check to see if the unit agent has started (the
		// presence of the unitstates row indicates this).
		unitState, err := op.unit.State()
		if err != nil {
			return false, errors.Trace(err)
		}
		return unitState.Modified(), nil
	}
	if agentErr == nil {
		ready, err := isReady()
		if op.FatalError(err) {
			return nil, errors.Trace(err)
		}
		if ready {
			return setDyingOps(agentErr)
		}
	}
	switch agentStatusInfo.Status {
	case status.Error, status.Allocating:
	default:
		err := errors.Errorf("unexpected unit state - unit with status %v is not deployed", agentStatusInfo.Status)
		if op.FatalError(err) {
			return nil, err
		}
	}

	statusOp := txn.Op{
		C:      statusesC,
		Id:     op.unit.st.docID(agentStatusDocId),
		Assert: bson.D{{"status", agentStatusInfo.Status}},
	}
	removeAsserts := isAliveDoc
	if op.Force {
		removeAsserts = bson.D{{"life", op.unit.doc.Life}}
	}
	removeAsserts = append(removeAsserts, bson.DocElem{
		"$and", []bson.D{
			unitHasNoSubordinates,
			unitHasNoStorageAttachments,
		},
	})
	// If the unit is unassigned, ensure it is not assigned in the interim.
	if !isAssigned && shouldBeAssigned {
		removeAsserts = append(removeAsserts, bson.DocElem{"machineid", ""})
	}

	// When 'force' is set, this call will return some, if not all, needed operations.
	// All operational errors encountered will be added to the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	removeOps, err := op.unit.removeOps(op.Store, removeAsserts, &op.ForcedOperation, op.DestroyStorage)
	if err == errAlreadyRemoved {
		return nil, errAlreadyDying
	} else if op.FatalError(err) {
		return nil, err
	}
	ops := []txn.Op{statusOp}
	ops = append(ops, removeOps...)
	op.Removed = true
	return ops, nil
}

// destroyHostOps returns all necessary operations to destroy the application unit's host machine,
// or ensure that the conditions preventing its destruction remain stable through the transaction.
// When 'force' is set, this call will return needed operations
// and accumulate all operational errors encountered on the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (u *Unit) destroyHostOps(a *Application, op *ForcedOperation) (ops []txn.Op, err error) {
	if a.doc.Subordinate {
		return []txn.Op{{
			C:      unitsC,
			Id:     u.st.docID(u.doc.Principal),
			Assert: txn.DocExists,
			Update: bson.D{{"$pull", bson.D{{"subordinates", u.doc.Name}}}},
		}}, nil
	} else if u.doc.MachineId == "" {
		unitLogger.Tracef(context.TODO(), "unit %v unassigned", u)
		return nil, nil
	}

	m, err := u.st.Machine(u.doc.MachineId)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, nil
		}
		return nil, err
	}
	node, err := u.st.ControllerNode(u.doc.MachineId)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}
	haveControllerNode := err == nil
	hasVote := haveControllerNode && node.HasVote()

	containerCheck := true // whether container conditions allow destroying the host machine
	containers, err := m.Containers()
	if op.FatalError(err) {
		return nil, err
	}
	if len(containers) > 0 {
		ops = append(ops, txn.Op{
			C:      containerRefsC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"children.0", bson.D{{"$exists", 1}}}},
		})
		containerCheck = false
	} else {
		ops = append(ops, txn.Op{
			C:  containerRefsC,
			Id: m.doc.DocID,
			Assert: bson.D{{"$or", []bson.D{
				{{"children", bson.D{{"$size", 0}}}},
				{{"children", bson.D{{"$exists", false}}}},
			}}},
		})
	}

	isController := m.IsManager()
	machineCheck := true // whether host machine conditions allow destroy
	if len(m.doc.Principals) != 1 || m.doc.Principals[0] != u.doc.Name {
		machineCheck = false
	} else if isController {
		// Check that the machine does not have any responsibilities that
		// prevent a lifecycle change.
		machineCheck = false
	} else if hasVote {
		machineCheck = false
	}

	// assert that the machine conditions pertaining to host removal conditions
	// remain the same throughout the transaction.
	var machineAssert bson.D
	var controllerNodeAssert interface{}
	if machineCheck {
		machineAssert = bson.D{{"$and", []bson.D{
			{{"principals", []string{u.doc.Name}}},
			{{"jobs", bson.D{{"$nin", []MachineJob{JobManageModel}}}}},
		}}}
		controllerNodeAssert = txn.DocMissing
		if haveControllerNode {
			controllerNodeAssert = bson.D{{"has-vote", false}}
		}
	} else {
		machineAssert = bson.D{{"$or", []bson.D{
			{{"principals", bson.D{{"$ne", []string{u.doc.Name}}}}},
			{{"jobs", bson.D{{"$in", []MachineJob{JobManageModel}}}}},
		}}}
		if isController {
			controllerNodeAssert = txn.DocExists
		}
	}

	// If removal conditions satisfied by machine & container docs, we can
	// destroy it, in addition to removing the unit principal.
	machineUpdate := bson.D{{"$pull", bson.D{{"principals", u.doc.Name}}}}
	var cleanupOps []txn.Op
	if machineCheck && containerCheck {
		machineUpdate = append(machineUpdate, bson.D{{"$set", bson.D{{"life", Dying}}}}...)
		if !op.Force {
			cleanupOps = []txn.Op{newCleanupOp(cleanupDyingMachine, m.doc.Id, op.Force)}
		} else {
			cleanupOps = []txn.Op{newCleanupOp(cleanupForceDestroyedMachine, m.doc.Id, op.MaxWait)}
		}
	}

	ops = append(ops, txn.Op{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: machineAssert,
		Update: machineUpdate,
	})
	if controllerNodeAssert != nil {
		ops = append(ops, txn.Op{
			C:      controllerNodesC,
			Id:     m.st.docID(m.Id()),
			Assert: controllerNodeAssert,
		})
	}

	return append(ops, cleanupOps...), nil
}

// removeOps returns the operations necessary to remove the unit, assuming
// the supplied asserts apply to the unit document.
// When 'force' is set, this call will return needed operations
// accumulating all operational errors in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (u *Unit) removeOps(store objectstore.ObjectStore, asserts bson.D, op *ForcedOperation, destroyStorage bool) ([]txn.Op, error) {
	app, err := u.st.Application(u.doc.Application)
	if errors.Is(err, errors.NotFound) {
		// If the application has been removed, the unit must already have been.
		return nil, errAlreadyRemoved
	} else if err != nil {
		// If we cannot find application, no amount of force will succeed after this point.
		return nil, err
	}
	return app.removeUnitOps(store, u, asserts, op, destroyStorage)
}

var unitHasNoSubordinates = bson.D{{
	"$or", []bson.D{
		{{"subordinates", bson.D{{"$size", 0}}}},
		{{"subordinates", bson.D{{"$exists", false}}}},
	},
}}

var unitHasNoStorageAttachments = bson.D{{
	"$or", []bson.D{
		{{"storageattachmentcount", 0}},
		{{"storageattachmentcount", bson.D{{"$exists", false}}}},
	},
}}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. If the unit has subordinates, it will
// return ErrUnitHasSubordinates; otherwise, if it has storage instances,
// it will return ErrUnitHasStorageInstances.
func (u *Unit) EnsureDead() (err error) {
	if u.doc.Life == Dead {
		return nil
	}
	defer func() {
		if err == nil {
			u.doc.Life = Dead
		}
	}()
	assert := append(notDeadDoc, bson.DocElem{
		"$and", []bson.D{
			unitHasNoSubordinates,
			unitHasNoStorageAttachments,
		},
	})
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: assert,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}}
	if err := u.st.db().RunTransaction(ops); err != txn.ErrAborted {
		return err
	}
	if notDead, err := isNotDead(u.st, unitsC, u.doc.DocID); err != nil {
		return err
	} else if !notDead {
		return nil
	}
	if err := u.Refresh(); errors.Is(err, errors.NotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if len(u.doc.Subordinates) > 0 {
		return stateerrors.ErrUnitHasSubordinates
	}
	return stateerrors.ErrUnitHasStorageAttachments
}

// RemoveOperation returns a model operation that will remove the unit.
func (u *Unit) RemoveOperation(store objectstore.ObjectStore, force bool) *RemoveUnitOperation {
	return &RemoveUnitOperation{
		unit:            &Unit{st: u.st, doc: u.doc, modelType: u.modelType},
		ForcedOperation: ForcedOperation{Force: force},
		Store:           store,
	}
}

// RemoveUnitOperation is a model operation for removing a unit.
type RemoveUnitOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// unit holds the unit to remove.
	unit *Unit

	// Store is the object store to use for blob access.
	Store objectstore.ObjectStore
}

// Build is part of the ModelOperation interface.
func (op *RemoveUnitOperation) Build(attempt int) ([]txn.Op, error) {
	if op.Store == nil {
		return nil, errors.New("no object store provided")
	}
	if attempt > 0 {
		if err := op.unit.Refresh(); errors.Is(err, errors.NotFound) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// When 'force' is set on the operation, this call will return both needed operations
	// as well as all operational errors encountered.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.removeOps(); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			logger.Warningf(context.TODO(), "forcing unit removal for %v despite error %v", op.unit.Name(), err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

// Done is part of the ModelOperation interface.
func (op *RemoveUnitOperation) Done(err error) error {
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "cannot remove unit %q", op.unit)
		}
		op.AddError(errors.Errorf("force removing unit %q proceeded despite encountering ERROR %v", op.unit, err))
	}
	return nil
}

// Remove removes the unit from state, and may remove its application as well, if
// the application is Dying and no other references to it exist. It will fail if
// the unit is not Dead.
func (u *Unit) Remove(store objectstore.ObjectStore) error {
	_, err := u.RemoveWithForce(store, false, time.Duration(0))
	return err
}

// RemoveWithForce removes the unit from state similar to the unit.Remove() but
// it ignores errors.
// In addition, this function also returns all non-fatal operational errors
// encountered.
func (u *Unit) RemoveWithForce(store objectstore.ObjectStore, force bool, maxWait time.Duration) ([]error, error) {
	op := u.RemoveOperation(store, force)
	op.MaxWait = maxWait
	err := u.st.ApplyOperation(op)
	return op.Errors, err
}

// When 'force' is set, this call will return needed operations
// and all operational errors will be accumulated in operation itself.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (op *RemoveUnitOperation) removeOps() (ops []txn.Op, err error) {
	if op.unit.doc.Life != Dead {
		return nil, errors.New("unit is not dead")
	}
	// Now the unit is Dead, we can be sure that it's impossible for it to
	// enter relation scopes (once it's Dying, we can be sure of this; but
	// EnsureDead does not require that it already be Dying, so this is the
	// only point at which we can safely backstop lp:1233457 and mitigate
	// the impact of unit agent bugs that leave relation scopes occupied).
	relations, err := matchingRelations(op.unit.st, op.unit.doc.Application)
	if op.FatalError(err) {
		return nil, err
	} else {
		failRelations := false
		for _, rel := range relations {
			ru, err := rel.Unit(op.unit)
			if err != nil {
				op.AddError(err)
				failRelations = true
				continue
			}
			leaveScopeOps, err := ru.leaveScopeForcedOps(&op.ForcedOperation)
			if err != nil && err != jujutxn.ErrNoOperations {
				op.AddError(err)
				failRelations = true
			}
			ops = append(ops, leaveScopeOps...)
		}
		if !op.Force && failRelations {
			return nil, op.LastError()
		}
	}

	// Now we're sure we haven't left any scopes occupied by this unit, we
	// can safely remove the document.
	unitRemoveOps, err := op.unit.removeOps(op.Store, isDeadDoc, &op.ForcedOperation, false)
	if op.FatalError(err) {
		return nil, err
	}
	return append(ops, unitRemoveOps...), nil
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate applications deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.doc.Principal == ""
}

// SubordinateNames returns the names of any subordinate units.
func (u *Unit) SubordinateNames() []string {
	subNames := make([]string, len(u.doc.Subordinates))
	copy(subNames, u.doc.Subordinates)
	return subNames
}

// RelationsJoined returns the relations for which the unit has entered scope
// and neither left it nor prepared to leave it
func (u *Unit) RelationsJoined() ([]*Relation, error) {
	return u.relations(func(ru *RelationUnit) (bool, error) {
		return ru.Joined()
	})
}

// RelationsInScope returns the relations for which the unit has entered scope
// and not left it.
func (u *Unit) RelationsInScope() ([]*Relation, error) {
	return u.relations(func(ru *RelationUnit) (bool, error) {
		return ru.InScope()
	})
}

type relationPredicate func(ru *RelationUnit) (bool, error)

// relations implements RelationsJoined and RelationsInScope.
func (u *Unit) relations(predicate relationPredicate) ([]*Relation, error) {
	candidates, err := matchingRelations(u.st, u.doc.Application)
	if err != nil {
		return nil, err
	}
	var filtered []*Relation
	for _, relation := range candidates {
		relationUnit, err := relation.Unit(u)
		if err != nil {
			return nil, err
		}
		if include, err := predicate(relationUnit); err != nil {
			return nil, err
		} else if include {
			filtered = append(filtered, relation)
		}
	}
	return filtered, nil
}

// PrincipalName returns the name of the unit's principal.
// If the unit is not a subordinate, false is returned.
func (u *Unit) PrincipalName() (string, bool) {
	return u.doc.Principal, u.doc.Principal != ""
}

// machine returns the unit's machine.
//
// machine is part of the machineAssignable interface.
func (u *Unit) machine() (*Machine, error) {
	id, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Annotatef(err, "unit %v cannot get assigned machine", u)
	}
	m, err := u.st.Machine(id)
	if err != nil {
		return nil, errors.Annotatef(err, "unit %v misses machine id %v", u, id)
	}
	return m, nil
}

// noAssignedMachineOp is part of the machineAssignable interface.
func (u *Unit) noAssignedMachineOp() txn.Op {
	id := u.doc.DocID
	if u.doc.Principal != "" {
		id = u.doc.Principal
	}
	return txn.Op{
		C:      unitsC,
		Id:     id,
		Assert: bson.D{{"machineid", ""}},
	}
}

// PublicAddress returns the public address of the unit.
func (u *Unit) PublicAddress() (network.SpaceAddress, error) {
	if !u.ShouldBeAssigned() {
		return u.scopedAddress("public")
	}
	m, err := u.machine()
	if err != nil {
		unitLogger.Tracef(context.TODO(), "%v", err)
		return network.SpaceAddress{}, errors.Trace(err)
	}
	return m.PublicAddress()
}

// PrivateAddress returns the private address of the unit.
func (u *Unit) PrivateAddress() (network.SpaceAddress, error) {
	if !u.ShouldBeAssigned() {
		addr, err := u.scopedAddress("private")
		if network.IsNoAddressError(err) {
			return u.containerAddress()
		}
		return addr, errors.Trace(err)
	}
	m, err := u.machine()
	if err != nil {
		unitLogger.Tracef(context.TODO(), "%v", err)
		return network.SpaceAddress{}, errors.Trace(err)
	}
	return m.PrivateAddress()
}

// AllAddresses returns the public and private addresses
// plus the container address of the unit (if known).
// Only relevant for CAAS models - will return an empty
// slice for IAAS models.
func (u *Unit) AllAddresses() (addrs network.SpaceAddresses, _ error) {
	if u.ShouldBeAssigned() {
		return addrs, nil
	}

	// First the addresses of the service.
	serviceAddrs, err := u.serviceAddresses()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		addrs = append(addrs, serviceAddrs...)
	}

	// Then the container address.
	containerAddr, err := u.containerAddress()
	if network.IsNoAddressError(err) {
		return addrs, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	addrs = append(addrs, containerAddr)
	return addrs, nil
}

// serviceAddresses returns the addresses of the service
// managing the pods in which the unit workload is running.
func (u *Unit) serviceAddresses() (network.SpaceAddresses, error) {
	app, err := u.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceInfo, err := app.ServiceInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return serviceInfo.Addresses(), nil
}

// containerAddress returns the address of the pod's container.
func (u *Unit) containerAddress() (network.SpaceAddress, error) {
	containerInfo, err := u.cloudContainer()
	if errors.Is(err, errors.NotFound) {
		return network.SpaceAddress{}, network.NoAddressError("container")
	}
	if err != nil {
		return network.SpaceAddress{}, errors.Trace(err)
	}
	addr := containerInfo.Address
	if addr == nil {
		return network.SpaceAddress{}, network.NoAddressError("container")
	}
	return addr.networkAddress(), nil
}

func (u *Unit) scopedAddress(scope string) (network.SpaceAddress, error) {
	addresses, err := u.AllAddresses()
	if err != nil {
		return network.SpaceAddress{}, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return network.SpaceAddress{}, network.NoAddressError(scope)
	}
	getStrictPublicAddr := func(addresses network.SpaceAddresses) (network.SpaceAddress, bool) {
		addr, ok := addresses.OneMatchingScope(network.ScopeMatchPublic)
		return addr, ok && addr.Scope == network.ScopePublic
	}

	getInternalAddr := func(addresses network.SpaceAddresses) (network.SpaceAddress, bool) {
		return addresses.OneMatchingScope(network.ScopeMatchCloudLocal)
	}

	var addrMatch func(network.SpaceAddresses) (network.SpaceAddress, bool)
	switch scope {
	case "public":
		addrMatch = getStrictPublicAddr
	case "private":
		addrMatch = getInternalAddr
	default:
		return network.SpaceAddress{}, errors.NotValidf("address scope %q", scope)
	}

	addr, found := addrMatch(addresses)
	if !found {
		return network.SpaceAddress{}, network.NoAddressError(scope)
	}
	return addr, nil
}

// Refresh refreshes the contents of the Unit from the underlying
// state. It an error that satisfies errors.IsNotFound if the unit has
// been removed.
func (u *Unit) Refresh() error {
	units, closer := u.st.db().GetCollection(unitsC)
	defer closer()

	err := units.FindId(u.doc.DocID).One(&u.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("unit %q", u)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh unit %q", u)
	}
	return nil
}

// ContainerStatus returns the container status for a unit.
func (u *Unit) ContainerStatus() (status.StatusInfo, error) {
	return getStatus(u.st.db(), u.globalCloudContainerKey(), "cloud container")
}

// CharmURL returns the charm URL this unit is currently using.
func (u *Unit) CharmURL() *string {
	return u.doc.CharmURL
}

// SetCharmURL marks the unit as currently using the supplied charm URL.
// No checks are performed on the supplied URL, and it is assumed to be
// properly stored in dqlite.
func (u *Unit) SetCharmURL(curl string) error {
	if curl == "" {
		return errors.Errorf("cannot set empty charm url")
	}

	db, dbCloser := u.st.newDB()
	defer dbCloser()
	units, uCloser := db.GetCollection(unitsC)
	defer uCloser()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// NOTE: We're explicitly allowing SetCharmURL to succeed
			// when the unit is Dying, because application/charm upgrades
			// should still be allowed to apply to dying units, so
			// that bugs in departed/broken hooks can be addressed at
			// runtime.
			if notDead, err := isNotDeadWithSession(units, u.doc.DocID); err != nil {
				return nil, errors.Trace(err)
			} else if !notDead {
				return nil, stateerrors.ErrDead
			}
		}
		sel := bson.D{{"_id", u.doc.DocID}, {"charmurl", curl}}
		if count, err := units.Find(sel).Count(); err != nil {
			return nil, errors.Trace(err)
		} else if count == 1 {
			// Already set
			return nil, jujutxn.ErrNoOperations
		}

		// Set the new charm URL.
		differentCharm := bson.D{{"charmurl", bson.D{{"$ne", curl}}}}

		return []txn.Op{{
			C:      unitsC,
			Id:     u.doc.DocID,
			Assert: append(notDeadDoc, differentCharm...),
			Update: bson.D{{"$set", bson.D{{"charmurl", curl}}}},
		}}, nil
	}
	err := u.st.db().Run(buildTxn)
	if err == nil {
		u.doc.CharmURL = &curl
	}
	return err
}

// charm returns the charm for the unit, or the application if the unit's charm
// has not been set yet.
func (u *Unit) charm() (CharmRefFull, error) {
	cURL := u.CharmURL()
	if cURL == nil {
		app, err := u.Application()
		if err != nil {
			return nil, err
		}
		cURL, _ = app.CharmURL()
	}

	if cURL == nil {
		return nil, errors.Errorf("missing charm URL for %q", u.Name())
	}

	var ch CharmRefFull
	err := retry.Call(retry.CallArgs{
		Attempts: 20,
		Delay:    50 * time.Millisecond,
		Func: func() error {
			var err error
			ch, err = u.st.Charm(*cURL)
			return err
		},
		Clock: u.st.clock(),
		NotifyFunc: func(err error, attempt int) {
			logger.Warningf(context.TODO(), "error getting charm for unit %q. Retrying (attempt %d): %v", u.Name(), attempt, err)
		},
	})

	return ch, errors.Annotatef(err, "getting charm for %s", u)
}

// assertCharmOps returns txn.Ops to assert the current charm of the unit.
// If the unit currently has no charm URL set, then the application's charm
// URL will be checked by the txn.Ops also.
func (u *Unit) assertCharmOps(ch CharmRefFull) []txn.Op {
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.Name,
		Assert: bson.D{{"charmurl", u.doc.CharmURL}},
	}}
	if u.doc.CharmURL != nil {
		appName := u.ApplicationName()
		ops = append(ops, txn.Op{
			C:      applicationsC,
			Id:     appName,
			Assert: bson.D{{"charmurl", ch.URL()}},
		})
	}
	return ops
}

// Tag returns a name identifying the unit.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (u *Unit) Tag() names.Tag {
	return u.UnitTag()
}

// Kind returns a human readable name identifying the unit workload kind.
func (u *Unit) Kind() string {
	return u.Tag().Kind() + "-workload"
}

// UnitTag returns a names.UnitTag representing this Unit, unless the
// unit Name is invalid, in which case it will panic
func (u *Unit) UnitTag() names.UnitTag {
	return names.NewUnitTag(u.Name())
}

func unitNotAssignedError(u *Unit) error {
	msg := fmt.Sprintf("unit %q is not assigned to a machine", u)
	return errors.NewNotAssigned(nil, msg)
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id string, err error) {
	if u.doc.MachineId == "" {
		return "", unitNotAssignedError(u)
	}
	return u.doc.MachineId, nil
}

var (
	machineNotCleanErr = errors.New("machine is dirty")
	alreadyAssignedErr = errors.New("unit is already assigned to a machine")
	inUseErr           = errors.New("machine is not unused")
)

// assignToMachine is the internal version of AssignToMachine.
func (u *Unit) assignToMachine(
	m MachineRef,
	unused bool,
) (err error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		u, m := u, m // don't change outer vars
		if attempt > 0 {
			var err error
			u, err = u.st.Unit(u.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			m, err = u.st.Machine(m.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		return u.assignToMachineOps(m, unused)
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	u.doc.MachineId = m.Id()
	m.AddPrincipal(u.doc.Name)
	return nil
}

// assignToMachineOps returns txn.Ops to assign a unit to a machine.
// assignToMachineOps returns specific errors in some cases:
// - machineNotAliveErr when the machine is not alive.
// - unitNotAliveErr when the unit is not alive.
// - alreadyAssignedErr when the unit has already been assigned
// - inUseErr when the machine already has a unit assigned (if unused is true)
func (u *Unit) assignToMachineOps(
	m MachineRef,
	unused bool,
) ([]txn.Op, error) {
	if u.Life() != Alive {
		return nil, unitNotAliveErr
	}
	if u.doc.MachineId != "" {
		if u.doc.MachineId != m.Id() {
			return nil, alreadyAssignedErr
		}
		return nil, jujutxn.ErrNoOperations
	}
	if unused && !m.Clean() {
		return nil, inUseErr
	}
	storageParams, err := u.storageParams()
	if err != nil {
		return nil, errors.Trace(err)
	}
	sb, err := NewStorageConfigBackend(u.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storagePools, err := storagePools(sb, storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateUnitMachineAssignment(
		u.st, m, u.doc.Base, u.doc.Principal != "", storagePools,
	); err != nil {
		return nil, errors.Trace(err)
	}
	storageOps, volumesAttached, filesystemsAttached, err := sb.hostStorageOps(m.Id(), storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// addMachineStorageAttachmentsOps will add a txn.Op that ensures
	// that no filesystems were concurrently added to the machine if
	// any of the filesystems being attached specify a location.
	attachmentOps, err := addMachineStorageAttachmentsOps(
		u.st, m, volumesAttached, filesystemsAttached,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageOps = append(storageOps, attachmentOps...)

	assert := append(isAliveDoc, bson.D{{
		// The unit's subordinates must not change while we're
		// assigning it to a machine, to ensure machine storage
		// is created for subordinate units.
		"subordinates", u.doc.Subordinates,
	}, {
		"$or", []bson.D{
			{{"machineid", ""}},
			{{"machineid", m.Id()}},
		},
	}}...)
	massert := append(isAliveDoc, bson.D{{
		// The machine must be able to accept a unit.
		"jobs", bson.M{"$in": []MachineJob{JobHostUnits}},
	}}...)
	if unused {
		massert = append(massert, bson.D{{"clean", bson.D{{"$ne", false}}}}...)
	}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: assert,
		Update: bson.D{{"$set", bson.D{{"machineid", m.Id()}}}},
	}, {
		C:      machinesC,
		Id:     m.DocID(),
		Assert: massert,
		Update: bson.D{{"$addToSet", bson.D{{"principals", u.doc.Name}}}, {"$set", bson.D{{"clean", false}}}},
	},
		removeStagedAssignmentOp(u.doc.DocID),
	}
	ops = append(ops, storageOps...)
	return ops, nil
}

// validateUnitMachineAssignment validates the parameters for assigning a unit
// to a specified machine.
func validateUnitMachineAssignment(
	st *State,
	m MachineRef,
	base Base,
	isSubordinate bool,
	storagePools set.Strings,
) (err error) {
	if m.Life() != Alive {
		return machineNotAliveErr
	}
	if isSubordinate {
		return fmt.Errorf("unit is a subordinate")
	}
	if !base.compatibleWith(m.Base()) {
		return fmt.Errorf("base does not match: unit has %q, machine has %q", base.DisplayString(), m.Base().DisplayString())
	}
	canHost := false
	for _, j := range m.Jobs() {
		if j == JobHostUnits {
			canHost = true
			break
		}
	}
	if !canHost {
		return fmt.Errorf("machine %q cannot host units", m)
	}
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	if err := validateDynamicMachineStoragePools(sb, m, storagePools); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// validateDynamicMachineStorageParams validates that the provided machine
// storage parameters are compatible with the specified machine.
func validateDynamicMachineStorageParams(
	m *Machine,
	params *storageParams,
) error {
	sb, err := NewStorageConfigBackend(m.st)
	if err != nil {
		return errors.Trace(err)
	}
	pools, err := storagePools(sb, params)
	if err != nil {
		return err
	}
	if err := validateDynamicMachineStoragePools(sb.storageBackend, m, pools); err != nil {
		return err
	}
	// Validate the volume/filesystem attachments for the machine.
	for volumeTag := range params.volumeAttachments {
		volume, err := getVolumeByTag(sb.mb, volumeTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !volume.Detachable() && volume.doc.HostId != m.Id() {
			return errors.Errorf(
				"storage is non-detachable (bound to machine %s)",
				volume.doc.HostId,
			)
		}
	}
	for filesystemTag := range params.filesystemAttachments {
		filesystem, err := getFilesystemByTag(sb.mb, filesystemTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !filesystem.Detachable() && filesystem.doc.HostId != m.Id() {
			host := storageAttachmentHost(filesystem.doc.HostId)
			return errors.Errorf(
				"storage is non-detachable (bound to %s)",
				names.ReadableString(host),
			)
		}
	}
	return nil
}

// storagePools returns the names of storage pools in each of the
// volume, filesystem and attachments in the machine storage parameters.
func storagePools(sb *storageConfigBackend, params *storageParams) (set.Strings, error) {
	pools := make(set.Strings)
	for _, v := range params.volumes {
		v, err := sb.volumeParamsWithDefaults(v.Volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pools.Add(v.Pool)
	}
	for _, f := range params.filesystems {
		f, err := sb.filesystemParamsWithDefaults(f.Filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pools.Add(f.Pool)
	}
	for volumeTag := range params.volumeAttachments {
		volume, err := sb.Volume(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if params, ok := volume.Params(); ok {
			pools.Add(params.Pool)
		} else {
			info, err := volume.Info()
			if err != nil {
				return nil, errors.Trace(err)
			}
			pools.Add(info.Pool)
		}
	}
	for filesystemTag := range params.filesystemAttachments {
		filesystem, err := sb.Filesystem(filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if params, ok := filesystem.Params(); ok {
			pools.Add(params.Pool)
		} else {
			info, err := filesystem.Info()
			if err != nil {
				return nil, errors.Trace(err)
			}
			pools.Add(info.Pool)
		}
	}
	return pools, nil
}

// validateDynamicMachineStoragePools validates that all of the specified
// storage pools support dynamic storage provisioning. If any provider doesn't
// support dynamic storage, then an IsNotSupported error is returned.
func validateDynamicMachineStoragePools(sb *storageBackend, m MachineRef, pools set.Strings) error {
	if pools.IsEmpty() {
		return nil
	}
	return validateDynamicStoragePools(sb, pools, m.ContainerType())
}

// validateDynamicStoragePools validates that all of the specified storage
// providers support dynamic storage provisioning. If any provider doesn't
// support dynamic storage, then an IsNotSupported error is returned.
func validateDynamicStoragePools(sb *storageBackend, pools set.Strings, containerType instance.ContainerType) error {
	for pool := range pools {
		providerType, p, _, err := poolStorageProvider(sb, pool)
		if err != nil {
			return errors.Trace(err)
		}
		if containerType != "" && !provider.AllowedContainerProvider(providerType) {
			// TODO(axw) later we might allow *any* storage, and
			// passthrough/bindmount storage. That would imply either
			// container creation time only, or requiring containers
			// to be restarted to pick up new configuration.
			return errors.NotSupportedf("adding storage of type %q to %s container", providerType, containerType)
		}
		if !p.Dynamic() {
			return errors.NewNotSupported(err, fmt.Sprintf(
				"%q storage provider does not support dynamic storage",
				providerType,
			))
		}
	}
	return nil
}

func assignContextf(err *error, unitName string, target string) {
	if *err != nil {
		*err = errors.Annotatef(*err,
			"cannot assign unit %q to %s",
			unitName, target,
		)
	}
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(
	m *Machine,
) (err error) {
	defer assignContextf(&err, u.Name(), fmt.Sprintf("machine %s", m))
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	return u.assignToMachine(m, false)
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachineRef(
	m MachineRef,
) (err error) {
	defer assignContextf(&err, u.Name(), fmt.Sprintf("machine %s", m))
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	return u.assignToMachine(m, false)
}

// assignToNewMachineOps returns txn.Ops to assign the unit to a machine
// created according to the supplied params, with the supplied constraints.
func (u *Unit) assignToNewMachineOps(
	template MachineTemplate,
	parentId string,
	containerType instance.ContainerType,
) (*Machine, []txn.Op, error) {

	if u.Life() != Alive {
		return nil, nil, unitNotAliveErr
	}
	if u.doc.MachineId != "" {
		return nil, nil, alreadyAssignedErr
	}

	template.principals = []string{u.doc.Name}
	template.Dirty = true

	var (
		mdoc *machineDoc
		ops  []txn.Op
		err  error
	)
	switch {
	case parentId == "" && containerType == "":
		mdoc, ops, err = u.st.addMachineOps(template)
	case parentId == "":
		if containerType == "" {
			return nil, nil, errors.New("assignToNewMachine called without container type (should never happen)")
		}
		// The new parent machine is clean and only hosts units,
		// regardless of its child.
		parentParams := template
		parentParams.Jobs = []MachineJob{JobHostUnits}
		mdoc, ops, err = u.st.addMachineInsideNewMachineOps(template, parentParams, containerType)
	default:
		mdoc, ops, err = u.st.addMachineInsideMachineOps(template, parentId, containerType)
	}
	if err != nil {
		return nil, nil, err
	}

	// Ensure the host machine is really clean.
	if parentId != "" {
		mparent, err := u.st.Machine(parentId)
		if err != nil {
			return nil, nil, err
		}
		if !mparent.Clean() {
			return nil, nil, machineNotCleanErr
		}
		containers, err := mparent.Containers()
		if err != nil {
			return nil, nil, err
		}
		if len(containers) > 0 {
			return nil, nil, machineNotCleanErr
		}
		parentDocId := u.st.docID(parentId)
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     parentDocId,
			Assert: bson.D{{"clean", true}},
		}, txn.Op{
			C:      containerRefsC,
			Id:     parentDocId,
			Assert: bson.D{hasNoContainersTerm},
		})
	}

	// The unit's subordinates must not change while we're
	// assigning it to a machine, to ensure machine storage
	// is created for subordinate units.
	subordinatesUnchanged := bson.D{{"subordinates", u.doc.Subordinates}}
	isUnassigned := bson.D{{"machineid", ""}}
	asserts := append(isAliveDoc, isUnassigned...)
	asserts = append(asserts, subordinatesUnchanged...)

	ops = append(ops, txn.Op{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: asserts,
		Update: bson.D{{"$set", bson.D{{"machineid", mdoc.Id}}}},
	},
		removeStagedAssignmentOp(u.doc.DocID),
	)
	return &Machine{u.st, *mdoc}, ops, nil
}

// Constraints returns the unit's deployment constraints.
func (u *Unit) Constraints() (*constraints.Value, error) {
	cons, err := readConstraints(u.st, u.globalAgentKey())
	if errors.Is(err, errors.NotFound) {
		// Lack of constraints indicates lack of unit.
		return nil, errors.NotFoundf("unit")
	} else if err != nil {
		return nil, err
	}
	if !cons.HasArch() && !cons.HasInstanceType() {
		app, err := u.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if origin := app.CharmOrigin(); origin != nil && origin.Platform != nil {
			if origin.Platform.Architecture != "" {
				cons.Arch = &origin.Platform.Architecture
			}
		}
		if !cons.HasArch() {
			a := constraints.ArchOrDefault(cons, nil)
			cons.Arch = &a
		}
	}
	return &cons, nil
}

// AssignToNewMachine assigns the unit to a new machine, with constraints
// determined according to the application and model constraints at the
// time of unit creation.
func (u *Unit) AssignToNewMachine() (err error) {
	defer assignContextf(&err, u.Name(), "new machine")
	return u.assignToNewMachine("")
}

// assignToNewMachine assigns the unit to a new machine with the
// optional placement directive, with constraints determined according
// to the application and model constraints at the time of unit creation.
func (u *Unit) assignToNewMachine(placement string) error {
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	var m *Machine
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var err error
		u := u // don't change outer var
		if attempt > 0 {
			u, err = u.st.Unit(u.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		cons, err := u.Constraints()
		if err != nil {
			return nil, err
		}
		var containerType instance.ContainerType
		if cons.HasContainer() {
			containerType = *cons.Container
		}
		storageParams, err := u.storageParams()
		if err != nil {
			return nil, errors.Trace(err)
		}
		template := MachineTemplate{
			Base:                  u.doc.Base,
			Constraints:           *cons,
			Jobs:                  []MachineJob{JobHostUnits},
			Placement:             placement,
			Dirty:                 placement != "",
			Volumes:               storageParams.volumes,
			VolumeAttachments:     storageParams.volumeAttachments,
			Filesystems:           storageParams.filesystems,
			FilesystemAttachments: storageParams.filesystemAttachments,
		}
		// Get the ops necessary to create a new machine, and the
		// machine doc that will be added with those operations
		// (which includes the machine id).
		var ops []txn.Op
		m, ops, err = u.assignToNewMachineOps(template, "", containerType)
		return ops, err
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	u.doc.MachineId = m.doc.Id
	return nil
}

type byStorageInstance []StorageAttachment

func (b byStorageInstance) Len() int { return len(b) }

func (b byStorageInstance) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byStorageInstance) Less(i, j int) bool {
	return b[i].StorageInstance().String() < b[j].StorageInstance().String()
}

// storageParams returns parameters for creating volumes/filesystems
// and volume/filesystem attachments when a unit is instantiated.
func (u *Unit) storageParams() (*storageParams, error) {
	params, err := unitStorageParams(u)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, name := range u.doc.Subordinates {
		sub, err := u.st.Unit(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subParams, err := unitStorageParams(sub)
		if err != nil {
			return nil, errors.Trace(err)
		}
		params = combineStorageParams(params, subParams)
	}
	return params, nil
}

func unitStorageParams(u *Unit) (*storageParams, error) {
	sb, err := NewStorageBackend(u.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAttachments, err := sb.UnitStorageAttachments(u.UnitTag())
	if err != nil {
		return nil, errors.Annotate(err, "getting storage attachments")
	}
	ch, err := u.charm()
	if err != nil {
		return nil, errors.Annotate(err, "getting charm")
	}

	// Sort storage attachments so the volume ids are consistent (for testing).
	sort.Sort(byStorageInstance(storageAttachments))

	var storageInstances []*storageInstance
	for _, storageAttachment := range storageAttachments {
		storage, err := sb.storageInstance(storageAttachment.StorageInstance())
		if err != nil {
			return nil, errors.Annotatef(err, "getting storage instance")
		}
		storageInstances = append(storageInstances, storage)
	}
	return storageParamsForUnit(sb, storageInstances, u.UnitTag(), u.Base(), ch.Meta())
}

func storageParamsForUnit(
	sb *storageBackend, storageInstances []*storageInstance, tag names.UnitTag, base Base, chMeta *charm.Meta,
) (*storageParams, error) {

	var volumes []HostVolumeParams
	var filesystems []HostFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)
	for _, storage := range storageInstances {
		storageParams, err := storageParamsForStorageInstance(
			sb, chMeta, base.OS, storage,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}

		volumes = append(volumes, storageParams.volumes...)
		for k, v := range storageParams.volumeAttachments {
			volumeAttachments[k] = v
		}

		filesystems = append(filesystems, storageParams.filesystems...)
		for k, v := range storageParams.filesystemAttachments {
			filesystemAttachments[k] = v
		}
	}
	result := &storageParams{
		volumes,
		volumeAttachments,
		filesystems,
		filesystemAttachments,
	}
	return result, nil
}

// storageParamsForStorageInstance returns parameters for creating
// volumes/filesystems and volume/filesystem attachments for a host that
// the unit will be assigned to. These parameters are based on a given storage
// instance.
func storageParamsForStorageInstance(
	sb *storageBackend,
	charmMeta *charm.Meta,
	osName string,
	storage *storageInstance,
) (*storageParams, error) {

	charmStorage := charmMeta.Storage[storage.StorageName()]

	var volumes []HostVolumeParams
	var filesystems []HostFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)

	switch storage.Kind() {
	case StorageKindFilesystem:
		location, err := FilesystemMountPoint(charmStorage, storage.StorageTag(), osName)
		if err != nil {
			return nil, errors.Annotatef(
				err, "getting filesystem mount point for storage %s",
				storage.StorageName(),
			)
		}
		filesystemAttachmentParams := FilesystemAttachmentParams{
			locationAutoGenerated: charmStorage.Location == "", // auto-generated location
			Location:              location,
			ReadOnly:              charmStorage.ReadOnly,
		}
		var volumeBacked bool
		if filesystem, err := sb.StorageInstanceFilesystem(storage.StorageTag()); err == nil {
			// The filesystem already exists, so just attach it.
			// When creating ops to attach the storage to the
			// machine, we will check if the attachment already
			// exists, and whether the storage can be attached to
			// the machine.
			if !charmStorage.Shared {
				// The storage is not shared, so make sure that it is
				// not currently attached to any other machine. If it
				// is, it should be in the process of being detached.
				existing, err := sb.FilesystemAttachments(filesystem.FilesystemTag())
				if err != nil {
					return nil, errors.Trace(err)
				}
				if len(existing) > 0 {
					return nil, errors.Errorf(
						"%s is attached to %s",
						names.ReadableString(filesystem.FilesystemTag()),
						names.ReadableString(existing[0].Host()),
					)
				}
			}
			filesystemAttachments[filesystem.FilesystemTag()] = filesystemAttachmentParams
			if _, err := filesystem.Volume(); err == nil {
				// The filesystem is volume-backed, so make sure we attach the volume too.
				volumeBacked = true
			}
		} else if errors.Is(err, errors.NotFound) {
			filesystemParams := FilesystemParams{
				storage: storage.StorageTag(),
				Pool:    storage.doc.Constraints.Pool,
				Size:    storage.doc.Constraints.Size,
			}
			filesystems = append(filesystems, HostFilesystemParams{
				filesystemParams, filesystemAttachmentParams,
			})
		} else {
			return nil, errors.Annotatef(err, "getting filesystem for storage %q", storage.Tag().Id())
		}

		if !volumeBacked {
			break
		}
		// Fall through to attach the volume that backs the filesystem.
		fallthrough

	case StorageKindBlock:
		volumeAttachmentParams := VolumeAttachmentParams{
			charmStorage.ReadOnly,
		}
		if volume, err := sb.StorageInstanceVolume(storage.StorageTag()); err == nil {
			// The volume already exists, so just attach it. When
			// creating ops to attach the storage to the machine,
			// we will check if the attachment already exists, and
			// whether the storage can be attached to the machine.
			if !charmStorage.Shared {
				// The storage is not shared, so make sure that it is
				// not currently attached to any other machine. If it
				// is, it should be in the process of being detached.
				existing, err := sb.VolumeAttachments(volume.VolumeTag())
				if err != nil {
					return nil, errors.Trace(err)
				}
				if len(existing) > 0 {
					return nil, errors.Errorf(
						"%s is attached to %s",
						names.ReadableString(volume.VolumeTag()),
						names.ReadableString(existing[0].Host()),
					)
				}
			}
			volumeAttachments[volume.VolumeTag()] = volumeAttachmentParams
		} else if errors.Is(err, errors.NotFound) {
			volumeParams := VolumeParams{
				storage: storage.StorageTag(),
				Pool:    storage.doc.Constraints.Pool,
				Size:    storage.doc.Constraints.Size,
			}
			volumes = append(volumes, HostVolumeParams{
				volumeParams, volumeAttachmentParams,
			})
		} else {
			return nil, errors.Annotatef(err, "getting volume for storage %q", storage.Tag().Id())
		}
	default:
		return nil, errors.Errorf("invalid storage kind %v", storage.Kind())
	}
	result := &storageParams{
		volumes,
		volumeAttachments,
		filesystems,
		filesystemAttachments,
	}
	return result, nil
}

var hasNoContainersTerm = bson.DocElem{
	"$or", []bson.D{
		{{"children", bson.D{{"$size", 0}}}},
		{{"children", bson.D{{"$exists", false}}}},
	}}

// UnassignFromMachine removes the assignment between this unit and the
// machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	// TODO check local machine id and add an assert that the
	// machine id is as expected.
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"machineid", ""}}}},
	}}
	if u.doc.MachineId != "" {
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     u.st.docID(u.doc.MachineId),
			Assert: txn.DocExists,
			Update: bson.D{{"$pull", bson.D{{"principals", u.doc.Name}}}},
		})
	}
	err = u.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot unassign unit %q from machine: %v", u, onAbort(err, errors.NotFoundf("machine")))
	}
	u.doc.MachineId = ""
	return nil
}

// ActionSpecsByName is a map of action names to their respective ActionSpec.
type ActionSpecsByName map[string]charm.ActionSpec

// PrepareActionPayload returns the payload to use in creating an action for this unit.
// Note that the use of spec.InsertDefaults mutates payload.
func (u *Unit) PrepareActionPayload(name string, payload map[string]interface{}, parallel *bool, executionGroup *string) (map[string]interface{}, bool, string, error) {
	if len(name) == 0 {
		return nil, false, "", errors.New("no action name given")
	}

	// If the action is predefined inside juju, get spec from map
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		specs, err := u.ActionSpecs()
		if err != nil {
			return nil, false, "", err
		}
		spec, ok = specs[name]
		if !ok {
			return nil, false, "", errors.Errorf("action %q not defined on unit %q", name, u.Name())
		}
	}
	// Reject bad payloads before attempting to insert defaults.
	err := spec.ValidateParams(payload)
	if err != nil {
		return nil, false, "", errors.Trace(err)
	}
	payloadWithDefaults, err := spec.InsertDefaults(payload)
	if err != nil {
		return nil, false, "", errors.Trace(err)
	}

	runParallel := spec.Parallel
	if parallel != nil {
		runParallel = *parallel
	}
	runExecutionGroup := spec.ExecutionGroup
	if executionGroup != nil {
		runExecutionGroup = *executionGroup
	}
	return payloadWithDefaults, runParallel, runExecutionGroup, nil
}

// ActionSpecs gets the ActionSpec map for the Unit's charm.
func (u *Unit) ActionSpecs() (ActionSpecsByName, error) {
	none := ActionSpecsByName{}
	ch, err := u.charm()
	if err != nil {
		return none, errors.Trace(err)
	}
	chActions := ch.Actions()
	if chActions == nil || len(chActions.ActionSpecs) == 0 {
		return none, errors.Errorf("no actions defined on charm %q", ch.URL())
	}
	return chActions.ActionSpecs, nil
}

// CancelAction removes a pending Action from the queue for this
// ActionReceiver and marks it as cancelled.
func (u *Unit) CancelAction(action Action) (Action, error) {
	return action.Finish(ActionResults{Status: ActionCancelled})
}

// WatchActionNotifications starts and returns a StringsWatcher that
// notifies when actions with Id prefixes matching this Unit are added
func (u *Unit) WatchActionNotifications() StringsWatcher {
	return u.st.watchActionNotificationsFilteredBy(u)
}

// WatchPendingActionNotifications is part of the ActionReceiver interface.
func (u *Unit) WatchPendingActionNotifications() StringsWatcher {
	return u.st.watchEnqueuedActionsFilteredBy(u)
}

// Actions returns a list of actions pending or completed for this unit.
func (u *Unit) Actions() ([]Action, error) {
	return u.st.matchingActions(u)
}

// CompletedActions returns a list of actions that have finished for
// this unit.
func (u *Unit) CompletedActions() ([]Action, error) {
	return u.st.matchingActionsCompleted(u)
}

// PendingActions returns a list of actions pending for this unit.
func (u *Unit) PendingActions() ([]Action, error) {
	return u.st.matchingActionsPending(u)
}

// RunningActions returns a list of actions running on this unit.
func (u *Unit) RunningActions() ([]Action, error) {
	return u.st.matchingActionsRunning(u)
}

// StorageConstraints returns the unit's storage constraints.
func (u *Unit) StorageConstraints() (map[string]StorageConstraints, error) {
	if u.doc.CharmURL == nil {
		app, err := u.st.Application(u.doc.Application)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return app.StorageConstraints()
	}
	key := applicationStorageConstraintsKey(u.doc.Application, u.doc.CharmURL)
	cons, err := readStorageConstraints(u.st, key)
	if errors.Is(err, errors.NotFound) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return cons, nil
}

type addUnitOpsArgs struct {
	unitDoc            *unitDoc
	containerDoc       *cloudContainerDoc
	agentStatusDoc     statusDoc
	workloadStatusDoc  *statusDoc
	workloadVersionDoc *statusDoc
}

// addUnitOps returns the operations required to add a unit to the units
// collection, along with all the associated expected other unit entries. This
// method is used by both the *Application.addUnitOpsWithCons method and the
// migration import code.
func addUnitOps(st *State, args addUnitOpsArgs) ([]txn.Op, error) {
	name := args.unitDoc.Name
	agentGlobalKey := unitAgentGlobalKey(name)

	// TODO: consider the constraints op
	// TODO: consider storageOps
	var prereqOps []txn.Op
	if args.containerDoc != nil {
		prereqOps = append(prereqOps, txn.Op{
			C:      cloudContainersC,
			Id:     args.containerDoc.Id,
			Insert: args.containerDoc,
			Assert: txn.DocMissing,
		})
	}
	prereqOps = append(prereqOps,
		createStatusOp(st, agentGlobalKey, args.agentStatusDoc),
		createStatusOp(st, unitGlobalKey(name), *args.workloadStatusDoc),
		createStatusOp(st, globalWorkloadVersionKey(name), *args.workloadVersionDoc),
	)

	return append(prereqOps, txn.Op{
		C:      unitsC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: args.unitDoc,
	}), nil
}
