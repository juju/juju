// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/joyent/gosdc/cloudapi"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
)

const (
	firewallRuleAll = "FROM tag %s TO tag juju ALLOW %s %s"
)

var (
	firewallSinglePortRule = regexp.MustCompile("FROM tag [a-z0-9 \\-]+ TO (?:tag|vm) [a-z0-9 \\-]+ ALLOW (?P<protocol>[a-z]+) PORT (?P<port>[0-9]+)")
	firewallMultiPortRule  = regexp.MustCompile("FROM tag [a-z0-9 \\-]+ TO (?:tag|vm) [a-z0-9 \\-]+ ALLOW (?P<protocol>[a-z]+) \\(\\s*(?P<ports>PORT [0-9]+(?: AND PORT [0-9]+)*)\\s*\\)")
)

// Helper method to create a firewall rule string for the given port
func createFirewallRuleAll(env *joyentEnviron, portRange network.PortRange) string {
	ports := []string{}
	for p := portRange.FromPort; p <= portRange.ToPort; p++ {
		ports = append(ports, fmt.Sprintf("PORT %d", p))
	}
	var portList string
	if len(ports) > 1 {
		portList = fmt.Sprintf("( %s )", strings.Join(ports, " AND "))
	} else if len(ports) == 1 {
		portList = ports[0]
	}
	return fmt.Sprintf(firewallRuleAll, env.Config().Name(), strings.ToLower(portRange.Protocol), portList)
}

// Helper method to check if a firewall rule string already exist
func ruleExists(rules []cloudapi.FirewallRule, rule string) (bool, string) {
	for _, r := range rules {
		if strings.EqualFold(r.Rule, rule) {
			return true, r.Id
		}
	}

	return false, ""
}

// Helper method to get port from the given firewall rules
func getPorts(envName string, rules []cloudapi.FirewallRule) []network.PortRange {
	ports := []network.PortRange{}
	for _, r := range rules {
		rule := r.Rule
		if r.Enabled && strings.HasPrefix(rule, "FROM tag "+envName) && strings.Contains(rule, "PORT") {
			if firewallSinglePortRule.MatchString(rule) {
				parts := firewallSinglePortRule.FindStringSubmatch(rule)
				if len(parts) != 3 {
					continue
				}
				protocol := parts[1]
				n, _ := strconv.Atoi(parts[2])
				ports = append(ports, network.PortRange{Protocol: protocol, FromPort: n, ToPort: n})
			} else if firewallMultiPortRule.MatchString(rule) {
				parts := firewallMultiPortRule.FindStringSubmatch(rule)
				if len(parts) != 3 {
					continue
				}
				protocol := parts[1]
				portStrings := strings.Split(parts[2], " AND ")
				pFrom := 0
				pTo := 0
				// parse multiple ports into port ranges
				// so PORT 20 AND PORT 21 becomes a port range {20-21}
				for _, portString := range portStrings {
					portString = portString[strings.LastIndex(portString, "PORT")+5:]
					port, _ := strconv.Atoi(portString)
					if pFrom == 0 {
						// starting new port range
						pFrom = port
						pTo = port
					} else if port == pTo+1 {
						// continuing port range
						pTo = port
					} else {
						// port range broken
						ports = append(ports, network.PortRange{Protocol: protocol, FromPort: pFrom, ToPort: pTo})
						pFrom = port
						pTo = port
					}
				}
				if pFrom != 0 {
					ports = append(ports, network.PortRange{Protocol: protocol, FromPort: pFrom, ToPort: pTo})
				}

			}
		}
	}

	network.SortPortRanges(ports)
	return ports
}

func (env *joyentEnviron) OpenPorts(ports []network.PortRange) error {
	if env.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on environment", env.Config().FirewallMode())
	}

	fwRules, err := env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return fmt.Errorf("cannot get firewall rules: %v", err)
	}

	for _, p := range ports {
		rule := createFirewallRuleAll(env, p)
		if e, id := ruleExists(fwRules, rule); e {
			_, err := env.compute.cloudapi.EnableFirewallRule(id)
			if err != nil {
				return fmt.Errorf("couldn't enable rule %s: %v", rule, err)
			}
		} else {
			_, err := env.compute.cloudapi.CreateFirewallRule(cloudapi.CreateFwRuleOpts{
				Enabled: true,
				Rule:    rule,
			})
			if err != nil {
				return fmt.Errorf("couldn't create rule %s: %v", rule, err)
			}
		}
	}

	logger.Infof("ports %v opened in environment", ports)

	return nil
}

func (env *joyentEnviron) ClosePorts(ports []network.PortRange) error {
	if env.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on environment", env.Config().FirewallMode())
	}

	fwRules, err := env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return fmt.Errorf("cannot get firewall rules: %v", err)
	}

	for _, p := range ports {
		rule := createFirewallRuleAll(env, p)
		if e, id := ruleExists(fwRules, rule); e {
			_, err := env.compute.cloudapi.DisableFirewallRule(id)
			if err != nil {
				return fmt.Errorf("couldn't disable rule %s: %v", rule, err)
			}
		} else {
			_, err := env.compute.cloudapi.CreateFirewallRule(cloudapi.CreateFwRuleOpts{
				Enabled: false,
				Rule:    rule,
			})
			if err != nil {
				return fmt.Errorf("couldn't create rule %s: %v", rule, err)
			}
		}
	}

	logger.Infof("ports %v closed in environment", ports)

	return nil
}

func (env *joyentEnviron) Ports() ([]network.PortRange, error) {
	if env.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from environment", env.Config().FirewallMode())
	}

	fwRules, err := env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return nil, fmt.Errorf("cannot get firewall rules: %v", err)
	}

	return getPorts(env.Config().Name(), fwRules), nil
}
