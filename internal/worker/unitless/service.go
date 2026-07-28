// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/watcher"
	domainunitless "github.com/juju/juju/domain/unitless"
)

// ScriptletService supplies the worker with scriptlet application state.
type ScriptletService interface {
	// WatchScriptletApplications emits application UUIDs whose scriptlet
	// execution should be scheduled.
	WatchScriptletApplications(context.Context) (watcher.StringsWatcher, error)

	// GetApplicationScriptlet returns the Starform scriptlet sources for an
	// application.
	GetApplicationScriptlet(ctx context.Context, applicationUUID coreapplication.UUID) (domainunitless.Scriptlet, error)

	// WatchApplicationEvents emits event names that should be dispatched for
	// the application.
	WatchApplicationEvents(ctx context.Context, applicationUUID coreapplication.UUID) (watcher.StringsWatcher, error)

	// GetScriptletEvent returns an Event with attributes related to input event
	// name, relevant to the application with the input UUID.
	GetScriptletEvent(ctx context.Context, applicationUUID coreapplication.UUID, eventName string) (domainunitless.Event, error)
}
