package mstate

import (
	"fmt"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/mstate/presence"
	"launchpad.net/juju-core/trivial"
	"strconv"
	"time"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
}

// machineDoc represents the internal state of a machine in MongoDB.
type machineDoc struct {
	Id         int `bson:"_id"`
	InstanceId string
	Life       Life
	Tools Tools
	TxnRevno   int64 `bson:"txn-revno"`
}

func newMachine(st *State, doc *machineDoc) *Machine {
	return &Machine{st: st, doc: *doc}
}

// Id returns the machine id.
func (m *Machine) Id() int {
	return m.doc.Id
}

// globalKey returns the global database key for the machine.
func (m *Machine) globalKey() string {
	return "m#" + m.String()
}

// Life returns whether the machine is Alive, Dying or Dead.
func (m *Machine) Life() Life {
	return m.doc.Life
}

// AgentTools returns the tools that the agent is currently running.
func (m *Machine) AgentTools() (*Tools, error) {
	tools := m.doc.Tools
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
		Assert: D{{"life", Alive}},
		Update: D{{"$set", D{{"tools", *t}}}},
	}}
	err = m.st.runner.Run(ops, "", nil)
	if err != nil {
		return deadOnAbort(err)
	}
	m.doc.Tools = *t
	return nil
}

// It does nothing otherwise.
func (m *Machine) Kill() error {
	err := ensureLife(m.st, m.st.machines, m.doc.Id, Dying, "machine")
	if err != nil {
		return err
	}
	m.doc.Life = Dying
	return nil
}

// Die sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (m *Machine) Die() error {
	err := ensureLife(m.st, m.st.machines, m.doc.Id, Dead, "machine")
	if err != nil {
		return err
	}
	m.doc.Life = Dead
	return nil
}

func (m *Machine) Refresh() error {
	doc := machineDoc{}
	err := m.st.machines.FindId(m.doc.Id).One(&doc)
	if err != nil {
		return fmt.Errorf("cannot refresh machine %v: %v", m, err)
	}
	m.doc = doc
	return nil
}

// Watch returns a watcher that fires when the machine changes.
func (m *Machine) Watch() *MachineWatcher {
	return newMachineWatcher(m)
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

// InstanceId returns the provider specific machine id for this machine.
func (m *Machine) InstanceId() (string, error) {
	if m.doc.InstanceId == "" {
		msg := fmt.Sprintf("instance id for machine %d is not set", m.Id())
		return "", &NotFoundError{msg}
	}
	return m.doc.InstanceId, nil
}

// Units returns all the units that have been assigned to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot get units assigned to machine %s", m)
	pudocs := []unitDoc{}
	err = m.st.units.Find(D{{"machineid", m.doc.Id}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	for _, pudoc := range pudocs {
		units = append(units, newUnit(m.st, &pudoc))
		docs := []unitDoc{}
		sel := D{{"principal", pudoc.Name}, {"life", Alive}}
		err = m.st.units.Find(sel).All(&docs)
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
func (m *Machine) SetInstanceId(id string) error {
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: D{{"life", Alive}},
		Update: D{{"$set", D{{"instanceid", id}}}},
	}}
	err := m.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set instance id of machine %s: %v", m, deadOnAbort(err))
	}
	m.doc.InstanceId = id
	return nil
}

// String returns a unique description of this machine.
func (m *Machine) String() string {
	return strconv.Itoa(m.doc.Id)
}
