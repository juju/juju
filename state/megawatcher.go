package state

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Delta holds details of a change to the environment.
type Delta struct {
	// If Removed is true, the entity has been removed;
	// otherwise it has been created or changed.
	Removed bool
	// Entity holds data about the entity that has changed.
	Entity EntityInfo
}

// MarshalJSON implements json.Unmarshaller.
func (d *Delta) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(d.Entity)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	c := "change"
	if d.Removed {
		c = "remove"
	}
	fmt.Fprintf(&buf, "%q,%q,", d.Entity.EntityKind(), c)
	buf.Write(b)
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	// EntityId returns the collection-specific identifier for the entity.
	EntityId() interface{}
	// EntityKind returns the kind of entity (for example "machine",
	// "service", ...)
	EntityKind() string
}

var (
	_ EntityInfo = (*MachineInfo)(nil)
	_ EntityInfo = (*ServiceInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
)

// MachineInfo holds the information about a Machine
// that is watched by StateWatcher.
type MachineInfo struct {
	Id         string `bson:"_id"`
	InstanceId string
}

func (i *MachineInfo) EntityId() interface{} { return i.Id }
func (i *MachineInfo) EntityKind() string    { return "machine" }

type ServiceInfo struct {
	Name    string `bson:"_id"`
	Exposed bool
}

func (i *ServiceInfo) EntityId() interface{} { return i.Name }
func (i *ServiceInfo) EntityKind() string    { return "service" }

type UnitInfo struct {
	Name    string `bson:"_id"`
	Service string
}

func (i *UnitInfo) EntityId() interface{} { return i.Name }
func (i *UnitInfo) EntityKind() string    { return "unit" }

type RelationInfo struct {
	Key string `bson:"_id"`
}

func (i *RelationInfo) EntityId() interface{} { return i.Key }
func (i *RelationInfo) EntityKind() string    { return "relation" }

// StateWatcher watches any changes to the state.
type StateWatcher struct {
	// TODO: hold the last revid that the StateWatcher saw.
}

func newStateWatcher(st *State) *StateWatcher {
	return &StateWatcher{}
}

func (w *StateWatcher) Err() error {
	return nil
}

// Stop stops the watcher.
func (w *StateWatcher) Stop() error {
	// This may not need to do anything at all.
	return nil
}

// Next retrieves all changes that have happened since the given revision
// number, blocking until there are some changes available.  It also
// returns the revision number of the latest change.
func (w *StateWatcher) Next() (*[]Delta, error) {
	// This is a stub to make progress with the higher level coding.
	return &[]Delta{
		Delta{
			Removed: false,
			Entity: &ServiceInfo{
				Name:    "Example",
				Exposed: true,
			},
		},
		Delta{
			Removed: true,
			Entity: &UnitInfo{
				Name:    "MyUnit",
				Service: "Example",
			},
		},
	}, nil
}
