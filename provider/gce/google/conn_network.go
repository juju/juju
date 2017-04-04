// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/network"
)

// IngressRules build a list of all open port ranges for a given firewall name
// (within the Connection's project) and returns it. If the firewall
// does not exist then the list will be empty and no error is returned.
func (gce Connection) IngressRules(fwname string) ([]network.IngressRule, error) {
	firewalls, err := gce.raw.GetFirewalls(gce.projectID, fwname)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "while getting ports from GCE")
	}

	var rules []network.IngressRule
	for _, firewall := range firewalls {
		sourceRanges := firewall.SourceRanges
		if len(sourceRanges) == 0 {
			sourceRanges = []string{"0.0.0.0/0"}
		}
		var ranges []network.PortRange
		for _, allowed := range firewall.Allowed {
			for _, portRangeStr := range allowed.Ports {
				portRange, err := network.ParsePortRange(portRangeStr)
				if err != nil {
					return rules, errors.Annotate(err, "bad ports from GCE")
				}
				portRange.Protocol = allowed.IPProtocol
				ranges = append(ranges, portRange)
			}
		}

		collapsed := network.CombinePortRanges(ranges...)
		for _, portRange := range collapsed {
			rule, _ := network.NewIngressRule(portRange.Protocol, portRange.FromPort, portRange.ToPort, sourceRanges...)
			rules = append(rules, rule)
		}
	}

	return rules, nil
}

// OpenPorts sends a request to the GCE API to open the provided port
// ranges on the named firewall. If the firewall does not exist yet it
// is created, with the provided port ranges opened. Otherwise the
// existing firewall is updated to add the provided port ranges to the
// ports it already has open. The call blocks until the ports are
// opened or the request fails.
func (gce Connection) OpenPorts(target string, rules ...network.IngressRule) error {
	if len(rules) == 0 {
		return nil
	}

	// First gather the current ingress rules.
	currentRules, err := gce.IngressRules(target)
	if err != nil {
		return errors.Trace(err)
	}
	currentRuleSet := newRuleSet(currentRules...)

	// Get the rules for the target from the current rules.
	currentFirewallRules := currentRuleSet.getFirewallRules(target)

	// From the input rules, compose the firewall specs we want to add.
	inputRuleSet := newRuleSet(rules...)
	inputFirewallRules := inputRuleSet.getFirewallRules(target)

	// For each input rule, either create a new firewall or update
	// an existing one depending on what exists already.
	var sortedNames []string
	for name := range inputFirewallRules {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		portsByProtocol := inputFirewallRules[name]
		inputCidrs := inputRuleSet.getCIDRs(name)

		// First check to see if there's any existing firewall with the same ports as what we want.
		existingFirewallName, ok := matchProtocolPorts(currentFirewallRules, portsByProtocol)
		if !ok {
			// If not, look for any existing firewall with the same source CIDRs.
			existingFirewallName, ok = matchSourceCIDRs(currentFirewallRules, currentRuleSet, inputCidrs)
		}

		if !ok {
			// Create a new firewall.
			spec := firewallSpec(name, target, inputCidrs, portsByProtocol)
			if err := gce.raw.AddFirewall(gce.projectID, spec); err != nil {
				return errors.Annotatef(err, "opening port(s) %+v", rules)
			}
			continue
		}

		// An existing firewall exists with either same same ports or the same source
		// CIDRs as what we have been asked to open. Either way, we just need to update
		// the existing firewall.

		// Merge the ports.
		existingFirewallRules := currentFirewallRules[existingFirewallName]
		portsByProtocol = existingFirewallRules.union(portsByProtocol)

		// Merge the CIDRs
		cidrs := set.NewStrings(currentRuleSet.getCIDRs(existingFirewallName)...)
		inputCidrs = cidrs.Union(set.NewStrings(inputCidrs...)).SortedValues()

		// Copy new firewall details into required firewall spec.
		spec := firewallSpec(name, target, inputCidrs, portsByProtocol)
		if err := gce.raw.UpdateFirewall(gce.projectID, existingFirewallName, spec); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", rules)
		}
	}
	return nil
}

// matchProtocolPorts returns the matching firewall name and true if any of the current
// firewall ports (ignoring CIDR values) matches the ports to match.
func matchProtocolPorts(currentPorts map[string]protocolPorts, match protocolPorts) (string, bool) {
	for fwname, current := range currentPorts {
		if current.String() == match.String() {
			return fwname, true
		}
	}
	return "", false
}

// matchSourceCIDRs returns the matching firewall name and true if any of the current firewall rules
// has the same CIDR source values as those specified.
func matchSourceCIDRs(currentPorts map[string]protocolPorts, currentRuleSet RuleSet, matchCIDRs []string) (string, bool) {
	matchCidrSet := set.NewStrings(matchCIDRs...)
	for fwname := range currentPorts {
		currentCidrs := currentRuleSet.getCIDRs(fwname)
		diff := matchCidrSet.Difference(set.NewStrings(currentCidrs...))
		if diff.IsEmpty() {
			return fwname, true
		}
	}
	return "", false
}

// ClosePorts sends a request to the GCE API to close the provided port
// ranges on the named firewall. If the firewall does not exist nothing
// happens. If the firewall is left with no ports then it is removed.
// Otherwise it will be left with just the open ports it has that do not
// match the provided port ranges. The call blocks until the ports are
// closed or the request fails.
func (gce Connection) ClosePorts(target string, rules ...network.IngressRule) error {
	// First gather the current ingress rules.
	currentRules, err := gce.IngressRules(target)
	if err != nil {
		return errors.Trace(err)
	}
	currentRuleSet := newRuleSet(currentRules...)

	// Get the rules for the target from the current rules.
	currentFirewallRules := currentRuleSet.getFirewallRules(target)

	// From the input rules, compose the firewall specs we want to add.
	inputRuleSet := newRuleSet(rules...)
	inputFirewallRules := inputRuleSet.getFirewallRules(target)

	// For each input rule, either create a new firewall or update
	// an existing one depending on what exists already.
	for name, portsByProtocol := range inputFirewallRules {
		inputCidrs := inputRuleSet.getCIDRs(name)

		existingFirewallName, allPortsMatch := matchProtocolPorts(currentFirewallRules, portsByProtocol)
		if allPortsMatch {
			// All the ports match so it may be that just a CIDR needs to be removed.
			cidrs := set.NewStrings(currentRuleSet.getCIDRs(existingFirewallName)...)
			remainingCidrs := cidrs.Difference(set.NewStrings(inputCidrs...)).SortedValues()

			// If all CIDRs are also to be removed, we can delete the firewall.
			if len(remainingCidrs) == 0 {
				// Delete a firewall.
				// TODO(ericsnow) Handle case where firewall does not exist.
				if err := gce.raw.RemoveFirewall(gce.projectID, existingFirewallName); err != nil {
					return errors.Annotatef(err, "closing port(s) %+v", rules)
				}
				continue
			}

			// Update the existing firewall with the remaining CIDRs.
			portsByProtocol = currentFirewallRules[existingFirewallName]
			spec := firewallSpec(name, target, remainingCidrs, portsByProtocol)
			if err := gce.raw.UpdateFirewall(gce.projectID, existingFirewallName, spec); err != nil {
				return errors.Annotatef(err, "closing port(s) %+v", rules)
			}
			continue
		}

		existingFirewallName, sourceCIDRMatch := matchSourceCIDRs(currentFirewallRules, currentRuleSet, inputCidrs)
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
		existingFirewallRules := currentFirewallRules[existingFirewallName]
		portsByProtocol = existingFirewallRules.remove(portsByProtocol)

		// Copy new firewall details into required firewall spec.
		spec := firewallSpec(name, target, inputCidrs, portsByProtocol)
		if err := gce.raw.UpdateFirewall(gce.projectID, existingFirewallName, spec); err != nil {
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
