// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/juju/network"
)

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
