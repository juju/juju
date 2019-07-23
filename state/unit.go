// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	mgoutils "github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/tools"
)

var unitLogger = loggo.GetLogger("juju.state.unit")

// AssignmentPolicy controls what machine a unit will be assigned to.
type AssignmentPolicy string

const (
	// AssignLocal indicates that all application units should be assigned
	// to machine 0.
	AssignLocal AssignmentPolicy = "local"

	// AssignClean indicates that every application unit should be assigned
	// to a machine which never previously has hosted any units, and that
	// new machines should be launched if required.
	AssignClean AssignmentPolicy = "clean"

	// AssignCleanEmpty indicates that every application unit should be assigned
	// to a machine which never previously has hosted any units, and which is not
	// currently hosting any containers, and that new machines should be launched if required.
	AssignCleanEmpty AssignmentPolicy = "clean-empty"

	// AssignNew indicates that every application unit should be assigned to a new
	// dedicated machine.  A new machine will be launched for each new unit.
	AssignNew AssignmentPolicy = "new"
)

// ResolvedMode describes the way state transition errors
// are resolved.
type ResolvedMode string

// These are available ResolvedMode values.
const (
	ResolvedNone       ResolvedMode = ""
	ResolvedRetryHooks ResolvedMode = "retry-hooks"
	ResolvedNoHooks    ResolvedMode = "no-hooks"
)

// port identifies a network port number for a particular protocol.
// TODO(mue) Not really used anymore, se bellow. Can be removed when
// cleaning unitDoc.
type port struct {
	Protocol string `bson:"protocol"`
	Number   int    `bson:"number"`
}

// unitDoc represents the internal state of a unit in MongoDB.
// Note the correspondence with UnitInfo in apiserver/params.
type unitDoc struct {
	DocID                  string `bson:"_id"`
	Name                   string `bson:"name"`
	ModelUUID              string `bson:"model-uuid"`
	Application            string
	Series                 string
	CharmURL               *charm.URL
	Principal              string
	Subordinates           []string
	StorageAttachmentCount int `bson:"storageattachmentcount"`
	MachineId              string
	Resolved               ResolvedMode
	Tools                  *tools.Tools `bson:",omitempty"`
	Life                   Life
	TxnRevno               int64 `bson:"txn-revno"`
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
	return u.modelType == ModelTypeIAAS
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
		return nil, fmt.Errorf("unit charm not set")
	}

	// TODO (manadart 2019-02-21) Factor the current generation into this call.
	s, err := charmSettingsWithDefaults(u.st, u.doc.CharmURL, u.doc.Application, model.GenerationMaster)
	if err != nil {
		return nil, errors.Annotatef(err, "charm config for unit %q", u.Name())
	}
	return s, nil
}

// ApplicationName returns the application name.
func (u *Unit) ApplicationName() string {
	return u.doc.Application
}

// Series returns the deployed charm's series.
func (u *Unit) Series() string {
	return u.doc.Series
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

// globalWorkloadVersionKey returns the global database key for the
// workload version status key for this unit.
func globalWorkloadVersionKey(name string) string {
	return unitGlobalKey(name) + "#sat#workload-version"
}

// globalAgentKey returns the global database key for the unit.
func (u *Unit) globalAgentKey() string {
	return unitAgentGlobalKey(u.doc.Name)
}

// globalMeterStatusKey returns the global database key for the meter status of the unit.
func (u *Unit) globalMeterStatusKey() string {
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
	if errors.IsNotFound(err) {
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
		badge:     "workload",
		globalKey: u.globalWorkloadVersionKey(),
		status:    status.Active,
		message:   version,
		updated:   &now,
	})
}

// WorkloadVersionHistory returns a HistoryGetter which enables the
// caller to request past workload version changes.
func (u *Unit) WorkloadVersionHistory() *HistoryGetter {
	return &HistoryGetter{st: u.st, globalKey: u.globalWorkloadVersionKey()}
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
func (u *Unit) SetAgentVersion(v version.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set agent version for unit %q", u)
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
		return onAbort(err, ErrDead)
	}
	u.doc.Tools = versionedTool
	return nil
}

// SetPassword sets the password for the machine's agent.
func (u *Unit) SetPassword(password string) error {
	if len(password) < utils.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	return u.setPasswordHash(utils.AgentPasswordHash(password))
}

// setPasswordHash sets the underlying password hash in the database directly
// to the value supplied. This is split out from SetPassword to allow direct
// manipulation in tests (to check for backwards compatibility).
func (u *Unit) setPasswordHash(passwordHash string) error {
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"passwordhash", passwordHash}}}},
	}}
	err := u.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of unit %q: %v", u, onAbort(err, ErrDead))
	}
	u.doc.PasswordHash = passwordHash
	return nil
}

// PasswordValid returns whether the given password is valid
// for the given unit.
func (u *Unit) PasswordValid(password string) bool {
	agentHash := utils.AgentPasswordHash(password)
	if agentHash == u.doc.PasswordHash {
		return true
	}
	return false
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
func (op *UpdateUnitOperation) Build(attempt int) ([]txn.Op, error) {
	op.setStatusDocs = make(map[string]statusDoc)

	containerInfo, err := op.unit.cloudContainer()
	if err != nil && !errors.IsNotFound(err) {
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
		logger.Debugf("unit %q has provider id %q which changed to %q",
			op.unit.Name(), containerInfo.ProviderId, newProviderId)
	}

	if op.props.ProviderId != nil {
		containerInfo.ProviderId = newProviderId
	}
	if op.props.Address != nil {
		networkAddr := network.NewScopedAddress(*op.props.Address, network.ScopeMachineLocal)
		addr := fromNetworkAddress(networkAddr, OriginProvider)
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
			if !errors.IsNotFound(err) {
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

		modifiedStatus := caasUnitDisplayStatus(unitStatus, cloudContainerStatus, true)
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
	// We can't include in the ops slice the necessary status history updates,
	// so as with existing practice, do a best effort update of status history.
	for key, doc := range op.setStatusDocs {
		probablyUpdateStatusHistory(op.unit.st.db(), key, doc)
	}
	return nil
}

// Destroy, when called on a Alive unit, advances its lifecycle as far as
// possible; it otherwise has no effect. In most situations, the unit's
// life is just set to Dying; but if a principal unit that is not assigned
// to a provisioned machine is Destroyed, it will be removed from state
// directly.
func (u *Unit) Destroy() error {
	errs, err := u.DestroyWithForce(false, time.Duration(0))
	if len(errs) != 0 {
		logger.Warningf("operational errors destroying unit %v: %v", u.Name(), errs)
	}
	return err
}

// DestroyWithForce does the same thing as Destroy() but
// ignores errors.
func (u *Unit) DestroyWithForce(force bool, maxWait time.Duration) (errs []error, err error) {
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			u.doc.Life = Dying
		}
	}()
	op := u.DestroyOperation()
	op.Force = force
	op.MaxWait = maxWait
	err = u.st.ApplyOperation(op)
	return op.Errors, err
}

// DestroyOperation returns a model operation that will destroy the unit.
func (u *Unit) DestroyOperation() *DestroyUnitOperation {
	return &DestroyUnitOperation{
		unit: &Unit{st: u.st, doc: u.doc, modelType: u.modelType},
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
}

// Build is part of the ModelOperation interface.
func (op *DestroyUnitOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.unit.Refresh(); errors.IsNotFound(err) {
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
			logger.Warningf("forcing unit destruction for %v despite error %v", op.unit.Name(), err)
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
	if err := op.eraseHistory(); err != nil {
		if !op.Force {
			logger.Errorf("cannot delete history for unit %q: %v", op.unit.globalKey(), err)
		}
		op.AddError(errors.Errorf("force erase unit's %q history proceeded despite encountering ERROR %v", op.unit.globalKey(), err))
	}
	return nil
}

func (op *DestroyUnitOperation) eraseHistory() error {
	if err := eraseStatusHistory(op.unit.st, op.unit.globalKey()); err != nil {
		one := errors.Annotate(err, "workload")
		if !op.Force {
			return one
		}
		op.AddError(one)
	}
	if err := eraseStatusHistory(op.unit.st, op.unit.globalAgentKey()); err != nil {
		one := errors.Annotate(err, "agent")
		if !op.Force {
			return one
		}
		op.AddError(one)
	}
	if err := eraseStatusHistory(op.unit.st, op.unit.globalWorkloadVersionKey()); err != nil {
		one := errors.Annotate(err, "version")
		if !op.Force {
			return one
		}
		op.AddError(one)
	}
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

	// if the minUnits document exists, we need to increment the revno so that
	// it is obvious the min units count is changing.
	minUnitsOp := minUnitsTriggerOp(op.unit.st, op.unit.ApplicationName())
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
		minUnitsExists, err := doesMinUnitsExist(op.unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if minUnitsExists {
			ops = append(ops, minUnitsOp)
		}
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
	if errors.IsNotFound(agentErr) {
		return nil, errAlreadyDying
	} else if agentErr != nil {
		if !op.Force {
			return nil, errors.Trace(agentErr)
		}
	}

	// This has to be a function since we want to delay the evaluation of the value,
	// in case agent erred out.
	notAllocating := func() bool {
		return (isAssigned || !shouldBeAssigned) && agentStatusInfo.Status != status.Allocating
	}
	if agentErr == nil && notAllocating() {
		return setDyingOps(agentErr)
	}
	switch agentStatusInfo.Status {
	case status.Error, status.Allocating:
	default:
		err := errors.Errorf("unexpected unit state - unit with status %v is not deployed", agentStatusInfo.Status)
		if !op.Force {
			return nil, err
		}
		op.AddError(err)
	}

	statusOp := txn.Op{
		C:      statusesC,
		Id:     op.unit.st.docID(agentStatusDocId),
		Assert: bson.D{{"status", agentStatusInfo.Status}},
	}
	removeAsserts := append(isAliveDoc, bson.DocElem{
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
	removeOps, err := op.unit.removeOps(removeAsserts, &op.ForcedOperation, op.DestroyStorage)
	if err == errAlreadyRemoved {
		return nil, errAlreadyDying
	} else if err != nil {
		if !op.Force {
			return nil, err
		}
		op.AddError(err)
	}
	ops := []txn.Op{statusOp, minUnitsOp}
	ops = append(ops, removeOps...)
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
		unitLogger.Tracef("unit %v unassigned", u)
		return nil, nil
	}

	m, err := u.st.Machine(u.doc.MachineId)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	containerCheck := true // whether container conditions allow destroying the host machine
	containers, err := m.Containers()
	if err != nil {
		if !op.Force {
			return nil, err
		}
		op.AddError(err)
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
	} else if m.doc.HasVote {
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
			{{"hasvote", bson.D{{"$ne", true}}}},
		}}}
		controllerNodeAssert = txn.DocMissing
		_, err = m.st.ControllerNode(m.Id())
		if err == nil {
			controllerNodeAssert = bson.D{{"has-vote", false}}
		}
	} else {
		machineAssert = bson.D{{"$or", []bson.D{
			{{"principals", bson.D{{"$ne", []string{u.doc.Name}}}}},
			{{"jobs", bson.D{{"$in", []MachineJob{JobManageModel}}}}},
			{{"hasvote", true}},
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
func (u *Unit) removeOps(asserts bson.D, op *ForcedOperation, destroyStorage bool) ([]txn.Op, error) {
	app, err := u.st.Application(u.doc.Application)
	if errors.IsNotFound(err) {
		// If the application has been removed, the unit must already have been.
		return nil, errAlreadyRemoved
	} else if err != nil {
		// If we cannot find application, no amount of force will succeed after this point.
		return nil, err
	}
	return app.removeUnitOps(u, asserts, op, destroyStorage)
}

// ErrUnitHasSubordinates is a standard error to indicate that a Unit
// cannot complete an operation to end its life because it still has
// subordinate applications.
var ErrUnitHasSubordinates = errors.New("unit has subordinates")

var unitHasNoSubordinates = bson.D{{
	"$or", []bson.D{
		{{"subordinates", bson.D{{"$size", 0}}}},
		{{"subordinates", bson.D{{"$exists", false}}}},
	},
}}

// ErrUnitHasStorageAttachments is a standard error to indicate that
// a Unit cannot complete an operation to end its life because it still
// has storage attachments.
var ErrUnitHasStorageAttachments = errors.New("unit has storage attachments")

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
	if err := u.Refresh(); errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	if len(u.doc.Subordinates) > 0 {
		return ErrUnitHasSubordinates
	}
	return ErrUnitHasStorageAttachments
}

// RemoveOperation returns a model operation that will remove the unit.
func (u *Unit) RemoveOperation(force bool) *RemoveUnitOperation {
	return &RemoveUnitOperation{
		unit:            &Unit{st: u.st, doc: u.doc, modelType: u.modelType},
		ForcedOperation: ForcedOperation{Force: force},
	}
}

// ForcedOperation that allowas accumulation of operational errors and
// can be forced.
type ForcedOperation struct {
	// Force controls whether or not the removal of a unit
	// will be forced, i.e. ignore operational errors.
	Force bool

	// Errors contains errors encountered while applying this operation.
	// Generally, these are non-fatal errors that have been encountered
	// during, say, force. They may not have prevented the operation from being
	// aborted but the user might still want to know about them.
	Errors []error

	// MaxWait specifies the amount of time that each step in relation destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait time.Duration
}

// AddError adds an error to the collection of errors for this operation.
func (op *ForcedOperation) AddError(one ...error) {
	op.Errors = append(op.Errors, one...)
}

// LastError returns last added error for this operation.
func (op *ForcedOperation) LastError() error {
	if len(op.Errors) == 0 {
		return nil
	}
	return op.Errors[len(op.Errors)-1]
}

// RemoveUnitOperation is a model operation for removing a unit.
type RemoveUnitOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// unit holds the unit to remove.
	unit *Unit
}

// Build is part of the ModelOperation interface.
func (op *RemoveUnitOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.unit.Refresh(); errors.IsNotFound(err) {
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
			logger.Warningf("forcing unit removal for %v despite error %v", op.unit.Name(), err)
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
func (u *Unit) Remove() error {
	_, err := u.RemoveWithForce(false, time.Duration(0))
	return err
}

// RemoveWithForce removes the unit from state similar to the unit.Remove() but
// it ignores errors.
// In addition, this function also returns all non-fatal operational errors
// encountered.
func (u *Unit) RemoveWithForce(force bool, maxWait time.Duration) ([]error, error) {
	op := u.RemoveOperation(force)
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
	relations, err := applicationRelations(op.unit.st, op.unit.doc.Application)
	if err != nil {
		if !op.Force {
			return nil, err
		}
		op.AddError(err)
	} else {
		failRelations := false
		for _, rel := range relations {
			ru, err := rel.Unit(op.unit)
			if err != nil {
				op.AddError(err)
				failRelations = true
				continue
			}
			leaveScopOps, err := ru.leaveScopeForcedOps(&op.ForcedOperation)
			if err != nil && err != jujutxn.ErrNoOperations {
				op.AddError(err)
				failRelations = true
			}
			ops = append(ops, leaveScopOps...)
		}
		if !op.Force && failRelations {
			return nil, op.LastError()
		}
	}

	// Now we're sure we haven't left any scopes occupied by this unit, we
	// can safely remove the document.
	unitRemoveOps, err := op.unit.removeOps(isDeadDoc, &op.ForcedOperation, false)
	if err != nil {
		if !op.Force {
			return nil, err
		}
		op.AddError(err)
	}
	return append(ops, unitRemoveOps...), nil
}

// Resolved returns the resolved mode for the unit.
func (u *Unit) Resolved() ResolvedMode {
	return u.doc.Resolved
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
	candidates, err := applicationRelations(u.st, u.doc.Application)
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

// DeployerTag returns the tag of the agent responsible for deploying
// the unit. If no such entity can be determined, false is returned.
func (u *Unit) DeployerTag() (names.Tag, bool) {
	if u.doc.Principal != "" {
		return names.NewUnitTag(u.doc.Principal), true
	} else if u.doc.MachineId != "" {
		return names.NewMachineTag(u.doc.MachineId), true
	}
	return nil, false
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
func (u *Unit) PublicAddress() (network.Address, error) {
	if !u.ShouldBeAssigned() {
		return u.scopedAddress("public")
	}
	m, err := u.machine()
	if err != nil {
		unitLogger.Tracef("%v", err)
		return network.Address{}, errors.Trace(err)
	}
	return m.PublicAddress()
}

// PrivateAddress returns the private address of the unit.
func (u *Unit) PrivateAddress() (network.Address, error) {
	if !u.ShouldBeAssigned() {
		return u.scopedAddress("private")
	}
	m, err := u.machine()
	if err != nil {
		unitLogger.Tracef("%v", err)
		return network.Address{}, errors.Trace(err)
	}
	return m.PrivateAddress()
}

func (u *Unit) scopedAddress(scope string) (network.Address, error) {
	addr, err := u.serviceAddress(scope)
	if err == nil {
		return addr, nil
	}
	if network.IsNoAddressError(err) {
		return u.containerAddress()
	}
	return network.Address{}, errors.Trace(err)
}

// AllAddresses returns the public and private addresses
// plus the container address of the unit (if known).
// Only relevant for CAAS models - will return an empty
// slice for IAAS models.
func (u *Unit) AllAddresses() ([]network.Address, error) {
	if u.ShouldBeAssigned() {
		return nil, nil
	}

	// First the addresses of the service.
	app, err := u.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceInfo, err := app.ServiceInfo()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		return serviceInfo.Addresses(), nil
	}

	// If there's no service deployed then it's ok
	// to fallback to the container address.
	addr, err := u.containerAddress()
	if network.IsNoAddressError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []network.Address{addr}, nil
}

// containerAddress returns the address of the pod's container.
func (u *Unit) containerAddress() (network.Address, error) {
	containerInfo, err := u.cloudContainer()
	if errors.IsNotFound(err) {
		return network.Address{}, network.NoAddressError("container")
	}
	if err != nil {
		return network.Address{}, errors.Trace(err)
	}
	addr := containerInfo.Address
	if addr == nil {
		return network.Address{}, network.NoAddressError("container")
	}
	return addr.networkAddress(), nil
}

// serviceAddress returns the address of the service
// managing the pods in which the unit workload is running.
func (u *Unit) serviceAddress(scope string) (network.Address, error) {
	addresses, err := u.AllAddresses()
	if err != nil {
		return network.Address{}, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return network.Address{}, network.NoAddressError(scope)
	}
	getStrictPublicAddr := func(addresses []network.Address) (network.Address, bool) {
		addr, ok := network.SelectPublicAddress(addresses)
		return addr, ok && addr.Scope == network.ScopePublic
	}

	getInternalAddr := func(addresses []network.Address) (network.Address, bool) {
		return network.SelectInternalAddress(addresses, false)
	}

	var addrMatch func([]network.Address) (network.Address, bool)
	switch scope {
	case "public":
		addrMatch = getStrictPublicAddr
	case "private":
		addrMatch = getInternalAddr
	default:
		return network.Address{}, errors.NotValidf("address scope %q", scope)
	}

	addr, found := addrMatch(addresses)
	if !found {
		return network.Address{}, network.NoAddressError(scope)
	}
	return addr, nil
}

// AvailabilityZone returns the name of the availability zone into which
// the unit's machine instance was provisioned.
func (u *Unit) AvailabilityZone() (string, error) {
	m, err := u.machine()
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.AvailabilityZone()
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
		return fmt.Errorf("cannot refresh unit %q: %v", u, err)
	}
	return nil
}

// Agent Returns an agent by its unit's name.
func (u *Unit) Agent() *UnitAgent {
	return newUnitAgent(u.st, u.Tag(), u.Name())
}

// AgentHistory returns an StatusHistoryGetter which can
//be used to query the status history of the unit's agent.
func (u *Unit) AgentHistory() status.StatusHistoryGetter {
	return u.Agent()
}

// SetAgentStatus calls SetStatus for this unit's agent, this call
// is equivalent to the former call to SetStatus when Agent and Unit
// where not separate entities.
func (u *Unit) SetAgentStatus(agentStatus status.StatusInfo) error {
	agent := newUnitAgent(u.st, u.Tag(), u.Name())
	s := status.StatusInfo{
		Status:  agentStatus.Status,
		Message: agentStatus.Message,
		Data:    agentStatus.Data,
		Since:   agentStatus.Since,
	}
	return agent.SetStatus(s)
}

// AgentStatus calls Status for this unit's agent, this call
// is equivalent to the former call to Status when Agent and Unit
// where not separate entities.
func (u *Unit) AgentStatus() (status.StatusInfo, error) {
	agent := newUnitAgent(u.st, u.Tag(), u.Name())
	return agent.Status()
}

// StatusHistory returns a slice of at most <size> StatusInfo items
// or items as old as <date> or items newer than now - <delta> time
// representing past statuses for this unit.
func (u *Unit) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        u.st.db(),
		globalKey: u.globalKey(),
		filter:    filter,
	}
	return statusHistory(args)
}

// Status returns the status of the unit.
// This method relies on globalKey instead of globalAgentKey since it is part of
// the effort to separate Unit from UnitAgent. Now the Status for UnitAgent is in
// the UnitAgent struct.
func (u *Unit) Status() (status.StatusInfo, error) {
	// The current health spec says when a hook error occurs, the workload should
	// be in error state, but the state model more correctly records the agent
	// itself as being in error. So we'll do that model translation here.
	// TODO(fwereade) as on unitagent, this transformation does not belong here.
	// For now, pretend we're always reading the unit status.
	info, err := getStatus(u.st.db(), u.globalAgentKey(), "unit")
	if err != nil {
		return status.StatusInfo{}, err
	}
	if info.Status != status.Error {
		info, err = getStatus(u.st.db(), u.globalKey(), "unit")
		if err != nil {
			return status.StatusInfo{}, err
		}
	}
	return info, nil
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
// This method relies on globalKey instead of globalAgentKey since it is part of
// the effort to separate Unit from UnitAgent. Now the SetStatus for UnitAgent is in
// the UnitAgent struct.
func (u *Unit) SetStatus(unitStatus status.StatusInfo) error {
	if !status.ValidWorkloadStatus(unitStatus.Status) {
		return errors.Errorf("cannot set invalid status %q", unitStatus.Status)
	}

	var newHistory *statusDoc
	if u.modelType == ModelTypeCAAS {
		// Caas Charms currently have no way to query workload status;
		// Cloud container status might contradict what the charm is
		// attempting to set, make sure the right history is set.
		cloudContainerStatus, err := getStatus(u.st.db(), globalCloudContainerKey(u.Name()), "cloud container")
		if err != nil {
			if !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
		}
		expectWorkload, err := expectWorkload(u.st, u.ApplicationName())
		if err != nil {
			return errors.Trace(err)
		}
		newHistory, err = caasHistoryRewriteDoc(unitStatus, cloudContainerStatus, expectWorkload, caasUnitDisplayStatus, u.st.clock())
		if err != nil {
			return errors.Trace(err)
		}
	}

	return setStatus(u.st.db(), setStatusParams{
		badge:            "unit",
		globalKey:        u.globalKey(),
		status:           unitStatus.Status,
		message:          unitStatus.Message,
		rawData:          unitStatus.Data,
		updated:          timeOrNow(unitStatus.Since, u.st.clock()),
		historyOverwrite: newHistory,
	})
}

// OpenPortsOnSubnet opens the given port range and protocol for the unit on the
// given subnet, which can be empty. When non-empty, subnetID must refer to an
// existing, alive subnet, otherwise an error is returned. Returns an error if
// opening the requested range conflicts with another already opened range on
// the same subnet and and the unit's assigned machine.
func (u *Unit) OpenPortsOnSubnet(subnetID, protocol string, fromPort, toPort int) (err error) {
	ports, err := NewPortRange(u.Name(), fromPort, toPort, protocol)
	if err != nil {
		return errors.Annotatef(err, "invalid port range %v-%v/%v", fromPort, toPort, protocol)
	}
	defer errors.DeferredAnnotatef(&err, "cannot open ports %v for unit %q on subnet %q", ports, u, subnetID)

	machineID, err := u.AssignedMachineId()
	if err != nil {
		return errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	if err := u.checkSubnetAliveWhenSet(subnetID); err != nil {
		return errors.Trace(err)
	}

	machinePorts, err := getOrCreatePorts(u.st, machineID, subnetID)
	if err != nil {
		return errors.Annotate(err, "cannot get or create ports")
	}

	return machinePorts.OpenPorts(ports)
}

func (u *Unit) checkSubnetAliveWhenSet(subnetID string) error {
	if subnetID == "" {
		return nil
	} else if !names.IsValidSubnet(subnetID) {
		return errors.Errorf("invalid subnet ID %q", subnetID)
	}

	subnet, err := u.st.Subnet(subnetID)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotatef(err, "getting subnet %q", subnetID)
	} else if errors.IsNotFound(err) || subnet.Life() != Alive {
		return errors.Errorf("subnet %q not found or not alive", subnetID)
	}
	return nil
}

// ClosePortsOnSubnet closes the given port range and protocol for the unit on
// the given subnet, which can be empty. When non-empty, subnetID must refer to
// an existing, alive subnet, otherwise an error is returned.
func (u *Unit) ClosePortsOnSubnet(subnetID, protocol string, fromPort, toPort int) (err error) {
	ports, err := NewPortRange(u.Name(), fromPort, toPort, protocol)
	if err != nil {
		return errors.Annotatef(err, "invalid port range %v-%v/%v", fromPort, toPort, protocol)
	}
	defer errors.DeferredAnnotatef(&err, "cannot close ports %v for unit %q on subnet %q", ports, u, subnetID)

	machineID, err := u.AssignedMachineId()
	if err != nil {
		return errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	if err := u.checkSubnetAliveWhenSet(subnetID); err != nil {
		return errors.Trace(err)
	}

	machinePorts, err := getOrCreatePorts(u.st, machineID, subnetID)
	if err != nil {
		return errors.Annotate(err, "cannot get or create ports")
	}

	return machinePorts.ClosePorts(ports)
}

// OpenPorts opens the given port range and protocol for the unit, if it does
// not conflict with another already opened range on the unit's assigned
// machine.
//
// TODO(dimitern): This should be removed once we use OpenPortsOnSubnet across
// the board, passing subnet IDs explicitly.
func (u *Unit) OpenPorts(protocol string, fromPort, toPort int) error {
	return u.OpenPortsOnSubnet("", protocol, fromPort, toPort)
}

// ClosePorts closes the given port range and protocol for the unit.
//
// TODO(dimitern): This should be removed once we use ClosePortsOnSubnet across
// the board, passing subnet IDs explicitly.
func (u *Unit) ClosePorts(protocol string, fromPort, toPort int) (err error) {
	return u.ClosePortsOnSubnet("", protocol, fromPort, toPort)
}

// OpenPortOnSubnet opens the given port and protocol for the unit on the given
// subnet, which can be empty. When non-empty, subnetID must refer to an
// existing, alive subnet, otherwise an error is returned.
func (u *Unit) OpenPortOnSubnet(subnetID, protocol string, number int) error {
	return u.OpenPortsOnSubnet(subnetID, protocol, number, number)
}

// ClosePortOnSubnet closes the given port and protocol for the unit on the given
// subnet, which can be empty. When non-empty, subnetID must refer to an
// existing, alive subnet, otherwise an error is returned.
func (u *Unit) ClosePortOnSubnet(subnetID, protocol string, number int) error {
	return u.ClosePortsOnSubnet(subnetID, protocol, number, number)
}

// OpenPort opens the given port and protocol for the unit.
//
// TODO(dimitern): This should be removed once we use OpenPort(s)OnSubnet across
// the board, passing subnet IDs explicitly.
func (u *Unit) OpenPort(protocol string, number int) error {
	return u.OpenPortOnSubnet("", protocol, number)
}

// ClosePort closes the given port and protocol for the unit.
//
// TODO(dimitern): This should be removed once we use ClosePortsOnSubnet across
// the board, passing subnet IDs explicitly.
func (u *Unit) ClosePort(protocol string, number int) error {
	return u.ClosePortOnSubnet("", protocol, number)
}

// OpenedPortsOnSubnet returns a slice containing the open port ranges of the
// unit on the given subnet ID, which can be empty. When subnetID is not empty,
// it must refer to an existing, alive subnet, otherwise an error is returned.
// Also, when no ports are yet open for the unit on that subnet, no error and
// empty slice is returned.
func (u *Unit) OpenedPortsOnSubnet(subnetID string) ([]corenetwork.PortRange, error) {
	machineID, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	if err := u.checkSubnetAliveWhenSet(subnetID); err != nil {
		return nil, errors.Trace(err)
	}

	machinePorts, err := getPorts(u.st, machineID, subnetID)
	var result []corenetwork.PortRange
	if errors.IsNotFound(err) {
		return result, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "failed getting ports for unit %q, subnet %q", u, subnetID)
	}
	ports := machinePorts.PortsForUnit(u.Name())
	for _, port := range ports {
		result = append(result, corenetwork.PortRange{
			Protocol: port.Protocol,
			FromPort: port.FromPort,
			ToPort:   port.ToPort,
		})
	}
	corenetwork.SortPortRanges(result)
	return result, nil
}

// OpenedPorts returns a slice containing the open port ranges of the
// unit.
//
// TODO(dimitern): This should be removed once we use OpenedPortsOnSubnet across
// the board, passing subnet IDs explicitly.
func (u *Unit) OpenedPorts() ([]corenetwork.PortRange, error) {
	return u.OpenedPortsOnSubnet("")
}

// CharmURL returns the charm URL this unit is currently using.
func (u *Unit) CharmURL() (*charm.URL, bool) {
	if u.doc.CharmURL == nil {
		return nil, false
	}
	return u.doc.CharmURL, true
}

// SetCharmURL marks the unit as currently using the supplied charm URL.
// An error will be returned if the unit is dead, or the charm URL not known.
func (u *Unit) SetCharmURL(curl *charm.URL) error {
	if curl == nil {
		return fmt.Errorf("cannot set nil charm url")
	}

	db, closer := u.st.newDB()
	defer closer()
	units, closer := db.GetCollection(unitsC)
	defer closer()
	charms, closer := db.GetCollection(charmsC)
	defer closer()

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
				return nil, ErrDead
			}
		}
		sel := bson.D{{"_id", u.doc.DocID}, {"charmurl", curl}}
		if count, err := units.Find(sel).Count(); err != nil {
			return nil, errors.Trace(err)
		} else if count == 1 {
			// Already set
			return nil, jujutxn.ErrNoOperations
		}
		if count, err := charms.FindId(curl.String()).Count(); err != nil {
			return nil, errors.Trace(err)
		} else if count < 1 {
			return nil, errors.Errorf("unknown charm url %q", curl)
		}

		// Add a reference to the application settings for the new charm.
		incOps, err := appCharmIncRefOps(u.st, u.doc.Application, curl, false)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Set the new charm URL.
		differentCharm := bson.D{{"charmurl", bson.D{{"$ne", curl}}}}
		ops := append(incOps,
			txn.Op{
				C:      unitsC,
				Id:     u.doc.DocID,
				Assert: append(notDeadDoc, differentCharm...),
				Update: bson.D{{"$set", bson.D{{"charmurl", curl}}}},
			})
		if u.doc.CharmURL != nil {
			// Drop the reference to the old charm.
			// Since we can force this now, let's.. There is no point hanging on to the old charm.
			op := &ForcedOperation{Force: true}
			decOps, err := appCharmDecRefOps(u.st, u.doc.Application, u.doc.CharmURL, true, op)
			if err != nil {
				// No need to stop further processing if the old key could not be removed.
				logger.Errorf("could not remove old charm references for %v:%v", u.doc.CharmURL, err)
			}
			if len(op.Errors) != 0 {
				logger.Errorf("could not remove old charm references for %v:%v", u.doc.CharmURL, op.Errors)
			}
			ops = append(ops, decOps...)
		}
		return ops, nil
	}
	err := u.st.db().Run(buildTxn)
	if err == nil {
		u.doc.CharmURL = curl
	}
	return err
}

// charm returns the charm for the unit, or the application if the unit's charm
// has not been set yet.
func (u *Unit) charm() (*Charm, error) {
	curl, ok := u.CharmURL()
	if !ok {
		app, err := u.Application()
		if err != nil {
			return nil, err
		}
		curl = app.doc.CharmURL
	}
	ch, err := u.st.Charm(curl)
	return ch, errors.Annotatef(err, "getting charm for %s", u)
}

// assertCharmOps returns txn.Ops to assert the current charm of the unit.
// If the unit currently has no charm URL set, then the application's charm
// URL will be checked by the txn.Ops also.
func (u *Unit) assertCharmOps(ch *Charm) []txn.Op {
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.Name,
		Assert: bson.D{{"charmurl", u.doc.CharmURL}},
	}}
	if _, ok := u.CharmURL(); !ok {
		appName := u.ApplicationName()
		ops = append(ops, txn.Op{
			C:      applicationsC,
			Id:     appName,
			Assert: bson.D{{"charmurl", ch.URL()}},
		})
	}
	return ops
}

// AgentPresence returns whether the respective remote agent is alive.
func (u *Unit) AgentPresence() (bool, error) {
	pwatcher := u.st.workers.presenceWatcher()
	if u.ShouldBeAssigned() {
		return pwatcher.Alive(u.globalAgentKey())
	}
	// Units in CAAS models rely on the operator pings.
	// These are for the application itself.
	app, err := u.Application()
	if err != nil {
		return false, errors.Trace(err)
	}
	appAlive, err := pwatcher.Alive(app.globalKey())
	if err != nil {
		return false, errors.Trace(err)
	}
	return appAlive, nil
}

// Tag returns a name identifying the unit.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (u *Unit) Tag() names.Tag {
	return u.UnitTag()
}

// UnitTag returns a names.UnitTag representing this Unit, unless the
// unit Name is invalid, in which case it will panic
func (u *Unit) UnitTag() names.UnitTag {
	return names.NewUnitTag(u.Name())
}

// WaitAgentPresence blocks until the respective agent is alive.
// This should really only be used in the test suite.
func (u *Unit) WaitAgentPresence(timeout time.Duration) (err error) {
	defer errors.DeferredAnnotatef(&err, "waiting for agent of unit %q", u)
	ch := make(chan presence.Change)
	pwatcher := u.st.workers.presenceWatcher()
	pwatcher.Watch(u.globalAgentKey(), ch)
	defer pwatcher.Unwatch(u.globalAgentKey(), ch)
	pingBatcher := u.st.getPingBatcher()
	if err := pingBatcher.Sync(); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			// TODO(fwereade): 2016-03-17 lp:1558657
			return fmt.Errorf("still not alive after timeout")
		case <-pwatcher.Dead():
			return pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for unit %q", u))
}

// SetAgentPresence signals that the agent for unit u is alive.
// It returns the started pinger.
func (u *Unit) SetAgentPresence() (*presence.Pinger, error) {
	presenceCollection := u.st.getPresenceCollection()
	recorder := u.st.getPingBatcher()
	m, err := u.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	p := presence.NewPinger(presenceCollection, m.ModelTag(), u.globalAgentKey(),
		func() presence.PingRecorder { return u.st.getPingBatcher() })
	err = p.Start()
	if err != nil {
		return nil, err
	}
	// Make sure this Agent status is written to the database before returning.
	recorder.Sync()
	return p, nil
}

func unitNotAssignedError(u *Unit) error {
	msg := fmt.Sprintf("unit %q is not assigned to a machine", u)
	return errors.NewNotAssigned(nil, msg)
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id string, err error) {
	if u.IsPrincipal() {
		if u.doc.MachineId == "" {
			return "", unitNotAssignedError(u)
		}
		return u.doc.MachineId, nil
	}

	units, closer := u.st.db().GetCollection(unitsC)
	defer closer()

	pudoc := unitDoc{}
	err = units.FindId(u.doc.Principal).One(&pudoc)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf("principal unit %q of %q", u.doc.Principal, u)
	} else if err != nil {
		return "", err
	}
	if pudoc.MachineId == "" {
		return "", unitNotAssignedError(u)
	}
	return pudoc.MachineId, nil
}

var (
	machineNotCleanErr = errors.New("machine is dirty")
	alreadyAssignedErr = errors.New("unit is already assigned to a machine")
	inUseErr           = errors.New("machine is not unused")
)

// assignToMachine is the internal version of AssignToMachine.
func (u *Unit) assignToMachine(m *Machine, unused bool) (err error) {
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
	u.doc.MachineId = m.doc.Id
	m.doc.Clean = false
	return nil
}

// assignToMachineOps returns txn.Ops to assign a unit to a machine.
// assignToMachineOps returns specific errors in some cases:
// - machineNotAliveErr when the machine is not alive.
// - unitNotAliveErr when the unit is not alive.
// - alreadyAssignedErr when the unit has already been assigned
// - inUseErr when the machine already has a unit assigned (if unused is true)
func (u *Unit) assignToMachineOps(m *Machine, unused bool) ([]txn.Op, error) {
	if u.Life() != Alive {
		return nil, unitNotAliveErr
	}
	if u.doc.MachineId != "" {
		if u.doc.MachineId != m.Id() {
			return nil, alreadyAssignedErr
		}
		return nil, jujutxn.ErrNoOperations
	}
	if unused && !m.doc.Clean {
		return nil, inUseErr
	}
	storageParams, err := u.storageParams()
	if err != nil {
		return nil, errors.Trace(err)
	}
	sb, err := NewStorageBackend(u.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storagePools, err := storagePools(sb, storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateUnitMachineAssignment(
		m, u.doc.Series, u.doc.Principal != "", storagePools,
	); err != nil {
		return nil, errors.Trace(err)
	}
	storageOps, volumesAttached, filesystemsAttached, err := sb.hostStorageOps(m.doc.Id, storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// addMachineStorageAttachmentsOps will add a txn.Op that ensures
	// that no filesystems were concurrently added to the machine if
	// any of the filesystems being attached specify a location.
	attachmentOps, err := addMachineStorageAttachmentsOps(
		m, volumesAttached, filesystemsAttached,
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
	massert := isAliveDoc
	if unused {
		massert = append(massert, bson.D{{"clean", bson.D{{"$ne", false}}}}...)
	}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: assert,
		Update: bson.D{{"$set", bson.D{{"machineid", m.doc.Id}}}},
	}, {
		C:      machinesC,
		Id:     m.doc.DocID,
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
	m *Machine,
	series string,
	isSubordinate bool,
	storagePools set.Strings,
) (err error) {
	if m.Life() != Alive {
		return machineNotAliveErr
	}
	if isSubordinate {
		return fmt.Errorf("unit is a subordinate")
	}
	if series != m.doc.Series {
		return fmt.Errorf("series does not match")
	}
	canHost := false
	for _, j := range m.doc.Jobs {
		if j == JobHostUnits {
			canHost = true
			break
		}
	}
	if !canHost {
		return fmt.Errorf("machine %q cannot host units", m)
	}
	sb, err := NewStorageBackend(m.st)
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
func validateDynamicMachineStorageParams(m *Machine, params *storageParams) error {
	sb, err := NewStorageBackend(m.st)
	if err != nil {
		return errors.Trace(err)
	}
	pools, err := storagePools(sb, params)
	if err != nil {
		return err
	}
	if err := validateDynamicMachineStoragePools(sb, m, pools); err != nil {
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
func storagePools(sb *storageBackend, params *storageParams) (set.Strings, error) {
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
func validateDynamicMachineStoragePools(sb *storageBackend, m *Machine, pools set.Strings) error {
	if pools.IsEmpty() {
		return nil
	}
	if m.ContainerType() != "" {
		// TODO(axw) consult storage providers to check if they
		// support adding storage to containers. Loop is fine,
		// for example.
		//
		// TODO(axw) later we might allow *any* storage, and
		// passthrough/bindmount storage. That would imply either
		// container creation time only, or requiring containers
		// to be restarted to pick up new configuration.
		return errors.NotSupportedf("adding storage to %s container", m.ContainerType())
	}
	return validateDynamicStoragePools(sb, pools)
}

// validateDynamicStoragePools validates that all of the specified storage
// providers support dynamic storage provisioning. If any provider doesn't
// support dynamic storage, then an IsNotSupported error is returned.
func validateDynamicStoragePools(sb *storageBackend, pools set.Strings) error {
	for pool := range pools {
		providerType, provider, _, err := poolStorageProvider(sb, pool)
		if err != nil {
			return errors.Trace(err)
		}
		if !provider.Dynamic() {
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
func (u *Unit) AssignToMachine(m *Machine) (err error) {
	defer assignContextf(&err, u.Name(), fmt.Sprintf("machine %s", m))
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
	if errors.IsNotFound(err) {
		// Lack of constraints indicates lack of unit.
		return nil, errors.NotFoundf("unit")
	} else if err != nil {
		return nil, err
	}
	return &cons, nil
}

// AssignToNewMachineOrContainer assigns the unit to a new machine,
// with constraints determined according to the application and
// model constraints at the time of unit creation. If a
// container is required, a clean, empty machine instance is required
// on which to create the container. An existing clean, empty instance
// is first searched for, and if not found, a new one is created.
func (u *Unit) AssignToNewMachineOrContainer() (err error) {
	defer assignContextf(&err, u.Name(), "new machine or container")
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	cons, err := u.Constraints()
	if err != nil {
		return err
	}
	if !cons.HasContainer() {
		return u.AssignToNewMachine()
	}

	// Find a clean, empty machine on which to create a container.
	hostCons := *cons
	noContainer := instance.NONE
	hostCons.Container = &noContainer
	query, err := u.findCleanMachineQuery(true, &hostCons)
	if err != nil {
		return err
	}
	machinesCollection, closer := u.st.db().GetCollection(machinesC)
	defer closer()
	var host machineDoc
	if err := machinesCollection.Find(query).One(&host); err == mgo.ErrNotFound {
		// No existing clean, empty machine so create a new one. The
		// container constraint will be used by AssignToNewMachine to
		// create the required container.
		return u.AssignToNewMachine()
	} else if err != nil {
		return err
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
		template := MachineTemplate{
			Series:      u.doc.Series,
			Constraints: *cons,
			Jobs:        []MachineJob{JobHostUnits},
		}
		var ops []txn.Op
		m, ops, err = u.assignToNewMachineOps(template, host.Id, *cons.Container)
		return ops, err
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		if errors.Cause(err) == machineNotCleanErr {
			// The clean machine was used before we got a chance
			// to use it so just stick the unit on a new machine.
			return u.AssignToNewMachine()
		}
		return errors.Trace(err)
	}
	u.doc.MachineId = m.doc.Id
	return nil
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
			Series:                u.doc.Series,
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

func (b byStorageInstance) Len() int      { return len(b) }
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
	return storageParamsForUnit(sb, storageInstances, u.UnitTag(), u.Series(), ch.Meta())
}

func storageParamsForUnit(
	sb *storageBackend, storageInstances []*storageInstance, tag names.UnitTag, series string, chMeta *charm.Meta,
) (*storageParams, error) {

	var volumes []HostVolumeParams
	var filesystems []HostFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)
	for _, storage := range storageInstances {
		storageParams, err := storageParamsForStorageInstance(
			sb, chMeta, tag, series, storage,
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
	unit names.UnitTag,
	series string,
	storage *storageInstance,
) (*storageParams, error) {

	charmStorage := charmMeta.Storage[storage.StorageName()]

	var volumes []HostVolumeParams
	var filesystems []HostFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)

	switch storage.Kind() {
	case StorageKindFilesystem:
		location, err := FilesystemMountPoint(charmStorage, storage.StorageTag(), series)
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
		} else if errors.IsNotFound(err) {
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
		} else if errors.IsNotFound(err) {
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

var noCleanMachines = errors.New("all eligible machines in use")

// AssignToCleanMachine assigns u to a machine which is marked as clean. A machine
// is clean if it has never had any principal units assigned to it.
// If there are no clean machines besides any machine(s) running JobHostEnviron,
// an error is returned.
// This method does not take constraints into consideration when choosing a
// machine (lp:1161919).
func (u *Unit) AssignToCleanMachine() (m *Machine, err error) {
	return u.assignToCleanMaybeEmptyMachine(false)
}

// AssignToCleanEmptyMachine assigns u to a machine which is marked as clean and is also
// not hosting any containers. A machine is clean if it has never had any principal units
// assigned to it. If there are no clean machines besides any machine(s) running JobHostEnviron,
// an error is returned.
// This method does not take constraints into consideration when choosing a
// machine (lp:1161919).
func (u *Unit) AssignToCleanEmptyMachine() (m *Machine, err error) {
	return u.assignToCleanMaybeEmptyMachine(true)
}

var hasContainerTerm = bson.DocElem{
	"$and", []bson.D{
		{{"children", bson.D{{"$not", bson.D{{"$size", 0}}}}}},
		{{"children", bson.D{{"$exists", true}}}},
	}}

var hasNoContainersTerm = bson.DocElem{
	"$or", []bson.D{
		{{"children", bson.D{{"$size", 0}}}},
		{{"children", bson.D{{"$exists", false}}}},
	}}

// findCleanMachineQuery returns a Mongo query to find clean (and maybe empty)
// machines with characteristics matching the specified constraints.
func (u *Unit) findCleanMachineQuery(requireEmpty bool, cons *constraints.Value) (bson.D, error) {
	db, closer := u.st.newDB()
	defer closer()

	// Select all machines that can accept principal units and are clean.
	var containerRefs []machineContainers
	// If we need empty machines, first build up a list of machine ids which
	// have containers so we can exclude those.
	if requireEmpty {
		containerRefsCollection, closer := db.GetCollection(containerRefsC)
		defer closer()

		err := containerRefsCollection.Find(bson.D{hasContainerTerm}).All(&containerRefs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	omitMachineIds := make([]string, len(containerRefs))
	for i, cref := range containerRefs {
		omitMachineIds[i] = cref.Id
	}

	// Exclude machines that are locked for series upgrade.
	locked, err := u.st.upgradeSeriesMachineIds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	omitMachineIds = append(omitMachineIds, locked...)

	// Also exclude containers on machines locked for series upgrade.
	for _, id := range locked {
		m, err := u.st.Machine(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cIds, err := m.Containers()
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		omitMachineIds = append(omitMachineIds, cIds...)
	}

	terms := bson.D{
		{"life", Alive},
		{"series", u.doc.Series},
		{"jobs", []MachineJob{JobHostUnits}},
		{"clean", true},
		{"machineid", bson.D{{"$nin", omitMachineIds}}},
	}
	// Add the container filter term if necessary.
	var containerType instance.ContainerType
	if cons.Container != nil {
		containerType = *cons.Container
	}
	if containerType == instance.NONE {
		terms = append(terms, bson.DocElem{"containertype", ""})
	} else if containerType != "" {
		terms = append(terms, bson.DocElem{"containertype", string(containerType)})
	}

	// Find the ids of machines which satisfy any required hardware
	// constraints. If there is no instanceData for a machine, that
	// machine is not considered as suitable for deploying the unit.
	// This can happen if the machine is not yet provisioned. It may
	// be that when the machine is provisioned it will be found to
	// be suitable, but we don't know that right now and it's best
	// to err on the side of caution and exclude such machines.
	var suitableInstanceData []instanceData
	var suitableTerms bson.D
	if cons.HasArch() {
		suitableTerms = append(suitableTerms, bson.DocElem{"arch", *cons.Arch})
	}
	if cons.HasMem() {
		suitableTerms = append(suitableTerms, bson.DocElem{"mem", bson.D{{"$gte", *cons.Mem}}})
	}
	if cons.RootDisk != nil && *cons.RootDisk > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"rootdisk", bson.D{{"$gte", *cons.RootDisk}}})
	}
	if cons.RootDiskSource != nil && *cons.RootDiskSource != "" {
		suitableTerms = append(suitableTerms, bson.DocElem{"rootdisksource", *cons.RootDiskSource})
	}
	if cons.HasCpuCores() {
		suitableTerms = append(suitableTerms, bson.DocElem{"cpucores", bson.D{{"$gte", *cons.CpuCores}}})
	}
	if cons.HasCpuPower() {
		suitableTerms = append(suitableTerms, bson.DocElem{"cpupower", bson.D{{"$gte", *cons.CpuPower}}})
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"tags", bson.D{{"$all", *cons.Tags}}})
	}
	if cons.HasZones() {
		suitableTerms = append(suitableTerms, bson.DocElem{"availzone", bson.D{{"$in", *cons.Zones}}})
	}
	if len(suitableTerms) > 0 {
		instanceDataCollection, closer := db.GetCollection(instanceDataC)
		defer closer()
		err := instanceDataCollection.Find(suitableTerms).Select(bson.M{"_id": 1}).All(&suitableInstanceData)
		if err != nil {
			return nil, err
		}
		var suitableIds = make([]string, len(suitableInstanceData))
		for i, m := range suitableInstanceData {
			suitableIds[i] = m.DocID
		}
		terms = append(terms, bson.DocElem{"_id", bson.D{{"$in", suitableIds}}})
	}
	return terms, nil
}

// assignToCleanMaybeEmptyMachine implements AssignToCleanMachine and AssignToCleanEmptyMachine.
// A 'machine' may be a machine instance or container depending on the application constraints.
func (u *Unit) assignToCleanMaybeEmptyMachine(requireEmpty bool) (*Machine, error) {
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
		var ops []txn.Op
		m, ops, err = u.assignToCleanMaybeEmptyMachineOps(requireEmpty)
		return ops, err
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	u.doc.MachineId = m.doc.Id
	m.doc.Clean = false
	return m, nil
}

func (u *Unit) assignToCleanMaybeEmptyMachineOps(requireEmpty bool) (_ *Machine, _ []txn.Op, err error) {
	failure := func(err error) (*Machine, []txn.Op, error) {
		return nil, nil, err
	}

	context := "clean"
	if requireEmpty {
		context += ", empty"
	}
	context += " machine"

	if u.doc.Principal != "" {
		err = fmt.Errorf("unit is a subordinate")
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}

	sb, err := NewStorageBackend(u.st)
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}

	// If required storage is not all dynamic, then assigning
	// to a new machine is required.
	storageParams, err := u.storageParams()
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	storagePools, err := storagePools(sb, storageParams)
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	if err := validateDynamicStoragePools(sb, storagePools); err != nil {
		if errors.IsNotSupported(err) {
			return failure(noCleanMachines)
		}
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}

	// Get the unit constraints to see what deployment requirements we have to adhere to.
	cons, err := u.Constraints()
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	query, err := u.findCleanMachineQuery(requireEmpty, cons)
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}

	// Find all of the candidate machines, and associated
	// instances for those that are provisioned. Instances
	// will be distributed across in preference to
	// unprovisioned machines.
	machinesCollection, closer := u.st.db().GetCollection(machinesC)
	defer closer()
	var mdocs []*machineDoc
	if err := machinesCollection.Find(query).All(&mdocs); err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	var unprovisioned []*Machine
	var instances []instance.Id
	instanceMachines := make(map[instance.Id]*Machine)
	for _, mdoc := range mdocs {
		m := newMachine(u.st, mdoc)
		inst, err := m.InstanceId()
		if errors.IsNotProvisioned(err) {
			unprovisioned = append(unprovisioned, m)
		} else if err != nil {
			assignContextf(&err, u.Name(), context)
			return failure(err)
		} else {
			instances = append(instances, inst)
			instanceMachines[inst] = m
		}
	}

	// Filter the list of instances that are suitable for
	// distribution, and then map them back to machines.
	//
	// TODO(axw) 2014-05-30 #1324904
	// Shuffle machines to reduce likelihood of collisions.
	// The partition of provisioned/unprovisioned machines
	// must be maintained.
	var limitZones []string
	if cons.HasZones() {
		limitZones = *cons.Zones
	}
	if instances, err = distributeUnit(u, instances, limitZones); err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	machines := make([]*Machine, len(instances), len(instances)+len(unprovisioned))
	for i, inst := range instances {
		m, ok := instanceMachines[inst]
		if !ok {
			err := fmt.Errorf("invalid instance returned: %v", inst)
			assignContextf(&err, u.Name(), context)
			return failure(err)
		}
		machines[i] = m
	}
	machines = append(machines, unprovisioned...)

	// TODO(axw) 2014-05-30 #1253704
	// We should not select a machine that is in the process
	// of being provisioned. There's no point asserting that
	// the machine hasn't been provisioned, as there'll still
	// be a period of time during which the machine may be
	// provisioned without the fact having yet been recorded
	// in state.
	for _, m := range machines {
		// Check that the unit storage is compatible with
		// the machine in question.
		if err := validateDynamicMachineStorageParams(m, storageParams); err != nil {
			if errors.IsNotSupported(err) {
				continue
			}
			assignContextf(&err, u.Name(), context)
			return failure(err)
		}
		ops, err := u.assignToMachineOps(m, true)
		if err == nil {
			return m, ops, nil
		}
		switch errors.Cause(err) {
		case inUseErr, machineNotAliveErr:
		default:
			assignContextf(&err, u.Name(), context)
			return failure(err)
		}
	}
	return failure(noCleanMachines)
}

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

// AddAction adds a new Action of type name and using arguments payload to
// this Unit, and returns its ID.  Note that the use of spec.InsertDefaults
// mutates payload.
func (u *Unit) AddAction(name string, payload map[string]interface{}) (Action, error) {
	if len(name) == 0 {
		return nil, errors.New("no action name given")
	}

	// If the action is predefined inside juju, get spec from map
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		specs, err := u.ActionSpecs()
		if err != nil {
			return nil, err
		}
		spec, ok = specs[name]
		if !ok {
			return nil, errors.Errorf("action %q not defined on unit %q", name, u.Name())
		}
	}
	// Reject bad payloads before attempting to insert defaults.
	err := spec.ValidateParams(payload)
	if err != nil {
		return nil, err
	}
	payloadWithDefaults, err := spec.InsertDefaults(payload)
	if err != nil {
		return nil, err
	}

	m, err := u.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m.EnqueueAction(u.Tag(), name, payloadWithDefaults)
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
		return none, errors.Errorf("no actions defined on charm %q", ch.String())
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

// Resolve marks the unit as having had any previous state transition
// problems resolved, and informs the unit that it may attempt to
// reestablish normal workflow. The retryHooks parameter informs
// whether to attempt to reexecute previous failed hooks or to continue
// as if they had succeeded before.
func (u *Unit) Resolve(retryHooks bool) error {
	// We currently check agent status to see if a unit is
	// in error state. As the new Juju Health work is completed,
	// this will change to checking the unit status.
	statusInfo, err := u.Status()
	if err != nil {
		return err
	}
	if statusInfo.Status != status.Error {
		return errors.Errorf("unit %q is not in an error state", u)
	}
	mode := ResolvedNoHooks
	if retryHooks {
		mode = ResolvedRetryHooks
	}
	return u.SetResolved(mode)
}

// SetResolved marks the unit as having had any previous state transition
// problems resolved, and informs the unit that it may attempt to
// reestablish normal workflow. The resolved mode parameter informs
// whether to attempt to reexecute previous failed hooks or to continue
// as if they had succeeded before.
func (u *Unit) SetResolved(mode ResolvedMode) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set resolved mode for unit %q", u)
	switch mode {
	case ResolvedRetryHooks, ResolvedNoHooks:
	default:
		return fmt.Errorf("invalid error resolution mode: %q", mode)
	}
	// TODO(fwereade): assert unit has error status.
	resolvedNotSet := bson.D{{"resolved", ResolvedNone}}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: append(notDeadDoc, resolvedNotSet...),
		Update: bson.D{{"$set", bson.D{{"resolved", mode}}}},
	}}
	if err := u.st.db().RunTransaction(ops); err == nil {
		u.doc.Resolved = mode
		return nil
	} else if err != txn.ErrAborted {
		return err
	}
	if ok, err := isNotDead(u.st, unitsC, u.doc.DocID); err != nil {
		return err
	} else if !ok {
		return ErrDead
	}
	// For now, the only remaining assert is that resolved was unset.
	return fmt.Errorf("already resolved")
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() error {
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"resolved", ResolvedNone}}}},
	}}
	err := u.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot clear resolved mode for unit %q: %v", u, errors.NotFoundf("unit"))
	}
	u.doc.Resolved = ResolvedNone
	return nil
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
	if errors.IsNotFound(err) {
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
	meterStatusDoc     *meterStatusDoc
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
		createMeterStatusOp(st, agentGlobalKey, args.meterStatusDoc),
		createStatusOp(st, globalWorkloadVersionKey(name), *args.workloadVersionDoc),
	)

	// Freshly-created units will not have a charm URL set; migrated
	// ones will, and they need to maintain their refcounts. If we
	// relax the restrictions on migrating apps mid-upgrade, this
	// will need to be more sophisticated, because it might need to
	// create the settings doc.
	if curl := args.unitDoc.CharmURL; curl != nil {
		appName := args.unitDoc.Application
		charmRefOps, err := appCharmIncRefOps(st, appName, curl, false)
		if err != nil {
			return nil, errors.Trace(err)
		}
		prereqOps = append(prereqOps, charmRefOps...)
	}

	return append(prereqOps, txn.Op{
		C:      unitsC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: args.unitDoc,
	}), nil
}

// HistoryGetter allows getting the status history based on some identifying key.
type HistoryGetter struct {
	st        *State
	globalKey string
}

// StatusHistory implements status.StatusHistoryGetter.
func (g *HistoryGetter) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        g.st.db(),
		globalKey: g.globalKey,
		filter:    filter,
	}
	return statusHistory(args)
}

// GetSpaceForBinding returns the space name associated with the specified endpoint.
func (u *Unit) GetSpaceForBinding(bindingName string) (string, error) {
	app, err := u.Application()
	if err != nil {
		return "", errors.Trace(err)
	}

	bindings, err := app.EndpointBindings()
	if err != nil {
		return "", errors.Trace(err)
	}
	boundSpace, known := bindings[bindingName]
	if !known {
		// If default binding is not explicitly defined we'll use default space
		if bindingName == corenetwork.DefaultSpaceName {
			return corenetwork.DefaultSpaceName, nil
		}
		return "", errors.NewNotValid(nil, fmt.Sprintf("binding name %q not defined by the unit's charm", bindingName))
	}
	return boundSpace, nil
}

// UpgradeSeriesStatus returns the upgrade status of the units assigned machine.
func (u *Unit) UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error) {
	machine, err := u.machine()
	if err != nil {
		return "", err
	}
	return machine.UpgradeSeriesUnitStatus(u.Name())
}

// SetUpgradeSeriesStatus sets the upgrade status of the units assigned machine.
func (u *Unit) SetUpgradeSeriesStatus(status model.UpgradeSeriesStatus, message string) error {
	machine, err := u.machine()
	if err != nil {
		return err
	}
	return machine.SetUpgradeSeriesUnitStatus(u.Name(), status, message)
}
