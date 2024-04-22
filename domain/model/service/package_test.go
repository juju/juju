// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/juju/core/status"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination status_mock_test.go github.com/juju/juju/core/status StatusHistoryFactory,StatusHistorySetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	statusHistoryFactory *MockStatusHistoryFactory
	statusHistorySetter  *MockStatusHistorySetter
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusHistoryFactory = NewMockStatusHistoryFactory(ctrl)
	s.statusHistorySetter = NewMockStatusHistorySetter(ctrl)

	return ctrl
}

func (s *baseSuite) expectStatusHistory(c *gc.C) {
	// We use a random UUID for the model namespace. We should only ever receive
	// one request though.
	s.statusHistoryFactory.EXPECT().StatusHistorySetterForModel(gomock.Any()).Return(s.statusHistorySetter)
	s.statusHistorySetter.EXPECT().SetStatusHistory(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(kind status.HistoryKind, s status.Status, id string) error {
		c.Check(kind, gc.Equals, status.KindModel)
		c.Check(s, gc.Equals, status.Available)
		return nil
	})
}
