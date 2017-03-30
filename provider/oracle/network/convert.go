// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/juju/network"
	"github.com/juju/utils"
)

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
