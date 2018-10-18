// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/utils/arch"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/testing"
)

const ETag = "eTag"

// NoOpCallback can be passed to methods that receive a callback for setting
// status messages.
var NoOpCallback = func(st status.Status, info string, data map[string]interface{}) error { return nil }

// BaseSuite facilitates LXD testing.
// Do not instantiate this suite directly.
type BaseSuite struct {
	coretesting.BaseSuite
	arch string
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.arch = arch.HostArch()
}

func (s *BaseSuite) Arch() string {
	return s.arch
}

func (s *BaseSuite) NewMockServerClustered(ctrl *gomock.Controller, serverName string) *MockContainerServer {
	mutate := func(s *lxdapi.Server) {
		s.APIExtensions = []string{"network", "clustering"}
		s.Environment.ServerClustered = true
		s.Environment.ServerName = serverName
	}
	return s.NewMockServer(ctrl, mutate)
}

// NewMockServerWithExtensions initialises a mock container server.
// The return from GetServer indicates the input supported API extensions.
func (s *BaseSuite) NewMockServerWithExtensions(ctrl *gomock.Controller, extensions ...string) *MockContainerServer {
	return s.NewMockServer(ctrl, func(s *lxdapi.Server) { s.APIExtensions = extensions })
}

// NewMockServer initialises a mock container server and adds an
// expectation for the GetServer function, which is called each time NewClient
// is used to instantiate our wrapper.
// Any input mutations are applied to the return from the first GetServer call.
func (s *BaseSuite) NewMockServer(ctrl *gomock.Controller, svrMutations ...func(*lxdapi.Server)) *MockContainerServer {
	svr := NewMockContainerServer(ctrl)

	cfg := &lxdapi.Server{}
	for _, f := range svrMutations {
		f(cfg)
	}
	svr.EXPECT().GetServer().Return(cfg, ETag, nil)

	return svr
}
