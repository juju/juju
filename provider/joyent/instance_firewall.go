// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"

	"github.com/joyent/gosdc/cloudapi"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
)

const (
	firewallRuleVm = "FROM tag %s TO vm %s ALLOW %s %s"
)

// Helper method to create a firewall rule string for the given machine Id and port
func createFirewallRuleVm(envName string, machineId string, portRange network.PortRange) string {
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
	return fmt.Sprintf(firewallRuleVm, envName, machineId, strings.ToLower(portRange.Protocol), portList)
}

func (inst *joyentInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance", inst.env.Config().FirewallMode())
	}

	fwRules, err := inst.env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return fmt.Errorf("cannot get firewall rules: %v", err)
	}

	machineId = string(inst.Id())
	for _, p := range ports {
		rule := createFirewallRuleVm(inst.env.Config().Name(), machineId, p)
		if e, id := ruleExists(fwRules, rule); e {
			_, err := inst.env.compute.cloudapi.EnableFirewallRule(id)
			if err != nil {
				return fmt.Errorf("couldn't enable rule %s: %v", rule, err)
			}
		} else {
			_, err := inst.env.compute.cloudapi.CreateFirewallRule(cloudapi.CreateFwRuleOpts{
				Enabled: true,
				Rule:    rule,
			})
			if err != nil {
				return fmt.Errorf("couldn't create rule %s: %v", rule, err)
			}
		}
	}

	logger.Infof("ports %v opened for instance %q", ports, machineId)

	return nil
}

func (inst *joyentInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance", inst.env.Config().FirewallMode())
	}

	fwRules, err := inst.env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return fmt.Errorf("cannot get firewall rules: %v", err)
	}

	machineId = string(inst.Id())
	for _, p := range ports {
		rule := createFirewallRuleVm(inst.env.Config().Name(), machineId, p)
		if e, id := ruleExists(fwRules, rule); e {
			_, err := inst.env.compute.cloudapi.DisableFirewallRule(id)
			if err != nil {
				return fmt.Errorf("couldn't disable rule %s: %v", rule, err)
			}
		} else {
			_, err := inst.env.compute.cloudapi.CreateFirewallRule(cloudapi.CreateFwRuleOpts{
				Enabled: false,
				Rule:    rule,
			})
			if err != nil {
				return fmt.Errorf("couldn't create rule %s: %v", rule, err)
			}
		}
	}

	logger.Infof("ports %v closed for instance %q", ports, machineId)

	return nil
}

func (inst *joyentInstance) Ports(machineId string) ([]network.PortRange, error) {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance", inst.env.Config().FirewallMode())
	}

	machineId = string(inst.Id())
	fwRules, err := inst.env.compute.cloudapi.ListMachineFirewallRules(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot get firewall rules: %v", err)
	}

	return getPorts(inst.env.Config().Name(), fwRules), nil
}
