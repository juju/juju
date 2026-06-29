// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/logging"
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

func (s *serviceSuite) TestSetLokiConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	}
	s.st.EXPECT().SetLokiConfig(gomock.Any(), gomock.Any(), config).Return(nil)

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetLokiConfigEmptyReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiConfig(c.Context(), logging.LokiConfig{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetLokiConfigInvalidURLReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiConfig(c.Context(), logging.LokiConfig{
		Endpoint: "not-a-valid-url",
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetLokiConfigInsecureSkipVerifyTrue(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boolVal := true
	config := logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: &boolVal,
	}
	s.st.EXPECT().SetLokiConfig(gomock.Any(), gomock.Any(), config).Return(nil)

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetLokiConfigInsecureSkipVerifyFalse(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boolVal := false
	config := logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: &boolVal,
	}
	s.st.EXPECT().SetLokiConfig(gomock.Any(), gomock.Any(), config).Return(nil)

	err := NewWatchableService(s.st, s.watcherFactory).SetLokiConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetLokiConfigInsecureSkipVerifyTrue(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boolVal := true
	s.st.EXPECT().GetLokiConfig(gomock.Any()).Return(logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: &boolVal,
	}, nil)

	config, err := NewWatchableService(s.st, s.watcherFactory).GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.InsecureSkipVerify, tc.NotNil)
	c.Check(*config.InsecureSkipVerify, tc.Equals, true)
}

func (s *serviceSuite) TestGetLokiConfigInsecureSkipVerifyNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiConfig(gomock.Any()).Return(logging.LokiConfig{
		Endpoint:           "http://loki:3100/loki/api/v1/push",
		CACertificate:      "ca-cert",
		InsecureSkipVerify: nil,
	}, nil)

	config, err := NewWatchableService(s.st, s.watcherFactory).GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.InsecureSkipVerify, tc.IsNil)
}

func (s *serviceSuite) TestGetLokiConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiConfig(gomock.Any()).Return(logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	}, nil)

	config, err := NewWatchableService(s.st, s.watcherFactory).GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.Endpoint, tc.Equals, "http://loki:3100/loki/api/v1/push")
	c.Check(config.CACertificate, tc.Equals, "ca-cert")
}

func (s *serviceSuite) TestGetLokiConfigNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiConfig(gomock.Any()).Return(logging.LokiConfig{}, loggingerrors.LokiConfigNotFound)

	_, err := NewWatchableService(s.st, s.watcherFactory).GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIs, loggingerrors.LokiConfigNotFound)
}

func (s *serviceSuite) TestGetLokiConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetLokiConfig(gomock.Any()).Return(logging.LokiConfig{}, errors.Errorf("boom"))

	_, err := NewWatchableService(s.st, s.watcherFactory).GetLokiConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestDeleteLokiConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteLokiConfig(gomock.Any()).Return(nil)

	err := NewWatchableService(s.st, s.watcherFactory).DeleteLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteLokiConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteLokiConfig(gomock.Any()).Return(errors.Errorf("boom"))

	err := NewWatchableService(s.st, s.watcherFactory).DeleteLokiConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestWatchLokiConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().NamespaceForWatchLokiConfig().Return("logging_loki_config")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(s.notifyWatcher, nil)

	w, err := NewWatchableService(s.st, s.watcherFactory).WatchLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Not(tc.IsNil))
}

func (s *serviceSuite) TestWatchLokiConfigWatcherFactoryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().NamespaceForWatchLokiConfig().Return("logging_loki_config")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, errors.Errorf("watcher boom"))

	_, err := NewWatchableService(s.st, s.watcherFactory).WatchLokiConfig(c.Context())
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
