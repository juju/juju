// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type utilsSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&utilsSuite{})

func (s *utilsSuite) TestGenerateUserControllerAccessToken(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	controllerCMD := NewMockControllerCommand(ctrl)
	clientStore := NewMockClientStore(ctrl)

	controllerCMD.EXPECT().ControllerName().Return("controller-k1", nil)
	controllerCMD.EXPECT().ClientStore().Return(clientStore)

	config := proxy.ProxierConfig{
		APIHost:             "https://localhost:1234",
		CAData:              "cadata",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token",
	}
	proxier := proxy.NewProxier(config)
	clientStore.EXPECT().ControllerByName("controller-k1").Return(&jujuclient.ControllerDetails{
		APIEndpoints: []string{""},
		Proxy:        &jujuclient.ProxyConfWrapper{Proxier: proxier},
	}, nil)

	token, error := user.GenerateUserControllerAccessToken(controllerCMD, "foo", []byte("bar"))
	c.Assert(error, tc.ErrorIsNil)
	c.Assert(token, tc.Equals, "MIHOEwNmb28wAhMABANiYXITDWNvbnRyb2xsZXItazEMga50eXBlOiBrdWJlcm5ldGVzLXBvcnQtZm9yd2FyZApjb25maWc6CiAgYXBpLWhvc3Q6IGh0dHBzOi8vbG9jYWxob3N0OjEyMzQKICBjYS1jZXJ0OiAiIgogIG5hbWVzcGFjZTogdGVzdAogIHJlbW90ZS1wb3J0OiAiODEyMyIKICBzZXJ2aWNlOiB0ZXN0CiAgc2VydmljZS1hY2NvdW50LXRva2VuOiB0b2tlbgoA")
}
