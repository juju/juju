// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
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

// TestControllerConfig asserts the happy path for reading controller config
// including schema-known fields that exercise the deserializeMap coercion
// branch.
func (s *serviceSuite) TestControllerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ctrlUUID := jujutesting.ControllerTag.Id()
	caCert := jujutesting.CACert

	s.state.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]string{
		controller.ControllerUUIDKey: ctrlUUID,
		controller.CACertKey:         caCert,
		controller.AuditingEnabled:   "true",
		controller.AuditLogMaxSize:   "100",
	}, nil)

	cfg, err := NewService(s.state).ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.ControllerUUID(), tc.Equals, ctrlUUID)
	gotCACert, ok := cfg.CACert()
	c.Assert(ok, tc.IsTrue)
	c.Check(gotCACert, tc.Equals, caCert)
	c.Check(cfg.AuditingEnabled(), tc.IsTrue)
	c.Check(cfg.AuditLogMaxSizeMB(), tc.Equals, 100)
}

// TestControllerConfigStateError asserts that a state-layer error is
// propagated when reading the controller config.
func (s *serviceSuite) TestControllerConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ControllerConfig(gomock.Any()).Return(nil, errors.New("boom"))

	_, err := NewService(s.state).ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "unable to get controller config: boom")
}

// TestControllerConfigMissingUUID asserts that a NotFound error is returned
// when the controller UUID is absent from the state map.
func (s *serviceSuite) TestControllerConfigMissingUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]string{
		controller.CACertKey: jujutesting.CACert,
	}, nil)

	_, err := NewService(s.state).ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestControllerConfigMissingCACert asserts that a NotFound error is returned
// when the CA cert is absent from the state map.
func (s *serviceSuite) TestControllerConfigMissingCACert(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]string{
		controller.ControllerUUIDKey: jujutesting.ControllerTag.Id(),
	}, nil)

	_, err := NewService(s.state).ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestControllerConfigDeserializeError asserts that an error from
// deserializeMap (invalid value for a known schema field) is propagated.
func (s *serviceSuite) TestControllerConfigDeserializeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]string{
		controller.ControllerUUIDKey: jujutesting.ControllerTag.Id(),
		controller.CACertKey:         jujutesting.CACert,
		controller.AuditingEnabled:   "not-a-bool",
	}, nil)

	_, err := NewService(s.state).ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, `unable to coerce controller config: .*`)
}

// TestControllerConfigNewConfigError asserts that an error from
// controller.NewConfig (e.g. invalid controller UUID) is propagated.
func (s *serviceSuite) TestControllerConfigNewConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]string{
		controller.ControllerUUIDKey: "not-a-valid-uuid",
		controller.CACertKey:         jujutesting.CACert,
	}, nil)

	_, err := NewService(s.state).ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, `unable to create controller config: .*`)
}

// TestGetSSHServerPort asserts the port is read from the single config key
// rather than the whole controller config.
func (s *serviceSuite) TestGetSSHServerPort(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetControllerConfigValue(gomock.Any(), controller.SSHServerPort).Return("2223", true, nil)

	port, err := NewService(s.state).GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(port, tc.Equals, 2223)
}

// TestGetSSHServerPortDefault asserts the default port is returned when the
// config key is not set.
func (s *serviceSuite) TestGetSSHServerPortDefault(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetControllerConfigValue(gomock.Any(), controller.SSHServerPort).Return("", false, nil)

	port, err := NewService(s.state).GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(port, tc.Equals, controller.DefaultSSHServerPort)
}

// TestGetSSHServerPortInvalid asserts a non-numeric stored value is reported as
// an error rather than silently ignored.
func (s *serviceSuite) TestGetSSHServerPortInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetControllerConfigValue(gomock.Any(), controller.SSHServerPort).Return("not-a-port", true, nil)

	_, err := NewService(s.state).GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorMatches, `parsing SSH server port "not-a-port": .*`)
}

// TestGetSSHServerPortStateError asserts a state-layer error is propagated.
func (s *serviceSuite) TestGetSSHServerPortStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetControllerConfigValue(gomock.Any(), controller.SSHServerPort).Return("", false, errors.New("boom"))

	_, err := NewService(s.state).GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorMatches, "getting SSH server port: boom")
}

func (s *serviceSuite) TestUpdateControllerConfigSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg, coerced := makeDefaultConfig("file")

	k1 := controller.AuditingEnabled
	k2 := controller.PublicDNSAddress

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, []string{k1, k2}).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, []string{k1, k2})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateControllerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg, coerced := makeDefaultConfig("file")

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, nil).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, nil)
	c.Assert(err, tc.ErrorMatches, "updating controller config state: boom")
}

// TestUpdateControllerConfigUnknownKey asserts that updating with a key
// that is not a controller-only attribute is rejected.
func (s *serviceSuite) TestUpdateControllerConfigUnknownKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{
		"not-a-real-key": "value",
	}

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, nil)
	c.Assert(err, tc.ErrorMatches, `unknown controller config setting "not-a-real-key"`)
}

// TestUpdateControllerConfigImmutableKey asserts that updating a controller
// config key that is not in AllowedUpdateConfigAttributes is rejected.
func (s *serviceSuite) TestUpdateControllerConfigImmutableKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{
		controller.APIPort: 1234,
	}

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, nil)
	c.Assert(err, tc.ErrorMatches, `can not change "api-port" after bootstrap`)
}

// TestUpdateControllerConfigCoercionError asserts that a value which
// fails schema coercion for a known field is rejected.
func (s *serviceSuite) TestUpdateControllerConfigCoercionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{
		controller.AuditingEnabled: "not-a-bool",
	}

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, nil)
	c.Assert(err, tc.ErrorMatches, `unable to coerce controller config key .*`)
}

// TestUpdateControllerConfigRemoveUnknownKey asserts that removing a key
// that is not a controller-only attribute is rejected.
func (s *serviceSuite) TestUpdateControllerConfigRemoveUnknownKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{}

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, []string{"not-a-real-key"})
	c.Assert(err, tc.ErrorMatches, `unknown controller config setting "not-a-real-key"`)
}

// TestUpdateControllerConfigRemoveImmutableKey asserts that removing a
// key not in AllowedUpdateConfigAttributes is rejected.
func (s *serviceSuite) TestUpdateControllerConfigRemoveImmutableKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{}

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(c.Context(), cfg, []string{controller.CACertKey})
	c.Assert(err, tc.ErrorMatches, `can not change "ca-cert" after bootstrap`)
}

// TestUpdateControllerConfigStripsControllerUUID asserts that the
// controller-uuid key is removed from the attrs passed to state, as
// it must never be updated.
func (s *serviceSuite) TestUpdateControllerConfigStripsControllerUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{
		controller.AuditingEnabled: true,
	}

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updateAttrs map[string]string, _ []string) error {
			_, hasUUID := updateAttrs[controller.ControllerUUIDKey]
			c.Check(hasUUID, tc.IsFalse)
			return nil
		},
	)

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

// TestWatchControllerConfigNoNamespaces asserts that an error is returned
// when the state reports no namespaces to watch.
func (s *serviceSuite) TestWatchControllerConfigNoNamespaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().NamespacesForWatchControllerConfig().Return(nil)

	_, err := NewWatchableService(s.state, s.watcherFactory).WatchControllerConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "no namespaces for watching controller config")
}

// TestWatchControllerConfigWatcherFactoryError asserts that a watcher
// factory error is propagated.
func (s *serviceSuite) TestWatchControllerConfigWatcherFactoryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AllKeysQuery().Return("query")
	s.state.EXPECT().NamespacesForWatchControllerConfig().Return([]string{"controller_config", "controller"})
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("watcher boom"))

	_, err := NewWatchableService(s.state, s.watcherFactory).WatchControllerConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "watcher boom")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.stringsWatcher = NewMockStringsWatcher(ctrl)

	return ctrl
}

func makeDefaultConfig(objectType string) (controller.Config, map[string]string) {
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
