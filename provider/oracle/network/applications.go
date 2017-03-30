// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/juju/network"
	"github.com/juju/utils"
)

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

func (f Firewall) ensureApplication(portRange network.PortRange, cache *[]response.SecApplication) (string, error) {
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
