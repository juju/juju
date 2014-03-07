// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"time"

	"github.com/juju/loggo"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
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

const (
	ResolvedNone       ResolvedMode = ""
	ResolvedRetryHooks ResolvedMode = "retry-hooks"
	ResolvedNoHooks    ResolvedMode = "no-hooks"
)

// unitDoc represents the internal state of a unit in MongoDB.
// Note the correspondence with UnitInfo in state/api/params.
type unitDoc struct {
	Name           string `bson:"_id"`
	Service        string
	Series         string
	CharmURL       *charm.URL
	Principal      string
	Subordinates   []string
	PublicAddress  string
	PrivateAddress string
	MachineId      string
	Resolved       ResolvedMode
	Tools          *tools.Tools `bson:",omitempty"`
	Ports          []instance.Port
	Life           Life
	TxnRevno       int64 `bson:"txn-revno"`
	PasswordHash   string
}

// Unit represents the state of a service unit.
type Unit struct {
	st  *State
	doc unitDoc
	annotator
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
	charm, err := u.st.Charm(u.doc.CharmURL)
	if err != nil {
		return nil, err
	}
	result := charm.Config().DefaultSettings()
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
// It an error that satisfies IsNotFound if the tools have not yet been set.
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
	defer utils.ErrorContextf(&err, "cannot set agent version for unit %q", u)
	if err = checkVersionValidity(v); err != nil {
		return err
	}
	tools := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"tools", tools}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return onAbort(err, errDead)
	}
	u.doc.Tools = tools
	return nil
}

// SetMongoPassword sets the password the agent responsible for the unit
// should use to communicate with the state servers.  Previous passwords
// are invalidated.
func (u *Unit) SetMongoPassword(password string) error {
	return u.st.setMongoPassword(u.Tag(), password)
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
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"passwordhash", passwordHash}}}},
	}}
	err := u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of unit %q: %v", u, onAbort(err, errDead))
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
	for i := 0; i < 5; i++ {
		switch ops, err := unit.destroyOps(); err {
		case errRefresh:
		case errAlreadyDying:
			return nil
		case nil:
			if err := unit.st.runTransaction(ops); err != txn.ErrAborted {
				return err
			}
		default:
			return err
		}
		if err := unit.Refresh(); errors.IsNotFoundError(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return ErrExcessiveContention
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
	minUnitsOp := minUnitsTriggerOp(u.st, u.ServiceName())
	setDyingOps := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"life", Dying}}}},
	}, minUnitsOp}
	if u.doc.Principal != "" {
		return setDyingOps, nil
	} else if len(u.doc.Subordinates) != 0 {
		return setDyingOps, nil
	}

	sdocId := u.globalKey()
	sdoc, err := getStatus(u.st, sdocId)
	if errors.IsNotFoundError(err) {
		return nil, errAlreadyDying
	} else if err != nil {
		return nil, err
	}
	if sdoc.Status != params.StatusPending {
		return setDyingOps, nil
	}
	ops := []txn.Op{{
		C:      u.st.statuses.Name,
		Id:     sdocId,
		Assert: D{{"status", params.StatusPending}},
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

var errAlreadyRemoved = stderrors.New("entity has already been removed")

// removeOps returns the operations necessary to remove the unit, assuming
// the supplied asserts apply to the unit document.
func (u *Unit) removeOps(asserts D) ([]txn.Op, error) {
	svc, err := u.st.Service(u.doc.Service)
	if errors.IsNotFoundError(err) {
		// If the service has been removed, the unit must already have been.
		return nil, errAlreadyRemoved
	} else if err != nil {
		return nil, err
	}
	return svc.removeUnitOps(u, asserts)
}

var ErrUnitHasSubordinates = stderrors.New("unit has subordinates")

var unitHasNoSubordinates = D{{
	"$or", []D{
		{{"subordinates", D{{"$size", 0}}}},
		{{"subordinates", D{{"$exists", false}}}},
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
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: append(notDeadDoc, unitHasNoSubordinates...),
		Update: D{{"$set", D{{"life", Dead}}}},
	}}
	if err := u.st.runTransaction(ops); err != txn.ErrAborted {
		return err
	}
	if notDead, err := isNotDead(u.st.units, u.doc.Name); err != nil {
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
	defer utils.ErrorContextf(&err, "cannot remove unit %q", u)
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
	for i := 0; i < 5; i++ {
		switch ops, err := unit.removeOps(isDeadDoc); err {
		case errRefresh:
		case errAlreadyRemoved:
			return nil
		case nil:
			if err := u.st.runTransaction(ops); err != txn.ErrAborted {
				return err
			}
		default:
			return err
		}
		if err := unit.Refresh(); errors.IsNotFoundError(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return ErrExcessiveContention
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

// DeployerTag returns the tag of the agent responsible for deploying
// the unit. If no such entity can be determined, false is returned.
func (u *Unit) DeployerTag() (string, bool) {
	if u.doc.Principal != "" {
		return names.UnitTag(u.doc.Principal), true
	} else if u.doc.MachineId != "" {
		return names.MachineTag(u.doc.MachineId), true
	}
	return "", false
}

// PrincipalName returns the name of the unit's principal.
// If the unit is not a subordinate, false is returned.
func (u *Unit) PrincipalName() (string, bool) {
	return u.doc.Principal, u.doc.Principal != ""
}

// addressesOfMachine returns Addresses of the related machine if present.
func (u *Unit) addressesOfMachine() []instance.Address {
	id := u.doc.MachineId
	if id != "" {
		m, err := u.st.Machine(id)
		if err == nil {
			return m.Addresses()
		}
		unitLogger.Errorf("unit %v misses machine id %v", u, id)
	}
	return nil
}

// PublicAddress returns the public address of the unit and whether it is valid.
func (u *Unit) PublicAddress() (string, bool) {
	publicAddress := u.doc.PublicAddress
	addresses := u.addressesOfMachine()
	if len(addresses) > 0 {
		publicAddress = instance.SelectPublicAddress(addresses)
	}
	return publicAddress, publicAddress != ""
}

// PrivateAddress returns the private address of the unit and whether it is valid.
func (u *Unit) PrivateAddress() (string, bool) {
	privateAddress := u.doc.PrivateAddress
	addresses := u.addressesOfMachine()
	if len(addresses) > 0 {
		privateAddress = instance.SelectInternalAddress(addresses, false)
	}
	return privateAddress, privateAddress != ""
}

// Refresh refreshes the contents of the Unit from the underlying
// state. It an error that satisfies IsNotFound if the unit has been removed.
func (u *Unit) Refresh() error {
	err := u.st.units.FindId(u.doc.Name).One(&u.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("unit %q", u)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh unit %q: %v", u, err)
	}
	return nil
}

// Status returns the status of the unit.
func (u *Unit) Status() (status params.Status, info string, data params.StatusData, err error) {
	doc, err := getStatus(u.st, u.globalKey())
	if err != nil {
		return "", "", nil, err
	}
	status = doc.Status
	info = doc.StatusInfo
	data = doc.StatusData
	return
}

// SetStatus sets the status of the unit. The optional values
// allow to pass additional helpful status data.
func (u *Unit) SetStatus(status params.Status, info string, data params.StatusData) error {
	doc := statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}
	if err := doc.validateSet(); err != nil {
		return err
	}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDeadDoc,
	},
		updateStatusOp(u.st, u.globalKey(), doc),
	}
	err := u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set status of unit %q: %v", u, onAbort(err, errDead))
	}
	return nil
}

// OpenPort sets the policy of the port with protocol and number to be opened.
func (u *Unit) OpenPort(protocol string, number int) (err error) {
	port := instance.Port{Protocol: protocol, Number: number}
	defer utils.ErrorContextf(&err, "cannot open port %v for unit %q", port, u)
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDeadDoc,
		Update: D{{"$addToSet", D{{"ports", port}}}},
	}}
	err = u.st.runTransaction(ops)
	if err != nil {
		return onAbort(err, errDead)
	}
	found := false
	for _, p := range u.doc.Ports {
		if p == port {
			break
		}
	}
	if !found {
		u.doc.Ports = append(u.doc.Ports, port)
	}
	return nil
}

// ClosePort sets the policy of the port with protocol and number to be closed.
func (u *Unit) ClosePort(protocol string, number int) (err error) {
	port := instance.Port{Protocol: protocol, Number: number}
	defer utils.ErrorContextf(&err, "cannot close port %v for unit %q", port, u)
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDeadDoc,
		Update: D{{"$pull", D{{"ports", port}}}},
	}}
	err = u.st.runTransaction(ops)
	if err != nil {
		return onAbort(err, errDead)
	}
	newPorts := make([]instance.Port, 0, len(u.doc.Ports))
	for _, p := range u.doc.Ports {
		if p != port {
			newPorts = append(newPorts, p)
		}
	}
	u.doc.Ports = newPorts
	return nil
}

// OpenedPorts returns a slice containing the open ports of the unit.
func (u *Unit) OpenedPorts() []instance.Port {
	ports := append([]instance.Port{}, u.doc.Ports...)
	instance.SortPorts(ports)
	return ports
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
func (u *Unit) SetCharmURL(curl *charm.URL) (err error) {
	defer func() {
		if err == nil {
			u.doc.CharmURL = curl
		}
	}()
	if curl == nil {
		return fmt.Errorf("cannot set nil charm url")
	}
	for i := 0; i < 5; i++ {
		if notDead, err := isNotDead(u.st.units, u.doc.Name); err != nil {
			return err
		} else if !notDead {
			return fmt.Errorf("unit %q is dead", u)
		}
		sel := D{{"_id", u.doc.Name}, {"charmurl", curl}}
		if count, err := u.st.units.Find(sel).Count(); err != nil {
			return err
		} else if count == 1 {
			// Already set
			return nil
		}
		if count, err := u.st.charms.FindId(curl).Count(); err != nil {
			return err
		} else if count < 1 {
			return fmt.Errorf("unknown charm url %q", curl)
		}

		// Add a reference to the service settings for the new charm.
		incOp, err := settingsIncRefOp(u.st, u.doc.Service, curl, false)
		if err != nil {
			return err
		}

		// Set the new charm URL.
		differentCharm := D{{"charmurl", D{{"$ne", curl}}}}
		ops := []txn.Op{
			incOp,
			{
				C:      u.st.units.Name,
				Id:     u.doc.Name,
				Assert: append(notDeadDoc, differentCharm...),
				Update: D{{"$set", D{{"charmurl", curl}}}},
			}}
		if u.doc.CharmURL != nil {
			// Drop the reference to the old charm.
			decOps, err := settingsDecRefOps(u.st, u.doc.Service, u.doc.CharmURL)
			if err != nil {
				return err
			}
			ops = append(ops, decOps...)
		}
		if err := u.st.runTransaction(ops); err != txn.ErrAborted {
			return err
		}
	}
	return ErrExcessiveContention
}

// AgentAlive returns whether the respective remote agent is alive.
func (u *Unit) AgentAlive() (bool, error) {
	return u.st.pwatcher.Alive(u.globalKey())
}

// Tag returns a name identifying the unit that is safe to use
// as a file name.  The returned name will be different from other
// Tag values returned by any other entities from the same state.
func (u *Unit) Tag() string {
	return names.UnitTag(u.Name())
}

// WaitAgentAlive blocks until the respective agent is alive.
func (u *Unit) WaitAgentAlive(timeout time.Duration) (err error) {
	defer utils.ErrorContextf(&err, "waiting for agent of unit %q", u)
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

// SetAgentAlive signals that the agent for unit u is alive.
// It returns the started pinger.
func (u *Unit) SetAgentAlive() (*presence.Pinger, error) {
	p := presence.NewPinger(u.st.presence, u.globalKey())
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
	pudoc := unitDoc{}
	err = u.st.units.Find(D{{"_id", u.doc.Principal}}).One(&pudoc)
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
	assert := append(isAliveDoc, D{
		{"$or", []D{
			{{"machineid", ""}},
			{{"machineid", m.Id()}},
		}},
	}...)
	massert := isAliveDoc
	if unused {
		massert = append(massert, D{{"clean", D{{"$ne", false}}}}...)
	}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: assert,
		Update: D{{"$set", D{{"machineid", m.doc.Id}}}},
	}, {
		C:      u.st.machines.Name,
		Id:     m.doc.Id,
		Assert: massert,
		Update: D{{"$addToSet", D{{"principals", u.doc.Name}}}, {"$set", D{{"clean", false}}}},
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
		ops = append(ops, txn.Op{
			C:      u.st.machines.Name,
			Id:     parentId,
			Assert: D{{"clean", true}},
		}, txn.Op{
			C:      u.st.containerRefs.Name,
			Id:     parentId,
			Assert: D{hasNoContainersTerm},
		})
	}
	isUnassigned := D{{"machineid", ""}}
	asserts := append(isAliveDoc, isUnassigned...)
	ops = append(ops, txn.Op{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: asserts,
		Update: D{{"$set", D{{"machineid", mdoc.Id}}}},
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

// constraints is a helper function to return a unit's deployment constraints.
func (u *Unit) constraints() (*constraints.Value, error) {
	cons, err := readConstraints(u.st, u.globalKey())
	if errors.IsNotFoundError(err) {
		// Lack of constraints indicates lack of unit.
		return nil, errors.NotFoundf("unit")
	} else if err != nil {
		return nil, err
	}
	return &cons, nil
}

// AssignToNewMachineOrContainer assigns the unit to a new machine, with constraints
// determined according to the service and environment constraints at the time of unit creation.
// If a container is required, a clean, empty machine instance is required on which to create
// the container. An existing clean, empty instance is first searched for, and if not found,
// a new one is created.
func (u *Unit) AssignToNewMachineOrContainer() (err error) {
	defer assignContextf(&err, u, "new machine or container")
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	cons, err := u.constraints()
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
	query, err := u.findCleanMachineQuery(true, &hostCons)
	if err != nil {
		return err
	}
	err = query.One(&host)
	if err == mgo.ErrNotFound {
		// No existing clean, empty machine so create a new one.
		// The container constraint will be used by AssignToNewMachine to create the required container.
		return u.AssignToNewMachine()
	} else if err != nil {
		return err
	}
	template := MachineTemplate{
		Series:      u.doc.Series,
		Constraints: *cons,
		Jobs:        []MachineJob{JobHostUnits},
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
	cons, err := u.constraints()
	if err != nil {
		return err
	}
	var containerType instance.ContainerType
	// Configure to create a new container if required.
	if cons.HasContainer() {
		containerType = *cons.Container
	}
	template := MachineTemplate{
		Series:      u.doc.Series,
		Constraints: *cons,
		Jobs:        []MachineJob{JobHostUnits},
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

// AssignToCleanMachine assigns u to a machine which is marked as clean and is also
// not hosting any containers. A machine is clean if it has never had any principal units
// assigned to it. If there are no clean machines besides any machine(s) running JobHostEnviron,
// an error is returned.
// This method does not take constraints into consideration when choosing a
// machine (lp:1161919).
func (u *Unit) AssignToCleanEmptyMachine() (m *Machine, err error) {
	return u.assignToCleanMaybeEmptyMachine(true)
}

var hasContainerTerm = bson.DocElem{
	"$and", []D{
		{{"children", D{{"$not", D{{"$size", 0}}}}}},
		{{"children", D{{"$exists", true}}}},
	}}

var hasNoContainersTerm = bson.DocElem{
	"$or", []D{
		{{"children", D{{"$size", 0}}}},
		{{"children", D{{"$exists", false}}}},
	}}

// findCleanMachineQuery returns a Mongo query to find clean (and possibly empty) machines with
// characteristics matching the specified constraints.
func (u *Unit) findCleanMachineQuery(requireEmpty bool, cons *constraints.Value) (*mgo.Query, error) {
	// Select all machines that can accept principal units and are clean.
	var containerRefs []machineContainers
	// If we need empty machines, first build up a list of machine ids which have containers
	// so we can exclude those.
	if requireEmpty {
		err := u.st.containerRefs.Find(D{hasContainerTerm}).All(&containerRefs)
		if err != nil {
			return nil, err
		}
	}
	var machinesWithContainers = make([]string, len(containerRefs))
	for i, cref := range containerRefs {
		machinesWithContainers[i] = cref.Id
	}
	terms := D{
		{"life", Alive},
		{"series", u.doc.Series},
		{"jobs", []MachineJob{JobHostUnits}},
		{"clean", true},
		{"_id", D{{"$nin", machinesWithContainers}}},
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
	var suitableTerms D
	if cons.Arch != nil && *cons.Arch != "" {
		suitableTerms = append(suitableTerms, bson.DocElem{"arch", *cons.Arch})
	}
	if cons.Mem != nil && *cons.Mem > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"mem", D{{"$gte", *cons.Mem}}})
	}
	if cons.RootDisk != nil && *cons.RootDisk > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"rootdisk", D{{"$gte", *cons.RootDisk}}})
	}
	if cons.CpuCores != nil && *cons.CpuCores > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"cpucores", D{{"$gte", *cons.CpuCores}}})
	}
	if cons.CpuPower != nil && *cons.CpuPower > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"cpupower", D{{"$gte", *cons.CpuPower}}})
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 {
		suitableTerms = append(suitableTerms, bson.DocElem{"tags", D{{"$all", *cons.Tags}}})
	}
	if len(suitableTerms) > 0 {
		err := u.st.instanceData.Find(suitableTerms).Select(bson.M{"_id": 1}).All(&suitableInstanceData)
		if err != nil {
			return nil, err
		}
		var suitableIds = make([]string, len(suitableInstanceData))
		for i, m := range suitableInstanceData {
			suitableIds[i] = m.Id
		}
		terms = append(terms, bson.DocElem{"_id", D{{"$in", suitableIds}}})
	}
	return u.st.machines.Find(terms), nil
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
	cons, err := u.constraints()
	if err != nil {
		assignContextf(&err, u, context)
		return nil, err
	}
	query, err := u.findCleanMachineQuery(requireEmpty, cons)
	if err != nil {
		assignContextf(&err, u, context)
		return nil, err
	}

	// TODO(rog) Fix so this is more efficient when there are concurrent uses.
	// Possible solution: pick the highest and the smallest id of all
	// unused machines, and try to assign to the first one >= a random id in the
	// middle.
	iter := query.Batch(1).Prefetch(0).Iter()
	var mdoc machineDoc
	for iter.Next(&mdoc) {
		m := newMachine(u.st, &mdoc)
		err := u.assignToMachine(m, true)
		if err == nil {
			return m, nil
		}
		if err != inUseErr && err != machineNotAliveErr {
			assignContextf(&err, u, context)
			return nil, err
		}
	}
	if err := iter.Err(); err != nil {
		assignContextf(&err, u, context)
		return nil, err
	}
	return nil, noCleanMachines
}

// UnassignFromMachine removes the assignment between this unit and the
// machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	// TODO check local machine id and add an assert that the
	// machine id is as expected.
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"machineid", ""}}}},
	}}
	if u.doc.MachineId != "" {
		ops = append(ops, txn.Op{
			C:      u.st.machines.Name,
			Id:     u.doc.MachineId,
			Assert: txn.DocExists,
			Update: D{{"$pull", D{{"principals", u.doc.Name}}}},
		})
	}
	err = u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot unassign unit %q from machine: %v", u, onAbort(err, errors.NotFoundf("machine")))
	}
	u.doc.MachineId = ""
	return nil
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) (err error) {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"publicaddress", address}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set public address of unit %q: %v", u, onAbort(err, errors.NotFoundf("unit")))
	}
	u.doc.PublicAddress = address
	return nil
}

// SetPrivateAddress sets the private address of the unit.
func (u *Unit) SetPrivateAddress(address string) error {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"privateaddress", address}}}},
	}}
	err := u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set private address of unit %q: %v", u, onAbort(err, errors.NotFoundf("unit")))
	}
	u.doc.PrivateAddress = address
	return nil
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
	if status != params.StatusError {
		return fmt.Errorf("unit %q is not in an error state", u)
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
	defer utils.ErrorContextf(&err, "cannot set resolved mode for unit %q", u)
	switch mode {
	case ResolvedRetryHooks, ResolvedNoHooks:
	default:
		return fmt.Errorf("invalid error resolution mode: %q", mode)
	}
	// TODO(fwereade): assert unit has error status.
	resolvedNotSet := D{{"resolved", ResolvedNone}}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: append(notDeadDoc, resolvedNotSet...),
		Update: D{{"$set", D{{"resolved", mode}}}},
	}}
	if err := u.st.runTransaction(ops); err == nil {
		u.doc.Resolved = mode
		return nil
	} else if err != txn.ErrAborted {
		return err
	}
	if ok, err := isNotDead(u.st.units, u.doc.Name); err != nil {
		return err
	} else if !ok {
		return errDead
	}
	// For now, the only remaining assert is that resolved was unset.
	return fmt.Errorf("already resolved")
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() error {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"resolved", ResolvedNone}}}},
	}}
	err := u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot clear resolved mode for unit %q: %v", u, errors.NotFoundf("unit"))
	}
	u.doc.Resolved = ResolvedNone
	return nil
}
