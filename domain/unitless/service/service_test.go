// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/unitless"
	unitlessservice "github.com/juju/juju/domain/unitless/service"
)

type serviceSuite struct{}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestWatchScriptletApplications(c *tc.C) {
	w, err := unitlessservice.NewWatchableService(&stubState{}).WatchScriptletApplications(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(<-w.Changes(), tc.HasLen, 0)
}

func (s *serviceSuite) TestGetApplicationScriptlet(c *tc.C) {
	st := &stubState{}
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	scriptlet, err := unitlessservice.NewService(st).GetApplicationScriptlet(
		c.Context(), applicationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scriptlet, tc.DeepEquals, unitless.Scriptlet{})
	c.Check(st.applicationUUID, tc.Equals, applicationUUID.String())
}

func (s *serviceSuite) TestGetApplicationScriptletInvalidApplicationUUID(c *tc.C) {
	st := &stubState{}
	_, err := unitlessservice.NewService(st).GetApplicationScriptlet(
		c.Context(), coreapplication.UUID("not-valid"),
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `application UUID: uuid "not-valid" not valid`)
	c.Check(st.applicationUUID, tc.Equals, "")
}

func (s *serviceSuite) TestWatchApplicationEvents(c *tc.C) {
	w, err := unitlessservice.NewWatchableService(&stubState{}).WatchApplicationEvents(
		c.Context(), tc.Must(c, coreapplication.NewUUID),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(<-w.Changes(), tc.HasLen, 0)
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

func (s *serviceSuite) TestGetScriptletEventEmptyEventName(c *tc.C) {
	st := &stubState{}
	_, err := unitlessservice.NewService(st).GetScriptletEvent(
		c.Context(), tc.Must(c, coreapplication.NewUUID), "",
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "empty event name not valid")
	c.Check(st.applicationUUID, tc.Equals, "")
}

type stubState struct {
	applicationUUID string
	eventName       string
}

func (s *stubState) GetApplicationScriptlet(
	_ context.Context,
	applicationUUID string,
) (unitless.Scriptlet, error) {
	s.applicationUUID = applicationUUID
	return unitless.Scriptlet{}, nil
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
