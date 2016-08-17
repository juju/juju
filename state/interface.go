// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

// EntityFinder is implemented by *State. See State.FindEntity
// for documentation on the method.
type EntityFinder interface {
	FindEntity(tag names.Tag) (Entity, error)
}

// Entity represents any entity that can be returned
// by State.FindEntity. All entities have a tag.
type Entity interface {
	Tag() names.Tag
}

// EntityWithApplication is implemented by Units it is intended
// for anything that can return its Application.
type EntityWithApplication interface {
	Application() (*Application, error)
}

// Lifer represents an entity with a life.
type Lifer interface {
	Life() Life
}

// LifeBinder represents an entity whose lifespan is bindable
// to that of another entity.
type LifeBinder interface {
	Lifer

	// LifeBinding either returns the tag of an entity to which this
	// entity's lifespan is bound; the result may be nil, indicating
	// that the entity's lifespan is not bound to anything.
	//
	// The types of tags that may be returned are depdendent on the
	// entity type. For example, a Volume may be bound to a Filesystem,
	// but a Filesystem may not be bound to a Filesystem.
	LifeBinding() names.Tag
}

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

// Remover represents entities with a Remove method.
type Remover interface {
	Remove() error
}

// Authenticator represents entites capable of handling password
// authentication.
type Authenticator interface {
	Refresh() error
	SetPassword(pass string) error
	PasswordValid(pass string) bool
}

// NotifyWatcherFactory represents an entity that
// can be watched.
type NotifyWatcherFactory interface {
	Watch() NotifyWatcher
}

// AgentEntity represents an entity that can
// have an agent responsible for it.
type AgentEntity interface {
	Entity
	Lifer
	Authenticator
	AgentTooler
	status.StatusSetter
	EnsureDeader
	Remover
	NotifyWatcherFactory
}

// CloudAccessor defines the methods needed to obtain information
// about clouds and credentials.
type CloudAccessor interface {
	Cloud(cloud string) (cloud.Cloud, error)
	CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error)
}

// ModelAccessor defines the methods needed to watch for model
// config changes, and read the model config.
type ModelAccessor interface {
	WatchForModelConfigChanges() NotifyWatcher
	ModelConfig() (*config.Config, error)
}

// ControllerAccessor defines the methods needed to
// access controller information.
type ControllerAccessor interface {
	ControllerConfig() (controller.Config, error)
}

// UnitsWatcher defines the methods needed to retrieve an entity (a
// machine or a service) and watch its units.
type UnitsWatcher interface {
	Entity
	WatchUnits() StringsWatcher
}

// ModelMachinesWatcher defines a single method -
// WatchModelMachines.
type ModelMachinesWatcher interface {
	WatchModelMachines() StringsWatcher
}

// InstanceIdGetter defines a single method - InstanceId.
type InstanceIdGetter interface {
	InstanceId() (instance.Id, error)
}

// ActionsWatcher defines the methods an entity exposes to watch Actions
// queued up for itself
type ActionsWatcher interface {
	Entity
	WatchActionNotifications() StringsWatcher
}

// ActionReceiver describes Entities that can have Actions queued for
// them, and that can get ActionRelated information about those actions.
// TODO(jcw4) consider implementing separate Actor classes for this
// interface; for example UnitActor that implements this interface, and
// takes a Unit and performs all these actions.
type ActionReceiver interface {
	Entity

	// AddAction queues an action with the given name and payload for this
	// ActionReceiver.
	AddAction(name string, payload map[string]interface{}) (Action, error)

	// CancelAction removes a pending Action from the queue for this
	// ActionReceiver and marks it as cancelled.
	CancelAction(action Action) (Action, error)

	// WatchActionNotifications returns a StringsWatcher that will notify
	// on changes to the queued actions for this ActionReceiver.
	WatchActionNotifications() StringsWatcher

	// Actions returns the list of Actions queued and completed for this
	// ActionReceiver.
	Actions() ([]Action, error)

	// CompletedActions returns the list of Actions completed for this
	// ActionReceiver.
	CompletedActions() ([]Action, error)

	// PendingActions returns the list of Actions queued for this
	// ActionReceiver.
	PendingActions() ([]Action, error)

	// RunningActions returns the list of Actions currently running for
	// this ActionReceiver.
	RunningActions() ([]Action, error)
}

// GlobalEntity specifies entity.
type GlobalEntity interface {
	globalKey() string
	Tag() names.Tag
}

// Action represents  an instance of an action designated for a unit or machine
// in the model.
type Action interface {
	Entity

	// Id returns the local id of the Action.
	Id() string

	// Receiver returns the Name of the ActionReceiver for which this action
	// is enqueued.  Usually this is a Unit Name().
	Receiver() string

	// Name returns the name of the action, as defined in the charm.
	Name() string

	// Parameters will contain a structure representing arguments or parameters to
	// an action, and is expected to be validated by the Unit using the Charm
	// definition of the Action.
	Parameters() map[string]interface{}

	// Enqueued returns the time the action was added to state as a pending
	// Action.
	Enqueued() time.Time

	// Started returns the time that the Action execution began.
	Started() time.Time

	// Completed returns the completion time of the Action.
	Completed() time.Time

	// Status returns the final state of the action.
	Status() ActionStatus

	// Results returns the structured output of the action and any error.
	Results() (map[string]interface{}, string)

	// ActionTag returns an ActionTag constructed from this action's
	// Prefix and Sequence.
	ActionTag() names.ActionTag

	// Begin marks an action as running, and logs the time it was started.
	// It asserts that the action is currently pending.
	Begin() (Action, error)

	// Finish removes action from the pending queue and captures the output
	// and end state of the action.
	Finish(results ActionResults) (Action, error)
}
