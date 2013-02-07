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

// Destroy sets the machine lifecycle to Dying if it is Alive. It does
// nothing otherwise. Destroy will fail if the machine has principal
// units assigned, or if the machine has JobManageEnviron.
func (m *Machine) Destroy() error {
	return m.advanceLifecycle(Dying)
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageEnviron.
func (m *Machine) EnsureDead() error {
	return m.advanceLifecycle(Dead)
}

// advanceLifecycle ensures that the machine's lifecycle is no earlier
// than the supplied value. If the machine already has that lifecycle
// value, or a later one, no changes will be made to remote state. If
// the machine has any responsibilities that preclude a valid change in
// lifecycle, it will return an error.
func (m0 *Machine) advanceLifecycle(life Life) (err error) {
	m := m0
	defer func() {
		if err == nil {
			// The machine's lifecycle is known to have advanced; it may be
			// known to have already advanced further than requested, in
			// which case we set the latest known valid value.
			if m == nil {
				life = Dead
			} else if m.doc.Life > life {
				life = m.doc.Life
			}
			m0.doc.Life = life
		}
	}()
	// op and
	op := txn.Op{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Update: D{{"$set", D{{"life", life}}}},
	}
	advanceAsserts := D{
		{"jobs", D{{"$nin", []MachineJob{JobManageEnviron}}}},
		{"$or", []D{
			{{"principals", D{{"$size", 0}}}},
			{{"principals", D{{"$exists", false}}}},
		}},
	}
	// 3 atempts: one with original data, one with refreshed data, and a final
	// one intended to determine the cause of failure of the preceding attempt.
	for i := 0; i < 3; i++ {
		// If the transaction was aborted, grab a fresh copy of the machine data.
		if i != 0 {
			if m, err = m.st.Machine(m.doc.Id); IsNotFound(err) {
				return nil
			} else if err != nil {
				return err
			}
		}
		// Check that the life change is sane, and collect the assertions
		// necessary to determine that it remains so.
		switch life {
		case Dying:
			if m.doc.Life != Alive {
				return nil
			}
			op.Assert = append(advanceAsserts, isAliveDoc...)
		case Dead:
			if m.doc.Life == Dead {
				return nil
			}
			op.Assert = append(advanceAsserts, notDeadDoc...)
		default:
			panic(fmt.Errorf("cannot advance lifecycle to %v", life))
		}
		// Check that the machine does not have any responsibilities that
		// prevent a lifecycle change.
		for _, j := range m.doc.Jobs {
			if j == JobManageEnviron {
				// (NOTE: When we enable multiple JobManageEnviron machines,
				// the restriction will become "there must be at least one
				// machine with this job".)
				return fmt.Errorf("machine %s is required by the environment", m.doc.Id)
			}
		}
		if len(m.doc.Principals) != 0 {
			return fmt.Errorf("machine %s has unit %q assigned", m.doc.Id, m.doc.Principals[0])
		}
		// Run the transaction...
		if err := m.st.runner.Run([]txn.Op{op}, "", nil); err != txn.ErrAborted {
			return err
		}
		// ...and retry on abort.
	}
	// In very rare circumstances, the final iteration above will have determined
	// no cause of failure, and attempted a final transaction: if this also failed,
	// we can be sure that the machine document is changing very fast, in a somewhat
	// surprising fashion, and that it is sensible to back off for now.
	return fmt.Errorf("machine %s cannot advance lifecycle: %v", m, ErrExcessiveContention)
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
