// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/relation"
)

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
	LocalSettings() (Settings, error)

	// UnitNames returns a list of the remote units in the relation.
	UnitNames() []string

	// RemoteSettings returns the settings of any remote unit in the relation.
	RemoteSettings(unit string) (Settings, error)

	// Suspended returns true if the relation is suspended.
	Suspended() bool

	// SetStatus sets the relation's status.
	SetStatus(relation.Status) error
}

// Context is the interface that all hook helper commands
// depend on to interact with the rest of the system.
type Context interface {
	hookContext
	relationHookContext
}

// HookContext represents the information and functionality that is
// common to all charm hooks.
type hookContext interface {
	ContextApplication
	ContextNetworking
	ContextRelations
	ContextStatus
}

// RelationHookContext is the context for a relation hook.
type RelationHookContext interface {
	hookContext
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

// ContextApplication is the part of a hook context related to the application.
type ContextApplication interface {
	// ApplicationName returns the executing application's name.
	ApplicationName() string

	// ConfigSettings returns the current configuration of the executing unit.
	ApplicationConfig() (charm.Settings, error)

	// SetContainerSpec updates the yaml spec used to create a container.
	SetContainerSpec(specYaml, unitName string) error
}

// ContextStatus is the part of a hook context related to the application's status.
type ContextStatus interface {
	// ApplicationStatus returns the executing application status.
	ApplicationStatus() (StatusInfo, error)

	// SetApplicationStatus updates the status for the application.
	SetApplicationStatus(appStatus StatusInfo) error
}

// ContextNetworking is the part of a hook context related to network
// interface of the unit's instance.
type ContextNetworking interface {
	// NetworkInfo returns the network info for the given bindings on the given relation.
	NetworkInfo(bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error)
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

// Settings is implemented by types that manipulate unit settings.
type Settings interface {
	Map() map[string]string
	Set(string, string)
	Delete(string)
}
