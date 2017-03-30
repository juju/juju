// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/utils"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	commonProvider "github.com/juju/juju/provider/oracle/common"
)

// Firewaller exposes methods for managing network ports.
// Exacly like the environ.Firewaller but with additional functionality
type Firewaller interface {
	// embedd the envrions.Firewaller interface
	// to make network Firewaller implementation also
	// implement the environ one and provide aditional
	// functionality for the oracle cloud provider
	environs.Firewaller

	// Return all machine ingress rules for a given machine id
	MachineIngressRules(id string) ([]network.IngressRule, error)

	// OpenPortsOnInstance will open ports on the given machine id instance and
	// adds the firewall rules provided as ingress networking rules
	OpenPortsOnInstance(machineId string, rules []network.IngressRule) error

	// ClosePortsOnInstnace will close ports on the given machine id instance with
	// the given firewall rules provided as ingress networking rules
	ClosePortsOnInstance(machineId string, rules []network.IngressRule) error

	// CreateMachineSecLists creates a security list inside the cloud
	// attaching the given machine id and the port
	CreateMachineSecLists(id string, port int) ([]string, error)

	// DeleteMachineSecList will delete the security list on the given machine
	// id
	DeleteMachineSecList(id string) error

	// CreateDefaultACLAndRules will create a acl rules on the defaul cloud
	// settings on the given machine id
	CreateDefaultACLAndRules(id string) (response.Acl, error)

	// RemoveACLAndRules will remove all acl rules on a given machine id
	RemoveACLAndRules(id string) error
}

var _ Firewaller = (*Firewall)(nil)

// FirewallerAPI abstration to proivde all sets of
// api operations, joining them to a group behaviour api
// that acts like a firewall
type FirewallerAPI interface {
	commonProvider.Composer
	commonProvider.RulesAPI
	commonProvider.AclAPI
	commonProvider.SecIpAPI
	commonProvider.IpAddressPrefixSetAPI
	commonProvider.SecListAPI
	commonProvider.ApplicationsAPI
	commonProvider.SecRulesAPI
}

// Firewall exposes methods for mapping network ports.
// This type implement the environ.Firewaller
type Firewall struct {
	// environ is the current oracle cloud environment
	// this will use to acces the underlying config
	environ environs.ConfigGetter
	// client is used to make operations on the oracle provider
	client FirewallerAPI
}

// NewFirewall returns a new firewall that can do network operation
// such as closing and opening ports inside the oracle cloud environmnet
func NewFirewall(cfg environs.ConfigGetter, client FirewallerAPI) *Firewall {
	return &Firewall{
		environ: cfg,
		client:  client,
	}
}

// OpenPorts can open ports given all ingress rules unde the global group name
func (f Firewall) OpenPorts(rules []network.IngressRule) error {
	mode := f.environ.Config().FirewallMode()
	if mode != config.FwGlobal {
		return fmt.Errorf(
			"invalid firewall mode %q for opening ports on model",
			mode,
		)
	}

	globalGroupName := f.globalGroupName()
	seclist, err := f.ensureSecList(f.client.ComposeName(globalGroupName))
	if err != nil {
		return errors.Trace(err)
	}
	err = f.ensureSecRules(seclist, rules)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ClosePorts can close the ports on the give ingress rules
func (f Firewall) ClosePorts(rules []network.IngressRule) error {
	groupName := f.globalGroupName()
	return f.closePortsOnList(f.client.ComposeName(groupName), rules)
}

// IngressRules returns the ingress rules applied to the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func (f Firewall) IngressRules() ([]network.IngressRule, error) {
	return f.GlobalIngressRules()
}

// MachineIngressRules returns all ingress rules that are in the machine group
// name from the oracle cloud account.
// If there are not any ingress rules under that name scope this will
// return nil, nil
func (f Firewall) MachineIngressRules(machineId string) ([]network.IngressRule, error) {
	seclist := f.machineGroupName(machineId)
	return f.getIngressRules(f.client.ComposeName(seclist))
}

// OpenPortsOnInstance will open ports on a given instance with the id
// and the ingress rules attached
func (f Firewall) OpenPortsOnInstance(machineId string, rules []network.IngressRule) error {
	machineGroup := f.machineGroupName(machineId)
	seclist, err := f.ensureSecList(f.client.ComposeName(machineGroup))
	if err != nil {
		return errors.Trace(err)
	}
	err = f.ensureSecRules(seclist, rules)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ClosePortsOnInstance close all ports on the the instance
// given the machineId and the ingress rules that are defined
func (f Firewall) ClosePortsOnInstance(machineId string, rules []network.IngressRule) error {
	// fetch the group name based on the machine id provided
	groupName := f.machineGroupName(machineId)
	return f.closePortsOnList(f.client.ComposeName(groupName), rules)
}

// CreateMachineSecLists will create all security lists and attach them
// to the instance with the id
func (f Firewall) CreateMachineSecLists(machineId string, apiPort int) ([]string, error) {
	defaultSecList, err := f.createDefaultGroupAndRules(apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	name := f.machineGroupName(machineId)
	resourceName := f.client.ComposeName(name)
	secList, err := f.ensureSecList(resourceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []string{
		defaultSecList.Name,
		secList.Name,
	}, nil
}

// DeleteMachineSecList will delete all security lists on the give
// machine id. If there's some associations to the give instance still
// this will not delete them.
func (f Firewall) DeleteMachineSecList(machineId string) error {
	listName := f.machineGroupName(machineId)
	globalListName := f.globalGroupName()
	err := f.maybeDeleteList(f.client.ComposeName(listName))
	if err != nil {
		return errors.Trace(err)
	}
	// check if we can delete the global list as well
	err = f.maybeDeleteList(f.client.ComposeName(globalListName))
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// CreateDefaultACLAndRules creates default ACL and rules for IP networks attached to
// units.
// NOTE (gsamfira): For now we apply an allow all on these ACLs. Traffic will be cloud-only
// between instances connected to the same ip network exchange (the equivalent of a space)
// There will be no public IP associated to interfaces connected to IP networks, so only
// instances connected to the same network, or a network managed by the same space will
// be able to connect. This will ensure that peers and units entering a relationship can connect
// to services deployed by a particular unit, without having to expose the application.
func (f Firewall) CreateDefaultACLAndRules(machineId string) (response.Acl, error) {
	var details response.Acl
	var err error
	description := fmt.Sprintf("ACL for machine %s", machineId)
	groupName := f.machineGroupName(machineId)
	resourceName := f.client.ComposeName(groupName)
	if err != nil {
		return response.Acl{}, err
	}
	rules := []api.SecurityRuleParams{
		api.SecurityRuleParams{
			Name:                   fmt.Sprintf("%s-allow-ingress", resourceName),
			Description:            "Allow all ingress",
			FlowDirection:          common.Ingress,
			EnabledFlag:            true,
			DstIpAddressPrefixSets: []string{},
			SecProtocols:           []string{},
			SrcIpAddressPrefixSets: []string{},
		},
		api.SecurityRuleParams{
			Name:                   fmt.Sprintf("%s-allow-egress", resourceName),
			Description:            "Allow all egress",
			FlowDirection:          common.Egress,
			EnabledFlag:            true,
			DstIpAddressPrefixSets: []string{},
			SecProtocols:           []string{},
			SrcIpAddressPrefixSets: []string{},
		},
	}
	details, err = f.client.AclDetails(resourceName)
	if err != nil {
		if api.IsNotFound(err) {
			details, err = f.client.CreateAcl(resourceName, description, true, nil)
			if err != nil {
				return response.Acl{}, errors.Trace(err)
			}
		} else {
			return response.Acl{}, errors.Trace(err)
		}
	}
	aclRules, err := f.getAllSecurityRules(details.Name)
	if err != nil {
		return response.Acl{}, errors.Trace(err)
	}

	var toAdd []api.SecurityRuleParams

	for _, val := range rules {
		found := false
		val.Acl = details.Name
		newRuleAsStub := f.convertSecurityRuleParamsToStub(val)
		for _, existing := range aclRules {
			existingRulesAsStub := f.convertSecurityRuleToStub(existing)
			if reflect.DeepEqual(existingRulesAsStub, newRuleAsStub) {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, val)
		}
	}
	for _, val := range toAdd {
		_, err := f.client.CreateSecurityRule(val)
		if err != nil {
			return response.Acl{}, errors.Trace(err)
		}
	}
	return details, nil
}

// RemoveACLAndRules removes all acl rules from the instance machine with the id given
func (f Firewall) RemoveACLAndRules(machineId string) error {
	groupName := f.machineGroupName(machineId)
	resourceName := f.client.ComposeName(groupName)
	secRules, err := f.getAllSecurityRules(resourceName)
	if err != nil {
		return err
	}
	for _, val := range secRules {
		err := f.client.DeleteSecurityRule(val.Name)
		if err != nil {
			if !api.IsNotFound(err) {
				return err
			}
		}
	}
	err = f.client.DeleteAcl(resourceName)
	if err != nil {
		if !api.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// GlobalIngressRules returns all ingress rules that are in the global group
// name from the oracle cloud account
// If there are not any ingress rules under the global group name this will
// return nil, nil
func (f Firewall) GlobalIngressRules() ([]network.IngressRule, error) {
	seclist := f.globalGroupName()
	return f.getIngressRules(f.client.ComposeName(seclist))
}

// getDefaultIngressRules will create the default ingressRules given an api port
func (f Firewall) getDefaultIngressRules(apiPort int) []network.IngressRule {
	return []network.IngressRule{
		network.IngressRule{
			PortRange: network.PortRange{
				FromPort: 22,
				ToPort:   22,
				Protocol: "tcp",
			},
			SourceCIDRs: []string{
				"0.0.0.0/0",
			},
		},
		network.IngressRule{
			PortRange: network.PortRange{
				FromPort: 3389,
				ToPort:   3389,
				Protocol: "tcp",
			},
			SourceCIDRs: []string{
				"0.0.0.0/0",
			},
		},
		network.IngressRule{
			PortRange: network.PortRange{
				FromPort: apiPort,
				ToPort:   apiPort,
				Protocol: "tcp",
			},
			SourceCIDRs: []string{
				"0.0.0.0/0",
			},
		},
		network.IngressRule{
			PortRange: network.PortRange{
				FromPort: controller.DefaultStatePort,
				ToPort:   controller.DefaultStatePort,
				Protocol: "tcp",
			},
			SourceCIDRs: []string{
				"0.0.0.0/0",
			},
		},
	}
}

// retrive all ip address sets from the oracle cloud environment
func (f Firewall) getAllIPAddressSets() ([]response.IpAddressPrefixSet, error) {
	sets, err := f.client.AllIpAddressPrefixSets(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sets.Result, nil
}

func (f Firewall) ensureIPAddressSet(
	ipSet []string,
) (response.IpAddressPrefixSet, error) {
	sets, err := f.getAllIPAddressSets()
	if err != nil {
		return response.IpAddressPrefixSet{}, err
	}
	for _, val := range sets {
		sort.Strings(ipSet)
		sort.Strings(val.IpAddressPrefixes)
		if reflect.DeepEqual(ipSet, val.IpAddressPrefixes) {
			return val, nil
		}
	}
	name, err := utils.NewUUID()
	if err != nil {
		return response.IpAddressPrefixSet{}, err
	}
	p := api.IpAddressPrefixSetParams{
		Description:       "Juju created prefix set",
		IpAddressPrefixes: ipSet,
		Name:              f.client.ComposeName(name.String()),
	}
	details, err := f.client.CreateIpAddressPrefixSet(p)
	if err != nil {
		return response.IpAddressPrefixSet{}, err
	}
	return details, nil
}

type stubSecurityRule struct {
	Acl                    string
	FlowDirection          common.FlowDirection
	DstIpAddressPrefixSets []string
	SecProtocols           []string
	SrcIpAddressPrefixSets []string
}

func (f Firewall) createDefaultGroupAndRules(apiPort int) (response.SecList, error) {
	rules := f.getDefaultIngressRules(apiPort)
	var details response.SecList
	var err error
	globalGroupName := f.globalGroupName()
	resourceName := f.client.ComposeName(globalGroupName)
	details, err = f.client.SecListDetails(resourceName)
	if err != nil {
		if api.IsNotFound(err) {
			details, err = f.ensureSecList(resourceName)
			if err != nil {
				return response.SecList{}, errors.Trace(err)
			}
		} else {
			return response.SecList{}, errors.Trace(err)
		}
	}

	err = f.ensureSecRules(details, rules)
	if err != nil {
		return response.SecList{}, errors.Trace(err)
	}
	return details, nil
}

// closePortsOnList on list will close all ports on a given list using the
// ingress ruled passed into the func
func (f Firewall) closePortsOnList(list string, rules []network.IngressRule) error {
	// get all security rules based on the dst_list=list
	secrules, err := f.getSecRules(list)
	if err != nil {
		return errors.Trace(err)
	}
	// converts all security rules into a map of ingress rules
	mapping, err := f.secRuleToIngresRule(secrules...)
	if err != nil {
		return errors.Trace(err)
	}
	//TODO (gsamfira): optimize this
	for name, rule := range mapping {
		sort.Strings(rule.SourceCIDRs)
		for _, ingressRule := range rules {
			sort.Strings(ingressRule.SourceCIDRs)
			if reflect.DeepEqual(rule, ingressRule) {
				err := f.client.DeleteSecRule(name)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
	return nil
}

// deleteAllSecRulesOnList will delete all security rules from a give
// security list
func (f Firewall) deleteAllSecRulesOnList(list string) error {
	// get all security rules from the oracle cloud account
	// that has the destination list match up with list provided as param
	secrules, err := f.getSecRules(list)
	if err != nil {
		return errors.Trace(err)
	}
	// delete all rules that are found from the previous step
	for _, rule := range secrules {
		err := f.client.DeleteSecRule(rule.Name)
		if err != nil {
			if api.IsNotFound(err) {
				continue
			}
			return errors.Trace(err)
		}
	}
	return nil
}

// maybeDeleteList delets all security rules
// under the security list name provided
func (f Firewall) maybeDeleteList(list string) error {
	filter := []api.Filter{
		api.Filter{
			Arg:   "seclist",
			Value: list,
		},
	}
	iter := 0
	found := true
	var assoc response.AllSecAssociations
	for {
		if iter >= 10 {
			break
		}
		assoc, err := f.client.AllSecAssociations(filter)
		if err != nil {
			return errors.Trace(err)
		}
		if len(assoc.Result) > 0 {
			time.Sleep(1 * time.Second)
			iter++
			continue
		}
		found = false
		break
	}
	if found {
		logger.Warningf(
			"seclist %s is still has some associations to instance(s): %v. Will not delete",
			list, assoc.Result,
		)
		return nil
	}
	err := f.deleteAllSecRulesOnList(list)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("deleting seclist %v", list)
	err = f.client.DeleteSecList(list)
	if err != nil {
		if api.IsNotFound(err) {
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// getIngressRules returns all ingress rules from the oracle cloud account
// given that security list name that maches
func (f Firewall) getIngressRules(seclist string) ([]network.IngressRule, error) {
	// get all security rules given the seclist name
	secrules, err := f.getSecRules(seclist)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// convert all security rules into a map of ingress rules
	ingressRules, err := f.convertFromSecRules(secrules...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// check if the seclist provided is found in the newly
	// ingress rules created from the previously security rules
	// if it's found return it
	if rules, ok := ingressRules[seclist]; ok {
		return rules, nil
	}
	// if we don't find anything just return empty slice and nil
	return []network.IngressRule{}, nil
}
