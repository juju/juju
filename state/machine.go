package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/trivial"
	"time"
)

// An InstanceId is a provider-specific identifier associated with an
// instance (physical or virtual machine allocated in the provider).
type InstanceId string

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
}

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob int

const (
	_ MachineJob = iota
	JobHostUnits
	JobManageEnviron
	JobServeAPI
)

var jobNames = []string{
	JobHostUnits:     "JobHostUnits",
	JobManageEnviron: "JobManageEnviron",
	JobServeAPI:      "JobServeAPI",
}

func (job MachineJob) String() string {
	j := int(job)
	if j <= 0 || j >= len(jobNames) {
		return fmt.Sprintf("<unknown job %d>", j)
	}
	return jobNames[j]
}

// machineDoc represents the internal state of a machine in MongoDB.
type machineDoc struct {
	Id           string `bson:"_id"`
	InstanceId   InstanceId
	Principals   []string
	Life         Life
	Tools        *Tools `bson:",omitempty"`
	TxnRevno     int64  `bson:"txn-revno"`
	Jobs         []MachineJob
	PasswordHash string
}

func newMachine(st *State, doc *machineDoc) *Machine {
	return &Machine{st: st, doc: *doc}
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.doc.Id
}

// globalKey returns the global database key for the machine.
func (m *Machine) globalKey() string {
	return "m#" + m.String()
}

// MachineEntityName returns the entity name for the
// machine with the given id.
func MachineEntityName(id string) string {
	return fmt.Sprintf("machine-%s", id)
}

// EntityName returns a name identifying the machine that is safe to use
// as a file name.  The returned name will be different from other
// EntityName values returned by any other entities from the same state.
func (m *Machine) EntityName() string {
	return MachineEntityName(m.Id())
}

// Life returns whether the machine is Alive, Dying or Dead.
func (m *Machine) Life() Life {
	return m.doc.Life
}

// Jobs returns the responsibilities that must be fulfilled by m's agent.
func (m *Machine) Jobs() []MachineJob {
	return m.doc.Jobs
}

// AgentTools returns the tools that the agent is currently running.
// It returns a *NotFoundError if the tools have not yet been set.
func (m *Machine) AgentTools() (*Tools, error) {
	if m.doc.Tools == nil {
		return nil, notFoundf("agent tools for machine %v", m)
	}
	tools := *m.doc.Tools
	return &tools, nil
}

// SetAgentTools sets the tools that the agent is currently running.
func (m *Machine) SetAgentTools(t *Tools) (err error) {
	defer trivial.ErrorContextf(&err, "cannot set agent tools for machine %v", m)
	if t.Series == "" || t.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"tools", t}}}},
	}}
	if err := m.st.runner.Run(ops, "", nil); err != nil {
		return onAbort(err, errNotAlive)
	}
	tools := *t
	m.doc.Tools = &tools
	return nil
}

// SetMongoPassword sets the password the agent responsible for the machine
// should use to communicate with the state servers.  Previous passwords
// are invalidated.
func (m *Machine) SetMongoPassword(password string) error {
	return m.st.setMongoPassword(m.EntityName(), password)
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	hp := trivial.PasswordHash(password)
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"passwordhash", hp}}}},
	}}
	if err := m.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set password of machine %v: %v", m, onAbort(err, errNotAlive))
	}
	m.doc.PasswordHash = hp
	return nil
}

// PasswordValid returns whether the given password is valid
// for the given machine.
func (m *Machine) PasswordValid(password string) bool {
	return trivial.PasswordHash(password) == m.doc.PasswordHash
}

// deathAsserts returns the conditions that must hold for a machine to
// become Dying or Dead.
func (m *Machine) deathAsserts() D {
	return D{
		{"jobs", D{{"$nin", []MachineJob{JobManageEnviron}}}},
		{"$or", []D{
			{{"principals", D{{"$size", 0}}}},
			{{"principals", D{{"$exists", false}}}},
		}},
	}
}

// deathFailureReason returns an error indicating why the machine may have
// failed to advance its lifecycle to Dying or Dead. If deathFailureReason
// returns no error, it is possible that the condition that caused the txn
// failure no longer holds; it does not automatically indicate bad state.
func (m *Machine) deathFailureReason(life Life) (err error) {
	if m, err = m.st.Machine(m.doc.Id); err != nil {
		return err
	}
	defer trivial.ErrorContextf(&err, "machine %s cannot become %s", m, life)
	for _, j := range m.doc.Jobs {
		if j == JobManageEnviron {
			// If and when we enable multiple JobManageEnviron machines, the
			// restriction will become "there must be at least one machine
			// with this job", and this will need to change.
			return fmt.Errorf("required by environment")
		}
	}
	if len(m.doc.Principals) != 0 {
		return fmt.Errorf("unit %q is assigned to it", m.doc.Principals[0])
	}
	return nil
}

// deathAttempts controls how many times we should attempt a death operation.
// A single failure without a diagnosed cause indicates that the operation
// should certainly be retried; subsequent unknown failures cannot ever
// unambiguously indicate bad state, because it is *possible* that the number
// of assigned units is flipping from 1 to 0 and back, perfectly timed to
// abort every txn but show no reason for the failure; but we believe this
// situation to be vanishingly unlikely, and so only retry once.
var deathAttempts = 2

// Destroy sets the machine lifecycle to Dying if it is Alive. It does
// nothing otherwise. Destroy will fail if the machine has principal
// units assigned, or if the machine has JobManageEnviron.
func (m *Machine) Destroy() (err error) {
	if m.doc.Life != Alive {
		return nil
	}
	defer func() {
		if err == nil {
			m.doc.Life = Dying
		}
	}()
	for i := 0; i < deathAttempts; i++ {
		ops := []txn.Op{{
			C:      m.st.machines.Name,
			Id:     m.doc.Id,
			Assert: append(m.deathAsserts(), isAliveDoc...),
			Update: D{{"$set", D{{"life", Dying}}}},
		}}
		if err := m.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
			return err
		}
		if alive, err := isAlive(m.st.machines, m.doc.Id); err != nil {
			return err
		} else if !alive {
			return nil
		}
		if err := m.deathFailureReason(Dying); err != nil {
			return err
		}
	}
	return fmt.Errorf("machine %s cannot become dying: please contact juju-dev@lists.ubuntu.com", m)
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageEnviron.
func (m *Machine) EnsureDead() (err error) {
	if m.doc.Life == Dead {
		return nil
	}
	defer func() {
		if err == nil {
			m.doc.Life = Dead
		}
	}()
	for i := 0; i < deathAttempts; i++ {
		ops := []txn.Op{{
			C:      m.st.machines.Name,
			Id:     m.doc.Id,
			Assert: append(m.deathAsserts(), notDeadDoc...),
			Update: D{{"$set", D{{"life", Dead}}}},
		}}
		if err := m.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
			return err
		}
		if notDead, err := isNotDead(m.st.machines, m.doc.Id); err != nil {
			return err
		} else if !notDead {
			return nil
		}
		if err := m.deathFailureReason(Dead); err != nil {
			return err
		}
	}
	return fmt.Errorf("machine %s cannot become dead: please contact juju-dev@lists.ubuntu.com", m)
}

// Remove removes the machine from state. It will fail if the machine is not
// Dead.
func (m *Machine) Remove() (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove machine %s", m.doc.Id)
	if m.doc.Life != Dead {
		return fmt.Errorf("machine is not dead")
	}
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Remove: true,
	}}
	return m.st.runner.Run(ops, "", nil)
}

// Refresh refreshes the contents of the machine from the underlying
// state. It returns a NotFoundError if the machine has been removed.
func (m *Machine) Refresh() error {
	doc := machineDoc{}
	err := m.st.machines.FindId(m.doc.Id).One(&doc)
	if err == mgo.ErrNotFound {
		return notFoundf("machine %v", m)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh machine %v: %v", m, err)
	}
	m.doc = doc
	return nil
}

// AgentAlive returns whether the respective remote agent is alive.
func (m *Machine) AgentAlive() (bool, error) {
	return m.st.pwatcher.Alive(m.globalKey())
}

// WaitAgentAlive blocks until the respective agent is alive.
func (m *Machine) WaitAgentAlive(timeout time.Duration) (err error) {
	defer trivial.ErrorContextf(&err, "waiting for agent of machine %v", m)
	ch := make(chan presence.Change)
	m.st.pwatcher.Watch(m.globalKey(), ch)
	defer m.st.pwatcher.Unwatch(m.globalKey(), ch)
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			return fmt.Errorf("still not alive after timeout")
		case <-m.st.pwatcher.Dead():
			return m.st.pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for machine %v", m))
}

// SetAgentAlive signals that the agent for machine m is alive.
// It returns the started pinger.
func (m *Machine) SetAgentAlive() (*presence.Pinger, error) {
	p := presence.NewPinger(m.st.presence, m.globalKey())
	err := p.Start()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// InstanceId returns the provider specific instance id for this machine.
func (m *Machine) InstanceId() (InstanceId, error) {
	if m.doc.InstanceId == "" {
		return "", notFoundf("instance id for machine %v", m)
	}
	return m.doc.InstanceId, nil
}

// Units returns all the units that have been assigned to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot get units assigned to machine %v", m)
	pudocs := []unitDoc{}
	err = m.st.units.Find(D{{"machineid", m.doc.Id}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	for _, pudoc := range pudocs {
		units = append(units, newUnit(m.st, &pudoc))
		docs := []unitDoc{}
		err = m.st.units.Find(D{{"principal", pudoc.Name}}).All(&docs)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			units = append(units, newUnit(m.st, &doc))
		}
	}
	return units, nil
}

// SetInstanceId sets the provider specific machine id for this machine.
func (m *Machine) SetInstanceId(id InstanceId) (err error) {
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"instanceid", id}}}},
	}}
	if err := m.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set instance id of machine %v: %v", m, onAbort(err, errNotAlive))
	}
	m.doc.InstanceId = id
	return nil
}

// String returns a unique description of this machine.
func (m *Machine) String() string {
	return m.doc.Id
}
