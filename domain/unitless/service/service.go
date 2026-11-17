// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/unitless"
	unitlessinternal "github.com/juju/juju/domain/unitless/internal"
	"github.com/juju/juju/internal/errors"
)

// State describes the persistence operations required by the unitless service.
type State interface {
	// GetScriptletApplication returns the scriptlet application associated with the
	// application identified by applicationUUID.
	GetScriptletApplication(
		ctx context.Context,
		applicationUUID string,
	) (unitlessinternal.ScriptletApplication, error)

	// GetScriptletEvent returns the named event for the application identified
	// by applicationUUID.
	GetScriptletEvent(
		ctx context.Context,
		applicationUUID string,
		eventName string,
	) (unitless.Event, error)

	// InitialWatchStatementScriptletApplications returns the initial query and
	// namespace used to watch applications backed by scriptlets.
	InitialWatchStatementScriptletApplications() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementApplicationConfigHash returns the initial query and
	// namespace used to watch an application's config hash.
	InitialWatchStatementApplicationConfigHash(applicationUUID string) (string, eventsource.NamespaceQuery)

	// GetApplicationConfigHash returns the SHA256 hash of the application
	// config for the application identified by applicationUUID. If the
	// application has no config hash, an empty string is returned.
	GetApplicationConfigHash(ctx context.Context, applicationUUID string) (string, error)

	// FilterScriptletApplications returns the input application UUIDs whose
	// charms have scriptlet sources.
	FilterScriptletApplications(ctx context.Context, applicationUUIDs []string) ([]string, error)
}

// WatcherFactory creates watchers over model database changes.
type WatcherFactory interface {
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service provides access to unitless application scriptlets and events.
type Service struct {
	st State
}

// NewService returns a new unitless service.
func NewService(st State) *Service {
	return &Service{st: st}
}

// GetScriptletApplication returns the scriptlet application associated with an
// application.
func (s *Service) GetScriptletApplication(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
) (unitless.ScriptletApplication, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return unitless.ScriptletApplication{}, errors.Errorf("application UUID: %w", err)
	}
	result, err := s.st.GetScriptletApplication(ctx, applicationUUID.String())
	if err != nil {
		return unitless.ScriptletApplication{}, errors.Capture(err)
	}
	if result.UUID == "" {
		return unitless.ScriptletApplication{}, nil
	}
	life, err := result.Life.Value()
	if err != nil {
		return unitless.ScriptletApplication{}, errors.Errorf("decoding application life: %w", err)
	}
	return unitless.ScriptletApplication{
		UUID: coreapplication.UUID(result.UUID),
		Name: result.Name,
		Life: life,
		Sources: transform.Slice(result.Sources, func(source unitlessinternal.ScriptSource) unitless.ScriptSource {
			return unitless.ScriptSource{
				LoadPath: source.LoadPath,
				Source:   source.Source,
			}
		}),
	}, nil
}

// GetScriptletEvent returns the event payload for an application event.
func (s *Service) GetScriptletEvent(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
	eventType unitless.EventType,
) (unitless.Event, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return unitless.Event{}, errors.Errorf("application UUID: %w", err)
	}
	if eventType == "" {
		return unitless.Event{}, errors.New("empty event type not valid").Add(coreerrors.NotValid)
	}
	return s.st.GetScriptletEvent(ctx, applicationUUID.String(), string(eventType))
}

// WatchableService provides access to unitless application scriptlets and
// events, including watchers for changes.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable unitless service.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service:        *NewService(st),
		watcherFactory: watcherFactory,
	}
}

// WatchScriptletApplications returns a watcher for applications that have a
// scriptlet.
func (s *WatchableService) WatchScriptletApplications(ctx context.Context) (watcher.StringsWatcher, error) {
	namespace, initialQuery := s.st.InitialWatchStatementScriptletApplications()
	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		applicationIDs := make([]string, len(changes))
		for i, change := range changes {
			applicationIDs[i] = change.Changed()
		}

		filtered, err := s.st.FilterScriptletApplications(ctx, applicationIDs)
		if err != nil {
			return nil, errors.Capture(err)
		}
		included := make(map[string]struct{}, len(filtered))
		for _, applicationID := range filtered {
			included[applicationID] = struct{}{}
		}

		result := make([]string, 0, len(filtered))
		for _, applicationID := range applicationIDs {
			if _, ok := included[applicationID]; ok {
				result = append(result, applicationID)
			}
		}
		return result, nil
	}
	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		"scriptlet applications watcher",
		mapper,
		eventsource.NamespaceFilter(namespace, changestream.Changed),
	)
}

// WatchScriptletApplicationDying returns a watcher that emits when the
// application changes life or is removed.
func (s *WatchableService) WatchScriptletApplicationDying(
	ctx context.Context, applicationUUID coreapplication.UUID,
) (watcher.NotifyWatcher, error) {
	if err := applicationUUID.Validate(); err != nil {
		return nil, errors.Errorf("application UUID: %w", err)
	}
	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"scriptlet application dying watcher",
		eventsource.PredicateFilter(
			"application",
			changestream.All,
			eventsource.EqualsPredicate(applicationUUID.String()),
		),
	)
}

// WatchApplicationEvents returns a watcher for events associated with an
// application. The watcher emits event names that should be dispatched for
// the application.
//
// The [unitless.EventConfigChanged] event is emitted when the application's
// config hash changes. The first config hash is always dispatched, so the
// application receives a config changed event for its current config when
// the watcher starts. Changes to another application's config hash are not
// reported.
func (s *WatchableService) WatchApplicationEvents(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return nil, errors.Errorf("application UUID: %w", err)
	}

	// sha256 is the current config hash for the application. It is seeded
	// by the initial query and updated as changes are dispatched.
	var sha256 string

	table, query := s.st.InitialWatchStatementApplicationConfigHash(applicationUUID.String())
	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
			initialResults, err := query(ctx, txn)
			if err != nil {
				return nil, errors.Capture(err)
			}
			if len(initialResults) == 1 {
				sha256 = initialResults[0]
			}

			// The first config hash is always dispatched, so the application
			// receives a config changed event for its current config.
			return []string{unitless.EventConfigChanged.String()}, nil
		},
		fmt.Sprintf("application events watcher for %q", applicationUUID),
		func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
			ctx, span := trace.Start(ctx, trace.NameFromFunc())
			defer span.End()

			// If there are no changes, return no changes.
			if len(changes) == 0 {
				return nil, nil
			}

			currentSHA256, err := s.st.GetApplicationConfigHash(ctx, applicationUUID.String())
			if err != nil {
				return nil, errors.Capture(err)
			}
			// If the hash hasn't changed, return no events. This includes
			// changes to another application's config hash, for which this
			// application's hash is unchanged.
			if currentSHA256 == sha256 {
				return nil, nil
			}
			sha256 = currentSHA256

			return []string{unitless.EventConfigChanged.String()}, nil
		},
		eventsource.NamespaceFilter(table, changestream.All),
	)
}
