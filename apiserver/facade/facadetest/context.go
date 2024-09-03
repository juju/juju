// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
)

// ModelContext implements facade.ModelContext in the simplest possible way.
type ModelContext struct {
	Auth_            facade.Authorizer
	Dispose_         func()
	Hub_             facade.Hub
	Resources_       facade.Resources
	WatcherRegistry_ facade.WatcherRegistry
	State_           *state.State
	StatePool_       *state.StatePool
	ID_              string
	ControllerUUID_  string
	ModelUUID_       model.UUID
	RequestRecorder_ facade.RequestRecorder

	LeadershipClaimer_     leadership.Claimer
	LeadershipRevoker_     leadership.Revoker
	LeadershipChecker_     leadership.Checker
	LeadershipPinner_      leadership.Pinner
	LeadershipReader_      leadership.Reader
	SingularClaimer_       lease.Claimer
	CharmhubHTTPClient_    facade.HTTPClient
	SSHImporterHTTPClient_ facade.HTTPClient
	ServiceFactory_        servicefactory.ServiceFactory
	ServiceFactoryGetter_  servicefactory.ServiceFactoryGetter
	ModelExporter_         facade.ModelExporter
	ModelImporter_         facade.ModelImporter
	ObjectStore_           objectstore.ObjectStore
	ControllerObjectStore_ objectstore.ObjectStore
	Logger_                logger.Logger

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

// ControllerUUID returns the controller unique identifier.
func (c ModelContext) ControllerUUID() string {
	return c.ControllerUUID_
}

// ModelUUID returns the model unique identifier.
func (c ModelContext) ModelUUID() model.UUID {
	return c.ModelUUID_
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

// HTTPClient returns an HTTP client to use for the given purpose. The following
// errors can be expected:
// - [ErrorHTTPClientPurposeInvalid] when the requested purpose is not
// understood by the context.
// - [ErrorHTTPClientForPurposeNotFound] when no http client can be found for
// the requested [HTTPClientPurpose].
func (c ModelContext) HTTPClient(purpose facade.HTTPClientPurpose) (facade.HTTPClient, error) {
	var client facade.HTTPClient

	switch purpose {
	case facade.CharmhubHTTPClient:
		client = c.CharmhubHTTPClient_
	case facade.HTTPClientPurposeUserSSHImport:
		client = c.SSHImporterHTTPClient_
	default:
		return nil, fmt.Errorf(
			"cannot get http client for purpose %q, purpose is not understood by the facade context%w",
			purpose, errors.Hide(facade.ErrorHTTPClientPurposeInvalid),
		)
	}

	if client == nil {
		return nil, fmt.Errorf(
			"cannot get http client for purpose %q: http client not found%w",
			purpose, errors.Hide(facade.ErrorHTTPClientForPurposeNotFound),
		)
	}

	return client, nil
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

func (c ModelContext) Logger() logger.Logger {
	return c.Logger_
}
