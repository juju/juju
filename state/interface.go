// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/objectstore"
)

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

// EnsureDeader with an EnsureDead method.
type EnsureDeader interface {
	EnsureDead() error
}

// Remover represents entities with a Remove method.
type Remover interface {
	Remove(objectstore.ObjectStore) error
}

// Authenticator represents entites capable of handling password
// authentication.
type Authenticator interface {
	SetPassword(pass string) error
	PasswordValid(pass string) bool
}

// NotifyWatcherFactory represents an entity that
// can be watched.
type NotifyWatcherFactory interface {
	Watch() NotifyWatcher
}

// UnitsWatcher defines the methods needed to retrieve an entity (a
// machine or an application) and watch its units.
type UnitsWatcher interface {
	Entity
	WatchUnits() StringsWatcher
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
	WatchPendingActionNotifications() StringsWatcher
}

// ActionReceiver describes Entities that can have Actions queued for
// them, and that can get ActionRelated information about those actions.
// TODO(jcw4) consider implementing separate Actor classes for this
// interface; for example UnitActor that implements this interface, and
// takes a Unit and performs all these actions.
type ActionReceiver interface {
	Entity

	// PrepareActionPayload returns the payload to use in creating an action for this receiver.
	PrepareActionPayload(name string, payload map[string]interface{}, parallel *bool, executionGroup *string) (map[string]interface{}, bool, string, error)

	// CancelAction removes a pending Action from the queue for this
	// ActionReceiver and marks it as cancelled.
	CancelAction(action Action) (Action, error)

	// WatchActionNotifications returns a StringsWatcher that will notify
	// on changes to the queued actions for this ActionReceiver.
	WatchActionNotifications() StringsWatcher

	// WatchPendingActionNotifications returns a StringsWatcher that will notify
	// on pending queued actions for this ActionReceiver.
	WatchPendingActionNotifications() StringsWatcher

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

	// Parallel returns true if the action can run without
	// needed to acquire the machine lock.
	Parallel() bool

	// ExecutionGroup is the group of actions which cannot
	// execute in parallel with each other.
	ExecutionGroup() string

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

	// Log adds message to the action's progress message array.
	Log(message string) error

	// Messages returns the action's progress messages.
	Messages() []ActionMessage

	// Cancel or Abort the action.
	Cancel() (Action, error)

	// Refresh the contents of the action.
	Refresh() error
}

// ApplicationEntity represents a local or remote application.
type ApplicationEntity interface {
	// Life returns the life status of the application.
	Life() Life
}
