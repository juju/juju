// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/network"
)

// RuleSet is used to manipulate port ranges for a collection of
// IngressRules. Each key is the identifier for a set of source CIDRs
// that are allowed for a set of port ranges.
type RuleSet map[string]*Firewall

func NewRuleSetFromRules(rules ...network.IngressRule) RuleSet {
	result := make(RuleSet)
	for _, rule := range rules {
		result.addRule(rule)
	}
	return result
}

func (rs RuleSet) addRule(rule network.IngressRule) {
	key := sourcecidrs(rule.SourceCIDRs).key()
	fw, ok := rs[key]
	if !ok {
		fw = &Firewall{
			SourceCIDRs:  rule.SourceCIDRs,
			AllowedPorts: make(protocolPorts),
		}
		rs[key] = fw
	}
	ports := fw.AllowedPorts
	ports[rule.Protocol] = append(ports[rule.Protocol], rule.PortRange)
}

func NewRulesetFromFirewalls(firewalls ...*compute.Firewall) (RuleSet, error) {
	result := make(RuleSet)
	for _, firewall := range firewalls {
		err := result.addFirewall(firewall)
		if err != nil {
			return result, errors.Trace(err)
		}
	}
	return result, nil
}

func (rs RuleSet) addFirewall(firewall *compute.Firewall) error {
	key := sourcecidrs(firewall.SourceRanges).key()
	if len(firewall.TargetTags) != 1 {
		return errors.Errorf(
			"firewall rule %q has %d targets (expected 1): %#v",
			firewall.Name,
			len(firewall.TargetTags),
			firewall.TargetTags,
		)
	}
	sourceRanges := firewall.SourceRanges
	if len(sourceRanges) == 0 {
		sourceRanges = []string{"0.0.0.0/0"}
	}
	result := &Firewall{
		Name:         firewall.Name,
		Target:       firewall.TargetTags[0],
		SourceCIDRs:  sourceRanges,
		AllowedPorts: make(protocolPorts),
	}
	for _, allowed := range firewall.Allowed {
		ranges := make([]network.PortRange, len(allowed.Ports))
		for i, rangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRange(rangeStr)
			if err != nil {
				return errors.Trace(err)
			}
			portRange.Protocol = allowed.IPProtocol
			ranges[i] = portRange
		}
		result.AllowedPorts[allowed.IPProtocol] = network.CombinePortRanges(ranges...)
	}
	if other, ok := rs[key]; ok {
		return errors.Errorf(
			"duplicate firewall rules found matching CIDRs %#v: %q and %q",
			firewall.SourceRanges,
			firewall.Name,
			other.Name,
		)
	}
	rs[key] = result
	return nil
}

func (rs RuleSet) matchProtocolPorts(ports protocolPorts) (*Firewall, bool) {
	for _, firewall := range rs {
		if firewall.AllowedPorts.String() == ports.String() {
			return firewall, true
		}
	}
	return nil, false
}

func (rs RuleSet) matchSourceCIDRs(cidrs []string) (*Firewall, bool) {
	result, ok := rs[sourcecidrs(cidrs).key()]
	return result, ok
}

// ToIngressRules converts this set of firewall rules to the ingress
// rules used elsewhere in Juju. This conversion throws away the rule
// name information, so these ingress rules can't be directly related
// back to the firewall rules they came from (except by matching
// source CIDRs and ports).
func (rs RuleSet) toIngressRules() ([]network.IngressRule, error) {
	var results []network.IngressRule
	for _, firewall := range rs {
		for _, portRanges := range firewall.AllowedPorts {
			for _, p := range portRanges {
				rule, err := network.NewIngressRule(p.Protocol, p.FromPort, p.ToPort, firewall.SourceCIDRs...)
				if err != nil {
					return nil, errors.Trace(err)
				}
				results = append(results, rule)
			}
		}
	}
	return results, nil
}

// sourcecidrs is used to calculate a unique key for a collection of
// cidrs.
type sourcecidrs []string

func (s sourcecidrs) key() string {
	src := strings.Join(s, ",")
	hash := sha256.New()
	hash.Write([]byte(src))
	hashStr := fmt.Sprintf("%x", hash.Sum(nil))
	return hashStr[:6]
}

// Firewall represents a GCE firewall - if it was constructed from a
// set of ingress rules the name and target information won't be
// populated.
type Firewall struct {
	Name         string
	Target       string
	SourceCIDRs  []string
	AllowedPorts protocolPorts
}

// protocolPorts maps a protocol eg "tcp" to a collection of
// port ranges for that protocol.
type protocolPorts map[string][]network.PortRange

func randomSuffix(f *Firewall) (string, error) {
	// For backwards compatibility, open rules for "0.0.0.0/0"
	// do not use any suffix in the name.
	if len(f.SourceCIDRs) == 0 || len(f.SourceCIDRs) == 1 && f.SourceCIDRs[0] == "0.0.0.0/0" {
		return "", nil
	}
	data := make([]byte, 4)
	_, err := rand.Read(data)
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("-%x", data), nil
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
