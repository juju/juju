// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"reflect"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/utils"
)

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
