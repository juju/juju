// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"

	"github.com/joyent/gosdc/cloudapi"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

const (
	firewallRuleVm = "FROM tag %s TO vm %s ALLOW %s %s"
)

// Helper method to create a firewall rule string for the given machine Id and port
func createFirewallRuleVm(envName string, machineId string, rule firewall.IngressRule) string {
	ports := []string{}
	for p := rule.PortRange.FromPort; p <= rule.PortRange.ToPort; p++ {
		ports = append(ports, fmt.Sprintf("PORT %d", p))
	}
	var portList string
	if len(ports) > 1 {
		portList = fmt.Sprintf("( %s )", strings.Join(ports, " AND "))
	} else if len(ports) == 1 {
		portList = ports[0]
	}
	return fmt.Sprintf(firewallRuleVm, envName, machineId, strings.ToLower(rule.PortRange.Protocol), portList)
}

func (inst *joyentInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
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

func (inst *joyentInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
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

func (inst *joyentInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	if inst.env.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ingress rules from instance", inst.env.Config().FirewallMode())
	}

	machineId = string(inst.Id())
	fwRules, err := inst.env.compute.cloudapi.ListMachineFirewallRules(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot get firewall rules: %v", err)
	}

	return getRules(inst.env.Config().Name(), fwRules)
}
