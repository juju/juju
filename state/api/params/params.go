package params

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ServiceDeploy holds the parameters for making the ServiceDeploy call.
type ServiceDeploy struct {
	ServiceName string
	Config      map[string]string
	ConfigYAML  string // Takes precedence over config if both are present.
	CharmUrl    string
	NumUnits    int
}

// ServiceExpose holds the parameters for making the ServiceExpose call.
type ServiceExpose struct {
	ServiceName string
}

// ServiceSet holds the parameters for a ServiceSet
// command. Options contains the configuration data.
type ServiceSet struct {
	ServiceName string
	Options     map[string]string
}

// ServiceSetYAML holds the parameters for
// a ServiceSetYAML command. Config contains the
// configuration data in YAML format.
type ServiceSetYAML struct {
	ServiceName string
	Config      string
}

// ServiceGet holds parameters for making the ServiceGet call.
type ServiceGet struct {
	ServiceName string
}

// ServiceGetResults holds results of the ServiceGet call.
type ServiceGetResults struct {
	Service  string
	Charm    string
	Settings map[string]interface{}
}

// ServiceUnexpose holds parameters for the ServiceUnexpose call.
type ServiceUnexpose struct {
	ServiceName string
}

// Resolved holds parameters for the Resolved call.
type Resolved struct {
	UnitName string
	Retry    bool
}

// ResolvedResults holds results of the Resolved call.
type ResolvedResults struct {
	Service  string
	Charm    string
	Settings map[string]interface{}
}

// ServiceAddUnits holds parameters for the AddUnits call.
type ServiceAddUnits struct {
	ServiceName string
	NumUnits    int
}

// ServiceDestroy holds the parameters for making the ServiceDestroy call.
type ServiceDestroy struct {
	ServiceName string
}

// Creds holds credentials for identifying an entity.
type Creds struct {
	EntityName string
	Password   string
}

// Machine holds details of a machine.
type Machine struct {
	InstanceId string
}

// EntityWatcherId holds the id of an EntityWatcher.
type EntityWatcherId struct {
	EntityWatcherId string
}

// AllWatcherId holds the id of an AllWatcher.
type AllWatcherId struct {
	AllWatcherId string
}

// AllWatcherNextResults holds deltas returned from calling AllWatcher.Next().
type AllWatcherNextResults struct {
	Deltas []Delta
}

// Password holds a password.
type Password struct {
	Password string
}

// Unit holds details of a unit.
type Unit struct {
	DeployerName string
	// TODO(rog) other unit attributes.
}

// User holds details of a user.
type User struct {
	// This is a placeholder for any information
	// that may be associated with a user in the
	// future.
}

// GetAnnotationsResults holds annotations associated with an entity.
type GetAnnotationsResults struct {
	Annotations map[string]string
}

// GetAnnotations stores parameters for making the GetAnnotations call.
type GetAnnotations struct {
	EntityId string
}

// SetAnnotation stores parameters for making the SetAnnotation call.
type SetAnnotation struct {
	EntityId string
	Key      string
	Value    string
}

// Delta holds details of a change to the environment.
type Delta struct {
	// If Removed is true, the entity has been removed;
	// otherwise it has been created or changed.
	Removed bool
	// Entity holds data about the entity that has changed.
	Entity EntityInfo
}

// MarshalJSON implements json.Marshaler.
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

// UnmarshalJSON implements json.Unmarshaler.
func (d *Delta) UnmarshalJSON(data []byte) error {
	var elements []json.RawMessage
	if err := json.Unmarshal(data, &elements); err != nil {
		return err
	}
	if len(elements) != 3 {
		return fmt.Errorf(
			"Expected 3 elements in top-level of JSON but got %d",
			len(elements))
	}
	var entityKind, operation string
	if err := json.Unmarshal(elements[0], &entityKind); err != nil {
		return err
	}
	if err := json.Unmarshal(elements[1], &operation); err != nil {
		return err
	}
	if operation == "remove" {
		d.Removed = true
	} else if operation != "change" {
		return fmt.Errorf("Unexpected operation %q", operation)
	}
	switch entityKind {
	case "machine":
		d.Entity = new(MachineInfo)
	case "service":
		d.Entity = new(ServiceInfo)
	case "unit":
		d.Entity = new(UnitInfo)
	case "relation":
		d.Entity = new(RelationInfo)
	default:
		return fmt.Errorf("Unexpected entity name %q", entityKind)
	}
	if err := json.Unmarshal(elements[2], &d.Entity); err != nil {
		return err
	}
	return nil
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

func (m MachineInfo) String() string {
	return fmt.Sprintf("machine{id %v; instance id %q}", m.Id, m.InstanceId)
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

// CharmInfo stores parameters for a CharmInfo call.
type CharmInfo struct {
	CharmURL string
}
