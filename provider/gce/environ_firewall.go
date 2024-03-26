// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
)

// globalFirewallName returns the name to use for the global firewall.
func (env *environ) globalFirewallName() string {
	return common.EnvFullName(env.uuid)
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	err := env.gce.OpenPorts(env.globalFirewallName(), rules)
	return google.HandleCredentialError(errors.Trace(err), ctx)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	err := env.gce.ClosePorts(env.globalFirewallName(), rules)
	return google.HandleCredentialError(errors.Trace(err), ctx)
}

// IngressRules returns the ingress rules applicable for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	rules, err := env.gce.IngressRules(env.globalFirewallName())
	return rules, google.HandleCredentialError(errors.Trace(err), ctx)
}

// SupportsRulesWithIPV6CIDRs returns true if the environment supports
// ingress rules containing IPV6 CIDRs.
//
// This is part of the environs.FirewallFeatureQuerier interface.
func (e *environ) SupportsRulesWithIPV6CIDRs(context.ProviderCallContext) (bool, error) {
	return true, nil
}
