// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	jujuk8s "github.com/juju/juju/secrets/provider/kubernetes"
)

type configSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) TestValidateConfig(c *gc.C) {
	p, err := provider.Provider(jujuk8s.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	configValidator, ok := p.(provider.ProviderConfig)
	c.Assert(ok, jc.IsTrue)
	for _, t := range []struct {
		cfg    map[string]interface{}
		oldCfg map[string]interface{}
		err    string
	}{{
		cfg: map[string]interface{}{"namespace": "foo"},
		err: "endpoint: expected string, got nothing",
	}, {
		cfg: map[string]interface{}{"endpoint": "newep"},
		err: "namespace: expected string, got nothing",
	}, {
		cfg:    map[string]interface{}{"endpoint": "newep", "namespace": "foo"},
		oldCfg: map[string]interface{}{"endpoint": "oldep", "namespace": "foo"},
		err:    `cannot change immutable field "endpoint"`,
	}, {
		cfg: map[string]interface{}{"endpoint": "newep", "namespace": "foo", "client-cert": "aaa"},
		err: `k8s config missing client key not valid`,
	}, {
		cfg: map[string]interface{}{"endpoint": "newep", "namespace": "foo", "client-key": "aaa"},
		err: `k8s config missing client certificate not valid`,
	}} {
		err = configValidator.ValidateConfig(t.oldCfg, t.cfg)
		c.Assert(err, gc.ErrorMatches, t.err)
	}
}
