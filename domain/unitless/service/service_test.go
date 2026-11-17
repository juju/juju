// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/unitless"
	unitlessinternal "github.com/juju/juju/domain/unitless/internal"
	unitlessservice "github.com/juju/juju/domain/unitless/service"
)

type serviceSuite struct{}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestWatchScriptletApplications(c *tc.C) {
	w, err := unitlessservice.NewWatchableService(&stubState{}, &stubWatcherFactory{}).
		WatchScriptletApplications(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(<-w.Changes(), tc.HasLen, 0)
}

func (s *serviceSuite) TestGetScriptletApplication(c *tc.C) {
	st := &stubState{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	scriptlet, err := unitlessservice.NewService(st).GetScriptletApplication(
		c.Context(), applicationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scriptlet, tc.DeepEquals, unitless.ScriptletApplication{
		UUID: applicationUUID,
		Name: "foo",
		Life: corelife.Alive,
		Sources: []unitless.ScriptSource{{
			LoadPath: "hook.star",
			Source:   "def init(): pass",
		}},
	})
	c.Check(st.applicationUUID, tc.Equals, applicationUUID.String())
}

func (s *serviceSuite) TestGetScriptletApplicationInvalidApplicationUUID(c *tc.C) {
	st := &stubState{}
	_, err := unitlessservice.NewService(st).GetScriptletApplication(
		c.Context(), coreapplication.UUID("not-valid"),
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
	c.Check(st.applicationUUID, tc.Equals, "")
}

func (s *serviceSuite) TestWatchApplicationEvents(c *tc.C) {
	st := &stubState{hash: "hash-1"}
	factory := &stubWatcherFactory{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	w, err := unitlessservice.NewWatchableService(st, factory).WatchApplicationEvents(
		c.Context(), applicationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(st.applicationUUID, tc.Equals, applicationUUID.String())

	// The first config hash is always dispatched as a config changed event.
	initialEvents, err := factory.initialQuery(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(initialEvents, tc.DeepEquals, []string{unitless.EventConfigChanged.String()})

	changes := []changestream.ChangeEvent{changeEvent{changed: applicationUUID.String()}}

	// A config hash change emits a config changed event.
	st.hash = "hash-2"
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
	st := &stubState{hash: "hash-1"}
	factory := &stubWatcherFactory{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	w, err := unitlessservice.NewWatchableService(st, factory).WatchApplicationEvents(
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
	st.hash = ""
	changes := []changestream.ChangeEvent{changeEvent{changed: applicationUUID.String()}}
	events, err := factory.mapper(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.HasLen, 0)
}

func (s *serviceSuite) TestWatchApplicationEventsInvalidApplicationUUID(c *tc.C) {
	st := &stubState{}
	_, err := unitlessservice.NewWatchableService(st, &stubWatcherFactory{}).WatchApplicationEvents(
		c.Context(), coreapplication.UUID("not-valid"),
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
	c.Check(st.applicationUUID, tc.Equals, "")
}

func (s *serviceSuite) TestGetScriptletEvent(c *tc.C) {
	st := &stubState{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	event, err := unitlessservice.NewService(st).GetScriptletEvent(
		c.Context(), applicationUUID, "config-changed",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(event, tc.DeepEquals, unitless.Event{})
	c.Check(st.applicationUUID, tc.Equals, applicationUUID.String())
	c.Check(st.eventName, tc.Equals, "config-changed")
}

func (s *serviceSuite) TestGetScriptletEventInvalidApplicationUUID(c *tc.C) {
	st := &stubState{}
	_, err := unitlessservice.NewService(st).GetScriptletEvent(
		c.Context(), coreapplication.UUID("not-valid"), "config-changed",
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
	c.Check(st.applicationUUID, tc.Equals, "")
}

func (s *serviceSuite) TestGetScriptletEventEmptyEventType(c *tc.C) {
	st := &stubState{}
	_, err := unitlessservice.NewService(st).GetScriptletEvent(
		c.Context(), tc.Must(c, coreapplication.NewUUID), "",
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "empty event type not valid")
	c.Check(st.applicationUUID, tc.Equals, "")
}

type stubState struct {
	applicationUUID string
	eventName       string
	hash            string
}

func (s *stubState) InitialWatchStatementScriptletApplications() (string, eventsource.NamespaceQuery) {
	return "application", func(context.Context, database.TxnRunner) ([]string, error) {
		return nil, nil
	}
}

func (s *stubState) FilterScriptletApplications(_ context.Context, applicationUUIDs []string) ([]string, error) {
	return applicationUUIDs, nil
}

func (s *stubState) GetScriptletApplication(
	_ context.Context,
	applicationUUID string,
) (unitlessinternal.ScriptletApplication, error) {
	s.applicationUUID = applicationUUID
	return unitlessinternal.ScriptletApplication{
		UUID: applicationUUID,
		Name: "foo",
		Life: 0,
		Sources: []unitlessinternal.ScriptSource{{
			LoadPath: "hook.star",
			Source:   "def init(): pass",
		}},
	}, nil
}

func (s *stubState) GetScriptletEvent(
	_ context.Context,
	applicationUUID string,
	eventName string,
) (unitless.Event, error) {
	s.applicationUUID = applicationUUID
	s.eventName = eventName
	return unitless.Event{}, nil
}

func (s *stubState) InitialWatchStatementApplicationConfigHash(applicationUUID string) (string, eventsource.NamespaceQuery) {
	s.applicationUUID = applicationUUID
	return "application_config_hash", func(context.Context, database.TxnRunner) ([]string, error) {
		return []string{s.hash}, nil
	}
}

func (s *stubState) GetApplicationConfigHash(_ context.Context, applicationUUID string) (string, error) {
	s.applicationUUID = applicationUUID
	return s.hash, nil
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
