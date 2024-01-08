// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/s3client"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package s3caller -destination package_mock_test.go github.com/juju/juju/core/objectstore Session
//go:generate go run go.uber.org/mock/mockgen -package s3caller -destination api_mocks.go github.com/juju/juju/api Connection

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	logger  s3client.Logger
	session *MockSession
	apiConn *MockConnection
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = coretesting.CheckLogger{
		Log: c,
	}
	s.session = NewMockSession(ctrl)
	s.apiConn = NewMockConnection(ctrl)

	return ctrl
}
