// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strconv"
	"strings"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/api/params"
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

	// OpenPort marks the supplied port for opening when the executing unit's
	// service is exposed.
	OpenPort(protocol string, port int) error

	// ClosePort ensures the supplied port is closed even when the executing
	// unit's service is exposed (unless it is opened separately by a co-
	// located unit).
	ClosePort(protocol string, port int) error

	// Config returns the current service configuration of the executing unit.
	ConfigSettings() (charm.Settings, error)

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

	// OwnerTag returns the owner of the service the executing units belongs to
	OwnerTag() string
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
