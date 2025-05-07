// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go github.com/juju/juju/domain/credential/service State,WatcherFactory,MachineService,MachineState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination validator_mock_test.go github.com/juju/juju/domain/credential/service CredentialValidator

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	state          *MockState
	validator      *MockCredentialValidator
	watcherFactory *MockWatcherFactory
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.validator = NewMockCredentialValidator(ctrl)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}
