// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/caas/kubernetes/provider/proxy"
)

type proxySuite struct {
}

var _ = gc.Suite(&proxySuite{})

func (p *proxySuite) TestProxierMarshalling(c *gc.C) {
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
