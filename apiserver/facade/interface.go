// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/names/v4"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
)

// Facade could be anything; it will be interpreted by the apiserver
// machinery such that certain exported methods will be made available
// as facade methods to connected clients.
type Facade interface{}

// Factory is a callback used to create a Facade.
type Factory func(Context) (Facade, error)

// LeadershipContext describes factory methods for objects that deliver
// specific lease-related capabilities
type LeadershipContext interface {

	// LeadershipClaimer returns a leadership.Claimer tied to a
	// specific model.
	LeadershipClaimer(modelUUID string) (leadership.Claimer, error)

	// LeadershipRevoker returns a leadership.Revoker tied to a
	// specific model.
	LeadershipRevoker(modelUUID string) (leadership.Revoker, error)

	// LeadershipChecker returns a leadership.Checker for this
	// context's model.
	LeadershipChecker() (leadership.Checker, error)

	// LeadershipPinner returns a leadership.Pinner for this
	// context's model.
	LeadershipPinner(modelUUID string) (leadership.Pinner, error)

	// LeadershipReader returns a leadership.Reader for this
	// context's model.
	LeadershipReader(modelUUID string) (leadership.Reader, error)

	// SingularClaimer returns a lease.Claimer for singular leases for
	// this context's model.
	SingularClaimer() (lease.Claimer, error)
}

// RaftContext describes methods for handling raft related capabilities.
type RaftContext interface {

	// ApplyLease attempts to apply the command on to the raft FSM. It only
	// takes a command and enqueues that against the raft instance. If the raft
	// instance is already processing a application, then back pressure is
	// applied to the caller and a ErrEnqueueDeadlineExceeded will be sent.
	// It's up to the caller to retry or drop depending on how the retry
	// algorithm is implemented.
	ApplyLease(context.Context, raftlease.Command) error
}

// Context exposes useful capabilities to a Facade.
type Context interface {
	// TODO (stickupkid): This shouldn't be embedded, instead this should be
	// in the form of `context.Leadership() Leadership`, which returns the
	// contents of the LeadershipContext.
	// Context should have a single responsibility, and that's access to other
	// types/objects.
	LeadershipContext

	// Cancel channel represents an indication from the API server that
	// all interruptable calls should stop. The channel is only ever
	// closed, and never sents values.
	Cancel() <-chan struct{}

	// Auth represents information about the connected client. You
	// should always be checking individual requests against Auth:
	// both state changes *and* data retrieval should be blocked
	// with apiservererrors.ErrPerm for any targets for which the client is
	// not *known* to have a responsibility or requirement.
	Auth() Authorizer

	// Dispose disposes the context and any resources related to
	// the API server facade object. Normally the context will not
	// be disposed until the API connection is closed. This is OK
	// except when contexts are dynamically generated, such as in
	// the case of watchers. When a facade context is no longer
	// needed, e.g. when a watcher is closed, then the context may
	// be disposed by calling this method.
	Dispose()

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

	// StatePool returns the state pool used by the apiserver to minimise the
	// creation of the expensive *State instances.
	StatePool() *state.StatePool

	// MultiwatcherFactory returns the factory to create multiwatchers.
	MultiwatcherFactory() multiwatcher.Factory

	// Controller returns the in-memory representation of the models
	// in the database.
	Controller() *cache.Controller

	// CachedModel returns the in-memory representation of the specified
	// model. This call will wait for the model to appear in the cache.
	// The method optimistically expects the model to exist in the cache
	// or appear very soon. If the model doesn't appear, the database is
	// checked. A NotFound error is returned if the model no longer exists
	// in the database, or a Timeout error is returned if the model didn't
	// appear, but should have.
	CachedModel(uuid string) (*cache.Model, error)

	// Presence returns an instance that is able to be asked for
	// the current model presence.
	Presence() Presence

	// Hub returns the central hub that the API server holds.
	// At least at this stage, facades only need to publish events.
	Hub() Hub

	// ID returns a string that should almost always be "", unless
	// this is a watcher facade, in which case it exists in lieu of
	// actual arguments in the Next() call, and is used as a key
	// into Resources to get the watcher in play. This is not really
	// a good idea; see Resources.
	ID() string

	// RequestRecorder defines a metrics collector for outbound requests.
	RequestRecorder() RequestRecorder

	// Raft returns a lease context for managing raft.
	Raft() RaftContext
}

// RequestRecorder is implemented by types that can record information about
// successful and unsuccessful http requests.
type RequestRecorder interface {
	// Record an outgoing request which produced an http.Response.
	Record(method string, url *url.URL, res *http.Response, rtt time.Duration)

	// Record an outgoing request which returned back an error.
	RecordError(method string, url *url.URL, err error)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Resources,Authorizer

// Authorizer represents the authenticated entity using the API server.
type Authorizer interface {

	// GetAuthTag returns the entity's tag.
	GetAuthTag() names.Tag

	// AuthController returns whether the authenticated entity is
	// a machine acting as a controller. Can't be removed from this
	// interface without introducing a dependency on something else
	// to look up that property: it's not inherent in the result of
	// GetAuthTag, as the other methods all are.
	AuthController() bool

	// TODO(wallyworld - bug 1733759) - the following auth methods should not be on this interface
	// eg introduce a utility func or something.

	// AuthMachineAgent returns true if the entity is a machine agent.
	AuthMachineAgent() bool

	// AuthApplicationAgent returns true if the entity is an application operator.
	AuthApplicationAgent() bool

	// AuthModelAgent returns true if the entity is a model operator.
	AuthModelAgent() bool

	// AuthUnitAgent returns true if the entity is a unit agent.
	AuthUnitAgent() bool

	// AuthOwner returns true if tag == .GetAuthTag().
	AuthOwner(tag names.Tag) bool

	// AuthClient returns true if the entity is an external user.
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

// Presence represents the current known state of API connections from agents
// to any of the API servers.
type Presence interface {
	ModelPresence(modelUUID string) ModelPresence
}

// ModelPresence represents the API server connections for a model.
type ModelPresence interface {
	// For a given non controller agent, return the Status for that agent.
	AgentStatus(agent string) (presence.Status, error)
}

// Hub represents the central hub that the API server has.
type Hub interface {
	Publish(topic string, data interface{}) (func(), error)
}
