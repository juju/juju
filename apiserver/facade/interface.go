// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/names/v6"

	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/state"
)

// Facade could be anything; it will be interpreted by the apiserver
// machinery such that certain exported methods will be made available
// as facade methods to connected clients.
type Facade interface{}

// Factory is a callback used to create a Facade.
type Factory func(stdCtx context.Context, modelCtx ModelContext) (Facade, error)

// MultiModelFactory is a callback used to create a Facade.
type MultiModelFactory func(stdCtx context.Context, modelCtx MultiModelContext) (Facade, error)

// LeadershipModelContext
type LeadershipModelContext interface {
	// LeadershipClaimer returns a leadership.Claimer for this
	// context's model.
	LeadershipClaimer() (leadership.Claimer, error)

	// LeadershipRevoker returns a leadership.Revoker for this
	// context's model.
	LeadershipRevoker() (leadership.Revoker, error)

	// LeadershipPinner returns a leadership.Pinner for this
	// context's model.
	LeadershipPinner() (leadership.Pinner, error)

	// LeadershipReader returns a leadership.Reader for this
	// context's model.
	LeadershipReader() (leadership.Reader, error)

	// LeadershipChecker returns a leadership.Checker for this
	// context's model.
	LeadershipChecker() (leadership.Checker, error)

	// SingularClaimer returns a lease.Claimer for singular leases for
	// this context's model.
	SingularClaimer() (lease.Claimer, error)
}

// MultiModelContext is a context that can operate on multiple models at once.
type MultiModelContext interface {
	ModelContext

	// DomainServicesForModel returns the services factory for a given model
	// uuid.
	DomainServicesForModel(context.Context, model.UUID) (services.DomainServices, error)

	// ObjectStoreForModel returns the object store for a given model uuid.
	ObjectStoreForModel(ctx context.Context, modelUUID string) (objectstore.ObjectStore, error)
}

// ModelContext exposes useful capabilities to a Facade for a given model.
type ModelContext interface {
	// TODO (stickupkid): This shouldn't be embedded, instead this should be
	// in the form of `context.Leadership() Leadership`, which returns the
	// contents of the LeadershipContext.
	// Context should have a single responsibility, and that's access to other
	// types/objects.
	LeadershipModelContext
	ModelMigrationFactory
	DomainServices
	ObjectStoreFactory
	Logger

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
	// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
	Resources() Resources

	// WatcherRegistry returns the watcher registry for this context. The
	// watchers are per-connection, and are cleaned up when the connection
	// is closed.
	WatcherRegistry() WatcherRegistry

	// State returns, /sigh, a *State. As yet, there is no way
	// around this; in the not-too-distant future, we hope, its
	// capabilities will migrate towards access via Resources.
	State() *state.State

	// StatePool returns the state pool used by the apiserver to minimise the
	// creation of the expensive *State instances.
	StatePool() *state.StatePool

	// ID returns a string that should almost always be "", unless
	// this is a watcher facade, in which case it exists in lieu of
	// actual arguments in the Next() call, and is used as a key
	// into Resources to get the watcher in play. This is not really
	// a good idea; see Resources.
	ID() string

	// ControllerUUID returns the controller's unique identifier.
	ControllerUUID() string

	// ModelUUID returns the model's unique identifier. All facade requests
	// are in the scope of a model. There are some exceptions to the rule, but
	// they are exceptions that prove the rule.
	ModelUUID() model.UUID

	// RequestRecorder defines a metrics collector for outbound requests.
	RequestRecorder() RequestRecorder

	// HTTPClient returns an HTTP client to use for the given purpose. The
	// following errors can be expected:
	// - [ErrorHTTPClientPurposeInvalid] when the requested purpose is not
	// understood by the context.
	// - [ErrorHTTPClientForPurposeNotFound] when no http client can be found
	// for the requested [HTTPClientPurpose].
	HTTPClient(corehttp.Purpose) (HTTPClient, error)

	// MachineTag returns the current machine tag.
	MachineTag() names.Tag

	// DataDir returns the data directory.
	DataDir() string

	// LogDir returns the log directory.
	LogDir() string

	// Clock returns a instance of the clock.
	Clock() clock.Clock
}

// ModelExporter defines a interface for exporting models.
type ModelExporter interface {
	// ExportModel exports the current model into a description model. This
	// can be serialized into yaml and then imported.
	ExportModel(context.Context, objectstore.ObjectStore) (description.Model, error)
	// ExportModelPartial exports the current model into a partial description
	// model. This can be serialized into yaml and then imported.
	ExportModelPartial(context.Context, state.ExportConfig, objectstore.ObjectStore) (description.Model, error)
}

// LegacyStateExporter describes interface on state required to export a
// model.
// Deprecated: This is being replaced with the ModelExporter.
type LegacyStateExporter interface {
	// Export generates an abstract representation of a model.
	Export(objectstore.ObjectStore) (description.Model, error)
	// ExportPartial produces a partial export based based on the input
	// config.
	ExportPartial(state.ExportConfig, objectstore.ObjectStore) (description.Model, error)
}

// ModelImporter defines an interface for importing models.
type ModelImporter interface {
	// ImportModel takes a serialized description model (yaml bytes) and returns
	// a state model and state state.
	ImportModel(ctx context.Context, bytes []byte) (*state.Model, *state.State, error)
}

// ModelMigrationFactory defines an interface for getting a model migrator.
type ModelMigrationFactory interface {
	// ModelExporter returns a model exporter for the current model.
	ModelExporter(context.Context, model.UUID, LegacyStateExporter) (ModelExporter, error)

	// ModelImporter returns a model importer.
	ModelImporter() ModelImporter
}

// DomainServices defines an interface for accessing all the services.
type DomainServices interface {
	// DomainServices returns the services factory for the current model.
	DomainServices() services.DomainServices
}

// ObjectStoreFactory defines an interface for accessing the object store.
type ObjectStoreFactory interface {
	// ObjectStore returns the object store for the current model.
	ObjectStore() objectstore.ObjectStore

	// ControllerObjectStore returns the object store for the controller.
	ControllerObjectStore() objectstore.ObjectStore
}

// Logger defines an interface for getting the apiserver logger instance.
type Logger interface {
	// Logger returns the apiserver logger instance.
	Logger() corelogger.Logger
}

// RequestRecorder is implemented by types that can record information about
// successful and unsuccessful http requests.
type RequestRecorder interface {
	// Record an outgoing request that produced a http.Response.
	Record(method string, url *url.URL, res *http.Response, rtt time.Duration)

	// RecordError records an outgoing request that returned back an error.
	RecordError(method string, url *url.URL, err error)
}

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
	HasPermission(ctx context.Context, operation permission.Access, target names.Tag) error

	// EntityHasPermission reports whether the given access is allowed for the given
	// target by the given entity.
	EntityHasPermission(ctx context.Context, entity names.Tag, operation permission.Access, target names.Tag) error
}

// Hub represents the central hub that the API server has.
type Hub interface {
	Publish(topic string, data interface{}) (func(), error)
}

// HTTPClient represents an HTTP client, for example, an *http.Client.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
