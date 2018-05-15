package testing

import (
	"github.com/golang/mock/gomock"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/utils/arch"
)

const ETag = "eTag"

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

// NewMockServer initialises a mock container server and adds an
// expectation for the GetServer function, which is called each time NewClient
// is used to instantiate our wrapper.
// The return from GetServer indicates the input supported API extensions.
func (s *BaseSuite) NewMockServer(ctrl *gomock.Controller, extensions ...string) *MockContainerServer {
	svr := NewMockContainerServer(ctrl)

	cfg := &lxdapi.Server{
		ServerUntrusted: lxdapi.ServerUntrusted{
			APIExtensions: extensions,
		},
	}
	svr.EXPECT().GetServer().Return(cfg, ETag, nil)

	return svr
}
