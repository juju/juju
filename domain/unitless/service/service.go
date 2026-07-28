// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/unitless"
	"github.com/juju/juju/internal/errors"
)

// State describes the persistence operations required by the unitless service.
type State interface {
	// GetApplicationScriptlet returns the scriptlet associated with the
	// application identified by applicationUUID.
	GetApplicationScriptlet(
		ctx context.Context,
		applicationUUID string,
	) (unitless.Scriptlet, error)

	// GetScriptletEvent returns the named event for the application identified
	// by applicationUUID.
	GetScriptletEvent(
		ctx context.Context,
		applicationUUID string,
		eventName string,
	) (unitless.Event, error)
}

// Service provides access to unitless application scriptlets and events.
type Service struct {
	st State
}

// NewService returns a new unitless service.
func NewService(st State) *Service {
	return &Service{st: st}
}

// GetApplicationScriptlet returns the scriptlet associated with an
// application.
func (s *Service) GetApplicationScriptlet(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
) (unitless.Scriptlet, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return unitless.Scriptlet{}, errors.Errorf("application UUID: %w", err)
	}
	return s.st.GetApplicationScriptlet(ctx, applicationUUID.String())
}

// GetScriptletEvent returns the event payload for an application event.
func (s *Service) GetScriptletEvent(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
	eventName string,
) (unitless.Event, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return unitless.Event{}, errors.Errorf("application UUID: %w", err)
	}
	if eventName == "" {
		return unitless.Event{}, errors.New("empty event name not valid").Add(coreerrors.NotValid)
	}
	return s.st.GetScriptletEvent(ctx, applicationUUID.String(), eventName)
}

// WatchableService provides access to unitless application scriptlets and
// events, including watchers for changes.
type WatchableService struct {
	Service
}

// NewWatchableService returns a new watchable unitless service.
func NewWatchableService(st State) *WatchableService {
	return &WatchableService{
		Service: *NewService(st),
	}
}

// WatchScriptletApplications returns a watcher for applications that have a
// scriptlet. It currently reports an empty initial set.
func (s *WatchableService) WatchScriptletApplications(context.Context) (watcher.StringsWatcher, error) {
	return watcher.TODO[[]string](), nil
}

// WatchApplicationEvents returns a watcher for events associated with an
// application. It currently reports an empty initial set.
func (s *WatchableService) WatchApplicationEvents(
	context.Context,
	coreapplication.UUID,
) (watcher.StringsWatcher, error) {
	return watcher.TODO[[]string](), nil
}
