// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/juju/juju/network"
	"sort"
)

// RuleSet is used to manipulate port ranges
// for a collection of IngressRules.
type RuleSet struct {
	rules []network.IngressRule

	// firewallCidrs is filled in each time the
	// firewall rules are determined from
	// the ingress rules.
	firewallCidrs map[string][]string
}

func newRuleSet(rules ...network.IngressRule) RuleSet {
	var result RuleSet
	result.rules = make([]network.IngressRule, len(rules))
	result.firewallCidrs = make(map[string][]string)
	copy(result.rules, rules)
	return result
}

func (rs RuleSet) getCIDRs(name string) []string {
	return rs.firewallCidrs[name]
}

// sourcecidrs is used to calculate a unique firewall
// name suffix for a collection of cidrs.
type sourcecidrs []string

func (s sourcecidrs) nameSuffix() string {
	if len(s) == 0 || len(s) == 1 && s[0] == "0.0.0.0/0" {
		return ""
	}
	src := strings.Join(s, ",")
	hash := sha256.New()
	hash.Write([]byte(src))
	hashStr := fmt.Sprintf("%x", hash.Sum(nil))
	return hashStr[:6]
}

// protocolPorts maps a protocol eg "tcp" to a collection of
// port ranges for that protocol.
type protocolPorts map[string][]network.PortRange

// getFirewallRules returns a map  of "firewallname" to ports to open
// fot that firewall, based on the ingress rules in the ruleset.
func (rs RuleSet) getFirewallRules(namePrefix string) map[string]protocolPorts {
	result := make(map[string]protocolPorts)
	for _, rule := range rs.rules {

		// We make a unique firewall name based on a has of the CIDRs.
		// For backwards compatibility, open rules for "0.0.0.0/0"
		// do not use any hash in the name.
		fwname := namePrefix
		suffix := sourcecidrs(rule.SourceCIDRs).nameSuffix()
		if suffix != "" {
			fwname = fwname + "-" + suffix
		}
		cidrs := rule.SourceCIDRs
		if len(cidrs) == 0 {
			cidrs = []string{"0.0.0.0/0"}
		}
		rs.firewallCidrs[fwname] = cidrs

		// Add the current port ranges to the firewall name.
		protocolPortsForName, ok := result[fwname]
		if !ok {
			protocolPortsForName = make(protocolPorts)
			result[fwname] = protocolPortsForName
		}
		portsForProtocol := protocolPortsForName[rule.Protocol]
		portsForProtocol = append(portsForProtocol, rule.PortRange)
		protocolPortsForName[rule.Protocol] = portsForProtocol
	}
	return result
}

func (pp protocolPorts) String() string {
	var sortedProtocols []string
	for protocol := range pp {
		sortedProtocols = append(sortedProtocols, protocol)
	}
	//sort.Strings(sortedProtocols)

	var parts []string
	for protocol := range pp {
		var ports []string
		for _, pr := range pp[protocol] {
			ports = append(ports, pr.String())
		}
		sort.Strings(ports)
		parts = append(parts, strings.Join(ports, ","))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// portStrings returns a list of stringified ports in the set
// for the given protocol.
func (pp protocolPorts) portStrings(protocol string) []string {
	var result []string
	ports := pp[protocol]
	for _, pr := range ports {
		portStr := fmt.Sprintf("%d", pr.FromPort)
		if pr.FromPort != pr.ToPort {
			portStr = fmt.Sprintf("%s-%d", portStr, pr.ToPort)
		}
		result = append(result, portStr)
	}
	return result
}

// union returns a new protocolPorts combining the port
// ranges from both.
func (pp protocolPorts) union(other protocolPorts) protocolPorts {
	result := make(protocolPorts)
	for protocol, ports := range pp {
		result[protocol] = ports
	}
	for protocol, otherPorts := range other {
		resultPorts := result[protocol]
		for _, other := range otherPorts {
			found := false
			for _, myRange := range resultPorts {
				if myRange == other {
					found = true
					break
				}
			}
			if !found {
				resultPorts = append(resultPorts, other)
			}
		}
		result[protocol] = resultPorts
	}
	return result
}

// remove returns a new protocolPorts with the port ranges
// in the specified protocolPorts removed.
func (pp protocolPorts) remove(other protocolPorts) protocolPorts {
	result := make(protocolPorts)
	for protocol, otherPorts := range other {
		myRange, ok := pp[protocol]
		if !ok {
			continue
		}
		var resultRange []network.PortRange
		for _, one := range myRange {
			found := false
			for _, other := range otherPorts {
				if one == other {
					found = true
					break
				}
			}
			if !found {
				resultRange = append(resultRange, one)
			}
		}
		if len(resultRange) > 0 {
			result[protocol] = resultRange
		}
	}
	return result
}
