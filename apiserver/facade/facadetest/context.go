// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state"
)

// Context implements facade.Context in the simplest possible way.
type Context struct {
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
	Cancel_              <-chan struct{}

	LeadershipClaimer_  leadership.Claimer
	LeadershipRevoker_  leadership.Revoker
	LeadershipChecker_  leadership.Checker
	LeadershipPinner_   leadership.Pinner
	LeadershipReader_   leadership.Reader
	SingularClaimer_    lease.Claimer
	CharmhubHTTPClient_ facade.HTTPClient
	ServiceFactory_     facade.APIServerServiceFactory
	ControllerDB_       changestream.WatchableDB
	Logger_             loggo.Logger

	MachineTag_ names.Tag
	DataDir_    string
	LogDir_     string

	// Identity is not part of the facade.Context interface, but is instead
	// used to make sure that the context objects are the same.
	Identity string
}

// Cancel is part of the facade.Context interface.
func (context Context) Cancel() <-chan struct{} {
	return context.Cancel_
}

// Auth is part of the facade.Context interface.
func (context Context) Auth() facade.Authorizer {
	return context.Auth_
}

// Dispose is part of the facade.Context interface.
func (context Context) Dispose() {
	context.Dispose_()
}

// Hub is part of the facade.Context interface.
func (context Context) Hub() facade.Hub {
	return context.Hub_
}

// MultiwatcherFactory is part of the facade.Context interface.
func (context Context) MultiwatcherFactory() multiwatcher.Factory {
	return context.MultiwatcherFactory_
}

// Resources is part of the facade.Context interface.
// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
func (context Context) Resources() facade.Resources {
	return context.Resources_
}

// WatcherRegistry returns the watcher registry for this context. The
// watchers are per-connection, and are cleaned up when the connection
// is closed.
func (context Context) WatcherRegistry() facade.WatcherRegistry {
	return context.WatcherRegistry_
}

// State is part of the facade.Context interface.
func (context Context) State() *state.State {
	return context.State_
}

// StatePool is part of the facade.Context interface.
func (context Context) StatePool() *state.StatePool {
	return context.StatePool_
}

// ID is part of the facade.Context interface.
func (context Context) ID() string {
	return context.ID_
}

// RequestRecorder defines a metrics collector for outbound requests.
func (context Context) RequestRecorder() facade.RequestRecorder {
	return context.RequestRecorder_
}

// Presence implements facade.Context.
func (context Context) Presence() facade.Presence {
	return context
}

// ModelPresence implements facade.Presence.
func (context Context) ModelPresence(modelUUID string) facade.ModelPresence {
	// Potentially may need to add stuff here at some stage.
	return nil
}

// LeadershipClaimer implements facade.Context.
func (context Context) LeadershipClaimer(modelUUID string) (leadership.Claimer, error) {
	return context.LeadershipClaimer_, nil
}

// LeadershipRevoker implements facade.Context.
func (context Context) LeadershipRevoker(modelUUID string) (leadership.Revoker, error) {
	return context.LeadershipRevoker_, nil
}

// LeadershipChecker implements facade.Context.
func (context Context) LeadershipChecker() (leadership.Checker, error) {
	return context.LeadershipChecker_, nil
}

// LeadershipPinner implements facade.Context.
func (context Context) LeadershipPinner(modelUUID string) (leadership.Pinner, error) {
	return context.LeadershipPinner_, nil
}

// LeadershipReader implements facade.Context.
func (context Context) LeadershipReader(modelUUID string) (leadership.Reader, error) {
	return context.LeadershipReader_, nil
}

// SingularClaimer implements facade.Context.
func (context Context) SingularClaimer() (lease.Claimer, error) {
	return context.SingularClaimer_, nil
}

func (context Context) HTTPClient(purpose facade.HTTPClientPurpose) facade.HTTPClient {
	switch purpose {
	case facade.CharmhubHTTPClient:
		return context.CharmhubHTTPClient_
	default:
		return nil
	}
}

func (context Context) ServiceFactory() facade.APIServerServiceFactory {
	return context.ServiceFactory_
}

func (context Context) ControllerDB() (changestream.WatchableDB, error) {
	return context.ControllerDB_, nil
}

// MachineTag returns the current machine tag.
func (context Context) MachineTag() names.Tag {
	return context.MachineTag_
}

// DataDir returns the data directory.
func (context Context) DataDir() string {
	return context.DataDir_
}

// LogDir returns the log directory.
func (context Context) LogDir() string {
	return context.LogDir_
}

func (context Context) Logger() loggo.Logger {
	return context.Logger_
}
