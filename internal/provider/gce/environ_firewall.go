// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"crypto/rand"
	"fmt"
	"sort"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/internal/provider/common"
)

var openCIDRs = set.NewStrings(firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR)

// globalFirewallName returns the name to use for the global firewall.
func (env *environ) globalFirewallName() string {
	return common.EnvFullName(env.uuid)
}

// firewallSpec expands a port range set in to computepb.FirewallAllowed
// and returns a computepb.Firewall for the provided name.
func firewallSpec(name, target string, sourceCIDRs []string, ports protocolPorts) *computepb.Firewall {
	if len(sourceCIDRs) == 0 {
		sourceCIDRs = []string{firewall.AllNetworksIPV4CIDR}
	}
	firewall := computepb.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: &name,
		// Network: (defaults to global)
		// SourceTags is not set.
		TargetTags:   []string{target},
		SourceRanges: sourceCIDRs,
	}

	var sortedProtocols []string
	for protocol := range ports {
		sortedProtocols = append(sortedProtocols, protocol)
	}
	sort.Strings(sortedProtocols)

	for _, protocol := range sortedProtocols {
		allowed := computepb.Allowed{
			IPProtocol: &protocol,
			Ports:      ports.portStrings(protocol),
		}
		firewall.Allowed = append(firewall.Allowed, &allowed)
	}
	return &firewall
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
// If a rule matching a set of source ranges doesn't
// already exist, it will be created - the name will be made unique
// using a random suffix.
func (env *environ) OpenPorts(ctx context.Context, target string, rules firewall.IngressRules) error {
	err := env.openPorts(ctx, target, rules)
	return env.HandleCredentialError(ctx, err)
}

// randomSuffixNamer tries to find a unique name for the firewall by
// appending a random suffix.
var randomSuffixNamer = func(sourceCIDRs []string, prefix string, existingNames set.Strings) (string, error) {
	// For backwards compatibility, open rules for "0.0.0.0/0" or ::/0
	// do not use any suffix in the name.
	if len(sourceCIDRs) == 0 || len(sourceCIDRs) == 1 && openCIDRs.Contains(sourceCIDRs[0]) {
		return prefix, nil
	}
	data := make([]byte, 4)
	for i := 0; i < 10; i++ {
		_, err := rand.Read(data)
		if err != nil {
			return "", errors.Trace(err)
		}
		name := fmt.Sprintf("%s-%x", prefix, data)
		if !existingNames.Contains(name) {
			return name, nil
		}
	}
	return "", errors.New("couldn't pick unique name after 10 attempts")
}

func (env *environ) openPorts(ctx context.Context, target string, rules firewall.IngressRules) error {
	if len(rules) == 0 {
		return nil
	}

	// First gather the current ingress rules.
	firewalls, err := env.gce.Firewalls(ctx, target)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}

	currentRuleSet, err := newRuleSetFromFirewalls(firewalls...)
	if err != nil {
		return errors.Trace(err)
	}

	// From the input rules, compose the firewall specs we want to add.
	inputRuleSet := newRuleSetFromRules(rules)

	// For each input rule, either create a new firewall or update
	// an existing one depending on what exists already.
	// The rules are keyed by a hash of the source CIDRs.
	var sortedKeys []string
	for key := range inputRuleSet {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	allNames := currentRuleSet.allNames()

	// Get the rules by sorted key for deterministic testing.
	for _, key := range sortedKeys {
		inputFirewall := inputRuleSet[key]

		// First check to see if there's any existing firewall with the same ports as what we want.
		existingFirewall, ok := currentRuleSet.matchProtocolPorts(inputFirewall.AllowedPorts)
		if !ok {
			// If not, look for any existing firewall with the same source CIDRs.
			existingFirewall, ok = currentRuleSet.matchSourceCIDRs(inputFirewall.SourceCIDRs)
		}

		if !ok {
			vpcLink, _, err := env.getVpcInfo(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			// Create a new firewall.
			name, err := randomSuffixNamer(inputFirewall.SourceCIDRs, target, allNames)
			if err != nil {
				return errors.Trace(err)
			}
			allNames.Add(name)
			spec := firewallSpec(name, target, inputFirewall.SourceCIDRs, inputFirewall.AllowedPorts)
			spec.Network = vpcLink
			if err := env.gce.AddFirewall(ctx, spec); err != nil {
				return errors.Annotatef(err, "adding firewall %q opening port(s) %+v", name, rules)
			}
			continue
		}

		// An existing firewall exists with either the same ports or the same source
		// CIDRs as what we have been asked to open. Either way, we just need to update
		// the existing firewall.

		// Merge the ports.
		allowedPorts := existingFirewall.AllowedPorts.union(inputFirewall.AllowedPorts)

		// Merge the CIDRs
		cidrs := set.NewStrings(existingFirewall.SourceCIDRs...)
		combinedCIDRs := cidrs.Union(set.NewStrings(inputFirewall.SourceCIDRs...)).SortedValues()

		// Copy new firewall details into required firewall spec.
		spec := firewallSpec(existingFirewall.Name, target, combinedCIDRs, allowedPorts)
		if err := env.gce.UpdateFirewall(ctx, existingFirewall.Name, spec); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", rules)
		}
	}
	return nil
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
// If the firewall does not exist nothing happens.
// If the firewall is left with no ports then it is removed.
// Otherwise it will be left with just the open ports it has that do not
// match the provided port ranges. The call blocks until the ports are
// closed or the request fails.
func (env *environ) ClosePorts(ctx context.Context, target string, rules firewall.IngressRules) error {
	err := env.closePorts(ctx, target, rules)
	return env.HandleCredentialError(ctx, err)
}

func (env *environ) closePorts(ctx context.Context, target string, rules firewall.IngressRules) error {
	// First gather the current ingress rules.
	firewalls, err := env.gce.Firewalls(ctx, target)
	if err != nil {
		return errors.Trace(err)
	}

	currentRuleSet, err := newRuleSetFromFirewalls(firewalls...)
	if err != nil {
		return errors.Trace(err)
	}

	// From the input rules, compose the firewall specs we want to add.
	inputRuleSet := newRuleSetFromRules(rules)

	// For each input firewall, find an existing firewall including it
	// and update or remove it.
	for _, inputFirewall := range inputRuleSet {
		existingFirewall, allPortsMatch := currentRuleSet.matchProtocolPorts(inputFirewall.AllowedPorts)
		if allPortsMatch {
			// All the ports match so it may be that just a CIDR needs to be removed.
			cidrs := set.NewStrings(existingFirewall.SourceCIDRs...)
			remainingCidrs := cidrs.Difference(set.NewStrings(inputFirewall.SourceCIDRs...)).SortedValues()

			// If all CIDRs are also to be removed, we can delete the firewall.
			if len(remainingCidrs) == 0 {
				// Delete a firewall.
				if err := env.gce.RemoveFirewall(ctx, existingFirewall.Name); err != nil {
					return errors.Annotatef(err, "deleting firewall %q closing port(s) %+v", existingFirewall.Name, rules)
				}
				continue
			}

			// Update the existing firewall with the remaining CIDRs.
			spec := firewallSpec(existingFirewall.Name, target, remainingCidrs, existingFirewall.AllowedPorts)
			if err := env.gce.UpdateFirewall(ctx, existingFirewall.Name, spec); err != nil {
				return errors.Annotatef(err, "closing port(s) %+v", rules)
			}
			continue
		}

		existingFirewall, sourceCIDRMatch := currentRuleSet.matchSourceCIDRs(inputFirewall.SourceCIDRs)
		if !sourceCIDRMatch {
			// We already know ports don't match, so if CIDRs don't match either, we either
			// have a partial match or no match.
			// No matches are a no-op. Partial matches might require splitting firewall rules
			// which is not supported at the moment. We'll return an error as it's better to
			// be overly cautious than accidentally leave ports open. The issue shouldn't occur
			// in practice unless people have manually played with the firewall rules.
			return errors.NotSupportedf("closing port(s) %+v over non-matching rules", rules)
		}

		// Delete the ports to close.
		remainingPorts := existingFirewall.AllowedPorts.remove(inputFirewall.AllowedPorts)

		// Copy new firewall details into required firewall spec.
		spec := firewallSpec(existingFirewall.Name, target, existingFirewall.SourceCIDRs, remainingPorts)
		if err := env.gce.UpdateFirewall(ctx, existingFirewall.Name, spec); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", rules)
		}
	}
	return nil
}

// IngressRules returns the ingress rules applicable for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) IngressRules(ctx context.Context, target string) (firewall.IngressRules, error) {
	firewalls, err := env.gce.Firewalls(ctx, target)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	ruleset, err := newRuleSetFromFirewalls(firewalls...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ruleset.toIngressRules()
}

func (env *environ) cleanupFirewall(ctx context.Context) error {
	err := env.gce.RemoveFirewall(ctx, env.globalFirewallName())
	return env.HandleCredentialError(ctx, err)
}

// OpenModelPorts opens the given port ranges on the model firewall
func (env *environ) OpenModelPorts(ctx context.Context, rules firewall.IngressRules) error {
	err := env.openPorts(ctx, env.globalFirewallName(), rules)
	return env.HandleCredentialError(ctx, errors.Trace(err))
}

// CloseModelPorts Closes the given port ranges on the model firewall
func (env *environ) CloseModelPorts(ctx context.Context, rules firewall.IngressRules) error {
	err := env.closePorts(ctx, env.globalFirewallName(), rules)
	return env.HandleCredentialError(ctx, errors.Trace(err))
}

// ModelIngressRules returns the set of ingress rules on the model firewall
func (env *environ) ModelIngressRules(ctx context.Context) (firewall.IngressRules, error) {
	rules, err := env.IngressRules(ctx, env.globalFirewallName())
	return rules, env.HandleCredentialError(ctx, errors.Trace(err))
}
