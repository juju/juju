// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	"github.com/juju/juju/core/watcher"
)

// ScriptletService supplies the worker with scriptlet application state.
type ScriptletService interface {
	// WatchScriptletApplications emits application UUIDs whose scriptlet
	// execution should be scheduled.
	WatchScriptletApplications(context.Context) (watcher.StringsWatcher, error)

	// GetApplicationScriptlet returns the Starform scriptlet sources for an
	// application.
	GetApplicationScriptlet(ctx context.Context, applicationUUID string) (Scriptlet, error)

	// WatchApplicationEvents emits event names that should be dispatched for
	// the application.
	WatchApplicationEvents(ctx context.Context, applicationUUID string) (watcher.StringsWatcher, error)

	// GetScriptletEvent returns an Event with attributes related to input event
	// name, relevant to the application with the input UUID.
	GetScriptletEvent(ctx context.Context, applicationUUID, eventName string) (Event, error)
}
