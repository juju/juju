// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"

	"github.com/joyent/gosdc/cloudapi"
)

const (
	firewallRuleVm = "FROM tag %s TO vm %s ALLOW %s PORT %d"
)

// Helper method to create a firewall rule string for the given machine Id and port
func createFirewallRuleVm(env *joyentEnviron, machineId string, port instance.Port) string {
	return fmt.Sprintf(firewallRuleVm, env.Name(), machineId, strings.ToLower(port.Protocol), port.Number)
}

func (inst *joyentInstance) OpenPorts(machineId string, ports []instance.Port) error {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance", inst.env.Config().FirewallMode())
	}

	fwRules, err := inst.env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return fmt.Errorf("cannot get firewall rules: %v", err)
	}

	machineId = string(inst.Id())
	for _, p := range ports {
		rule := createFirewallRuleVm(inst.env, machineId, p)
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

func (inst *joyentInstance) ClosePorts(machineId string, ports []instance.Port) error {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance", inst.env.Config().FirewallMode())
	}

	fwRules, err := inst.env.compute.cloudapi.ListFirewallRules()
	if err != nil {
		return fmt.Errorf("cannot get firewall rules: %v", err)
	}

	machineId = string(inst.Id())
	for _, p := range ports {
		rule := createFirewallRuleVm(inst.env, machineId, p)
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

func (inst *joyentInstance) Ports(machineId string) ([]instance.Port, error) {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance", inst.env.Config().FirewallMode())
	}

	machineId = string(inst.Id())
	fwRules, err := inst.env.compute.cloudapi.ListMachineFirewallRules(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot get firewall rules: %v", err)
	}

	return getPorts(inst.env, fwRules), nil
}
