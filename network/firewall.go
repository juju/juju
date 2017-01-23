// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
)

// IngressRule represents a range of ports and sources
// from which to allow ingress by incoming packets.
type IngressRule struct {
	// PortRange is the range of ports for which incoming
	// packets are allowed.
	PortRange

	// SourceCIDRs is a list of IP address blocks expressed in CIDR format
	// to which this rule applies.
	SourceCIDRs []string
}

// NewIngressRule returns an IngressRule for the specified port
// range. If no explicit source ranges are specified, there is no
// restriction from where incoming traffic originates.
func NewIngressRule(protocol string, from, to int, sourceCIDRs ...string) (IngressRule, error) {
	rule := IngressRule{
		PortRange: PortRange{
			Protocol: protocol,
			FromPort: from,
			ToPort:   to,
		},
	}
	for _, cidr := range sourceCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return IngressRule{}, errors.Trace(err)
		}
	}
	if len(sourceCIDRs) > 0 {
		rule.SourceCIDRs = sourceCIDRs
	}
	return rule, nil
}

// MustNewIngressRule returns an IngressRule for the specified port
// range. If no explicit source ranges are specified, there is no
// restriction from where incoming traffic originates.
// The method will panic if there is an error.
func MustNewIngressRule(protocol string, from, to int, sourceCIDRs ...string) IngressRule {
	rule, err := NewIngressRule(protocol, from, to, sourceCIDRs...)
	if err != nil {
		panic(err)
	}
	return rule
}

// NewOpenIngressRule returns an IngressRule for the specified port
// range. There is no restriction from where incoming traffic originates.
func NewOpenIngressRule(protocol string, from, to int) IngressRule {
	rule, _ := NewIngressRule(protocol, from, to)
	return rule
}

// String is the string representation of IngressRule.
func (r IngressRule) String() string {
	source := ""
	from := strings.Join(r.SourceCIDRs, ",")
	if from != "" && from != "0.0.0.0/0" {
		source = " from " + from
	}
	if r.FromPort == r.ToPort {
		return fmt.Sprintf("%d/%s%s", r.FromPort, strings.ToLower(r.Protocol), source)
	}
	return fmt.Sprintf("%d-%d/%s%s", r.FromPort, r.ToPort, strings.ToLower(r.Protocol), source)
}

// GoString is used to print values passed as an operand to a %#v format.
func (r IngressRule) GoString() string {
	return r.String()
}

type IngressRuleSlice []IngressRule

func (p IngressRuleSlice) Len() int      { return len(p) }
func (p IngressRuleSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p IngressRuleSlice) Less(i, j int) bool {
	p1 := p[i]
	p2 := p[j]
	if p1.Protocol != p2.Protocol {
		return p1.Protocol < p2.Protocol
	}
	if p1.FromPort != p2.FromPort {
		return p1.FromPort < p2.FromPort
	}
	if p1.ToPort != p2.ToPort {
		return p1.ToPort < p2.ToPort
	}
	s1 := strings.Join(p1.SourceCIDRs, ",")
	s2 := strings.Join(p2.SourceCIDRs, ",")
	return s1 < s2
}

// SortIngressRules sorts the given rules, first by protocol, then by ports.
func SortIngressRules(IngressRules []IngressRule) {
	sort.Sort(IngressRuleSlice(IngressRules))
}

// RulesFromPortRanges returns a slice of IngressRules
// corresponding to the specified port ranges, each rule
// having no source ranges respecified.
func RulesFromPortRanges(ports ...PortRange) []IngressRule {
	if ports == nil {
		return nil
	}
	rules := make([]IngressRule, len(ports))
	for i, p := range ports {
		// Since there are no source CIDRs, err will be nil.
		rules[i], _ = NewIngressRule(p.Protocol, p.FromPort, p.ToPort)
	}
	return rules
}

// PortRangesFromRules returns a slice of port ranges for the
// specified ingress rules, ignoring the source ranges.
// NB this method is used by the firewaller worker until updates
// are made to cater for source ranges in firewall rules.
func PortRangesFromRules(rules []IngressRule) []PortRange {
	if rules == nil {
		return nil
	}
	ports := make([]PortRange, len(rules))
	for i, r := range rules {
		ports[i] = PortRange{Protocol: r.Protocol, FromPort: r.FromPort, ToPort: r.ToPort}
	}
	return ports
}
