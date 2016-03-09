// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
)

// RebootPriority is the type used for reboot requests.
type RebootPriority int

const (
	// RebootSkip is a noop.
	RebootSkip RebootPriority = iota
	// RebootAfterHook means wait for current hook to finish before
	// rebooting.
	RebootAfterHook
	// RebootNow means reboot immediately, killing and requeueing the
	// calling hook
	RebootNow
)

// Context is the interface that all hook helper commands
// depend on to interact with the rest of the system.
type Context interface {
	HookContext
	relationHookContext
	actionHookContext
}

// HookContext represents the information and functionality that is
// common to all charm hooks.
type HookContext interface {
	ContextUnit
	ContextStatus
	ContextInstance
	ContextNetworking
	ContextLeadership
	ContextMetrics
	ContextStorage
	ContextComponents
	ContextRelations
}

// UnitHookContext is the context for a unit hook.
type UnitHookContext interface {
	HookContext
}

// RelationHookContext is the context for a relation hook.
type RelationHookContext interface {
	HookContext
	relationHookContext
}

type relationHookContext interface {
	// HookRelation returns the ContextRelation associated with the executing
	// hook if it was found, or an error if it was not found (or is not available).
	HookRelation() (ContextRelation, error)

	// RemoteUnitName returns the name of the remote unit the hook execution
	// is associated with if it was found, and an error if it was not found or is not
	// available.
	RemoteUnitName() (string, error)
}

// ActionHookContext is the context for an action hook.
type ActionHookContext interface {
	HookContext
	actionHookContext
}

type actionHookContext interface {
	// ActionParams returns the map of params passed with an Action.
	ActionParams() (map[string]interface{}, error)

	// UpdateActionResults inserts new values for use with action-set.
	// The results struct will be delivered to the controller upon
	// completion of the Action.
	UpdateActionResults(keys []string, value string) error

	// SetActionMessage sets a message for the Action.
	SetActionMessage(string) error

	// SetActionFailed sets a failure state for the Action.
	SetActionFailed() error
}

// ContextUnit is the part of a hook context related to the unit.
type ContextUnit interface {
	// UnitName returns the executing unit's name.
	UnitName() string

	// Config returns the current service configuration of the executing unit.
	ConfigSettings() (charm.Settings, error)
}

// ContextStatus is the part of a hook context related to the unit's status.
type ContextStatus interface {
	// UnitStatus returns the executing unit's current status.
	UnitStatus() (*StatusInfo, error)

	// SetUnitStatus updates the unit's status.
	SetUnitStatus(StatusInfo) error

	// ServiceStatus returns the executing unit's service status
	// (including all units).
	ServiceStatus() (ServiceStatusInfo, error)

	// SetServiceStatus updates the status for the unit's service.
	SetServiceStatus(StatusInfo) error
}

// ContextInstance is the part of a hook context related to the unit's instance.
type ContextInstance interface {
	// AvailabilityZone returns the executing unit's availability zone or an error
	// if it was not found (or is not available).
	AvailabilityZone() (string, error)

	// RequestReboot will set the reboot flag to true on the machine agent
	RequestReboot(prio RebootPriority) error
}

// ContextNetworking is the part of a hook context related to network
// interface of the unit's instance.
type ContextNetworking interface {
	// PublicAddress returns the executing unit's public address or an
	// error if it is not available.
	PublicAddress() (string, error)

	// PrivateAddress returns the executing unit's private address or an
	// error if it is not available.
	PrivateAddress() (string, error)

	// OpenPorts marks the supplied port range for opening when the
	// executing unit's service is exposed.
	OpenPorts(protocol string, fromPort, toPort int) error

	// ClosePorts ensures the supplied port range is closed even when
	// the executing unit's service is exposed (unless it is opened
	// separately by a co- located unit).
	ClosePorts(protocol string, fromPort, toPort int) error

	// OpenedPorts returns all port ranges currently opened by this
	// unit on its assigned machine. The result is sorted first by
	// protocol, then by number.
	OpenedPorts() []network.PortRange
}

// ContextLeadership is the part of a hook context related to the
// unit leadership.
type ContextLeadership interface {
	// IsLeader returns true if the local unit is known to be leader for at
	// least the next 30s.
	IsLeader() (bool, error)

	// LeaderSettings returns the current leader settings. Once leader settings
	// have been read in a given context, they will not be updated other than
	// via successful calls to WriteLeaderSettings.
	LeaderSettings() (map[string]string, error)

	// WriteLeaderSettings writes the supplied settings directly to state, or
	// fails if the local unit is not the service's leader.
	WriteLeaderSettings(map[string]string) error
}

// ContextMetrics is the part of a hook context related to metrics.
type ContextMetrics interface {
	// AddMetric records a metric to return after hook execution.
	AddMetric(string, string, time.Time) error
}

// ContextStorage is the part of a hook context related to storage
// resources associated with the unit.
type ContextStorage interface {
	// StorageTags returns a list of tags for storage instances
	// attached to the unit or an error if they are not available.
	StorageTags() ([]names.StorageTag, error)

	// Storage returns the ContextStorageAttachment with the supplied
	// tag if it was found, and an error if it was not found or is not
	// available to the context.
	Storage(names.StorageTag) (ContextStorageAttachment, error)

	// HookStorage returns the storage attachment associated
	// the executing hook if it was found, and an error if it
	// was not found or is not available.
	HookStorage() (ContextStorageAttachment, error)

	// AddUnitStorage saves storage constraints in the context.
	AddUnitStorage(map[string]params.StorageConstraints) error
}

// ContextComponents exposes modular Juju components as they relate to
// the unit in the context of the hook.
type ContextComponents interface {
	// Component returns the ContextComponent with the supplied name if
	// it was found.
	Component(name string) (ContextComponent, error)
}

// ContextRelations exposes the relations associated with the unit.
type ContextRelations interface {
	// Relation returns the relation with the supplied id if it was found, and
	// an error if it was not found or is not available.
	Relation(id int) (ContextRelation, error)

	// RelationIds returns the ids of all relations the executing unit is
	// currently participating in or an error if they are not available.
	RelationIds() ([]int, error)
}

// ContextComponent is a single modular Juju component as it relates to
// the current unit and hook. Components should implement this interfaces
// in a type-safe way. Ensuring checked type-conversions are preformed on
// the result and value interfaces. You will use the runner.RegisterComponentFunc
// to register a your components concrete ContextComponent implementation.
//
// See: process/context/context.go for an implementation example.
//
type ContextComponent interface {
	// Flush pushes the component's data to Juju state.
	// In the Flush implementation, call your components API.
	Flush() error
}

// ContextRelation expresses the capabilities of a hook with respect to a relation.
type ContextRelation interface {

	// Id returns an integer which uniquely identifies the relation.
	Id() int

	// Name returns the name the locally executing charm assigned to this relation.
	Name() string

	// FakeId returns a string of the form "relation-name:123", which uniquely
	// identifies the relation to the hook. In reality, the identification
	// of the relation is the integer following the colon, but the composed
	// name is useful to humans observing it.
	FakeId() string

	// Settings allows read/write access to the local unit's settings in
	// this relation.
	Settings() (Settings, error)

	// UnitNames returns a list of the remote units in the relation.
	UnitNames() []string

	// ReadSettings returns the settings of any remote unit in the relation.
	ReadSettings(unit string) (params.Settings, error)

	// NetworkConfig returns the network configuration for the relation.
	//
	// TODO(dimitern): Currently, only the Address is populated, add the
	// rest later.
	//
	// LKK Card: https://canonical.leankit.com/Boards/View/101652562/119258804
	NetworkConfig() ([]params.NetworkConfig, error)
}

// ContextStorageAttachment expresses the capabilities of a hook with
// respect to a storage attachment.
type ContextStorageAttachment interface {

	// Tag returns a tag which uniquely identifies the storage attachment
	// in the context of the unit.
	Tag() names.StorageTag

	// Kind returns the kind of the storage.
	Kind() storage.StorageKind

	// Location returns the location of the storage: the mount point for
	// filesystem-kind stores, and the device path for block-kind stores.
	Location() string
}

// Settings is implemented by types that manipulate unit settings.
type Settings interface {
	Map() params.Settings
	Set(string, string)
	Delete(string)
}

// newRelationIdValue returns a gnuflag.Value for convenient parsing of relation
// ids in ctx.
func newRelationIdValue(ctx Context, result *int) (*relationIdValue, error) {
	v := &relationIdValue{result: result, ctx: ctx}
	id := -1
	if r, err := ctx.HookRelation(); err == nil {
		id = r.Id()
		v.value = r.FakeId()
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	*result = id
	return v, nil
}

// relationIdValue implements gnuflag.Value for use in relation commands.
type relationIdValue struct {
	result *int
	ctx    Context
	value  string
}

// String returns the current value.
func (v *relationIdValue) String() string {
	return v.value
}

// Set interprets value as a relation id, if possible, and returns an error
// if it is not known to the system. The parsed relation id will be written
// to v.result.
func (v *relationIdValue) Set(value string) error {
	trim := value
	if idx := strings.LastIndex(trim, ":"); idx != -1 {
		trim = trim[idx+1:]
	}
	id, err := strconv.Atoi(trim)
	if err != nil {
		return fmt.Errorf("invalid relation id")
	}
	if _, err := v.ctx.Relation(id); err != nil {
		return errors.Trace(err)
	}
	*v.result = id
	v.value = value
	return nil
}

// newStorageIdValue returns a gnuflag.Value for convenient parsing of storage
// ids in ctx.
func newStorageIdValue(ctx Context, result *names.StorageTag) (*storageIdValue, error) {
	v := &storageIdValue{result: result, ctx: ctx}
	if s, err := ctx.HookStorage(); err == nil {
		*v.result = s.Tag()
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return v, nil
}

// storageIdValue implements gnuflag.Value for use in storage commands.
type storageIdValue struct {
	result *names.StorageTag
	ctx    Context
}

// String returns the current value.
func (v *storageIdValue) String() string {
	if *v.result == (names.StorageTag{}) {
		return ""
	}
	return v.result.Id()
}

// Set interprets value as a storage id, if possible, and returns an error
// if it is not known to the system. The parsed storage id will be written
// to v.result.
func (v *storageIdValue) Set(value string) error {
	if !names.IsValidStorage(value) {
		return errors.Errorf("invalid storage ID %q", value)
	}
	tag := names.NewStorageTag(value)
	if _, err := v.ctx.Storage(tag); err != nil {
		return errors.Trace(err)
	}
	*v.result = tag
	return nil
}
