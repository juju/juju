package oracle

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	ociCommon "github.com/hoenirvili/go-oracle-cloud/common"
	ociResponse "github.com/hoenirvili/go-oracle-cloud/response"

	"github.com/juju/errors"
	"github.com/juju/juju/network"
	"github.com/juju/utils"
)

func NewFirewall(env *oracleEnviron, client *oci.Client) *Firewall {

	return &Firewall{
		environ: env,
		client:  client,
	}
}

type Firewall struct {
	environ *oracleEnviron
	client  *oci.Client
}

func (f *Firewall) getAllApplications() ([]ociResponse.SecApplication, error) {
	//user defined applications
	applications, err := f.client.AllSecApplications(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defaultApps, err := f.client.DefaultSecApplications(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	allApps := []ociResponse.SecApplication{}
	for _, val := range applications.Result {
		if val.PortProtocolPair() == "" {
			// this should not really happen, but I get paranoid when I run out of coffee
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

func (f *Firewall) getAllApplicationsAsMap() (map[string]ociResponse.SecApplication, error) {
	apps, err := f.getAllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allApps := map[string]ociResponse.SecApplication{}
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

func (f *Firewall) globalGroupName() string {
	return fmt.Sprintf("juju-%s-global", f.environ.Config().UUID())
}

func (f *Firewall) machineGroupName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), machineId)
}

func (f *Firewall) newResourceName(appName string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), appName)
}

func (f *Firewall) ensureRulesExits(seclist ociResponse.SecList, rules []network.IngressRule) error {
	return nil
}

// getSecRules retrieves the security rules for a particular security list
func (f *Firewall) getSecRules(seclist ociResponse.SecList) ([]ociResponse.SecRule, error) {
	// We only care about ingress rules
	name := fmt.Sprintf("seclist:%s", seclist.Name)
	rulesFilter := []oci.Filter{
		oci.Filter{
			Arg:   "dst_list",
			Value: name,
		},
	}
	rules, err := f.client.AllSecRules(rulesFilter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// gsamfira: the oracle compute API does not allow filtering by action
	ret := []ociResponse.SecRule{}
	for _, val := range rules.Result {
		// gsamfira: We set a default policy of DENY. No use in worrying about
		// DENY rules (if by any chance someone add one manually for some reason)
		if val.Action != ociCommon.SecRulePermit {
			continue
		}
		ret = append(ret, val)
	}
	return ret, nil
}

func (f *Firewall) getAllIPLists() ([]ociResponse.SecIpList, error) {
	//user defined IP lists
	secIpLists, err := f.client.AllSecIpLists(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defaultSecIpLists, err := f.client.AllDefaultSecIpLists(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	allIpLists := []ociResponse.SecIpList{}
	for _, val := range secIpLists.Result {
		allIpLists = append(allIpLists, val) //[val.Name] = val
	}
	for _, val := range defaultSecIpLists.Result {
		allIpLists = append(allIpLists, val) //[val.Name] = val
	}
	return allIpLists, nil
}

func (f *Firewall) getAllIPListsAsMap() (map[string]ociResponse.SecIpList, error) {
	allIps, err := f.getAllIPLists()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allIpLists := map[string]ociResponse.SecIpList{}
	for _, val := range allIps {
		allIpLists[val.Name] = val
	}
	return allIpLists, nil
}

func (f *Firewall) isSecIpList(name string) bool {
	if strings.HasPrefix(name, "seciplist:") {
		return true
	}
	return false
}

func (f *Firewall) isSecList(name string) bool {
	if strings.HasPrefix(name, "seclist:") {
		return true
	}
	return false
}

func (f *Firewall) ensureApplication(portRange network.PortRange, cache *[]ociResponse.SecApplication) (string, error) {
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
		return "", err
	}
	secAppName := f.newResourceName(uuid.String())
	var dport string
	if portRange.FromPort == portRange.ToPort {
		dport = fmt.Sprintf("%s", portRange.FromPort)
	} else {
		dport = fmt.Sprintf("%s-%s", portRange.FromPort, portRange.ToPort)
	}
	name := f.client.ComposeName(secAppName)
	secAppParams := oci.SecApplicationParams{
		Description: "Juju created security application",
		Dport:       dport,
		Protocol:    ociCommon.Protocol(portRange.Protocol),
		Name:        name,
	}
	application, err := f.client.CreateSecApplication(secAppParams)
	if err != nil {
		return "", err
	}
	*cache = append(*cache, application)
	return application.Name, nil
}

// TODO (gsamfira):finish this
func (f *Firewall) ensureSecIpList(cidr []string, cache *[]ociResponse.SecIpList) (string, error) {
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
	logger.Debugf("Creating new IP list: %v", resource)
	secList, err := f.client.CreateSecIpList(
		"Juju created security IP list",
		resource, cidr)
	if err != nil {
		return "", errors.Trace(err)
	}
	*cache = append(*cache, secList)
	return secList.Name, nil
}

func (f *Firewall) ensureSecRules(seclist ociResponse.SecList, rules []network.IngressRule) error {
	secRules, err := f.getSecRules(seclist)
	if err != nil {
		return errors.Trace(err)
	}
	converted, err := f.convertFromSecRules(secRules)
	if err != nil {
		return errors.Trace(err)
	}
	asIngressRules := converted[seclist.Name]
	missing := []network.IngressRule{}
	for _, toAdd := range rules {
		found := false
		for _, exists := range asIngressRules {
			sort.Strings(toAdd.SourceCIDRs)
			sort.Strings(exists.SourceCIDRs)
			if reflect.DeepEqual(toAdd, exists) {
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

	asSecRule, err := f.convertToSecRules(seclist, missing)
	if err != nil {
		return errors.Trace(err)
	}

	for _, val := range asSecRule {
		logger.Debugf("creating secrule: %v", val)
		_, err = f.client.CreateSecRule(val)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (f *Firewall) convertToSecRules(seclist ociResponse.SecList, rules []network.IngressRule) ([]oci.SecRuleParams, error) {
	applications, err := f.getAllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	iplists, err := f.getAllIPLists()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := make([]oci.SecRuleParams, 0, len(rules))
	for _, val := range rules {
		app, err := f.ensureApplication(val.PortRange, &applications)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ipList, err := f.ensureSecIpList(val.SourceCIDRs, &iplists)
		if err != nil {
			return nil, errors.Trace(err)
		}
		dstList := fmt.Sprintf("seclist:%s", seclist.Name)
		srcList := fmt.Sprintf("seciplist:%s", ipList)
		rule := oci.SecRuleParams{
			Action:      ociCommon.SecRulePermit,
			Application: app,
			Description: "Juju created security rule",
			Disabled:    false,
			Dst_list:    dstList,
			Name:        f.client.ComposeName(val.String()),
			Src_list:    srcList,
		}
		logger.Debugf("Generated sec rule: %v", rule)
		ret = append(ret, rule)
	}
	return ret, nil
}

func (f *Firewall) convertApplicationToPortRange(app ociResponse.SecApplication) network.PortRange {
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

func (f *Firewall) convertFromSecRules(rules []ociResponse.SecRule) (map[string][]network.IngressRule, error) {
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
		// We only care about rules that have a destination set
		// to a security list. Those lists get attached to VMs
		if val.Dst_is_ip {
			continue
		}
		// We only care about rules that have an IP list as source
		if !val.Src_is_ip {
			continue
		}
		app := val.Application
		srcList := strings.TrimPrefix(val.Src_list, "seciplist:")
		dstList := strings.TrimPrefix(val.Src_list, "seclist:")
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

func (f *Firewall) createDefaultGroupAndRules(apiPort int) (ociResponse.SecList, error) {
	rules := []network.IngressRule{
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
	}
	var details ociResponse.SecList
	var err error
	uuid := f.environ.Config().UUID()
	description := fmt.Sprintf("global seclist for juju environment %s", uuid)
	globalGroupName := f.globalGroupName()
	resourceName := f.client.ComposeName(globalGroupName)
	details, err = f.client.SecListDetails(resourceName)
	if err != nil {
		if oci.IsNotFound(err) {
			details, err = f.client.CreateSecList(
				description,
				resourceName,
				ociCommon.SecRulePermit,
				ociCommon.SecRuleDeny)
			if err != nil {
				return ociResponse.SecList{}, errors.Trace(err)
			}
		} else {
			logger.Debugf("Got error of type: %T --> %v", err, err)
			return ociResponse.SecList{}, errors.Trace(err)
		}
	}

	err = f.ensureSecRules(details, rules)
	if err != nil {
		return ociResponse.SecList{}, errors.Trace(err)
	}
	return details, nil
}

func (f *Firewall) CreateMachineSecLists(machineId string, apiPort int) ([]string, error) {
	defaultSecList, err := f.createDefaultGroupAndRules(apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	name := f.machineGroupName(machineId)
	resourceName := f.client.ComposeName(name)
	secList, err := f.client.CreateSecList(
		"Juju created sec list",
		resourceName,
		ociCommon.SecRulePermit,
		ociCommon.SecRuleDeny)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []string{
		defaultSecList.Name,
		secList.Name,
	}, nil
}
