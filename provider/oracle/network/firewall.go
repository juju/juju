package network

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
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

// getAllApplications returns all security applications that are mapped
// so the firewall can use security rules. In the oracle cloud
// envirnoment applications are alis term for protocols
func (f Firewall) getAllApplications() ([]response.SecApplication, error) {
	// get all defined protocols from the current identity endpoint
	applications, err := f.client.AllSecApplications(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// get also default ones defined in the oracle cloud environment
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
			// (gsamfira)this should not really happen, but
			// I get paranoid when I run out of coffe
			continue
		}
		allApps = append(allApps, val)
	}
	return allApps, nil
}

// getAllApplicationsAsMap returns all security applications that
// are mapped so the firewaller cause security rules.
// Bassically just like applications method but it's composing the return
// as a map rathar than a slice
func (f Firewall) getAllApplicationsAsMap() (
	map[string]response.SecApplication, error) {

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

// globalGroupName returns the global group name
// based from the juju environ config uuid
func (f Firewall) globalGroupName() string {
	return fmt.Sprintf("juju-%s-global", f.environ.Config().UUID())
}

// machineGroupName returns the machine group name
// based from the juju environ config uuid
func (f Firewall) machineGroupName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), machineId)
}

// resourceName returns the resource name
// based from the juju environ config uuid
func (f Firewall) newResourceName(appName string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), appName)
}

// getSecRules retrieves the security rules for a particular security list
func (f Firewall) getSecRules(seclist string) ([]response.SecRule, error) {
	// we only care about ingress rules
	name := fmt.Sprintf("seclist:%s", seclist)
	// get all secure rules from the current oracle cloud identity
	// and filter through them based on dst_list=name
	rulesFilter := []api.Filter{
		api.Filter{
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

// getAllIPLists returns all sets of ip addresses or subnets external to the
// instnaces that are created in the oracle cloud environment
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
	for _, val := range secIpLists.Result {
		allIpLists = append(allIpLists, val) //[val.Name] = val
	}
	for _, val := range defaultSecIpLists.Result {
		allIpLists = append(allIpLists, val) //[val.Name] = val
	}
	return allIpLists, nil
}

// getAllIPListsAsMap returns all sets of ip addresses or subnets external to the
// instances that are created inside in the oracle cloud.
// This is exactly like ipLists func but rathar than returning a slice,
// we return a map of these.
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

func (f Firewall) isSecIpList(name string) bool {
	if strings.HasPrefix(name, "seciplist:") {
		return true
	}
	return false
}

func (f Firewall) isSecList(name string) bool {
	if strings.HasPrefix(name, "seclist:") {
		return true
	}
	return false
}

func (f Firewall) ensureMachineACLs(name string, tags []string) (response.Acl, error) {
	// globalACL := f.globalGroupName()
	// machineACL := f.machineGroupName(machineId)
	// globalAcl := f.CreateAcl(name, "Juju created ACL", true, tags)
	return response.Acl{}, nil
}

// ensureApplication takes a network.PortRange and a ptr to a slice of
// response.SecApplication aka protocols, in the oracle cloud environment
// After the creation of the security rule on those ports  the result will
// be appended into the SecApplication slice passed
// If the call is successful it will return the newly created security
// appplication name and nil
func (f *Firewall) ensureApplication(portRange network.PortRange, cache *[]response.SecApplication) (string, error) {
	// we should check if the security application is already created
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
	// make a the resource secure application name
	secAppName := f.newResourceName(uuid.String())
	var dport string
	if portRange.FromPort == portRange.ToPort {
		dport = strconv.Itoa(portRange.FromPort)
	} else {
		dport = fmt.Sprintf("%s-%s",
			strconv.Itoa(portRange.FromPort), strconv.Itoa(portRange.ToPort))
	}
	// create a new security application specifying
	// what port range the app should be allowd to use
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

// ensureSecList takes a name and creates a new security list with that name
// if the name is already there then it will return it or the newly created one
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

// ensureSecIpList takes cidrs and a ptr to a slice of
// response.SecIpList. After the creation of the security
// list with cidr the result will be appended into the SecIpList slice
// If the call is successful it will return the newly created security
// list name and nil
func (f Firewall) ensureSecIpList(
	cidr []string,
	cache *[]response.SecIpList,
) (string, error) {

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

// ensureSecRules ensures that the list passed has all the rules
// that it needs, if one is missing it will create it inside the oracle
// cloud environment and it will return nil
// if none rule is missing then it will return nil
func (f Firewall) ensureSecRules(
	seclist response.SecList,
	rules []network.IngressRule,
) error {

	// get all secuity rules that contains the seclist.Name
	secRules, err := f.getSecRules(seclist.Name)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("list %v has sec rules: %v", seclist.Name, secRules)
	// convert the secRules into map[string][]network.INgressRule
	converted, err := f.convertFromSecRules(secRules...)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("converted rules are: %v", converted)
	asIngressRules := converted[seclist.Name]
	missing := []network.IngressRule{}
	// search through all rules and find the missing ones
	for _, toAdd := range rules {
		found := false
		for _, exists := range asIngressRules {
			sort.Strings(toAdd.SourceCIDRs)
			sort.Strings(exists.SourceCIDRs)
			logger.Tracef("comparing %v to %v", toAdd.SourceCIDRs, exists.SourceCIDRs)
			if reflect.DeepEqual(toAdd, exists) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		missing = append(missing, toAdd) // append the missing rule
	}
	if len(missing) == 0 {
		return nil
	}
	logger.Tracef("Found missing rules: %v", missing)
	// convert the missing rules to sec rules back
	asSecRule, err := f.convertToSecRules(seclist, missing)
	if err != nil {
		return errors.Trace(err)
	}

	// for all sec rules that are missing
	// create one by one
	for _, val := range asSecRule {
		_, err = f.client.CreateSecRule(val)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
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

// get all security rules given the access control list name
func (f Firewall) getAllSecurityRules(
	aclName string,
) ([]response.SecurityRule, error) {
	rules, err := f.client.AllSecurityRules(nil)
	if err != nil {
		return nil, errors.Trace(err)
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
