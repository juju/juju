// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/utils"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	commonProvider "github.com/juju/juju/provider/oracle/common"
)

// Firewaller exposes methods for managing network ports.
type Firewaller interface {
	environs.Firewaller

	// Return all machine ingress rules for a given machine id
	MachineIngressRules(ctx context.ProviderCallContext, id string) (firewall.IngressRules, error)

	// OpenPortsOnInstance will open ports corresponding to the supplied rules
	// on the given instance
	OpenPortsOnInstance(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error

	// ClosePortsOnInstnace will close ports corresponding to the supplied rules
	// for a given instance.
	ClosePortsOnInstance(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error

	// CreateMachineSecLists creates a security list for the given instance.
	// It's worth noting that this function also ensures that the default environment
	// sec list is also present, and has the appropriate default rules.
	// The port parameter is the API port for the state machine, for which we need
	// to create rules.
	CreateMachineSecLists(id string, port int) ([]string, error)

	// DeleteMachineSecList will delete the security list on the given machine
	// id
	DeleteMachineSecList(id string) error

	// CreateDefaultACLAndRules will create a default ACL and associated rules, for
	// a given machine. This ACL applies to user defined IP networks, which are attached
	// to the instance.
	CreateDefaultACLAndRules(id string) (response.Acl, error)

	// RemoveACLAndRules will remove the ACL and any associated rules.
	RemoveACLAndRules(id string) error
}

var _ Firewaller = (*Firewall)(nil)

// FirewallerAPI defines methods necessary for interacting with the firewall
// feature of Oracle compute cloud
type FirewallerAPI interface {
	commonProvider.Composer
	commonProvider.RulesAPI
	commonProvider.AclAPI
	commonProvider.SecIpAPI
	commonProvider.IpAddressPrefixSetAPI
	commonProvider.SecListAPI
	commonProvider.ApplicationsAPI
	commonProvider.SecRulesAPI
	commonProvider.AssociationAPI
}

// Firewall implements environ.Firewaller
type Firewall struct {
	// environ is the current oracle cloud environment
	// this will use to access the underlying config
	environ environs.ConfigGetter
	// client is used to make operations on the oracle provider
	client FirewallerAPI
	clock  clock.Clock
}

// NewFirewall returns a new Firewall
func NewFirewall(cfg environs.ConfigGetter, client FirewallerAPI, c clock.Clock) *Firewall {
	return &Firewall{
		environ: cfg,
		client:  client,
		clock:   c,
	}
}

// OpenPorts is specified on the environ.Firewaller interface.
func (f Firewall) OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
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

// ClosePorts is specified on the environ.Firewaller interface.
func (f Firewall) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	groupName := f.globalGroupName()
	return f.closePortsOnList(ctx, f.client.ComposeName(groupName), rules)
}

// IngressRules is specified on the environ.Firewaller interface.
func (f Firewall) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	return f.GlobalIngressRules(ctx)
}

// MachineIngressRules returns all ingress rules from the machine specific sec list
func (f Firewall) MachineIngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	seclist := f.machineGroupName(machineId)
	return f.getIngressRules(ctx, f.client.ComposeName(seclist))
}

// OpenPortsOnInstance will open ports corresponding to the supplied rules
// on the given instance
func (f Firewall) OpenPortsOnInstance(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
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

// ClosePortsOnInstnace will close ports corresponding to the supplied rules
// for a given instance.
func (f Firewall) ClosePortsOnInstance(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	// fetch the group name based on the machine id provided
	groupName := f.machineGroupName(machineId)
	return f.closePortsOnList(ctx, f.client.ComposeName(groupName), rules)
}

// CreateMachineSecLists creates a security list for the given instance.
// It's worth noting that this function also ensures that the default environment
// sec list is also present, and has the appropriate default rules.
// The port parameter is the API port for the state machine, for which we need
// to create rules.
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

// DeleteMachineSecList will delete the security list on the given machine
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
		{
			Name:                   fmt.Sprintf("%s-allow-ingress", resourceName),
			Description:            "Allow all ingress",
			FlowDirection:          common.Ingress,
			EnabledFlag:            true,
			DstIpAddressPrefixSets: []string{},
			SecProtocols:           []string{},
			SrcIpAddressPrefixSets: []string{},
		},
		{
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

// RemoveACLAndRules will remove the ACL and any associated rules.
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

// GlobalIngressRules returns the ingress rules applied to the whole environment.
func (f Firewall) GlobalIngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	seclist := f.globalGroupName()
	return f.getIngressRules(ctx, f.client.ComposeName(seclist))
}

// getDefaultIngressRules will create the default ingressRules given an api port
func (f Firewall) getDefaultIngressRules(apiPort int) (firewall.IngressRules, error) {
	var rules firewall.IngressRules
	for _, port := range []int{22, 3389, apiPort, controller.DefaultStatePort} {
		pr, err := network.ParsePortRange(fmt.Sprintf("%d/tcp", port))
		if err != nil {
			return nil, errors.Trace(err)
		}
		rules = append(rules, firewall.NewIngressRule(pr, firewall.AllNetworksIPV4CIDR))
	}
	rules.Sort()
	return rules, nil
}

type stubSecurityRule struct {
	Acl                    string
	FlowDirection          common.FlowDirection
	DstIpAddressPrefixSets []string
	SecProtocols           []string
	SrcIpAddressPrefixSets []string
}

func (f Firewall) createDefaultGroupAndRules(apiPort int) (response.SecList, error) {
	rules, err := f.getDefaultIngressRules(apiPort)
	if err != nil {
		return response.SecList{}, errors.Trace(err)
	}
	var details response.SecList
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

// closePortsOnList on list will close all ports corresponding to the supplied ingress rules
// on a particular list
func (f Firewall) closePortsOnList(ctx context.ProviderCallContext, list string, rules firewall.IngressRules) error {
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
		for _, ingressRule := range rules {
			if !rule.EqualTo(ingressRule) {
				continue
			}
			if err := f.client.DeleteSecRule(name); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// deleteAllSecRulesOnList will delete all security rules from a give
// security list
func (f Firewall) deleteAllSecRulesOnList(list string) error {
	// get all security rules associated with this list
	secrules, err := f.getSecRules(list)
	if err != nil {
		return errors.Trace(err)
	}
	// delete everything
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

// maybeDeleteList tries to delete a security list. Lists that are still in use
// may not be deleted. When deleting an environment, we want to also cleanup the
// environment level sec list. This function attempts to delete a sec list. If the
// sec list still has some associations to any instance, we simply return and assume
// the last VM to get killed as part of the tear-down, will also remove the global
// list as well
func (f *Firewall) maybeDeleteList(list string) error {
	filter := []api.Filter{
		{
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
			<-f.clock.After(1 * time.Second)
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

// getIngressRules returns all rules associated with the given sec list
// values are converted and returned as firewall.IngressRules
func (f Firewall) getIngressRules(ctx context.ProviderCallContext, seclist string) (firewall.IngressRules, error) {
	// get all security rules associated with the seclist
	secrules, err := f.getSecRules(seclist)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// convert all security rules into a map of ingress rules
	ingressRules, err := f.convertFromSecRules(secrules...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if rules, ok := ingressRules[seclist]; ok {
		return rules, nil
	}
	return firewall.IngressRules{}, nil
}

// getAllApplications returns all security applications known to the
// oracle compute cloud. These are used as part of security rules
func (f Firewall) getAllApplications() ([]response.SecApplication, error) {
	// get all user defined sec applications
	applications, err := f.client.AllSecApplications(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// get also default ones defined in the provider
	defaultApps, err := f.client.DefaultSecApplications(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	allApps := []response.SecApplication{}
	for _, val := range applications.Result {
		if val.PortProtocolPair() == "" {
			// (gsamfira):this should not really happen,
			// but I get paranoid when I run out of coffee
			continue
		}
		allApps = append(allApps, val)
	}
	for _, val := range defaultApps.Result {
		if val.PortProtocolPair() == "" {
			continue
		}
		allApps = append(allApps, val)
	}
	return allApps, nil
}

// getAllApplicationsAsMap returns all sec applications as a map
func (f Firewall) getAllApplicationsAsMap() (map[string]response.SecApplication, error) {
	// get all defined protocols
	// from the current identity and default ones
	apps, err := f.getAllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// copy all of them into this map
	allApps := map[string]response.SecApplication{}
	for _, val := range apps {
		if val.String() == "" {
			continue
		}
		if _, ok := allApps[val.String()]; !ok {
			allApps[val.String()] = val
		}
	}
	return allApps, nil
}

func (f Firewall) ensureApplication(portRange network.PortRange, cache *[]response.SecApplication) (string, error) {
	// check if the security application is already created
	for _, val := range *cache {
		if val.PortProtocolPair() == portRange.String() {
			return val.Name, nil
		}
	}
	// We need to create a new application
	// There is always the chance of a race condition
	// when it comes to creating new resources.
	// ie: someone may have already created a matching
	// application between the time we fetched all of them
	// and the moment we actually got to create one
	// Worst thing that can happen is that we have a few duplicate
	// rules, that we cleanup anyway when we destroy the environment
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	// create new name for sec application
	secAppName := f.newResourceName(uuid.String())
	var dport string

	if portRange.FromPort == portRange.ToPort {
		dport = strconv.Itoa(portRange.FromPort)
	} else {
		dport = fmt.Sprintf("%s-%s",
			strconv.Itoa(portRange.FromPort), strconv.Itoa(portRange.ToPort))
	}
	// compose the provider resource name for the new application
	name := f.client.ComposeName(secAppName)
	secAppParams := api.SecApplicationParams{
		Description: "Juju created security application",
		Dport:       dport,
		Protocol:    common.Protocol(portRange.Protocol),
		Name:        name,
	}
	application, err := f.client.CreateSecApplication(secAppParams)
	if err != nil {
		return "", errors.Trace(err)
	}
	*cache = append(*cache, application)
	return application.Name, nil
}

// convertToSecRules converts network.IngressRules to api.SecRuleParams
func (f Firewall) convertToSecRules(seclist response.SecList, rules firewall.IngressRules) ([]api.SecRuleParams, error) {
	applications, err := f.getAllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	iplists, err := f.getAllIPLists()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := make([]api.SecRuleParams, 0, len(rules))
	// for every rule we need to ensure that the there is a relationship
	// between security applications and security IP lists
	// and from every one of them create a slice of security rule parameters
	for _, val := range rules {
		app, err := f.ensureApplication(val.PortRange, &applications)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ipList, err := f.ensureSecIpList(val.SourceCIDRs.SortedValues(), &iplists)
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
		// create the new security rule parameters
		rule := api.SecRuleParams{
			Action:      common.SecRulePermit,
			Application: app,
			Description: "Juju created security rule",
			Disabled:    false,
			Dst_list:    dstList,
			Name:        resourceName,
			Src_list:    srcList,
		}
		// append the new parameters rule
		ret = append(ret, rule)
	}
	return ret, nil
}

// convertApplicationToPortRange takes a SecApplication and
// converts it to a network.PortRange type
func (f Firewall) convertApplicationToPortRange(app response.SecApplication) network.PortRange {
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
func (f Firewall) convertFromSecRules(rules ...response.SecRule) (map[string]firewall.IngressRules, error) {
	applications, err := f.getAllApplicationsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}

	iplists, err := f.getAllIPListsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := map[string]firewall.IngressRules{}
	for _, val := range rules {
		app := val.Application
		srcList := strings.TrimPrefix(val.Src_list, "seciplist:")
		dstList := strings.TrimPrefix(val.Dst_list, "seclist:")
		portRange := f.convertApplicationToPortRange(applications[app])

		ret[dstList] = append(ret[dstList], firewall.NewIngressRule(portRange, iplists[srcList].Secipentries...))
	}
	return ret, nil
}

// secRuleToIngressRule convert all security rules into a map of ingress rules
func (f Firewall) secRuleToIngresRule(rules ...response.SecRule) (map[string]firewall.IngressRule, error) {
	applications, err := f.getAllApplicationsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	iplists, err := f.getAllIPListsAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := make(map[string]firewall.IngressRule)
	for _, val := range rules {
		app := val.Application
		srcList := strings.TrimPrefix(val.Src_list, "seciplist:")
		portRange := f.convertApplicationToPortRange(applications[app])
		if _, ok := ret[val.Name]; !ok {
			ret[val.Name] = firewall.NewIngressRule(portRange, iplists[srcList].Secipentries...)
		}
	}
	return ret, nil
}

func (f Firewall) convertSecurityRuleToStub(rules response.SecurityRule) stubSecurityRule {
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

// globalGroupName returns the global group name
// derived from the model UUID
func (f Firewall) globalGroupName() string {
	return fmt.Sprintf("juju-%s-global", f.environ.Config().UUID())
}

// machineGroupName returns the machine group name
// derived from the model UUID and the machine ID
func (f Firewall) machineGroupName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), machineId)
}

// resourceName returns the resource name
// derived from the model UUID and the name of the resource
func (f Firewall) newResourceName(appName string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), appName)
}

// get all security rules associated with an ACL
func (f Firewall) getAllSecurityRules(aclName string) ([]response.SecurityRule, error) {
	rules, err := f.client.AllSecurityRules(nil)
	if err != nil {
		return nil, err
	}
	if aclName == "" {
		return rules.Result, nil
	}
	var ret []response.SecurityRule
	for _, val := range rules.Result {
		if val.Acl == aclName {
			ret = append(ret, val)
		}
	}
	return ret, nil
}

// getSecRules retrieves the security rules associated with a particular security list
func (f Firewall) getSecRules(seclist string) ([]response.SecRule, error) {
	// we only care about ingress rules
	name := fmt.Sprintf("seclist:%s", seclist)
	rulesFilter := []api.Filter{
		{
			Arg:   "dst_list",
			Value: name,
		},
	}
	rules, err := f.client.AllSecRules(rulesFilter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// gsamfira: the oracle compute API does not allow filtering by action
	ret := []response.SecRule{}
	for _, val := range rules.Result {
		// gsamfira: We set a default policy of DENY. No use in worrying about
		// DENY rules (if by any chance someone add one manually for some reason)
		if val.Action != common.SecRulePermit {
			continue
		}
		// We only care about rules that have a destination set
		// to a security list. Those lists get attached to VMs
		// NOTE: someone decided, when writing the oracle API
		// that some fields should be bool, some should be string.
		// never mind they both are boolean values...but hey.
		// I swear...some people like to watch the world burn
		if val.Dst_is_ip == "true" {
			continue
		}
		// We only care about rules that have an IP list as source
		if val.Src_is_ip == "false" {
			continue
		}
		ret = append(ret, val)
	}
	return ret, nil
}

// ensureSecRules ensures that the list passed has all the rules
// that it needs, if one is missing it will create it inside the oracle
// cloud environment and it will return nil
// if none rule is missing then it will return nil
func (f Firewall) ensureSecRules(seclist response.SecList, rules firewall.IngressRules) error {
	// get all security rules associated with the seclist
	secRules, err := f.getSecRules(seclist.Name)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("list %v has sec rules: %v", seclist.Name, secRules)

	converted, err := f.convertFromSecRules(secRules...)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("converted rules are: %v", converted)
	asIngressRules := converted[seclist.Name]
	missing := firewall.IngressRules{}

	// search through all rules and find the missing ones
	for _, toAdd := range rules {
		found := false
		for _, exists := range asIngressRules {
			if toAdd.EqualTo(exists) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		missing = append(missing, toAdd)
	}
	if len(missing) == 0 {
		return nil
	}
	logger.Tracef("Found missing rules: %v", missing)
	// convert the missing rules back to sec rules
	asSecRule, err := f.convertToSecRules(seclist, missing)
	if err != nil {
		return errors.Trace(err)
	}

	for _, val := range asSecRule {
		_, err = f.client.CreateSecRule(val)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// ensureSecList creates a new seclist if one does not already exist.
// this function is idempotent
func (f Firewall) ensureSecList(name string) (response.SecList, error) {
	logger.Infof("Fetching details for list: %s", name)
	// check if the security list is already there
	details, err := f.client.SecListDetails(name)
	if err != nil {
		logger.Infof("Got error fetching details for %s: %v", name, err)
		if api.IsNotFound(err) {
			logger.Infof("Creating new seclist: %s", name)
			details, err := f.client.CreateSecList(
				"Juju created security list",
				name,
				common.SecRulePermit,
				common.SecRuleDeny)
			if err != nil {
				return response.SecList{}, err
			}
			return details, nil
		}
		return response.SecList{}, err
	}
	return details, nil
}

// getAllIPLists returns all IP lists known to the provider (both the user defined ones,
// and the default ones)
func (f Firewall) getAllIPLists() ([]response.SecIpList, error) {
	// get all security ip lists from the current identity endpoint
	secIpLists, err := f.client.AllSecIpLists(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// get all security ip lists from the default oracle cloud definitions
	defaultSecIpLists, err := f.client.AllDefaultSecIpLists(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	allIpLists := []response.SecIpList{}
	allIpLists = append(allIpLists, secIpLists.Result...)
	allIpLists = append(allIpLists, defaultSecIpLists.Result...)
	return allIpLists, nil
}

// getAllIPListsAsMap returns all IP lists as a map, with the key being
// the resource name
func (f Firewall) getAllIPListsAsMap() (map[string]response.SecIpList, error) {
	allIps, err := f.getAllIPLists()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allIpLists := map[string]response.SecIpList{}
	for _, val := range allIps {
		allIpLists[val.Name] = val
	}
	return allIpLists, nil
}

// ensureSecIpList ensures that a sec ip list with the provided cidr list
// exists. If one does not, it gets created. This function is idempotent.
func (f Firewall) ensureSecIpList(cidr []string, cache *[]response.SecIpList) (string, error) {
	sort.Strings(cidr)
	for _, val := range *cache {
		sort.Strings(val.Secipentries)
		if reflect.DeepEqual(val.Secipentries, cidr) {
			return val.Name, nil
		}
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	name := f.newResourceName(uuid.String())
	resource := f.client.ComposeName(name)
	secList, err := f.client.CreateSecIpList(
		"Juju created security IP list",
		resource, cidr)
	if err != nil {
		return "", errors.Trace(err)
	}
	*cache = append(*cache, secList)
	return secList.Name, nil
}
