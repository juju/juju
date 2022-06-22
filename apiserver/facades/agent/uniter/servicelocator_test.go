// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/uniter/mocks"
	"github.com/juju/testing"
)

type serviceLocatorSuite struct {
	testing.IsolationSuite

	backend *mocks.MockServiceLocatorBackend
}

var _ = gc.Suite(&serviceLocatorSuite{})

func (s *serviceLocatorSuite) assertBackendAPI(c *gc.C) (*uniter.ServiceLocatorAPI, *gomock.Controller, *mocks.MockServiceLocatorBackend) {
	ctrl := gomock.NewController(c)
	mockBackend := mocks.NewMockServiceLocatorBackend(ctrl)

	api := uniter.NewServiceLocatorAPI(
		mockBackend, loggo.GetLogger("juju.apiserver.facades.agent.uniter"))
	return api, ctrl, mockBackend
}
