// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/unitless"
	unitlessinternal "github.com/juju/juju/domain/unitless/internal"
	coretesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	coretesting.BaseSuite

	state *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestWatchScriptletApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InitialWatchStatementScriptletApplications().Return(
		"application", func(context.Context, database.TxnRunner) ([]string, error) {
			return nil, nil
		},
	)

	w, err := NewWatchableService(s.state, &stubWatcherFactory{}).
		WatchScriptletApplications(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(<-w.Changes(), tc.HasLen, 0)
}

func (s *serviceSuite) TestGetScriptletApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := tc.Must(c, corecharm.NewID)
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetScriptletApplication(gomock.Any(), applicationUUID.String()).Return(
		unitlessinternal.ScriptletApplication{
			UUID:      applicationUUID.String(),
			Name:      "foo",
			Life:      life.Alive,
			CharmUUID: charmUUID.String(),
			Sources: []unitlessinternal.ScriptSource{{
				LoadPath: "hook.star",
				Source:   "def init(): pass",
			}},
		}, nil,
	)

	scriptlet, err := NewService(s.state).GetScriptletApplication(
		c.Context(), applicationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scriptlet, tc.DeepEquals, unitless.ScriptletApplication{
		UUID:      applicationUUID,
		Name:      "foo",
		Life:      corelife.Alive,
		CharmUUID: charmUUID,
		Sources: []unitless.ScriptSource{{
			LoadPath: "hook.star",
			Source:   "def init(): pass",
		}},
	})
}

func (s *serviceSuite) TestGetScriptletApplicationInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetScriptletApplication(
		c.Context(), coreapplication.UUID("not-valid"),
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
}

func (s *serviceSuite) TestWatchApplicationEvents(c *tc.C) {
	defer s.setupMocks(c).Finish()

	factory := &stubWatcherFactory{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().InitialWatchStatementApplicationConfigHash(applicationUUID.String()).Return(
		"application_config_hash", func(context.Context, database.TxnRunner) ([]string, error) {
			return []string{"hash-1"}, nil
		},
	)
	s.state.EXPECT().GetApplicationConfigHash(gomock.Any(), applicationUUID.String()).Return("hash-2", nil).Times(2)

	w, err := NewWatchableService(s.state, factory).WatchApplicationEvents(
		c.Context(), applicationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// The first config hash is always dispatched as a config changed event.
	initialEvents, err := factory.initialQuery(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(initialEvents, tc.DeepEquals, []string{unitless.EventConfigChanged.String()})

	changes := []changestream.ChangeEvent{changeEvent{changed: applicationUUID.String()}}

	// A config hash change emits a config changed event.
	events, err := factory.mapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.DeepEquals, []string{unitless.EventConfigChanged.String()})

	// A change that leaves the application's config hash untouched emits no
	// events. This covers changes to another application's config hash.
	events, err = factory.mapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.HasLen, 0)
}

func (s *serviceSuite) TestWatchApplicationEventsConfigHashDeleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	factory := &stubWatcherFactory{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().InitialWatchStatementApplicationConfigHash(applicationUUID.String()).Return(
		"application_config_hash", func(context.Context, database.TxnRunner) ([]string, error) {
			return []string{"hash-1"}, nil
		},
	)
	s.state.EXPECT().GetApplicationConfigHash(gomock.Any(), applicationUUID.String()).Return("", nil)

	w, err := NewWatchableService(s.state, factory).WatchApplicationEvents(
		c.Context(), applicationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// The first config hash is always dispatched as a config changed event.
	_, err = factory.initialQuery(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	// When the application is destroyed, its config hash row is deleted and
	// the current hash is empty. No event is dispatched for the deletion;
	// the dying watcher stops the application without a final spurious
	// config changed event.
	changes := []changestream.ChangeEvent{changeEvent{changed: applicationUUID.String()}}
	events, err := factory.mapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.HasLen, 0)
}

func (s *serviceSuite) TestWatchApplicationEventsInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewWatchableService(s.state, &stubWatcherFactory{}).WatchApplicationEvents(
		c.Context(), coreapplication.UUID("not-valid"),
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
}

func (s *serviceSuite) TestGetScriptletEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetScriptletEvent(gomock.Any(), applicationUUID.String(), "config-changed").Return(unitless.Event{}, nil)

	event, err := NewService(s.state).GetScriptletEvent(
		c.Context(), applicationUUID, "config-changed",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(event, tc.DeepEquals, unitless.Event{})
}

func (s *serviceSuite) TestGetScriptletEventInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetScriptletEvent(
		c.Context(), coreapplication.UUID("not-valid"), "config-changed",
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
}

func (s *serviceSuite) TestGetScriptletEventEmptyEventType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state).GetScriptletEvent(
		c.Context(), tc.Must(c, coreapplication.NewUUID), "",
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "empty event type not valid")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	c.Cleanup(func() {
		s.state = nil
	})

	return ctrl
}

type changeEvent struct {
	changed string
}

func (e changeEvent) Changed() string               { return e.changed }
func (e changeEvent) Namespace() string             { return "application_config_hash" }
func (e changeEvent) Type() changestream.ChangeType { return changestream.Changed }

type stubWatcherFactory struct {
	initialQuery eventsource.NamespaceQuery
	summary      string
	mapper       eventsource.Mapper
}

func (f *stubWatcherFactory) NewNamespaceMapperWatcher(
	_ context.Context,
	initialQuery eventsource.NamespaceQuery,
	summary string,
	mapper eventsource.Mapper,
	_ eventsource.FilterOption,
	_ ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	f.initialQuery = initialQuery
	f.summary = summary
	f.mapper = mapper
	return watcher.TODO[[]string](), nil
}

func (f *stubWatcherFactory) NewNotifyWatcher(
	_ context.Context,
	_ string,
	_ eventsource.FilterOption,
	_ ...eventsource.FilterOption,
) (watcher.NotifyWatcher, error) {
	return watcher.TODO[struct{}](), nil
}
