// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/credential/service State,WatcherFactory,MachineService
//go:generate go run go.uber.org/mock/mockgen -package service -destination validator_mock_test.go github.com/juju/juju/domain/credential/service CredentialValidator

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	state          *MockState
	validator      *MockCredentialValidator
	watcherFactory *MockWatcherFactory
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.validator = NewMockCredentialValidator(ctrl)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}
