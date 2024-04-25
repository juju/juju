// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package units3caller

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package units3caller -destination package_mock_test.go github.com/juju/juju/core/objectstore Session
//go:generate go run go.uber.org/mock/mockgen -typed -package units3caller -destination api_mocks_test.go github.com/juju/juju/api Connection

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	logger  logger.Logger
	session *MockSession
	apiConn *MockConnection
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.session = NewMockSession(ctrl)
	s.apiConn = NewMockConnection(ctrl)

	return ctrl
}
