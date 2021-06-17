// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"github.com/juju/clock"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/cache"
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
	State_               *state.State
	StatePool_           *state.StatePool
	Controller_          *cache.Controller
	MultiwatcherFactory_ multiwatcher.Factory
	ID_                  string
	RequestRecorder_     facade.RequestRecorder
	Cancel_              <-chan struct{}

	LeadershipClaimer_ leadership.Claimer
	LeadershipRevoker_ leadership.Revoker
	LeadershipChecker_ leadership.Checker
	LeadershipPinner_  leadership.Pinner
	LeadershipReader_  leadership.Reader
	SingularClaimer_   lease.Claimer
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

// Controller is part of the facade.Context interface.
func (context Context) Controller() *cache.Controller {
	return context.Controller_
}

// MultiwatcherFactory is part of the facade.Context interface.
func (context Context) MultiwatcherFactory() multiwatcher.Factory {
	return context.MultiwatcherFactory_
}

// CachedModel is part of the facade.Context interface.
func (context Context) CachedModel(uuid string) (*cache.Model, error) {
	return context.Controller_.WaitForModel(uuid, clock.WallClock)
}

// Resources is part of the facade.Context interface.
func (context Context) Resources() facade.Resources {
	return context.Resources_
}

// State is part of the facade.Context interface.
func (context Context) State() *state.State {
	return context.State_
}

// StatePool is part of of the facade.Context interface.
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

// LeadershipPinner implements facade.Context.
func (context Context) LeadershipReader(modelUUID string) (leadership.Reader, error) {
	return context.LeadershipReader_, nil
}

// SingularClaimer implements facade.Context.
func (context Context) SingularClaimer() (lease.Claimer, error) {
	return context.SingularClaimer_, nil
}
