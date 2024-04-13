// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/jujuclient"
)

type proxyWrapperSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&proxyWrapperSuite{})

func (p *proxyWrapperSuite) TestMarshalling(c *gc.C) {
	config := proxy.ProxierConfig{
		APIHost:             "https://127.0.0.1:443",
		CAData:              "cadata====",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token====",
	}
	proxier := proxy.NewProxier(config)
	wrapper := &jujuclient.ProxyConfWrapper{proxier}
	data, err := yaml.Marshal(wrapper)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, `
type: kubernetes-port-forward
config:
    api-host: https://127.0.0.1:443
    ca-cert: cadata====
    namespace: test
    remote-port: "8123"
    service: test
    service-account-token: token====
`[1:])
}

func (p *proxyWrapperSuite) TestUnmarshalling(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := proxy.ProxierConfig{
		APIHost:             "https://127.0.0.1:443",
		CAData:              "cadata====",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token====",
	}
	proxier := proxy.NewProxier(config)
	rawConfig := map[string]interface{}{
		"api-host":              "https://127.0.0.1:443",
		"ca-cert":               "cadata====",
		"namespace":             "test",
		"remote-port":           "8123",
		"service":               "test",
		"service-account-token": "token====",
	}
	factory := NewMockProxyFactory(ctrl)
	factory.EXPECT().ProxierFromConfig("kubernetes-port-forward", rawConfig).Return(proxier, nil)
	p.PatchValue(&jujuclient.NewProxierFactory, func() (jujuclient.ProxyFactory, error) { return factory, nil })

	var wrapper jujuclient.ProxyConfWrapper
	err := yaml.Unmarshal([]byte(`
type: kubernetes-port-forward
config:
    api-host: https://127.0.0.1:443
    ca-cert: cadata====
    namespace: test
    remote-port: "8123"
    service: test
    service-account-token: token====
`[1:]), &wrapper)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrapper.Proxier.Type(), gc.Equals, "kubernetes-port-forward")
	rCfg, err := wrapper.Proxier.RawConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rCfg, gc.DeepEquals, rawConfig)
}
