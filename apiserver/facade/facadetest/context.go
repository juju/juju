// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
)

// ModelContext implements facade.ModelContext in the simplest possible way.
type ModelContext struct {
	Auth_                facade.Authorizer
	Dispose_             func()
	Hub_                 facade.Hub
	Resources_           facade.Resources
	WatcherRegistry_     facade.WatcherRegistry
	State_               *state.State
	StatePool_           *state.StatePool
	MultiwatcherFactory_ multiwatcher.Factory
	ID_                  string
	RequestRecorder_     facade.RequestRecorder

	LeadershipClaimer_     leadership.Claimer
	LeadershipRevoker_     leadership.Revoker
	LeadershipChecker_     leadership.Checker
	LeadershipPinner_      leadership.Pinner
	LeadershipReader_      leadership.Reader
	SingularClaimer_       lease.Claimer
	CharmhubHTTPClient_    facade.HTTPClient
	ServiceFactory_        servicefactory.ServiceFactory
	ServiceFactoryGetter_  servicefactory.ServiceFactoryGetter
	ModelExporter_         facade.ModelExporter
	ModelImporter_         facade.ModelImporter
	ObjectStore_           objectstore.ObjectStore
	ControllerObjectStore_ objectstore.ObjectStore
	Logger_                loggo.Logger

	MachineTag_ names.Tag
	DataDir_    string
	LogDir_     string

	// Identity is not part of the facade.ModelContext interface, but is instead
	// used to make sure that the context objects are the same.
	Identity string
}

// Auth is part of the facade.ModelContext interface.
func (c ModelContext) Auth() facade.Authorizer {
	return c.Auth_
}

// Dispose is part of the facade.ModelContext interface.
func (c ModelContext) Dispose() {
	c.Dispose_()
}

// Hub is part of the facade.ModelContext interface.
func (c ModelContext) Hub() facade.Hub {
	return c.Hub_
}

// MultiwatcherFactory is part of the facade.ModelContext interface.
func (c ModelContext) MultiwatcherFactory() multiwatcher.Factory {
	return c.MultiwatcherFactory_
}

// Resources is part of the facade.ModelContext interface.
// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
func (c ModelContext) Resources() facade.Resources {
	return c.Resources_
}

// WatcherRegistry returns the watcher registry for this c. The
// watchers are per-connection, and are cleaned up when the connection
// is closed.
func (c ModelContext) WatcherRegistry() facade.WatcherRegistry {
	return c.WatcherRegistry_
}

// ObjectStore is part of the facade.ModelContext interface.
// It returns the object store for this c.
func (c ModelContext) ObjectStore() objectstore.ObjectStore {
	return c.ObjectStore_
}

// ControllerObjectStore is part of the facade.ModelContext interface.
// It returns the object store for this c.
func (c ModelContext) ControllerObjectStore() objectstore.ObjectStore {
	return c.ControllerObjectStore_
}

// State is part of the facade.ModelContext interface.
func (c ModelContext) State() *state.State {
	return c.State_
}

// StatePool is part of the facade.ModelContext interface.
func (c ModelContext) StatePool() *state.StatePool {
	return c.StatePool_
}

// ID is part of the facade.ModelContext interface.
func (c ModelContext) ID() string {
	return c.ID_
}

// RequestRecorder defines a metrics collector for outbound requests.
func (c ModelContext) RequestRecorder() facade.RequestRecorder {
	return c.RequestRecorder_
}

// Presence implements facade.ModelContext.
func (c ModelContext) Presence() facade.Presence {
	return c
}

// ModelPresence implements facade.Presence.
func (c ModelContext) ModelPresence(modelUUID string) facade.ModelPresence {
	// Potentially may need to add stuff here at some stage.
	return nil
}

// LeadershipClaimer implements facade.ModelContext.
func (c ModelContext) LeadershipClaimer() (leadership.Claimer, error) {
	return c.LeadershipClaimer_, nil
}

// LeadershipRevoker implements facade.ModelContext.
func (c ModelContext) LeadershipRevoker() (leadership.Revoker, error) {
	return c.LeadershipRevoker_, nil
}

// LeadershipPinner implements facade.ModelContext.
func (c ModelContext) LeadershipPinner() (leadership.Pinner, error) {
	return c.LeadershipPinner_, nil
}

// LeadershipReader implements facade.ModelContext.
func (c ModelContext) LeadershipReader() (leadership.Reader, error) {
	return c.LeadershipReader_, nil
}

// LeadershipChecker implements facade.ModelContext.
func (c ModelContext) LeadershipChecker() (leadership.Checker, error) {
	return c.LeadershipChecker_, nil
}

// SingularClaimer implements facade.ModelContext.
func (c ModelContext) SingularClaimer() (lease.Claimer, error) {
	return c.SingularClaimer_, nil
}

// HTTPClient implements facade.ModelContext.
func (c ModelContext) HTTPClient(purpose facade.HTTPClientPurpose) facade.HTTPClient {
	switch purpose {
	case facade.CharmhubHTTPClient:
		return c.CharmhubHTTPClient_
	default:
		return nil
	}
}

// ServiceFactory implements facade.ModelContext.
func (c ModelContext) ServiceFactory() servicefactory.ServiceFactory {
	return c.ServiceFactory_
}

// ModelExporter returns a model exporter for the current model.
func (c ModelContext) ModelExporter(facade.LegacyStateExporter) facade.ModelExporter {
	return c.ModelExporter_
}

// ModelImporter returns a model importer.
func (c ModelContext) ModelImporter() facade.ModelImporter {
	return c.ModelImporter_
}

// MachineTag returns the current machine tag.
func (c ModelContext) MachineTag() names.Tag {
	return c.MachineTag_
}

// DataDir returns the data directory.
func (c ModelContext) DataDir() string {
	return c.DataDir_
}

// LogDir returns the log directory.
func (c ModelContext) LogDir() string {
	return c.LogDir_
}

func (c ModelContext) Logger() loggo.Logger {
	return c.Logger_
}

type noopLogger struct{}

func (noopLogger) Log([]logger.LogRecord) error { return nil }

func (noopLogger) Close() error { return nil }
func (c ModelContext) ModelLogger(modelUUID, modelName, modelOwner string) (logger.LoggerCloser, error) {
	return noopLogger{}, nil
}
