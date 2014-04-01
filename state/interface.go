// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// EntityFinder is implemented by *State. See State.FindEntity
// for documentation on the method.
type EntityFinder interface {
	FindEntity(tag string) (Entity, error)
}

var _ EntityFinder = (*State)(nil)

// Entity represents any entity that can be returned
// by State.FindEntity. All entities have a tag.
type Entity interface {
	Tag() string
}

var (
	_ Entity = (*Machine)(nil)
	_ Entity = (*Unit)(nil)
	_ Entity = (*Service)(nil)
	_ Entity = (*Environment)(nil)
	_ Entity = (*User)(nil)
)

type StatusSetter interface {
	SetStatus(status params.Status, info string, data params.StatusData) error
}

type StatusGetter interface {
	Status() (status params.Status, info string, data params.StatusData, err error)
}

var (
	_ StatusSetter = (*Machine)(nil)
	_ StatusSetter = (*Unit)(nil)
	_ StatusGetter = (*Machine)(nil)
	_ StatusGetter = (*Unit)(nil)
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

// MongoPassworder represents an entity that can
// have a mongo password set for it.
type MongoPassworder interface {
	SetMongoPassword(password string) error
}

var (
	_ MongoPassworder = (*Machine)(nil)
	_ MongoPassworder = (*Unit)(nil)
)

// Annotator represents entities capable of handling annotations.
type Annotator interface {
	Annotation(key string) (string, error)
	Annotations() (map[string]string, error)
	SetAnnotations(pairs map[string]string) error
}

var (
	_ Annotator = (*Machine)(nil)
	_ Annotator = (*Unit)(nil)
	_ Annotator = (*Service)(nil)
	_ Annotator = (*Environment)(nil)
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
	MongoPassworder
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
