// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/core/network"
)

// AllNetworksIPV4CIDR represents the zero address (quad-zero) CIDR for an IPV4
// network.
const AllNetworksIPV4CIDR = "0.0.0.0/0"

// AllNetworksIPV6CIDR represents the zero address (quad-zero) CIDR for an IPV6
// network.
const AllNetworksIPV6CIDR = "::/0"

// IngressRule represents a rule for allowing traffic from a set of source
// CIDRs to reach a particular port range.
type IngressRule struct {
	// The destination port range for the incoming traffic.
	PortRange network.PortRange

	// A set of CIDRs that describe the origin for incoming traffic. An
	// implicit 0.0.0.0/0 CIDR is assumed if no CIDRs are specified.
	SourceCIDRs set.Strings
}

// NewIngressRule creates a new IngressRule for allowing access to portRange
// from the list of sourceCIDRs. If no sourceCIDRs are specified, the rule
// will implicitly apply to all networks.
func NewIngressRule(portRange network.PortRange, sourceCIDRs ...string) IngressRule {
	return IngressRule{
		PortRange:   portRange,
		SourceCIDRs: set.NewStrings(sourceCIDRs...),
	}
}

// Validate ensures that the ingress rule contains valid source and destination
// parameters.
func (r IngressRule) Validate() error {
	if err := r.PortRange.Validate(); err != nil {
		return errors.Annotatef(err, "invalid destination for ingress rule")
	}

	for srcCIDR := range r.SourceCIDRs {
		if _, _, err := net.ParseCIDR(srcCIDR); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// String is the string representation of IngressRule.
func (r IngressRule) String() string {
	var buf bytes.Buffer
	_, _ = fmt.Fprint(&buf, r.PortRange.String())

	src := strings.Join(r.SourceCIDRs.SortedValues(), ",")
	if src != "" && src != AllNetworksIPV4CIDR && src != AllNetworksIPV6CIDR {
		_, _ = fmt.Fprintf(&buf, " from %s", src)
	}
	return buf.String()
}

// LessThan compares two IngressRule instances for equality.
func (r IngressRule) LessThan(other IngressRule) bool {
	// Check dst port ranges first.
	if r.PortRange != other.PortRange {
		return r.PortRange.LessThan(other.PortRange)
	}

	// Compare the source CIDRs. NOTE(achilleasa) this retains the
	// original behavior of the code moved out of the network package.
	thisSrc := strings.Join(r.SourceCIDRs.SortedValues(), ",")
	otherSrc := strings.Join(other.SourceCIDRs.SortedValues(), ",")
	return thisSrc < otherSrc
}

// IngressRules represents a collection of IngressRule instances.
type IngressRules []IngressRule

// Sort the rule list by port range and then by source CIDRs.
func (rules IngressRules) Sort() {
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].LessThan(rules[j])
	})
}

// Validate the list of ingress rules
func (rules IngressRules) Validate() error {
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	return nil
}
