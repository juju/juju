// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
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
)

type Firewaller interface {
	environs.Firewaller

	MachineIngressRules(id string) ([]network.IngressRule, error)

	OpenPortsOnInstance(machineId string, rules []network.IngressRule) error
	ClosePortsOnInstance(machineId string, rules []network.IngressRule) error

	CreateMachineSecLists(id string, port int) ([]string, error)
	DeleteMachineSecList(id string) error
	CreateDefaultACLAndRules(id string) (response.Acl, error)
	RemoveACLAndRules(id string) error
}

var _ Firewaller = (*Firewall)(nil)

// Firewall exposes methods for mapping network ports.
// This type implement the environ.Firewaller
type Firewall struct {
	// environ is the current oracle cloud environment
	// this will use to acces the underlying config
	environ environs.ConfigGetter
	// client is used to make operations on the oracle provider
	client *api.Client
}

// NewFirewall returns a new firewall that can do network operation
// such as closing and opening ports inside the oracle cloud environmnet
func NewFirewall(cfg environs.ConfigGetter, client *api.Client) *Firewall {
	return &Firewall{
		environ: cfg,
		client:  client,
	}
}

// convertToSecRules this will take a security
// list and a slice of network.IngressRule and it will create
// parameeters in order to call the security rule creation resource
func (f Firewall) convertToSecRules(
	seclist response.SecList,
	rules []network.IngressRule,
) ([]api.SecRuleParams, error) {

	// get all applications and default ones
	applications, err := f.getAllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// get all ip lists
	iplists, err := f.getAllIPLists()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := make([]api.SecRuleParams, 0, len(rules))
	// for every rule we need to ensure that the there is a relationship
	// between security applications and security ip lists
	// and from every one of them create a slice of security rule params
	for _, val := range rules {
		app, err := f.ensureApplication(val.PortRange, &applications)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ipList, err := f.ensureSecIpList(val.SourceCIDRs, &iplists)
		if err != nil {
			return nil, errors.Trace(err)
		}
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, errors.Trace(err)
		}
		name := f.newResourceName(uuid.String())
		resourceName := f.client.ComposeName(name)
		dstList := fmt.Sprintf("seclist:%s", seclist.Name)
		srcList := fmt.Sprintf("seciplist:%s", ipList)
		// create the new security rule param
		rule := api.SecRuleParams{
			Action:      common.SecRulePermit,
			Application: app,
			Description: "Juju created security rule",
			Disabled:    false,
			Dst_list:    dstList,
			Name:        resourceName,
			Src_list:    srcList,
		}
		// append the new param rule
		ret = append(ret, rule)
	}
	return ret, nil
}

// convertApplicationToPortRange takes a SecApplication and
// converts it to a network.PortRange type
func (f Firewall) convertApplicationToPortRange(
	app response.SecApplication,
) network.PortRange {

	appCopy := app
	if appCopy.Value2 == -1 {
		appCopy.Value2 = appCopy.Value1
	}
	return network.PortRange{
		FromPort: appCopy.Value1,
		ToPort:   appCopy.Value2,
		Protocol: string(appCopy.Protocol),
	}
}

// convertFromSecRules takes a slice of security rules and creates a map of them
func (f Firewall) convertFromSecRules(
	rules ...response.SecRule,
) (map[string][]network.IngressRule, error) {

	applications, err := f.getAllApplicationsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	iplists, err := f.getAllIPListsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := map[string][]network.IngressRule{}
	for _, val := range rules {
		app := val.Application
		srcList := strings.TrimPrefix(val.Src_list, "seciplist:")
		dstList := strings.TrimPrefix(val.Dst_list, "seclist:")
		portRange := f.convertApplicationToPortRange(applications[app])
		if _, ok := ret[dstList]; !ok {
			ret[dstList] = []network.IngressRule{
				network.IngressRule{
					PortRange:   portRange,
					SourceCIDRs: iplists[srcList].Secipentries,
				},
			}
		} else {
			toAdd := network.IngressRule{
				PortRange:   portRange,
				SourceCIDRs: iplists[srcList].Secipentries,
			}
			ret[dstList] = append(ret[dstList], toAdd)
		}
	}
	return ret, nil
}

// secRuleToIngressRule convert all security rules into a map of ingress rules
func (f Firewall) secRuleToIngresRule(
	rules ...response.SecRule,
) (map[string]network.IngressRule, error) {

	applications, err := f.getAllApplicationsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	iplists, err := f.getAllIPListsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := map[string]network.IngressRule{}
	for _, val := range rules {
		app := val.Application
		srcList := strings.TrimPrefix(val.Src_list, "seciplist:")
		portRange := f.convertApplicationToPortRange(applications[app])
		if _, ok := ret[val.Name]; !ok {
			ret[val.Name] = network.IngressRule{
				PortRange:   portRange,
				SourceCIDRs: iplists[srcList].Secipentries,
			}
		}
	}
	return ret, nil
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

func (f *Firewall) convertSecurityRuleToStub(rules response.SecurityRule) stubSecurityRule {
	sort.Strings(rules.DstIpAddressPrefixSets)
	sort.Strings(rules.SecProtocols)
	sort.Strings(rules.SrcIpAddressPrefixSets)
	return stubSecurityRule{
		Acl:                    rules.Acl,
		FlowDirection:          rules.FlowDirection,
		DstIpAddressPrefixSets: rules.DstIpAddressPrefixSets,
		SecProtocols:           rules.SecProtocols,
		SrcIpAddressPrefixSets: rules.SrcIpAddressPrefixSets,
	}
}

func (f Firewall) convertSecurityRuleParamsToStub(params api.SecurityRuleParams) stubSecurityRule {
	sort.Strings(params.DstIpAddressPrefixSets)
	sort.Strings(params.SecProtocols)
	sort.Strings(params.SrcIpAddressPrefixSets)
	return stubSecurityRule{
		Acl:                    params.Acl,
		FlowDirection:          params.FlowDirection,
		DstIpAddressPrefixSets: params.DstIpAddressPrefixSets,
		SecProtocols:           params.SecProtocols,
		SrcIpAddressPrefixSets: params.SrcIpAddressPrefixSets,
	}
}

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

// ClosePorts can close the ports on the give ingress rules
func (f Firewall) ClosePorts(rules []network.IngressRule) error {
	groupName := f.globalGroupName()
	return f.closePortsOnList(f.client.ComposeName(groupName), rules)
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

// GlobalIngressRules returns all ingress rules that are in the global group
// name from the oracle cloud account
// If there are not any ingress rules under the global group name this will
// return nil, nil
func (f Firewall) GlobalIngressRules() ([]network.IngressRule, error) {
	seclist := f.globalGroupName()
	return f.getIngressRules(f.client.ComposeName(seclist))
}

// MachineIngressRules returns all ingress rules that are in the machine group
// name from the oracle cloud account.
// If there are not any ingress rules under that name scope this will
// return nil, nil
func (f Firewall) MachineIngressRules(machineId string) ([]network.IngressRule, error) {
	seclist := f.machineGroupName(machineId)
	return f.getIngressRules(f.client.ComposeName(seclist))
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
