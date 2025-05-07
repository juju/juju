// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	jujuvault "github.com/juju/juju/internal/secrets/provider/vault"
)

type configSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&configSuite{})

func (s *configSuite) TestValidateConfig(c *tc.C) {
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	configValidator, ok := p.(provider.ProviderConfig)
	c.Assert(ok, tc.IsTrue)
	for _, t := range []struct {
		cfg    map[string]interface{}
		oldCfg map[string]interface{}
		err    string
	}{{
		cfg: map[string]interface{}{},
		err: "endpoint: expected string, got nothing",
	}, {
		cfg:    map[string]interface{}{"endpoint": "newep"},
		oldCfg: map[string]interface{}{"endpoint": "oldep"},
		err:    `cannot change immutable field "endpoint"`,
	}, {
		cfg: map[string]interface{}{"endpoint": "newep", "client-cert": "aaa"},
		err: `vault config missing client key not valid`,
	}, {
		cfg: map[string]interface{}{"endpoint": "newep", "client-key": "aaa"},
		err: `vault config missing client certificate not valid`,
	}} {
		err = configValidator.ValidateConfig(t.oldCfg, t.cfg, nil)
		c.Assert(err, tc.ErrorMatches, t.err)
	}
}
