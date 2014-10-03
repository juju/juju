// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// Context is the interface that all hook helper commands
// depend on to interact with the rest of the system.
type Context interface {

	// Unit returns the executing unit's name.
	UnitName() string

	// PublicAddress returns the executing unit's public address.
	PublicAddress() (string, bool)

	// PrivateAddress returns the executing unit's private address.
	PrivateAddress() (string, bool)

	// OpenPorst marks the supplied port range for opening when the
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

	// Config returns the current service configuration of the executing unit.
	ConfigSettings() (charm.Settings, error)

	// ActionParams returns the map of params passed with an Action.
	ActionParams() (map[string]interface{}, error)

	// UpdateActionResults inserts new values for use with action-set.
	// The results struct will be delivered to the state server upon
	// completion of the Action.
	UpdateActionResults(keys []string, value string) error

	// SetActionMessage sets a message for the Action.
	SetActionMessage(string) error

	// SetActionFailed sets a failure state for the Action.
	SetActionFailed() error

	// HookRelation returns the ContextRelation associated with the executing
	// hook if it was found, and whether it was found.
	HookRelation() (ContextRelation, bool)

	// RemoteUnitName returns the name of the remote unit the hook execution
	// is associated with if it was found, and whether it was found.
	RemoteUnitName() (string, bool)

	// Relation returns the relation with the supplied id if it was found, and
	// whether it was found.
	Relation(id int) (ContextRelation, bool)

	// RelationIds returns the ids of all relations the executing unit is
	// currently participating in.
	RelationIds() []int

	// OwnerTag returns the user tag of the service the executing
	// units belongs to.
	OwnerTag() string

	// AddMetric records a metric to return after hook execution.
	AddMetrics(string, string, time.Time) error
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
	ReadSettings(unit string) (params.RelationSettings, error)
}

// Settings is implemented by types that manipulate unit settings.
type Settings interface {
	Map() params.RelationSettings
	Set(string, string)
	Delete(string)
}

// newRelationIdValue returns a gnuflag.Value for convenient parsing of relation
// ids in ctx.
func newRelationIdValue(ctx Context, result *int) *relationIdValue {
	v := &relationIdValue{result: result, ctx: ctx}
	id := -1
	if r, found := ctx.HookRelation(); found {
		id = r.Id()
		v.value = r.FakeId()
	}
	*result = id
	return v
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
	if _, found := v.ctx.Relation(id); !found {
		return fmt.Errorf("unknown relation id")
	}
	*v.result = id
	v.value = value
	return nil
}
