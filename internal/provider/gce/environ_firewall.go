// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/internal/provider/common"
)

// globalFirewallName returns the name to use for the global firewall.
func (env *environ) globalFirewallName() string {
	return common.EnvFullName(env.uuid)
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ctx context.Context, rules firewall.IngressRules) error {
	err := env.gce.OpenPorts(env.globalFirewallName(), rules)
	return env.HandleCredentialError(ctx, err)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ctx context.Context, rules firewall.IngressRules) error {
	err := env.gce.ClosePorts(env.globalFirewallName(), rules)
	return env.HandleCredentialError(ctx, err)
}

// IngressRules returns the ingress rules applicable for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) IngressRules(ctx context.Context) (firewall.IngressRules, error) {
	rules, err := env.gce.IngressRules(env.globalFirewallName())
	return rules, env.HandleCredentialError(ctx, err)
}

func (env *environ) cleanupFirewall(ctx context.Context) error {
	err := env.gce.RemoveFirewall(env.globalFirewallName())
	return env.HandleCredentialError(ctx, err)
}
