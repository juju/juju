// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iptables

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
)

var logger = loggo.GetLogger("juju.network.iptables")

const (
	// iptablesIngressCommand is the comment attached to iptables
	// rules directly related to ingress rules.
	iptablesIngressComment = "juju ingress"

	// iptablesInternalCommand is the comment attached to iptables
	// rules that are not directly related to ingress rules.
	iptablesInternalComment = "juju internal"
)

// DropCommand represents an iptables DROP target command.
type DropCommand struct {
	DestinationAddress string
	Interface          string
}

// Render renders the command to a string which can be executed via
// bash in order to install the iptables rule.
func (c DropCommand) Render() string {
	args := []string{
		"sudo iptables",
		"-I INPUT",
		"-m state --state NEW",
		"-j DROP",
		"-m comment --comment", fmt.Sprintf("'%s'", iptablesInternalComment),
	}
	if c.DestinationAddress != "" {
		args = append(args, "-d", c.DestinationAddress)
	}
	if c.Interface != "" {
		args = append(args, "-i", c.Interface)
	}
	return strings.Join(args, " ")
}

// AcceptInternalCommand represents an iptables ACCEPT target command,
// for accepting traffic, optionally specifying a protocol, destination
// address, and destination port.
//
// This is intended only for allowing traffic according to Juju's internal
// rules, e.g. for API or SSH. This should not be used for managing the
// ingress rules for exposing applications.
type AcceptInternalCommand struct {
	DestinationAddress string
	DestinationPort    int
	Protocol           string
}

// Render renders the command to a string which can be executed via
// bash in order to install the iptables rule.
func (c AcceptInternalCommand) Render() string {
	args := []string{
		"sudo iptables",
		"-I INPUT",
		"-j ACCEPT",
		"-m comment --comment", fmt.Sprintf("'%s'", iptablesInternalComment),
	}
	if c.Protocol != "" {
		args = append(args, "-p", c.Protocol)
	}
	if c.DestinationAddress != "" {
		args = append(args, "-d", c.DestinationAddress)
	}
	if c.DestinationPort > 0 {
		args = append(args, "--dport", fmt.Sprint(c.DestinationPort))
	}
	return strings.Join(args, " ")
}

// IngressRuleCommand represents an iptables ACCEPT target command
// for ingress rules.
type IngressRuleCommand struct {
	Rule               firewall.IngressRule
	DestinationAddress string
	Delete             bool
}

// Render renders the command to a string which can be executed via
// bash in order to install the iptables rule.
func (c IngressRuleCommand) Render() string {
	// TODO(axw) 2017-12-11 #1737472
	// We shouldn't need to check for existing rules;
	// the firewaller is supposed to check the instance's
	// existing rules first, and only insert or remove as
	// needed. Fixing the firewaller is much more difficult,
	// and it really needs an overhaul.
	checkCommand := c.render("-C")
	if c.Delete {
		deleteCommand := c.render("-D")
		return fmt.Sprintf("(%s) && (%s)", checkCommand, deleteCommand)
	}
	insertCommand := c.render("-I")
	return fmt.Sprintf("(%s) || (%s)", checkCommand, insertCommand)
}

func (c IngressRuleCommand) render(commandFlag string) string {
	args := []string{
		"sudo", "iptables",
		commandFlag, "INPUT",
		"-j ACCEPT",
		"-p", c.Rule.PortRange.Protocol,
	}
	if c.DestinationAddress != "" {
		args = append(args, "-d", c.DestinationAddress)
	}
	if c.Rule.PortRange.Protocol == "icmp" {
		args = append(args, "--icmp-type 8")
	} else {
		if c.Rule.PortRange.ToPort-c.Rule.PortRange.FromPort > 0 {
			args = append(args,
				"-m multiport --dports",
				fmt.Sprintf("%d:%d", c.Rule.PortRange.FromPort, c.Rule.PortRange.ToPort),
			)
		} else {
			args = append(args, "--dport", fmt.Sprint(c.Rule.PortRange.FromPort))
		}
	}
	if len(c.Rule.SourceCIDRs) > 0 {
		args = append(args, "-s", strings.Join(c.Rule.SourceCIDRs.SortedValues(), ","))
	}
	// Comment always comes last.
	args = append(args,
		"-m comment --comment", fmt.Sprintf("'%s'", iptablesIngressComment),
	)
	return strings.Join(args, " ")
}

// ParseIngressRules parses the output of "iptables -L INPUT -n",
// extracting previously added ingress rules, as rendered by
// IngressRuleCommand.
func ParseIngressRules(r io.Reader) (firewall.IngressRules, error) {
	var rules firewall.IngressRules
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		rule, ok, err := parseIngressRule(strings.TrimSpace(line))
		if err != nil {
			logger.Warningf("failed to parse iptables line %q: %v", line, err)
			continue
		}
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Annotate(err, "reading iptables output")
	}
	return rules, nil
}

// parseIngressRule parses a single iptables output line, extracting
// an ingress rule if the line represents one, or returning false
// otherwise.
//
// The iptables rules we care about have the following format, and we
// will skip all other rules:
//
//	Chain INPUT (policy ACCEPT)
//	target     prot opt source               destination
//	ACCEPT     tcp  --  0.0.0.0/0            192.168.0.1  multiport dports 3456:3458 /* juju ingress */
//	ACCEPT     tcp  --  0.0.0.0/0            192.168.0.2  tcp dpt:12345 /* juju ingress */
//	ACCEPT     icmp --  0.0.0.0/0            10.0.0.1     icmptype 8 /* juju ingress */
func parseIngressRule(line string) (firewall.IngressRule, bool, error) {
	fail := func(err error) (firewall.IngressRule, bool, error) {
		return firewall.IngressRule{}, false, err
	}
	if !strings.HasPrefix(line, "ACCEPT") {
		return firewall.IngressRule{}, false, nil
	}

	// We only care about rules with the comment "juju ingress".
	if !strings.HasSuffix(line, "*/") {
		return firewall.IngressRule{}, false, nil
	}
	commentStart := strings.LastIndex(line, "/*")
	if commentStart == -1 {
		return firewall.IngressRule{}, false, nil
	}
	line, comment := line[:commentStart], line[commentStart+2:]
	comment = comment[:len(comment)-2]
	if strings.TrimSpace(comment) != iptablesIngressComment {
		return firewall.IngressRule{}, false, nil
	}

	const (
		fieldTarget      = 0
		fieldProtocol    = 1
		fieldOptions     = 2
		fieldSource      = 3
		fieldDestination = 4
	)
	fields := make([]string, 5)
	for i := range fields {
		field, remainder, ok := popField(line)
		if !ok {
			return fail(errors.Errorf("could not extract field %d", i))
		}
		fields[i] = field
		line = remainder
	}

	source := fields[fieldSource]
	proto := strings.ToLower(fields[fieldProtocol])

	var fromPort, toPort int
	if strings.HasPrefix(line, "multiport dports") {
		_, line, _ = popField(line) // pop "multiport"
		_, line, _ = popField(line) // pop "dports"
		portRange, _, ok := popField(line)
		if !ok {
			return fail(errors.New("could not extract port range"))
		}
		var err error
		fromPort, toPort, err = parsePortRange(portRange)
		if err != nil {
			return fail(errors.Trace(err))
		}
	} else if proto == "icmp" {
		fromPort, toPort = -1, -1
	} else {
		field, line, ok := popField(line)
		if !ok {
			return fail(errors.New("could not extract parameters"))
		}
		if field != proto {
			// parameters should look like
			// "tcp dpt:N" or "udp dpt:N".
			return fail(errors.New("unexpected parameter prefix"))
		}
		field, _, ok = popField(line)
		if !ok || !strings.HasPrefix(field, "dpt:") {
			return fail(errors.New("could not extract destination port"))
		}
		port, err := parsePort(strings.TrimPrefix(field, "dpt:"))
		if err != nil {
			return fail(errors.Trace(err))
		}
		fromPort = port
		toPort = port
	}

	rule := firewall.NewIngressRule(network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}, source)
	if err := rule.Validate(); err != nil {
		return fail(errors.Trace(err))
	}
	return rule, true, nil
}

// popField pops a pops a field off the front of the given string
// by splitting on the first run of whitespace, and returns the
// field and remainder. A boolean result is returned indicating
// whether or not a field was found.
func popField(s string) (field, remainder string, ok bool) {
	i := strings.IndexRune(s, ' ')
	if i == -1 {
		return s, "", s != ""
	}
	field, remainder = s[:i], strings.TrimLeft(s[i+1:], " ")
	return field, remainder, true
}

func parsePortRange(s string) (int, int, error) {
	fields := strings.Split(s, ":")
	if len(fields) != 2 {
		return -1, -1, errors.New("expected M:N")
	}
	from, err := parsePort(fields[0])
	if err != nil {
		return -1, -1, errors.Trace(err)
	}
	to, err := parsePort(fields[1])
	if err != nil {
		return -1, -1, errors.Trace(err)
	}
	return from, to, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return -1, errors.Trace(err)
	}
	return int(n), nil
}
