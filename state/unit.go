// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

var unitLogger = loggo.GetLogger("juju.state.unit")

// AssignmentPolicy controls what machine a unit will be assigned to.
type AssignmentPolicy string

const (
	// AssignLocal indicates that all service units should be assigned
	// to machine 0.
	AssignLocal AssignmentPolicy = "local"

	// AssignClean indicates that every service unit should be assigned
	// to a machine which never previously has hosted any units, and that
	// new machines should be launched if required.
	AssignClean AssignmentPolicy = "clean"

	// AssignCleanEmpty indicates that every service unit should be assigned
	// to a machine which never previously has hosted any units, and which is not
	// currently hosting any containers, and that new machines should be launched if required.
	AssignCleanEmpty AssignmentPolicy = "clean-empty"

	// AssignNew indicates that every service unit should be assigned to a new
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

// Unit represents the state of a service unit.
type Unit struct {
	st  *State
	doc unitDoc
}

func newUnit(st *State, udoc *unitDoc) *Unit {
	unit := &Unit{
		st:  st,
		doc: *udoc,
	}
	return unit
}

// Application returns the application.
func (u *Unit) Application() (*Application, error) {
	return u.st.Application(u.doc.Application)
}

// ConfigSettings returns the complete set of service charm config settings
// available to the unit. Unset values will be replaced with the default
// value for the associated option, and may thus be nil when no default is
// specified.
func (u *Unit) ConfigSettings() (charm.Settings, error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("unit charm not set")
	}
	settings, err := readSettings(u.st, settingsC, applicationSettingsKey(u.doc.Application, u.doc.CharmURL))
	if err != nil {
		return nil, err
	}
	chrm, err := u.st.Charm(u.doc.CharmURL)
	if err != nil {
		return nil, err
	}
	result := chrm.Config().DefaultSettings()
	for name, value := range settings.Map() {
		result[name] = value
	}
	return result, nil
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

// Life returns whether the unit is Alive, Dying or Dead.
func (u *Unit) Life() Life {
	return u.doc.Life
}

// WorkloadVersion returns the version of the running workload set by
// the charm (eg, the version of postgresql that is running, as
// opposed to the version of the postgresql charm).
func (u *Unit) WorkloadVersion() (string, error) {
	status, err := getStatus(u.st, u.globalWorkloadVersionKey(), "workload")
	if errors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return status.Message, nil
}

// SetWorkloadVersion sets the version of the workload that the unit
// is currently running.
func (u *Unit) SetWorkloadVersion(version string) error {
	// Store in status rather than an attribute of the unit doc - we
	// want to avoid everything being an attr of the main docs to
	// stop a swarm of watchers being notified for irrelevant changes.
	now := u.st.clock.Now()
	return setStatus(u.st, setStatusParams{
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
		return nil, errors.NotFoundf("agent tools for unit %q", u)
	}
	tools := *u.doc.Tools
	return &tools, nil
}

// SetAgentVersion sets the version of juju that the agent is
// currently running.
func (u *Unit) SetAgentVersion(v version.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set agent version for unit %q", u)
	if err = checkVersionValidity(v); err != nil {
		return err
	}
	tools := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"tools", tools}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return onAbort(err, ErrDead)
	}
	u.doc.Tools = tools
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
	err := u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of unit %q: %v", u, onAbort(err, ErrDead))
	}
	u.doc.PasswordHash = passwordHash
	return nil
}

// Return the underlying PasswordHash stored in the database. Used by the test
// suite to check that the PasswordHash gets properly updated to new values
// when compatibility mode is detected.
func (u *Unit) getPasswordHash() string {
	return u.doc.PasswordHash
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

// Destroy, when called on a Alive unit, advances its lifecycle as far as
// possible; it otherwise has no effect. In most situations, the unit's
// life is just set to Dying; but if a principal unit that is not assigned
// to a provisioned machine is Destroyed, it will be removed from state
// directly.
func (u *Unit) Destroy() (err error) {
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			u.doc.Life = Dying
		}
	}()
	unit := &Unit{st: u.st, doc: u.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := unit.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		switch ops, err := unit.destroyOps(); err {
		case errRefresh:
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, err
		}
		return nil, jujutxn.ErrNoOperations
	}
	if err = unit.st.run(buildTxn); err == nil {
		if historyErr := unit.eraseHistory(); historyErr != nil {
			logger.Errorf("cannot delete history for unit %q: %v", unit.globalKey(), err)
		}
		if err = unit.Refresh(); errors.IsNotFound(err) {
			return nil
		}
	}
	return err
}

func (u *Unit) eraseHistory() error {
	history, closer := u.st.getCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()

	if _, err := historyW.RemoveAll(bson.D{{"statusid", u.globalKey()}}); err != nil {
		return err
	}
	if _, err := historyW.RemoveAll(bson.D{{"statusid", u.globalAgentKey()}}); err != nil {
		return err
	}
	return nil
}

// destroyOps returns the operations required to destroy the unit. If it
// returns errRefresh, the unit should be refreshed and the destruction
// operations recalculated.
func (u *Unit) destroyOps() ([]txn.Op, error) {
	if u.doc.Life != Alive {
		return nil, errAlreadyDying
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
	minUnitsOp := minUnitsTriggerOp(u.st, u.ApplicationName())
	cleanupOp := newCleanupOp(cleanupDyingUnit, u.doc.Name)
	setDyingOp := txn.Op{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}
	setDyingOps := []txn.Op{setDyingOp, cleanupOp, minUnitsOp}
	if u.doc.Principal != "" {
		return setDyingOps, nil
	} else if len(u.doc.Subordinates)+u.doc.StorageAttachmentCount != 0 {
		return setDyingOps, nil
	}

	// See if the unit agent has started running.
	// If so then we can't set directly to dead.
	agentStatusDocId := u.globalAgentKey()
	agentStatusInfo, agentErr := getStatus(u.st, agentStatusDocId, "agent")
	if errors.IsNotFound(agentErr) {
		return nil, errAlreadyDying
	} else if agentErr != nil {
		return nil, errors.Trace(agentErr)
	}
	if agentStatusInfo.Status != status.Allocating {
		return setDyingOps, nil
	}

	ops := []txn.Op{{
		C:      statusesC,
		Id:     u.st.docID(agentStatusDocId),
		Assert: bson.D{{"status", status.Allocating}},
	}, minUnitsOp}
	removeAsserts := append(isAliveDoc, bson.DocElem{
		"$and", []bson.D{
			unitHasNoSubordinates,
			unitHasNoStorageAttachments,
		},
	})
	removeOps, err := u.removeOps(removeAsserts)
	if err == errAlreadyRemoved {
		return nil, errAlreadyDying
	} else if err != nil {
		return nil, err
	}
	return append(ops, removeOps...), nil
}

// destroyHostOps returns all necessary operations to destroy the service unit's host machine,
// or ensure that the conditions preventing its destruction remain stable through the transaction.
func (u *Unit) destroyHostOps(s *Application) (ops []txn.Op, err error) {
	if s.doc.Subordinate {
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

	machineUpdate := bson.D{{"$pull", bson.D{{"principals", u.doc.Name}}}}

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

	machineCheck := true // whether host machine conditions allow destroy
	if len(m.doc.Principals) != 1 || m.doc.Principals[0] != u.doc.Name {
		machineCheck = false
	} else if hasJob(m.doc.Jobs, JobManageModel) {
		// Check that the machine does not have any responsibilities that
		// prevent a lifecycle change.
		machineCheck = false
	} else if m.doc.HasVote {
		machineCheck = false
	}

	// assert that the machine conditions pertaining to host removal conditions
	// remain the same throughout the transaction.
	var machineAssert bson.D
	if machineCheck {
		machineAssert = bson.D{{"$and", []bson.D{
			{{"principals", []string{u.doc.Name}}},
			{{"jobs", bson.D{{"$nin", []MachineJob{JobManageModel}}}}},
			{{"hasvote", bson.D{{"$ne", true}}}},
		}}}
	} else {
		machineAssert = bson.D{{"$or", []bson.D{
			{{"principals", bson.D{{"$ne", []string{u.doc.Name}}}}},
			{{"jobs", bson.D{{"$in", []MachineJob{JobManageModel}}}}},
			{{"hasvote", true}},
		}}}
	}

	// If removal conditions satisfied by machine & container docs, we can
	// destroy it, in addition to removing the unit principal.
	if machineCheck && containerCheck {
		machineUpdate = append(machineUpdate, bson.D{{"$set", bson.D{{"life", Dying}}}}...)
	}

	ops = append(ops, txn.Op{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: machineAssert,
		Update: machineUpdate,
	})
	return ops, nil
}

// removeOps returns the operations necessary to remove the unit, assuming
// the supplied asserts apply to the unit document.
func (u *Unit) removeOps(asserts bson.D) ([]txn.Op, error) {
	svc, err := u.st.Application(u.doc.Application)
	if errors.IsNotFound(err) {
		// If the service has been removed, the unit must already have been.
		return nil, errAlreadyRemoved
	} else if err != nil {
		return nil, err
	}
	return svc.removeUnitOps(u, asserts)
}

// ErrUnitHasSubordinates is a standard error to indicate that a Unit
// cannot complete an operation to end its life because it still has
// subordinate services
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
	if err := u.st.runTransaction(ops); err != txn.ErrAborted {
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

// Remove removes the unit from state, and may remove its service as well, if
// the service is Dying and no other references to it exist. It will fail if
// the unit is not Dead.
func (u *Unit) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove unit %q", u)
	if u.doc.Life != Dead {
		return errors.New("unit is not dead")
	}

	// Now the unit is Dead, we can be sure that it's impossible for it to
	// enter relation scopes (once it's Dying, we can be sure of this; but
	// EnsureDead does not require that it already be Dying, so this is the
	// only point at which we can safely backstop lp:1233457 and mitigate
	// the impact of unit agent bugs that leave relation scopes occupied).
	relations, err := applicationRelations(u.st, u.doc.Application)
	if err != nil {
		return err
	}
	for _, rel := range relations {
		ru, err := rel.Unit(u)
		if err != nil {
			return err
		}
		if err := ru.LeaveScope(); err != nil {
			return err
		}
	}

	// Now we're sure we haven't left any scopes occupied by this unit, we
	// can safely remove the document.
	unit := &Unit{st: u.st, doc: u.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := unit.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		switch ops, err := unit.removeOps(isDeadDoc); err {
		case errRefresh:
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, err
		}
		return nil, jujutxn.ErrNoOperations
	}
	return unit.st.run(buildTxn)
}

// Resolved returns the resolved mode for the unit.
func (u *Unit) Resolved() ResolvedMode {
	return u.doc.Resolved
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.doc.Principal == ""
}

// SubordinateNames returns the names of any subordinate units.
func (u *Unit) SubordinateNames() []string {
	names := make([]string, len(u.doc.Subordinates))
	copy(names, u.doc.Subordinates)
	return names
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
	m, err := u.machine()
	if err != nil {
		unitLogger.Tracef("%v", err)
		return network.Address{}, errors.Trace(err)
	}
	return m.PublicAddress()
}

// PrivateAddress returns the private address of the unit.
func (u *Unit) PrivateAddress() (network.Address, error) {
	m, err := u.machine()
	if err != nil {
		unitLogger.Tracef("%v", err)
		return network.Address{}, errors.Trace(err)
	}
	return m.PrivateAddress()
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
	units, closer := u.st.getCollection(unitsC)
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
		st:        u.st,
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
	info, err := getStatus(u.st, u.globalAgentKey(), "unit")
	if err != nil {
		return status.StatusInfo{}, err
	}
	if info.Status != status.Error {
		info, err = getStatus(u.st, u.globalKey(), "unit")
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
	return setStatus(u.st, setStatusParams{
		badge:     "unit",
		globalKey: u.globalKey(),
		status:    unitStatus.Status,
		message:   unitStatus.Message,
		rawData:   unitStatus.Data,
		updated:   unitStatus.Since,
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
func (u *Unit) OpenedPortsOnSubnet(subnetID string) ([]network.PortRange, error) {
	machineID, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	if err := u.checkSubnetAliveWhenSet(subnetID); err != nil {
		return nil, errors.Trace(err)
	}

	machinePorts, err := getPorts(u.st, machineID, subnetID)
	result := []network.PortRange{}
	if errors.IsNotFound(err) {
		return result, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "failed getting ports for unit %q, subnet %q", u, subnetID)
	}
	ports := machinePorts.PortsForUnit(u.Name())
	for _, port := range ports {
		result = append(result, network.PortRange{
			Protocol: port.Protocol,
			FromPort: port.FromPort,
			ToPort:   port.ToPort,
		})
	}
	network.SortPortRanges(result)
	return result, nil
}

// OpenedPorts returns a slice containing the open port ranges of the
// unit.
//
// TODO(dimitern): This should be removed once we use OpenedPortsOnSubnet across
// the board, passing subnet IDs explicitly.
func (u *Unit) OpenedPorts() ([]network.PortRange, error) {
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
			// when the unit is Dying, because service/charm upgrades
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

		// Add a reference to the service settings for the new charm.
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
			decOps, err := appCharmDecRefOps(u.st, u.doc.Application, u.doc.CharmURL)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, decOps...)
		}
		return ops, nil
	}
	err := u.st.run(buildTxn)
	if err == nil {
		u.doc.CharmURL = curl
	}
	return err
}

// AgentPresence returns whether the respective remote agent is alive.
func (u *Unit) AgentPresence() (bool, error) {
	pwatcher := u.st.workers.PresenceWatcher()
	return pwatcher.Alive(u.globalAgentKey())
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
func (u *Unit) WaitAgentPresence(timeout time.Duration) (err error) {
	defer errors.DeferredAnnotatef(&err, "waiting for agent of unit %q", u)
	ch := make(chan presence.Change)
	pwatcher := u.st.workers.PresenceWatcher()
	pwatcher.Watch(u.globalAgentKey(), ch)
	defer pwatcher.Unwatch(u.globalAgentKey(), ch)
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
	p := presence.NewPinger(presenceCollection, u.st.ModelTag(), u.globalAgentKey())
	err := p.Start()
	if err != nil {
		return nil, err
	}
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

	units, closer := u.st.getCollection(unitsC)
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
	machineNotAliveErr = errors.New("machine is not alive")
	machineNotCleanErr = errors.New("machine is dirty")
	unitNotAliveErr    = errors.New("unit is not alive")
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
	if err := u.st.run(buildTxn); err != nil {
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
	storageParams, err := u.machineStorageParams()
	if err != nil {
		return nil, errors.Trace(err)
	}
	storagePools, err := machineStoragePools(m.st, storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateUnitMachineAssignment(
		m, u.doc.Series, u.doc.Principal != "", storagePools,
	); err != nil {
		return nil, errors.Trace(err)
	}
	storageOps, volumesAttached, filesystemsAttached, err := u.st.machineStorageOps(
		&m.doc, storageParams,
	)
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
	if err := validateDynamicMachineStoragePools(m, storagePools); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// validateDynamicMachineStorageParams validates that the provided machine
// storage parameters are compatible with the specified machine.
func validateDynamicMachineStorageParams(m *Machine, params *machineStorageParams) error {
	pools, err := machineStoragePools(m.st, params)
	if err != nil {
		return err
	}
	return validateDynamicMachineStoragePools(m, pools)
}

// machineStoragePools returns the names of storage pools in each of the
// volume, filesystem and attachments in the machine storage parameters.
func machineStoragePools(st *State, params *machineStorageParams) (set.Strings, error) {
	pools := make(set.Strings)
	for _, v := range params.volumes {
		v, err := st.volumeParamsWithDefaults(v.Volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pools.Add(v.Pool)
	}
	for _, f := range params.filesystems {
		f, err := st.filesystemParamsWithDefaults(f.Filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pools.Add(f.Pool)
	}
	for volumeTag := range params.volumeAttachments {
		volume, err := st.Volume(volumeTag)
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
		filesystem, err := st.Filesystem(filesystemTag)
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
func validateDynamicMachineStoragePools(m *Machine, pools set.Strings) error {
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
	return validateDynamicStoragePools(m.st, pools)
}

// validateDynamicStoragePools validates that all of the specified storage
// providers support dynamic storage provisioning. If any provider doesn't
// support dynamic storage, then an IsNotSupported error is returned.
func validateDynamicStoragePools(st *State, pools set.Strings) error {
	for pool := range pools {
		providerType, provider, err := poolStorageProvider(st, pool)
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
// with constraints determined according to the service and
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
	machinesCollection, closer := u.st.getCollection(machinesC)
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
	if err := u.st.run(buildTxn); err != nil {
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
// determined according to the service and model constraints at the
// time of unit creation.
func (u *Unit) AssignToNewMachine() (err error) {
	defer assignContextf(&err, u.Name(), "new machine")
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
		storageParams, err := u.machineStorageParams()
		if err != nil {
			return nil, errors.Trace(err)
		}
		template := MachineTemplate{
			Series:                u.doc.Series,
			Constraints:           *cons,
			Jobs:                  []MachineJob{JobHostUnits},
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
	if err := u.st.run(buildTxn); err != nil {
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

// machineStorageParams returns parameters for creating volumes/filesystems
// and volume/filesystem attachments for a machine that the unit will be
// assigned to.
func (u *Unit) machineStorageParams() (*machineStorageParams, error) {
	params, err := unitMachineStorageParams(u)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, name := range u.doc.Subordinates {
		sub, err := u.st.Unit(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subParams, err := unitMachineStorageParams(sub)
		if err != nil {
			return nil, errors.Trace(err)
		}
		params = combineMachineStorageParams(params, subParams)
	}
	return params, nil
}

func unitMachineStorageParams(u *Unit) (*machineStorageParams, error) {
	storageAttachments, err := u.st.UnitStorageAttachments(u.UnitTag())
	if err != nil {
		return nil, errors.Annotate(err, "getting storage attachments")
	}
	curl := u.doc.CharmURL
	if curl == nil {
		var err error
		app, err := u.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		curl, _ = app.CharmURL()
	}
	ch, err := u.st.Charm(curl)
	if err != nil {
		return nil, errors.Annotate(err, "getting charm")
	}
	allCons, err := u.StorageConstraints()
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage constraints")
	}

	// Sort storage attachments so the volume ids are consistent (for testing).
	sort.Sort(byStorageInstance(storageAttachments))

	chMeta := ch.Meta()

	var volumes []MachineVolumeParams
	var filesystems []MachineFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)
	for _, storageAttachment := range storageAttachments {
		storage, err := u.st.StorageInstance(storageAttachment.StorageInstance())
		if err != nil {
			return nil, errors.Annotatef(err, "getting storage instance")
		}
		machineParams, err := machineStorageParamsForStorageInstance(
			u.st, chMeta, u.UnitTag(), allCons, storage,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}

		volumes = append(volumes, machineParams.volumes...)
		for k, v := range machineParams.volumeAttachments {
			volumeAttachments[k] = v
		}

		filesystems = append(filesystems, machineParams.filesystems...)
		for k, v := range machineParams.filesystemAttachments {
			filesystemAttachments[k] = v
		}
	}
	result := &machineStorageParams{
		volumes,
		volumeAttachments,
		filesystems,
		filesystemAttachments,
	}
	return result, nil
}

// machineStorageParamsForStorageInstance returns parameters for creating
// volumes/filesystems and volume/filesystem attachments for a machine that
// the unit will be assigned to. These parameters are based on a given storage
// instance.
func machineStorageParamsForStorageInstance(
	st *State,
	charmMeta *charm.Meta,
	unit names.UnitTag,
	allCons map[string]StorageConstraints,
	storage StorageInstance,
) (*machineStorageParams, error) {

	charmStorage := charmMeta.Storage[storage.StorageName()]

	var volumes []MachineVolumeParams
	var filesystems []MachineFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)

	switch storage.Kind() {
	case StorageKindBlock:
		volumeAttachmentParams := VolumeAttachmentParams{
			charmStorage.ReadOnly,
		}
		if unit == storage.Owner() {
			// The storage instance is owned by the unit, so we'll need
			// to create a volume.
			cons := allCons[storage.StorageName()]
			volumeParams := VolumeParams{
				storage: storage.StorageTag(),
				binding: storage.StorageTag(),
				Pool:    cons.Pool,
				Size:    cons.Size,
			}
			volumes = append(volumes, MachineVolumeParams{
				volumeParams, volumeAttachmentParams,
			})
		} else {
			// The storage instance is owned by the service, so there
			// should be a (shared) volume already, for which we will
			// just add an attachment.
			volume, err := st.StorageInstanceVolume(storage.StorageTag())
			if err != nil {
				return nil, errors.Annotatef(err, "getting volume for storage %q", storage.Tag().Id())
			}
			volumeAttachments[volume.VolumeTag()] = volumeAttachmentParams
		}
	case StorageKindFilesystem:
		location, err := filesystemMountPoint(charmStorage, storage.StorageTag())
		if err != nil {
			return nil, errors.Annotatef(
				err, "getting filesystem mount point for storage %s",
				storage.StorageName(),
			)
		}
		filesystemAttachmentParams := FilesystemAttachmentParams{
			charmStorage.Location == "", // auto-generated location
			location,
			charmStorage.ReadOnly,
		}
		if unit == storage.Owner() {
			// The storage instance is owned by the unit, so we'll need
			// to create a filesystem.
			cons := allCons[storage.StorageName()]
			filesystemParams := FilesystemParams{
				storage: storage.StorageTag(),
				binding: storage.StorageTag(),
				Pool:    cons.Pool,
				Size:    cons.Size,
			}
			filesystems = append(filesystems, MachineFilesystemParams{
				filesystemParams, filesystemAttachmentParams,
			})
		} else {
			// The storage instance is owned by the service, so there
			// should be a (shared) filesystem already, for which we will
			// just add an attachment.
			filesystem, err := st.StorageInstanceFilesystem(storage.StorageTag())
			if err != nil {
				return nil, errors.Annotatef(err, "getting filesystem for storage %q", storage.Tag().Id())
			}
			filesystemAttachments[filesystem.FilesystemTag()] = filesystemAttachmentParams
		}
	default:
		return nil, errors.Errorf("invalid storage kind %v", storage.Kind())
	}
	result := &machineStorageParams{
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

// findCleanMachineQuery returns a Mongo query to find clean (and possibly empty) machines with
// characteristics matching the specified constraints.
func (u *Unit) findCleanMachineQuery(requireEmpty bool, cons *constraints.Value) (bson.D, error) {
	db, closer := u.st.newDB()
	defer closer()
	containerRefsCollection, closer := db.GetCollection(containerRefsC)
	defer closer()

	// Select all machines that can accept principal units and are clean.
	var containerRefs []machineContainers
	// If we need empty machines, first build up a list of machine ids which have containers
	// so we can exclude those.
	if requireEmpty {
		err := containerRefsCollection.Find(bson.D{hasContainerTerm}).All(&containerRefs)
		if err != nil {
			return nil, err
		}
	}
	var machinesWithContainers = make([]string, len(containerRefs))
	for i, cref := range containerRefs {
		machinesWithContainers[i] = cref.Id
	}
	terms := bson.D{
		{"life", Alive},
		{"series", u.doc.Series},
		{"jobs", []MachineJob{JobHostUnits}},
		{"clean", true},
		{"machineid", bson.D{{"$nin", machinesWithContainers}}},
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
	if cons.Arch != nil && *cons.Arch != "" {
		suitableTerms = append(suitableTerms, bson.DocElem{"arch", *cons.Arch})
	}
	if cons.Mem != nil && *cons.Mem > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"mem", bson.D{{"$gte", *cons.Mem}}})
	}
	if cons.RootDisk != nil && *cons.RootDisk > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"rootdisk", bson.D{{"$gte", *cons.RootDisk}}})
	}
	if cons.CpuCores != nil && *cons.CpuCores > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"cpucores", bson.D{{"$gte", *cons.CpuCores}}})
	}
	if cons.CpuPower != nil && *cons.CpuPower > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"cpupower", bson.D{{"$gte", *cons.CpuPower}}})
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"tags", bson.D{{"$all", *cons.Tags}}})
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
// A 'machine' may be a machine instance or container depending on the service constraints.
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
	if err := u.st.run(buildTxn); err != nil {
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

	// If required storage is not all dynamic, then assigning
	// to a new machine is required.
	storageParams, err := u.machineStorageParams()
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	storagePools, err := machineStoragePools(u.st, storageParams)
	if err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	if err := validateDynamicStoragePools(u.st, storagePools); err != nil {
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
	machinesCollection, closer := u.st.getCollection(machinesC)
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
		instance, err := m.InstanceId()
		if errors.IsNotProvisioned(err) {
			unprovisioned = append(unprovisioned, m)
		} else if err != nil {
			assignContextf(&err, u.Name(), context)
			return failure(err)
		} else {
			instances = append(instances, instance)
			instanceMachines[instance] = m
		}
	}

	// Filter the list of instances that are suitable for
	// distribution, and then map them back to machines.
	//
	// TODO(axw) 2014-05-30 #1324904
	// Shuffle machines to reduce likelihood of collisions.
	// The partition of provisioned/unprovisioned machines
	// must be maintained.
	if instances, err = distributeUnit(u, instances); err != nil {
		assignContextf(&err, u.Name(), context)
		return failure(err)
	}
	machines := make([]*Machine, len(instances), len(instances)+len(unprovisioned))
	for i, instance := range instances {
		m, ok := instanceMachines[instance]
		if !ok {
			err := fmt.Errorf("invalid instance returned: %v", instance)
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
	err = u.st.runTransaction(ops)
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
	return u.st.EnqueueAction(u.Tag(), name, payloadWithDefaults)
}

// ActionSpecs gets the ActionSpec map for the Unit's charm.
func (u *Unit) ActionSpecs() (ActionSpecsByName, error) {
	none := ActionSpecsByName{}
	curl, _ := u.CharmURL()
	if curl == nil {
		// If unit charm URL is not yet set, fall back to service
		svc, err := u.Application()
		if err != nil {
			return none, err
		}
		curl, _ = svc.CharmURL()
		if curl == nil {
			return none, errors.Errorf("no URL set for application %q", svc.Name())
		}
	}
	ch, err := u.st.Charm(curl)
	if err != nil {
		return none, errors.Annotatef(err, "unable to get charm with URL %q", curl.String())
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
func (u *Unit) Resolve(noretryHooks bool) error {
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
	mode := ResolvedRetryHooks
	if noretryHooks {
		mode = ResolvedNoHooks
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
	if err := u.st.runTransaction(ops); err == nil {
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
	err := u.st.runTransaction(ops)
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
	agentStatusDoc     statusDoc
	workloadStatusDoc  statusDoc
	workloadVersionDoc statusDoc
	meterStatusDoc     *meterStatusDoc
}

// addUnitOps returns the operations required to add a unit to the units
// collection, along with all the associated expected other unit entries. This
// method is used by both the *Service.addUnitOpsWithCons method and the
// migration import code.
func addUnitOps(st *State, args addUnitOpsArgs) ([]txn.Op, error) {
	name := args.unitDoc.Name
	agentGlobalKey := unitAgentGlobalKey(name)

	// TODO: consider the constraints op
	// TODO: consider storageOps
	prereqOps := []txn.Op{
		createStatusOp(st, unitGlobalKey(name), args.workloadStatusDoc),
		createStatusOp(st, agentGlobalKey, args.agentStatusDoc),
		createStatusOp(st, globalWorkloadVersionKey(name), args.workloadVersionDoc),
		createMeterStatusOp(st, agentGlobalKey, args.meterStatusDoc),
	}

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
		st:        g.st,
		globalKey: g.globalKey,
		filter:    filter,
	}
	return statusHistory(args)
}
