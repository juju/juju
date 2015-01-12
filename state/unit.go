// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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

// unitDoc represents the internal state of a unit in MongoDB.
// Note the correspondence with UnitInfo in apiserver/params.
type unitDoc struct {
	DocID        string `bson:"_id"`
	Name         string `bson:"name"`
	EnvUUID      string `bson:"env-uuid"`
	Service      string
	Series       string
	CharmURL     *charm.URL
	Principal    string
	Subordinates []string
	MachineId    string
	Resolved     ResolvedMode
	Tools        *tools.Tools `bson:",omitempty"`
	Life         Life
	TxnRevno     int64 `bson:"txn-revno"`
	PasswordHash string

	// No longer used - to be removed.
	Ports          []network.Port
	PublicAddress  string
	PrivateAddress string
}

// Unit represents the state of a service unit.
type Unit struct {
	st  *State
	doc unitDoc
	annotator
	presence.Presencer
}

func newUnit(st *State, udoc *unitDoc) *Unit {
	unit := &Unit{
		st:  st,
		doc: *udoc,
	}
	unit.annotator = annotator{
		globalKey: unit.globalKey(),
		tag:       unit.Tag(),
		st:        st,
	}
	return unit
}

// Service returns the service.
func (u *Unit) Service() (*Service, error) {
	return u.st.Service(u.doc.Service)
}

// ConfigSettings returns the complete set of service charm config settings
// available to the unit. Unset values will be replaced with the default
// value for the associated option, and may thus be nil when no default is
// specified.
func (u *Unit) ConfigSettings() (charm.Settings, error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("unit charm not set")
	}
	settings, err := readSettings(u.st, serviceSettingsKey(u.doc.Service, u.doc.CharmURL))
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

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.doc.Service
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
	return "u#" + name
}

// globalKey returns the global database key for the unit.
func (u *Unit) globalKey() string {
	return unitGlobalKey(u.doc.Name)
}

// Life returns whether the unit is Alive, Dying or Dead.
func (u *Unit) Life() Life {
	return u.doc.Life
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
	// In Juju 1.16 and older we used the slower password hash for unit
	// agents. So check to see if the supplied password matches the old
	// path, and if so, update it to the new mechanism.
	// We ignore any error in setting the password hash, as we'll just try
	// again next time
	if utils.UserPasswordHash(password, utils.CompatSalt) == u.doc.PasswordHash {
		logger.Debugf("%s logged in with old password hash, changing to AgentPasswordHash",
			u.Tag())
		u.setPasswordHash(agentHash)
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
		if err = unit.Refresh(); errors.IsNotFound(err) {
			return nil
		}
	}
	return err
}

var unitNotInstalled = bson.D{
	{"$or", []bson.D{
		{{"status", StatusPending}},
		{{"status", StatusAllocating}},
		{{"status", StatusInstalling}},
	}}}

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
	minUnitsOp := minUnitsTriggerOp(u.st, u.ServiceName())
	cleanupOp := u.st.newCleanupOp(cleanupDyingUnit, u.doc.Name)
	setDyingOps := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}, cleanupOp, minUnitsOp}
	if u.doc.Principal != "" {
		return setDyingOps, nil
	} else if len(u.doc.Subordinates) != 0 {
		return setDyingOps, nil
	}

	sdocId := u.globalKey()
	sdoc, err := getStatus(u.st, sdocId)
	if errors.IsNotFound(err) {
		return nil, errAlreadyDying
	} else if err != nil {
		return nil, err
	}
	if sdoc.Status != StatusPending && sdoc.Status != StatusAllocating && sdoc.Status != StatusInstalling {
		return setDyingOps, nil
	}
	ops := []txn.Op{{
		C:      statusesC,
		Id:     u.st.docID(sdocId),
		Assert: unitNotInstalled,
	}, minUnitsOp}
	removeAsserts := append(isAliveDoc, unitHasNoSubordinates...)
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
func (u *Unit) destroyHostOps(s *Service) (ops []txn.Op, err error) {
	if s.doc.Subordinate {
		return []txn.Op{{
			C:      unitsC,
			Id:     u.st.docID(u.doc.Principal),
			Assert: txn.DocExists,
			Update: bson.D{{"$pull", bson.D{{"subordinates", u.doc.Name}}}},
		}}, nil
	} else if u.doc.MachineId == "" {
		unitLogger.Errorf("unit %v unassigned", u)
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
	} else if hasJob(m.doc.Jobs, JobManageEnviron) {
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
			bson.D{{"principals", []string{u.doc.Name}}},
			bson.D{{"jobs", bson.D{{"$nin", []MachineJob{JobManageEnviron}}}}},
			bson.D{{"hasvote", bson.D{{"$ne", true}}}},
		}}}
	} else {
		machineAssert = bson.D{{"$or", []bson.D{
			bson.D{{"principals", bson.D{{"$ne", []string{u.doc.Name}}}}},
			bson.D{{"jobs", bson.D{{"$in", []MachineJob{JobManageEnviron}}}}},
			bson.D{{"hasvote", true}},
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

var errAlreadyRemoved = stderrors.New("entity has already been removed")

// removeOps returns the operations necessary to remove the unit, assuming
// the supplied asserts apply to the unit document.
func (u *Unit) removeOps(asserts bson.D) ([]txn.Op, error) {
	svc, err := u.st.Service(u.doc.Service)
	if errors.IsNotFound(err) {
		// If the service has been removed, the unit must already have been.
		return nil, errAlreadyRemoved
	} else if err != nil {
		return nil, err
	}
	return svc.removeUnitOps(u, asserts)
}

// ErrUnitHasSubordinates is a standard error to indicate that an a Unit
// cannot complete an operation to end it's life because it still has
// subordinate services
var ErrUnitHasSubordinates = stderrors.New("unit has subordinates")

var unitHasNoSubordinates = bson.D{{
	"$or", []bson.D{
		{{"subordinates", bson.D{{"$size", 0}}}},
		{{"subordinates", bson.D{{"$exists", false}}}},
	},
}}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. If the unit has subordinates, it will
// return ErrUnitHasSubordinates.
func (u *Unit) EnsureDead() (err error) {
	if u.doc.Life == Dead {
		return nil
	}
	defer func() {
		if err == nil {
			u.doc.Life = Dead
		}
	}()
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: append(notDeadDoc, unitHasNoSubordinates...),
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
	return ErrUnitHasSubordinates
}

// Remove removes the unit from state, and may remove its service as well, if
// the service is Dying and no other references to it exist. It will fail if
// the unit is not Dead.
func (u *Unit) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove unit %q", u)
	if u.doc.Life != Dead {
		return stderrors.New("unit is not dead")
	}

	// Now the unit is Dead, we can be sure that it's impossible for it to
	// enter relation scopes (once it's Dying, we can be sure of this; but
	// EnsureDead does not require that it already be Dying, so this is the
	// only point at which we can safely backstop lp:1233457 and mitigate
	// the impact of unit agent bugs that leave relation scopes occupied).
	relations, err := serviceRelations(u.st, u.doc.Service)
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
	candidates, err := serviceRelations(u.st, u.doc.Service)
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
func (u *Unit) machine() (*Machine, error) {
	id, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Annotatef(err, "unit %v cannot get assigned machine", u)
	}
	m, err := u.st.Machine(id)
	if err != nil {
		return nil, errors.Annotatef(err, "unit %v misses machine id %v", u)
	}
	return m, nil
}

// addressesOfMachine returns Addresses of the related machine if present.
func (u *Unit) addressesOfMachine() []network.Address {
	m, err := u.machine()
	if err != nil {
		unitLogger.Errorf("%v", err)
		return nil
	}
	return m.Addresses()
}

// PublicAddress returns the public address of the unit and whether it is valid.
func (u *Unit) PublicAddress() (string, bool) {
	var publicAddress string
	addresses := u.addressesOfMachine()
	if len(addresses) > 0 {
		publicAddress = network.SelectPublicAddress(addresses)
	}
	return publicAddress, publicAddress != ""
}

// PrivateAddress returns the private address of the unit and whether it is valid.
func (u *Unit) PrivateAddress() (string, bool) {
	var privateAddress string
	addresses := u.addressesOfMachine()
	if len(addresses) > 0 {
		privateAddress = network.SelectInternalAddress(addresses, false)
	}
	return privateAddress, privateAddress != ""
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

// Status returns the status of the unit.
func (u *Unit) Status() (status Status, info string, data map[string]interface{}, err error) {
	doc, err := getStatus(u.st, u.globalKey())
	if err != nil {
		return "", "", nil, err
	}
	status = doc.Status
	info = doc.StatusInfo
	data = doc.StatusData
	return
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
func (u *Unit) SetStatus(status Status, info string, data map[string]interface{}) error {
	doc, err := newUnitAgentStatusDoc(status, info, data)
	if err != nil {
		return err
	}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: notDeadDoc,
	},
		updateStatusOp(u.st, u.globalKey(), doc.statusDoc),
	}
	err = u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set status of unit %q: %v", u, onAbort(err, ErrDead))
	}
	return nil
}

// OpenPorts opens the given port range and protocol for the unit, if
// it does not conflict with another already opened range on the
// unit's assigned machine.
func (u *Unit) OpenPorts(protocol string, fromPort, toPort int) (err error) {
	ports, err := NewPortRange(u.Name(), fromPort, toPort, protocol)
	if err != nil {
		return errors.Annotatef(err, "invalid port range %v-%v/%v", fromPort, toPort, protocol)
	}
	defer errors.DeferredAnnotatef(&err, "cannot open ports %v for unit %q", ports, u)

	machineId, err := u.AssignedMachineId()
	if err != nil {
		return errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	// TODO(dimitern) 2014-09-10 bug #1337804: network name is
	// hard-coded until multiple network support lands
	machinePorts, err := getOrCreatePorts(u.st, machineId, network.DefaultPublic)
	if err != nil {
		return errors.Annotatef(err, "cannot get or create ports for machine %q", machineId)
	}

	return machinePorts.OpenPorts(ports)
}

// ClosePorts closes the given port range and protocol for the unit.
func (u *Unit) ClosePorts(protocol string, fromPort, toPort int) (err error) {
	ports, err := NewPortRange(u.Name(), fromPort, toPort, protocol)
	if err != nil {
		return errors.Annotatef(err, "invalid port range %v-%v/%v", fromPort, toPort, protocol)
	}
	defer errors.DeferredAnnotatef(&err, "cannot close ports %v for unit %q", ports, u)

	machineId, err := u.AssignedMachineId()
	if err != nil {
		return errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	// TODO(dimitern) 2014-09-10 bug #1337804: network name is
	// hard-coded until multiple network support lands
	machinePorts, err := getOrCreatePorts(u.st, machineId, network.DefaultPublic)
	if err != nil {
		return errors.Annotatef(err, "cannot get or create ports for machine %q", machineId)
	}

	return machinePorts.ClosePorts(ports)
}

// OpenPort opens the given port and protocol for the unit.
func (u *Unit) OpenPort(protocol string, number int) error {
	return u.OpenPorts(protocol, number, number)
}

// ClosePort closes the given port and protocol for the unit.
func (u *Unit) ClosePort(protocol string, number int) error {
	return u.ClosePorts(protocol, number, number)
}

// OpenedPorts returns a slice containing the open port ranges of the
// unit.
func (u *Unit) OpenedPorts() ([]network.PortRange, error) {
	machineId, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Annotatef(err, "unit %q has no assigned machine", u)
	}

	// TODO(dimitern) 2014-09-10 bug #1337804: network name is
	// hard-coded until multiple network support lands
	machinePorts, err := getPorts(u.st, machineId, network.DefaultPublic)
	result := []network.PortRange{}
	if err == nil {
		ports := machinePorts.PortsForUnit(u.Name())
		for _, port := range ports {
			result = append(result, network.PortRange{
				Protocol: port.Protocol,
				FromPort: port.FromPort,
				ToPort:   port.ToPort,
			})
		}
	} else {
		if !errors.IsNotFound(err) {
			return nil, errors.Annotatef(err, "failed getting ports for unit %q", u)
		}
	}
	network.SortPortRanges(result)
	return result, nil
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
	envUUID := u.st.EnvironUUID()
	units := getCollectionFromDB(db, unitsC, envUUID)
	charms := getCollectionFromDB(db, charmsC, envUUID)

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
		incOp, err := settingsIncRefOp(u.st, u.doc.Service, curl, false)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Set the new charm URL.
		differentCharm := bson.D{{"charmurl", bson.D{{"$ne", curl}}}}
		ops := []txn.Op{
			incOp,
			{
				C:      unitsC,
				Id:     u.doc.DocID,
				Assert: append(notDeadDoc, differentCharm...),
				Update: bson.D{{"$set", bson.D{{"charmurl", curl}}}},
			}}
		if u.doc.CharmURL != nil {
			// Drop the reference to the old charm.
			decOps, err := settingsDecRefOps(u.st, u.doc.Service, u.doc.CharmURL)
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
	return u.st.pwatcher.Alive(u.globalKey())
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
	u.st.pwatcher.Watch(u.globalKey(), ch)
	defer u.st.pwatcher.Unwatch(u.globalKey(), ch)
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			return fmt.Errorf("still not alive after timeout")
		case <-u.st.pwatcher.Dead():
			return u.st.pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for unit %q", u))
}

// SetAgentPresence signals that the agent for unit u is alive.
// It returns the started pinger.
func (u *Unit) SetAgentPresence() (*presence.Pinger, error) {
	presenceCollection := u.st.getPresence()
	p := presence.NewPinger(presenceCollection, u.globalKey())
	err := p.Start()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// NotAssignedError indicates that a unit is not assigned to a machine (and, in
// the case of subordinate units, that the unit's principal is not assigned).
type NotAssignedError struct{ Unit *Unit }

func (e *NotAssignedError) Error() string {
	return fmt.Sprintf("unit %q is not assigned to a machine", e.Unit)
}

// IsNotAssigned verifies that err is an instance of NotAssignedError
func IsNotAssigned(err error) bool {
	_, ok := err.(*NotAssignedError)
	return ok
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id string, err error) {
	if u.IsPrincipal() {
		if u.doc.MachineId == "" {
			return "", &NotAssignedError{u}
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
		return "", &NotAssignedError{u}
	}
	return pudoc.MachineId, nil
}

var (
	machineNotAliveErr = stderrors.New("machine is not alive")
	machineNotCleanErr = stderrors.New("machine is dirty")
	unitNotAliveErr    = stderrors.New("unit is not alive")
	alreadyAssignedErr = stderrors.New("unit is already assigned to a machine")
	inUseErr           = stderrors.New("machine is not unused")
)

// assignToMachine is the internal version of AssignToMachine,
// also used by AssignToUnusedMachine. It returns specific errors
// in some cases:
// - machineNotAliveErr when the machine is not alive.
// - unitNotAliveErr when the unit is not alive.
// - alreadyAssignedErr when the unit has already been assigned
// - inUseErr when the machine already has a unit assigned (if unused is true)
func (u *Unit) assignToMachine(m *Machine, unused bool) (err error) {
	if u.doc.Series != m.doc.Series {
		return fmt.Errorf("series does not match")
	}
	if u.doc.MachineId != "" {
		if u.doc.MachineId != m.Id() {
			return alreadyAssignedErr
		}
		return nil
	}
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
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
	// assignToMachine implies assignment to an existing machine,
	// which is only permitted if unit placement is supported.
	if err := u.st.supportsUnitPlacement(); err != nil {
		return err
	}
	assert := append(isAliveDoc, bson.D{
		{"$or", []bson.D{
			{{"machineid", ""}},
			{{"machineid", m.Id()}},
		}},
	}...)
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
	}}
	err = u.st.runTransaction(ops)
	if err == nil {
		u.doc.MachineId = m.doc.Id
		m.doc.Clean = false
		return nil
	}
	if err != txn.ErrAborted {
		return err
	}
	u0, err := u.st.Unit(u.Name())
	if err != nil {
		return err
	}
	m0, err := u.st.Machine(m.Id())
	if err != nil {
		return err
	}
	switch {
	case u0.Life() != Alive:
		return unitNotAliveErr
	case m0.Life() != Alive:
		return machineNotAliveErr
	case u0.doc.MachineId != "" || !unused:
		return alreadyAssignedErr
	}
	return inUseErr
}

func assignContextf(err *error, unit *Unit, target string) {
	if *err != nil {
		*err = fmt.Errorf("cannot assign unit %q to %s: %v", unit, target, *err)
	}
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(m *Machine) (err error) {
	defer assignContextf(&err, u, fmt.Sprintf("machine %s", m))
	return u.assignToMachine(m, false)
}

// assignToNewMachine assigns the unit to a machine created according to
// the supplied params, with the supplied constraints.
func (u *Unit) assignToNewMachine(template MachineTemplate, parentId string, containerType instance.ContainerType) error {
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
			return fmt.Errorf("assignToNewMachine called without container type (should never happen)")
		}
		// The new parent machine is clean and only hosts units,
		// regardless of its child.
		parentParams := template
		parentParams.Jobs = []MachineJob{JobHostUnits}
		mdoc, ops, err = u.st.addMachineInsideNewMachineOps(template, parentParams, containerType)
	default:
		// Container type is specified but no parent id.
		mdoc, ops, err = u.st.addMachineInsideMachineOps(template, parentId, containerType)
	}
	if err != nil {
		return err
	}
	// Ensure the host machine is really clean.
	if parentId != "" {
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
	isUnassigned := bson.D{{"machineid", ""}}
	asserts := append(isAliveDoc, isUnassigned...)
	ops = append(ops, txn.Op{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: asserts,
		Update: bson.D{{"$set", bson.D{{"machineid", mdoc.Id}}}},
	})

	err = u.st.runTransaction(ops)
	if err == nil {
		u.doc.MachineId = mdoc.Id
		return nil
	} else if err != txn.ErrAborted {
		return err
	}

	// If we assume that the machine ops will never give us an
	// operation that would fail (because the machine id(s) that it
	// chooses are unique), then the only reasons that the
	// transaction could have been aborted are:
	//  * the unit is no longer alive
	//  * the unit has been assigned to a different machine
	//  * the parent machine we want to create a container on was
	//  clean but became dirty
	unit, err := u.st.Unit(u.Name())
	if err != nil {
		return err
	}
	switch {
	case unit.Life() != Alive:
		return unitNotAliveErr
	case unit.doc.MachineId != "":
		return alreadyAssignedErr
	}
	if parentId == "" {
		return fmt.Errorf("cannot add top level machine: transaction aborted for unknown reason")
	}
	m, err := u.st.Machine(parentId)
	if err != nil {
		return err
	}
	if !m.Clean() {
		return machineNotCleanErr
	}
	containers, err := m.Containers()
	if err != nil {
		return err
	}
	if len(containers) > 0 {
		return machineNotCleanErr
	}
	return fmt.Errorf("cannot add container within machine: transaction aborted for unknown reason")
}

// Constraints returns the unit's deployment constraints.
func (u *Unit) Constraints() (*constraints.Value, error) {
	cons, err := readConstraints(u.st, u.globalKey())
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
// environment constraints at the time of unit creation. If a
// container is required, a clean, empty machine instance is required
// on which to create the container. An existing clean, empty instance
// is first searched for, and if not found, a new one is created.
func (u *Unit) AssignToNewMachineOrContainer() (err error) {
	defer assignContextf(&err, u, "new machine or container")
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
	var host machineDoc
	hostCons := *cons
	noContainer := instance.NONE
	hostCons.Container = &noContainer
	query, closer, err := u.findCleanMachineQuery(true, &hostCons)
	if err != nil {
		return err
	}
	defer closer()
	err = query.One(&host)
	if err == mgo.ErrNotFound {
		// No existing clean, empty machine so create a new one.
		// The container constraint will be used by AssignToNewMachine to create the required container.
		return u.AssignToNewMachine()
	} else if err != nil {
		return err
	}
	svc, err := u.Service()
	if err != nil {
		return err
	}
	requestedNetworks, err := svc.Networks()
	if err != nil {
		return err
	}
	template := MachineTemplate{
		Series:            u.doc.Series,
		Constraints:       *cons,
		Jobs:              []MachineJob{JobHostUnits},
		RequestedNetworks: requestedNetworks,
	}
	err = u.assignToNewMachine(template, host.Id, *cons.Container)
	if err == machineNotCleanErr {
		// The clean machine was used before we got a chance to use it so just
		// stick the unit on a new machine.
		return u.AssignToNewMachine()
	}
	return err
}

// AssignToNewMachine assigns the unit to a new machine, with constraints
// determined according to the service and environment constraints at the
// time of unit creation.
func (u *Unit) AssignToNewMachine() (err error) {
	defer assignContextf(&err, u, "new machine")
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	// Get the ops necessary to create a new machine, and the machine doc that
	// will be added with those operations (which includes the machine id).
	cons, err := u.Constraints()
	if err != nil {
		return err
	}
	var containerType instance.ContainerType
	// Configure to create a new container if required.
	if cons.HasContainer() {
		containerType = *cons.Container
	}
	svc, err := u.Service()
	if err != nil {
		return err
	}
	requestedNetworks, err := svc.Networks()
	if err != nil {
		return err
	}
	template := MachineTemplate{
		Series:            u.doc.Series,
		Constraints:       *cons,
		Jobs:              []MachineJob{JobHostUnits},
		RequestedNetworks: requestedNetworks,
	}
	return u.assignToNewMachine(template, "", containerType)
}

var noCleanMachines = stderrors.New("all eligible machines in use")

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
func (u *Unit) findCleanMachineQuery(requireEmpty bool, cons *constraints.Value) (_ *mgo.Query, _ func(), err error) {
	db, closer := u.st.newDB()
	defer func() {
		if err != nil {
			closer()
		}
	}()
	containerRefsCollection := db.C(containerRefsC)

	// Select all machines that can accept principal units and are clean.
	var containerRefs []machineContainers
	// If we need empty machines, first build up a list of machine ids which have containers
	// so we can exclude those.
	if requireEmpty {
		err = containerRefsCollection.Find(bson.D{hasContainerTerm}).All(&containerRefs)
		if err != nil {
			return nil, closer, err
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
		instanceData := db.C(instanceDataC)
		err := instanceData.Find(suitableTerms).Select(bson.M{"_id": 1}).All(&suitableInstanceData)
		if err != nil {
			return nil, closer, err
		}
		var suitableIds = make([]string, len(suitableInstanceData))
		for i, m := range suitableInstanceData {
			suitableIds[i] = m.DocID
		}
		terms = append(terms, bson.DocElem{"_id", bson.D{{"$in", suitableIds}}})
	}
	machines := db.C(machinesC)
	return machines.Find(terms), closer, nil
}

// assignToCleanMaybeEmptyMachine implements AssignToCleanMachine and AssignToCleanEmptyMachine.
// A 'machine' may be a machine instance or container depending on the service constraints.
func (u *Unit) assignToCleanMaybeEmptyMachine(requireEmpty bool) (m *Machine, err error) {
	context := "clean"
	if requireEmpty {
		context += ", empty"
	}
	context += " machine"

	if u.doc.Principal != "" {
		err = fmt.Errorf("unit is a subordinate")
		assignContextf(&err, u, context)
		return nil, err
	}

	// Get the unit constraints to see what deployment requirements we have to adhere to.
	cons, err := u.Constraints()
	if err != nil {
		assignContextf(&err, u, context)
		return nil, err
	}
	query, closer, err := u.findCleanMachineQuery(requireEmpty, cons)
	if err != nil {
		assignContextf(&err, u, context)
		return nil, err
	}
	defer closer()

	// Find all of the candidate machines, and associated
	// instances for those that are provisioned. Instances
	// will be distributed across in preference to
	// unprovisioned machines.
	var mdocs []*machineDoc
	if err := query.All(&mdocs); err != nil {
		assignContextf(&err, u, context)
		return nil, err
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
			assignContextf(&err, u, context)
			return nil, err
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
		assignContextf(&err, u, context)
		return nil, err
	}
	machines := make([]*Machine, len(instances), len(instances)+len(unprovisioned))
	for i, instance := range instances {
		m, ok := instanceMachines[instance]
		if !ok {
			err := fmt.Errorf("invalid instance returned: %v", instance)
			assignContextf(&err, u, context)
			return nil, err
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
		err := u.assignToMachine(m, true)
		if err == nil {
			return m, nil
		}
		if err != inUseErr && err != machineNotAliveErr {
			assignContextf(&err, u, context)
			return nil, err
		}
	}
	return nil, noCleanMachines
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
// this Unit, and returns its ID.
func (u *Unit) AddAction(name string, payload map[string]interface{}) (*Action, error) {
	if len(name) == 0 {
		return nil, errors.New("no action name given")
	}
	specs, err := u.ActionSpecs()
	if err != nil {
		return nil, err
	}
	spec, ok := specs[name]
	if !ok {
		return nil, errors.Errorf("action %q not defined on unit %q", name, u.Name())
	}
	_, err = spec.ValidateParams(payload)
	if err != nil {
		return nil, err
	}
	return u.st.EnqueueAction(u.Tag(), name, payload)
}

// ActionSpecs gets the ActionSpec map for the Unit's charm.
func (u *Unit) ActionSpecs() (ActionSpecsByName, error) {
	none := ActionSpecsByName{}
	curl, _ := u.CharmURL()
	if curl == nil {
		// If unit charm URL is not yet set, fall back to service
		svc, err := u.Service()
		if err != nil {
			return none, err
		}
		curl, _ = svc.CharmURL()
		if curl == nil {
			return none, errors.Errorf("no URL set for service %q", svc.Name())
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
func (u *Unit) CancelAction(action *Action) (*Action, error) {
	return action.Finish(ActionResults{Status: ActionCancelled})
}

// WatchActionNotifications starts and returns a StringsWatcher that
// notifies when actions with Id prefixes matching this Unit are added
func (u *Unit) WatchActionNotifications() StringsWatcher {
	return u.st.watchEnqueuedActionsFilteredBy(u)
}

// Actions returns a list of actions pending or completed for this unit.
func (u *Unit) Actions() ([]*Action, error) {
	return u.st.matchingActions(u)
}

// CompletedActions returns a list of actions that have finished for
// this unit.
func (u *Unit) CompletedActions() ([]*Action, error) {
	return u.st.matchingActionsCompleted(u)
}

// PendingActions returns a list of actions pending for this unit.
func (u *Unit) PendingActions() ([]*Action, error) {
	return u.st.matchingActionsPending(u)
}

// Resolve marks the unit as having had any previous state transition
// problems resolved, and informs the unit that it may attempt to
// reestablish normal workflow. The retryHooks parameter informs
// whether to attempt to reexecute previous failed hooks or to continue
// as if they had succeeded before.
func (u *Unit) Resolve(retryHooks bool) error {
	status, _, _, err := u.Status()
	if err != nil {
		return err
	}
	if status != StatusError {
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

// AddMetric adds a new batch of metrics to the database.
// A UUID for the metric will be generated and the new MetricBatch will be returned
func (u *Unit) AddMetrics(created time.Time, metrics []Metric) (*MetricBatch, error) {
	charmUrl, ok := u.CharmURL()
	if !ok {
		return nil, stderrors.New("failed to add metrics, couldn't find charm url")
	}
	service, err := u.Service()
	if err != nil {
		return nil, errors.Annotatef(err, "couldn't retrieve service whilst adding metrics")
	}
	return u.st.addMetrics(u.UnitTag(), charmUrl, created, metrics, service.MetricCredentials())
}
