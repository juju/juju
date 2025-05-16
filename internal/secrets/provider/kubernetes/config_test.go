// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/testhelpers"
)

type configSuite struct {
	testhelpers.IsolationSuite
}

func TestConfigSuite(t *stdtesting.T) { tc.Run(t, &configSuite{}) }
func (s *configSuite) TestValidateConfig(c *tc.C) {
	p, err := provider.Provider(BackendType)
	c.Assert(err, tc.ErrorIsNil)
	configValidator, ok := p.(provider.ProviderConfig)
	c.Assert(ok, tc.IsTrue)
	rotateInterval := time.Hour
	for _, t := range []struct {
		cfg                 map[string]interface{}
		oldCfg              map[string]interface{}
		tokenRotateInterval *time.Duration
		err                 string
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
	}, {
		cfg:                 map[string]interface{}{"endpoint": "newep", "namespace": "foo"},
		tokenRotateInterval: &rotateInterval,
		err:                 `k8s config missing service account not valid`,
	}} {
		err = configValidator.ValidateConfig(t.oldCfg, t.cfg, t.tokenRotateInterval)
		c.Assert(err, tc.ErrorMatches, t.err)
	}
}
