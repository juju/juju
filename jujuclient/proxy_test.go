// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/provider/kubernetes/proxy"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type proxyWrapperSuite struct {
	testhelpers.IsolationSuite
}

func TestProxyWrapperSuite(t *testing.T) {
	tc.Run(t, &proxyWrapperSuite{})
}

func (p *proxyWrapperSuite) TestMarshalling(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, `
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

func (p *proxyWrapperSuite) TestUnmarshalling(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wrapper.Proxier.Type(), tc.Equals, "kubernetes-port-forward")
	rCfg, err := wrapper.Proxier.RawConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rCfg, tc.DeepEquals, rawConfig)
}
