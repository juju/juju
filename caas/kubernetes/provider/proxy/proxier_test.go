// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas/kubernetes/provider/proxy"
)

type proxySuite struct {
}

var _ = tc.Suite(&proxySuite{})

func (p *proxySuite) TestProxierMarshalling(c *tc.C) {
	config := proxy.ProxierConfig{
		APIHost:             "https://localhost:1234",
		CAData:              "cadata",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token",
	}

	proxier := proxy.NewProxier(config)
	yamlConf, err := yaml.Marshal(proxier)
	c.Assert(err, jc.ErrorIsNil)

	unmarshalledConfig := proxy.ProxierConfig{}
	c.Assert(yaml.Unmarshal(yamlConf, &unmarshalledConfig), jc.ErrorIsNil)

	c.Assert(unmarshalledConfig, jc.DeepEquals, config)
}

func (p *proxySuite) TestSetAPIHost(c *tc.C) {
	config := proxy.ProxierConfig{
		APIHost: "https://localhost:1234",
	}

	proxier := proxy.NewProxier(config)
	proxier.SetAPIHost("https://localhost:666")
	yamlConf, err := yaml.Marshal(proxier)
	c.Assert(err, jc.ErrorIsNil)

	unmarshalledConfig := proxy.ProxierConfig{}
	c.Assert(yaml.Unmarshal(yamlConf, &unmarshalledConfig), jc.ErrorIsNil)

	config.APIHost = "https://localhost:666"
	c.Assert(unmarshalledConfig, jc.DeepEquals, config)
}

func (p *proxySuite) TestNewProxier(c *tc.C) {
	config := proxy.ProxierConfig{
		APIHost:             "https://localhost:1234",
		CAData:              "cadata",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token",
	}
	proxier := proxy.NewProxier(config)
	c.Assert(proxier.RESTConfig(), tc.DeepEquals, rest.Config{
		BearerToken: "token",
		Host:        "https://localhost:1234",
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte("cadata"),
		},
	})

	config = proxy.ProxierConfig{
		APIHost:             "https://localhost:1234",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token",
	}
	proxier = proxy.NewProxier(config)
	c.Assert(proxier.RESTConfig(), tc.DeepEquals, rest.Config{
		BearerToken: "token",
		Host:        "https://localhost:1234",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
}

func (p *proxySuite) TestInsecure(c *tc.C) {
	config := proxy.ProxierConfig{
		APIHost:             "https://localhost:1234",
		CAData:              "cadata",
		Namespace:           "test",
		RemotePort:          "8123",
		Service:             "test",
		ServiceAccountToken: "token",
	}
	proxier := proxy.NewProxier(config)
	c.Assert(proxier.RESTConfig(), tc.DeepEquals, rest.Config{
		BearerToken: "token",
		Host:        "https://localhost:1234",
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte("cadata"),
		},
	})
	proxier.Insecure()
	c.Assert(proxier.Config().CAData, tc.Equals, "")
	c.Assert(proxier.RESTConfig(), tc.DeepEquals, rest.Config{
		BearerToken: "token",
		Host:        "https://localhost:1234",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
}
