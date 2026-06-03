// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	st             *MockState
	watcherFactory *MockWatcherFactory
	notifyWatcher  *MockNotifyWatcher
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSetLokiEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetLokiEndpoint(gomock.Any(), gomock.Any(), "http://loki:3100/loki/api/v1/push").Return(nil)

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiEndpoint(c.Context(), "http://loki:3100/loki/api/v1/push")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetLokiEndpointEmptyReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiEndpoint(c.Context(), "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetLokiEndpointInvalidURLReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiEndpoint(c.Context(), "not-a-valid-url")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetLokiEndpointStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetLokiEndpoint(gomock.Any(), gomock.Any(), "http://loki:3100/loki/api/v1/push").Return(
		errors.Errorf("boom"),
	)

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiEndpoint(c.Context(), "http://loki:3100/loki/api/v1/push")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetLokiEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiEndpoint(gomock.Any()).Return("http://loki:3100/loki/api/v1/push", nil)

	endpoint, err := NewWatchableService(s.st, s.watcherFactory).GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
}

func (s *serviceSuite) TestGetLokiEndpointNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiEndpoint(gomock.Any()).Return("", loggingerrors.LokiEndpointNotFound)

	_, err := NewWatchableService(s.st, s.watcherFactory).GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiEndpointNotFound)
}

func (s *serviceSuite) TestGetLokiEndpointStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiEndpoint(gomock.Any()).Return("", errors.Errorf("boom"))

	_, err := NewWatchableService(s.st, s.watcherFactory).GetLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestDeleteLokiEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteLokiEndpoint(gomock.Any()).Return(nil)

	err := NewWatchableService(s.st, s.watcherFactory).DeleteLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteLokiEndpointStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteLokiEndpoint(gomock.Any()).Return(errors.Errorf("boom"))

	err := NewWatchableService(s.st, s.watcherFactory).DeleteLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestWatchLokiEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().NamespaceForWatchLokiEndpoint().Return("logging_loki_config")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(s.notifyWatcher, nil)

	w, err := NewWatchableService(s.st, s.watcherFactory).WatchLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Not(tc.IsNil))
}

func (s *serviceSuite) TestWatchLokiEndpointWatcherFactoryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().NamespaceForWatchLokiEndpoint().Return("logging_loki_config")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, errors.Errorf("watcher boom"))

	_, err := NewWatchableService(s.st, s.watcherFactory).WatchLokiEndpoint(c.Context())
	c.Assert(err, tc.ErrorMatches, "watcher boom")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.notifyWatcher = NewMockNotifyWatcher(ctrl)

	c.Cleanup(func() {
		s.st = nil
		s.watcherFactory = nil
		s.notifyWatcher = nil
	})

	return ctrl
}
