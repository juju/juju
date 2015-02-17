// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// EntityFinder is implemented by *State. See State.FindEntity
// for documentation on the method.
type EntityFinder interface {
	FindEntity(tag names.Tag) (Entity, error)
}

var _ EntityFinder = (*State)(nil)

// Entity represents any entity that can be returned
// by State.FindEntity. All entities have a tag.
type Entity interface {
	Tag() names.Tag
}

var (
	_ Entity = (*Machine)(nil)
	_ Entity = (*Unit)(nil)
	_ Entity = (*UnitAgent)(nil)
	_ Entity = (*Service)(nil)
	_ Entity = (*Environment)(nil)
	_ Entity = (*User)(nil)
	_ Entity = (*Action)(nil)
)

// Lifer represents an entity with a life.
type Lifer interface {
	Life() Life
}

var (
	_ Lifer = (*Machine)(nil)
	_ Lifer = (*Unit)(nil)
	_ Lifer = (*Service)(nil)
	_ Lifer = (*Relation)(nil)
)

// AgentTooler is implemented by entities
// that have associated agent tools.
type AgentTooler interface {
	AgentTools() (*tools.Tools, error)
	SetAgentVersion(version.Binary) error
}

// EnsureDeader with an EnsureDead method.
type EnsureDeader interface {
	EnsureDead() error
}

var (
	_ EnsureDeader = (*Machine)(nil)
	_ EnsureDeader = (*Unit)(nil)
)

// Remover represents entities with a Remove method.
type Remover interface {
	Remove() error
}

var (
	_ Remover = (*Machine)(nil)
	_ Remover = (*Unit)(nil)
)

// Authenticator represents entites capable of handling password
// authentication.
type Authenticator interface {
	Refresh() error
	SetPassword(pass string) error
	PasswordValid(pass string) bool
}

var (
	_ Authenticator = (*Machine)(nil)
	_ Authenticator = (*Unit)(nil)
	_ Authenticator = (*User)(nil)
)

// NotifyWatcherFactory represents an entity that
// can be watched.
type NotifyWatcherFactory interface {
	Watch() NotifyWatcher
}

var (
	_ NotifyWatcherFactory = (*Machine)(nil)
	_ NotifyWatcherFactory = (*Unit)(nil)
	_ NotifyWatcherFactory = (*Service)(nil)
	_ NotifyWatcherFactory = (*Environment)(nil)
)

// AgentEntity represents an entity that can
// have an agent responsible for it.
type AgentEntity interface {
	Entity
	Lifer
	Authenticator
	AgentTooler
	StatusSetter
	EnsureDeader
	Remover
	NotifyWatcherFactory
}

var (
	_ AgentEntity = (*Machine)(nil)
	_ AgentEntity = (*Unit)(nil)
)

// EnvironAccessor defines the methods needed to watch for environment
// config changes, and read the environment config.
type EnvironAccessor interface {
	WatchForEnvironConfigChanges() NotifyWatcher
	EnvironConfig() (*config.Config, error)
}

var _ EnvironAccessor = (*State)(nil)

// UnitsWatcher defines the methods needed to retrieve an entity (a
// machine or a service) and watch its units.
type UnitsWatcher interface {
	Entity
	WatchUnits() StringsWatcher
}

var _ UnitsWatcher = (*Machine)(nil)
var _ UnitsWatcher = (*Service)(nil)

// EnvironMachinesWatcher defines a single method -
// WatchEnvironMachines.
type EnvironMachinesWatcher interface {
	WatchEnvironMachines() StringsWatcher
}

var _ EnvironMachinesWatcher = (*State)(nil)

// InstanceIdGetter defines a single method - InstanceId.
type InstanceIdGetter interface {
	InstanceId() (instance.Id, error)
}

var _ InstanceIdGetter = (*Machine)(nil)

// ActionsWatcher defines the methods an entity exposes to watch Actions
// queued up for itself
type ActionsWatcher interface {
	Entity
	WatchActionNotifications() StringsWatcher
}

var (
	_ ActionsWatcher = (*Unit)(nil)
	// TODO(jcw4): when we implement service level Actions
	// _ ActionsWatcher = (*Service)(nil)
)

// ActionReceiver describes Entities that can have Actions queued for
// them, and that can get ActionRelated information about those actions.
// TODO(jcw4) consider implementing separate Actor classes for this
// interface; for example UnitActor that implements this interface, and
// takes a Unit and performs all these actions.
type ActionReceiver interface {
	Entity

	// AddAction queues an action with the given name and payload for this
	// ActionReceiver.
	AddAction(name string, payload map[string]interface{}) (*Action, error)

	// CancelAction removes a pending Action from the queue for this
	// ActionReceiver and marks it as cancelled.
	CancelAction(action *Action) (*Action, error)

	// WatchActionNotifications returns a StringsWatcher that will notify
	// on changes to the queued actions for this ActionReceiver.
	WatchActionNotifications() StringsWatcher

	// Actions returns the list of Actions queued and completed for this
	// ActionReceiver.
	Actions() ([]*Action, error)

	// CompletedActions returns the list of Actions completed for this
	// ActionReceiver.
	CompletedActions() ([]*Action, error)

	// PendingActions returns the list of Actions queued for this
	// ActionReceiver.
	PendingActions() ([]*Action, error)

	// RunningActions returns the list of Actions currently running for
	// this ActionReceiver.
	RunningActions() ([]*Action, error)
}

var (
	_ ActionReceiver = (*Unit)(nil)
	// TODO(jcw4) - use when Actions can be queued for Services.
	//_ ActionReceiver = (*Service)(nil)
)

// GlobalEntity specifies entity.
type GlobalEntity interface {
	globalKey() string
	Tag() names.Tag
}

var (
	_ GlobalEntity = (*Machine)(nil)
	_ GlobalEntity = (*Unit)(nil)
	_ GlobalEntity = (*Service)(nil)
	_ GlobalEntity = (*Charm)(nil)
	_ GlobalEntity = (*Environment)(nil)
)
