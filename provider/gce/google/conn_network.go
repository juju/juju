// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/network"
)

// FirewallRules collects the firewall rules for the given name
// (within the Connection's project) and returns them as a RuleSet. If
// no rules match the name the RuleSet will be empty and no error is
// returned.
func (gce Connection) firewallRules(fwname string) (ruleSet, error) {
	firewalls, err := gce.raw.GetFirewalls(gce.projectID, fwname)
	if errors.IsNotFound(err) {
		return make(ruleSet), nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall rules from GCE")
	}

	return newRuleSetFromFirewalls(firewalls...)
}

// IngressRules build a list of all open port ranges for a given firewall name
// (within the Connection's project) and returns it. If the firewall
// does not exist then the list will be empty and no error is returned.
func (gce Connection) IngressRules(fwname string) ([]network.IngressRule, error) {
	ruleset, err := gce.firewallRules(fwname)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ruleset.toIngressRules()
}

// FirewallNamer generates a unique name for a firewall given the firewall, a
// prefix and a set of current firewall rule names.
type FirewallNamer func(*firewall, string, set.Strings) (string, error)

// OpenPorts sends a request to the GCE API to open the provided port
// ranges on the named firewall. If the firewall does not exist yet it
// is created, with the provided port ranges opened. Otherwise the
// existing firewall is updated to add the provided port ranges to the
// ports it already has open. The call blocks until the ports are
// opened or the request fails.
func (gce Connection) OpenPorts(target string, namer FirewallNamer, rules ...network.IngressRule) error {
	if len(rules) == 0 {
		return nil
	}

	// First gather the current ingress rules.
	currentRuleSet, err := gce.firewallRules(target)
	if err != nil {
		return errors.Trace(err)
	}
	// From the input rules, compose the firewall specs we want to add.
	inputRuleSet := newRuleSetFromRules(rules...)

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
			// Create a new firewall.
			name, err := namer(inputFirewall, target, allNames)
			if err != nil {
				return errors.Trace(err)
			}
			allNames.Add(name)
			spec := firewallSpec(name, target, inputFirewall.SourceCIDRs, inputFirewall.AllowedPorts)
			if err := gce.raw.AddFirewall(gce.projectID, spec); err != nil {
				return errors.Annotatef(err, "opening port(s) %+v", rules)
			}
			continue
		}

		// An existing firewall exists with either same same ports or the same source
		// CIDRs as what we have been asked to open. Either way, we just need to update
		// the existing firewall.

		// Merge the ports.
		allowedPorts := existingFirewall.AllowedPorts.union(inputFirewall.AllowedPorts)

		// Merge the CIDRs
		cidrs := set.NewStrings(existingFirewall.SourceCIDRs...)
		combinedCIDRs := cidrs.Union(set.NewStrings(inputFirewall.SourceCIDRs...)).SortedValues()

		// Copy new firewall details into required firewall spec.
		spec := firewallSpec(existingFirewall.Name, target, combinedCIDRs, allowedPorts)
		if err := gce.raw.UpdateFirewall(gce.projectID, existingFirewall.Name, spec); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", rules)
		}
	}
	return nil
}

// RandomSuffixNamer tries to find a unique name for the firewall by
// appending a random suffix.
func RandomSuffixNamer(fw *firewall, target string, names set.Strings) (string, error) {
	// For backwards compatibility, open rules for "0.0.0.0/0"
	// do not use any suffix in the name.
	if len(fw.SourceCIDRs) == 0 || len(fw.SourceCIDRs) == 1 && fw.SourceCIDRs[0] == "0.0.0.0/0" {
		return "target", nil
	}
	data := make([]byte, 4)
	for i := 0; i < 10; i++ {
		_, err := rand.Read(data)
		if err != nil {
			return "", errors.Trace(err)
		}
		name := fmt.Sprintf("%s-%x", target, data)
		if !names.Contains(name) {
			return name, nil
		}
	}
	return "", errors.New("couldn't pick unique name after 10 attempts")
}

// ClosePorts sends a request to the GCE API to close the provided port
// ranges on the named firewall. If the firewall does not exist nothing
// happens. If the firewall is left with no ports then it is removed.
// Otherwise it will be left with just the open ports it has that do not
// match the provided port ranges. The call blocks until the ports are
// closed or the request fails.
func (gce Connection) ClosePorts(target string, rules ...network.IngressRule) error {
	// First gather the current ingress rules.
	currentRuleSet, err := gce.firewallRules(target)
	if err != nil {
		return errors.Trace(err)
	}

	// From the input rules, compose the firewall specs we want to add.
	inputRuleSet := newRuleSetFromRules(rules...)

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
				// TODO(ericsnow) Handle case where firewall does not exist.
				if err := gce.raw.RemoveFirewall(gce.projectID, existingFirewall.Name); err != nil {
					return errors.Annotatef(err, "closing port(s) %+v", rules)
				}
				continue
			}

			// Update the existing firewall with the remaining CIDRs.
			spec := firewallSpec(existingFirewall.Name, target, remainingCidrs, existingFirewall.AllowedPorts)
			if err := gce.raw.UpdateFirewall(gce.projectID, existingFirewall.Name, spec); err != nil {
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
		if err := gce.raw.UpdateFirewall(gce.projectID, existingFirewall.Name, spec); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", rules)
		}
	}
	return nil
}

// Subnetworks returns the subnets available in this region.
func (gce Connection) Subnetworks(region string) ([]*compute.Subnetwork, error) {
	results, err := gce.raw.ListSubnetworks(gce.projectID, region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// Networks returns the networks available.
func (gce Connection) Networks() ([]*compute.Network, error) {
	results, err := gce.raw.ListNetworks(gce.projectID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}
