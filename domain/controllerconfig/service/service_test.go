// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
	stringsWatcher *MockStringsWatcher
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestUpdateControllerConfigSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg, coerced := makeDefaultConfig()

	k1 := controller.AuditingEnabled
	k2 := controller.PublicDNSAddress

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, []string{k1, k2}).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, []string{k1, k2})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateControllerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg, coerced := makeDefaultConfig()

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, nil).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, nil)
	c.Assert(err, tc.ErrorMatches, "updating controller config state: boom")
}

func (s *serviceSuite) TestUpdateControllerValidationAllAtOnce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we allow the setting of all s3 config values in one

	cfg, coerced := makeDefaultConfig()

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, nil).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, nil)
	c.Assert(err, tc.ErrorIsNil)

}

func (s *serviceSuite) TestWatchControllerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	q := "the query does not matter"
	s.state.EXPECT().AllKeysQuery().Return(q)

	s.state.EXPECT().NamespacesForWatchControllerConfig().Return([]string{"controller_config", "controller"})
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(s.stringsWatcher, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).WatchControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.stringsWatcher = NewMockStringsWatcher(ctrl)

	return ctrl
}

func makeDefaultConfig() (controller.Config, map[string]string) {
	return controller.Config{
			controller.AuditingEnabled:     true,
			controller.AuditLogCaptureArgs: false,
			controller.AuditLogMaxBackups:  10,
			controller.PublicDNSAddress:    "controller.test.com:1234",
		}, map[string]string{
			controller.AuditingEnabled:     "true",
			controller.AuditLogCaptureArgs: "false",
			controller.AuditLogMaxBackups:  "10",
			controller.PublicDNSAddress:    "controller.test.com:1234",
		}
}
