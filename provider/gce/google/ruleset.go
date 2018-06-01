// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/network"
)

// ruleSet is used to manipulate port ranges for a collection of
// firewall rules or ingress rules. Each key is the identifier for a
// set of source CIDRs that are allowed for a set of port ranges.
type ruleSet map[string]*firewall

func newRuleSetFromRules(rules ...network.IngressRule) ruleSet {
	result := make(ruleSet)
	for _, rule := range rules {
		result.addRule(rule)
	}
	return result
}

func (rs ruleSet) addRule(rule network.IngressRule) {
	sourceCIDRs := rule.SourceCIDRs
	if len(sourceCIDRs) == 0 {
		sourceCIDRs = []string{"0.0.0.0/0"}
	}
	key := sourcecidrs(sourceCIDRs).key()
	fw, ok := rs[key]
	if !ok {
		fw = &firewall{
			SourceCIDRs:  sourceCIDRs,
			AllowedPorts: make(protocolPorts),
		}
		rs[key] = fw
	}
	ports := fw.AllowedPorts
	ports[rule.Protocol] = append(ports[rule.Protocol], rule.PortRange)
}

func newRuleSetFromFirewalls(firewalls ...*compute.Firewall) (ruleSet, error) {
	result := make(ruleSet)
	for _, firewall := range firewalls {
		err := result.addFirewall(firewall)
		if err != nil {
			return result, errors.Trace(err)
		}
	}
	return result, nil
}

func (rs ruleSet) addFirewall(fw *compute.Firewall) error {
	if len(fw.TargetTags) != 1 {
		return errors.Errorf(
			"firewall rule %q has %d targets (expected 1): %#v",
			fw.Name,
			len(fw.TargetTags),
			fw.TargetTags,
		)
	}
	sourceRanges := fw.SourceRanges
	if len(sourceRanges) == 0 {
		sourceRanges = []string{"0.0.0.0/0"}
	}
	key := sourcecidrs(sourceRanges).key()
	result := &firewall{
		Name:         fw.Name,
		Target:       fw.TargetTags[0],
		SourceCIDRs:  sourceRanges,
		AllowedPorts: make(protocolPorts),
	}
	for _, allowed := range fw.Allowed {
		ranges := make([]network.PortRange, len(allowed.Ports))
		for i, rangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRange(rangeStr)
			if err != nil {
				return errors.Trace(err)
			}
			portRange.Protocol = allowed.IPProtocol
			ranges[i] = portRange
		}
		p := result.AllowedPorts
		p[allowed.IPProtocol] = append(p[allowed.IPProtocol], ranges...)
	}
	for protocol, ranges := range result.AllowedPorts {
		result.AllowedPorts[protocol] = network.CombinePortRanges(ranges...)
	}
	if other, ok := rs[key]; ok {
		return errors.Errorf(
			"duplicate firewall rules found matching CIDRs %#v: %q and %q",
			fw.SourceRanges,
			fw.Name,
			other.Name,
		)
	}
	rs[key] = result
	return nil
}

func (rs ruleSet) matchProtocolPorts(ports protocolPorts) (*firewall, bool) {
	for _, fw := range rs {
		if fw.AllowedPorts.String() == ports.String() {
			return fw, true
		}
	}
	return nil, false
}

func (rs ruleSet) matchSourceCIDRs(cidrs []string) (*firewall, bool) {
	result, ok := rs[sourcecidrs(cidrs).key()]
	return result, ok
}

// ToIngressRules converts this set of firewall rules to the ingress
// rules used elsewhere in Juju. This conversion throws away the rule
// name information, so these ingress rules can't be directly related
// back to the firewall rules they came from (except by matching
// source CIDRs and ports).
func (rs ruleSet) toIngressRules() ([]network.IngressRule, error) {
	var results []network.IngressRule
	for _, fw := range rs {
		rules, err := fw.toIngressRules()
		if err != nil {
			return nil, errors.Trace(err)
		}
		results = append(results, rules...)
	}
	network.SortIngressRules(results)
	return results, nil
}

func (rs ruleSet) allNames() set.Strings {
	result := set.NewStrings()
	for _, fw := range rs {
		result.Add(fw.Name)
	}
	return result
}

// sourcecidrs is used to calculate a unique key for a collection of
// cidrs.
type sourcecidrs []string

func (s sourcecidrs) key() string {
	src := strings.Join(s.sorted(), ",")
	hash := sha256.New()
	hash.Write([]byte(src))
	hashStr := fmt.Sprintf("%x", hash.Sum(nil))
	return hashStr[:10]
}

func (s sourcecidrs) sorted() []string {
	values := make([]string, len(s))
	copy(values, s)
	sort.Strings(values)
	return values
}

// firewall represents a GCE firewall - if it was constructed from a
// set of ingress rules the name and target information won't be
// populated.
type firewall struct {
	Name         string
	Target       string
	SourceCIDRs  []string
	AllowedPorts protocolPorts
}

func (fw *firewall) toIngressRules() ([]network.IngressRule, error) {
	var results []network.IngressRule
	for _, portRanges := range fw.AllowedPorts {
		for _, p := range portRanges {
			rule, err := network.NewIngressRule(p.Protocol, p.FromPort, p.ToPort, fw.SourceCIDRs...)
			if err != nil {
				return nil, errors.Trace(err)
			}
			results = append(results, rule)
		}
	}
	return results, nil
}

// protocolPorts maps a protocol eg "tcp" to a collection of
// port ranges for that protocol.
type protocolPorts map[string][]network.PortRange

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
