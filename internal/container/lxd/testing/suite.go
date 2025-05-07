// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
)

const ETag = "eTag"

// NoOpCallback can be passed to methods that receive a callback for setting
// status messages.
var NoOpCallback = func(ctx context.Context, st status.Status, info string, data map[string]interface{}) error {
	return nil
}

// BaseSuite facilitates LXD testing.
// Do not instantiate this suite directly.
type BaseSuite struct {
	coretesting.BaseSuite
	arch string
}

func (s *BaseSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.arch = arch.HostArch()
}

func (s *BaseSuite) Arch() string {
	return s.arch
}

func (s *BaseSuite) NewMockServerClustered(ctrl *gomock.Controller, serverName string) *MockInstanceServer {
	mutate := func(s *lxdapi.Server) {
		s.APIExtensions = []string{"network", "clustering"}
		s.Environment.ServerClustered = true
		s.Environment.ServerName = serverName
	}
	return s.NewMockServer(ctrl, mutate)
}

// NewMockServerWithExtensions initialises a mock container server.
// The return from GetServer indicates the input supported API extensions.
func (s *BaseSuite) NewMockServerWithExtensions(ctrl *gomock.Controller, extensions ...string) *MockInstanceServer {
	return s.NewMockServer(ctrl, func(s *lxdapi.Server) { s.APIExtensions = extensions })
}

// NewMockServer initialises a mock container server and adds an
// expectation for the GetServer function, which is called each time NewClient
// is used to instantiate our wrapper.
// Any input mutations are applied to the return from the first GetServer call.
func (s *BaseSuite) NewMockServer(ctrl *gomock.Controller, svrMutations ...func(*lxdapi.Server)) *MockInstanceServer {
	svr := NewMockInstanceServer(ctrl)

	cfg := &lxdapi.Server{}
	for _, f := range svrMutations {
		f(cfg)
	}
	svr.EXPECT().GetServer().Return(cfg, ETag, nil)

	return svr
}
