// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"testing"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	dbtesting "github.com/juju/juju/database/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package eventqueue -destination package_mock_test.go github.com/juju/juju/core/watcher/eventqueue EventQueue,Logger

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	dbtesting.ControllerSuite

	queue  *MockEventQueue
	logger *MockLogger
}

func (s *baseSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.queue = NewMockEventQueue(ctrl)
	s.logger = NewMockLogger(ctrl)

	return ctrl
}

func (s *baseSuite) makeBaseWatcher() BaseWatcher {
	return MakeBaseWatcher(s.queue, s.TrackedDB(), s.logger)
}
