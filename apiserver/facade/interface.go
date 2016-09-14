// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// Facade could be anything; it will be interpreted by the apiserver
// machinery such that certain exported methods will be made available
// as facade methods to connected clients.
type Facade interface{}

// Factory is a callback used to create a Facade.
type Factory func(Context) (Facade, error)

// Context exposes useful capabilities to a Facade.
type Context interface {

	// Abort will be closed with the client connection. Any long-
	// running methods should pay attention to Abort, and terminate
	// with a sensible (non-nil) error when requested.
	Abort() <-chan struct{}

	// Auth represents information about the connected client. You
	// should always be checking individual requests against Auth:
	// both state changes *and* data retrieval should be blocked
	// with common.ErrPerm for any targets for which the client is
	// not *known* to have a responsibility or requirement.
	Auth() Authorizer

	// Resources exposes per-connection capabilities. By adding a
	// resource, you make it accessible by (returned) id to all
	// other facades used by this connection. It's mostly used to
	// pass watcher ids over to watcher-specific facades, but that
	// seems to be an antipattern: it breaks the separate-facades-
	// by-role advice, and makes it inconvenient to track a given
	// worker's watcher activity alongside its other communications.
	//
	// It's also used to hold some config strings used by various
	// consumers, because it's convenient; and the Pinger that
	// reports client presence in state, because every Resource gets
	// Stop()ped on conn close. Not all of these uses are
	// necessarily a great idea.
	Resources() Resources

	// State returns, /sigh, a *State. As yet, there is no way
	// around this; in the not-too-distant future, we hope, its
	// capabilities will migrate towards access via Resources.
	State() *state.State

	// ID returns a string that should almost always be "", unless
	// this is a watcher facade, in which case it exists in lieu of
	// actual arguments in the Next() call, and is used as a key
	// into Resources to get the watcher in play. This is not really
	// a good idea; see Resources.
	ID() string
}

// Authorizer represents the authenticated entity using the API server.
type Authorizer interface {

	// GetAuthTag returns the entity's tag.
	GetAuthTag() names.Tag

	// AuthModelManager returns whether the authenticated entity is
	// a machine running the environment manager job. Can't be
	// removed from this interface without introducing a dependency
	// on something else to look up that property: it's not inherent
	// in the result of GetAuthTag, as the other methods all are.
	AuthModelManager() bool

	// AuthMachineAgent returns true if the entity is a machine
	// agent. Doesn't need to be on this interface, should be a
	// utility func if anything.
	AuthMachineAgent() bool

	// AuthUnitAgent returns true if the entity is a unit agent.
	// Doesn't need to be on this interface, should be a utility
	// func if anything.
	AuthUnitAgent() bool

	// AuthOwner returns true if tag == .GetAuthTag(). Doesn't need
	// to be on this interface, should be a utility fun if anything.
	AuthOwner(tag names.Tag) bool

	// AuthClient returns true if the entity is an external user.
	// Doesn't need to be on this interface, should be a utility
	// func if anything.
	AuthClient() bool

	// HasPermission reports whether the given access is allowed for the given
	// target by the authenticated entity.
	HasPermission(operation permission.Access, target names.Tag) (bool, error)

	// UserHasPermission reports whether the given access is allowed for the given
	// target by the given user.
	UserHasPermission(user names.UserTag, operation permission.Access, target names.Tag) (bool, error)

	// ConnectedModel returns the UUID of the model to which the API
	// connection was made.
	ConnectedModel() string
}

// Resources allows you to store and retrieve Resource implementations.
//
// The lack of error returns are in deference to the existing
// implementation, not because they're a good idea.
type Resources interface {
	Register(Resource) string
	Get(string) Resource
	Stop(string) error
}

// Resource should almost certainly be worker.Worker: the current
// implementation renders the apiserver vulnerable to deadlock when
// shutting down. (See common.Resources.StopAll -- *that* should be a
// Kill() and a Wait(), so that connection cleanup can kill the
// resources early, along with everything else, and then just wait for
// all those things to finish.)
type Resource interface {
	Stop() error
}
