// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
	stringsWatcher *MockStringsWatcher
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestUpdateControllerConfigSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg, coerced := makeDefaultConfig("file")

	k1 := controller.AuditingEnabled
	k2 := controller.APIPortOpenDelay

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, []string{k1, k2}, gomock.Any()).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(context.Background(), cfg, []string{k1, k2})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateControllerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg, coerced := makeDefaultConfig("file")

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, nil, gomock.Any()).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(context.Background(), cfg, nil)
	c.Assert(err, gc.ErrorMatches, "updating controller config state: boom")
}

func (s *serviceSuite) TestUpdateControllerValidationNoError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we allow changes to the object-store-type config key, from
	// file to s3.

	cfg, coerced := makeDefaultConfig("s3")
	_, current := makeDefaultConfig("file")

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, nil, gomock.Any()).DoAndReturn(func(ctx context.Context, updateAttrs map[string]string, removeAttrs []string, validateModification ModificationValidatorFunc) error {
		return validateModification(current)
	})

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(context.Background(), cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateControllerValidationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we prevent and reject changes to the object-store-type
	// config key, from s3 to file.

	cfg, coerced := makeDefaultConfig("file")
	_, current := makeDefaultConfig("s3")

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coerced, nil, gomock.Any()).DoAndReturn(func(ctx context.Context, updateAttrs map[string]string, removeAttrs []string, validateModification ModificationValidatorFunc) error {
		return validateModification(current)
	})

	err := NewWatchableService(s.state, s.watcherFactory).UpdateControllerConfig(context.Background(), cfg, nil)
	c.Assert(err, gc.ErrorMatches, `updating controller config state: can not change "object-store-type" from "s3" to "file"`)
}

func (s *serviceSuite) TestWatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	q := "the query does not matter"
	s.state.EXPECT().AllKeysQuery().Return(q)
	s.watcherFactory.EXPECT().NewNamespaceWatcher("controller_config", changestream.All, q).Return(s.stringsWatcher, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
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
			controller.APIPortOpenDelay:    "100ms",
			controller.ObjectStoreType:     objectType,
		}, map[string]string{
			controller.AuditingEnabled:     "true",
			controller.AuditLogCaptureArgs: "false",
			controller.AuditLogMaxBackups:  "10",
			controller.PublicDNSAddress:    "controller.test.com:1234",
			controller.APIPortOpenDelay:    "100ms",
			controller.ObjectStoreType:     objectType,
		}
}
