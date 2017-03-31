package network_test

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

type FakeFirewallAPI struct {
	FakeComposeName func(string) string

	// FakeAllSecRules   func([]api.Filter) (response.AllSecRules, error)
	// FakeDeleteSecRule func(string) error
	// FakeCreateSecRule func(api.SecRuleParams) (response.SecRule, error)
	//
	// FakeAclDetails func(string) (response.Acl, error)
	// FakeCreateAcl  func(string, string, bool, []string) (response.Acl, error)
	// FakeDeleteAcl  func(string) error
	//
	// FakeAllSecIpLists        func([]api.Filter) (response.AllSecIpLists, error)
	// FakeCreateSecIpList      func(string, string, []string) (response.SecIpList, error)
	// FakeAllDefaultSecIpLists func([]api.Filter) (response.AllSecIpLists, error)
	//
	// FakeCreateIpAddressPrefixSet func(
	// 	api.IpAddressPrefixSetParams) (response.IpAddressPrefixSet, error)
	// FakeAllIpAddressPrefixSets func([]api.Filter) (response.AllIpAddressPrefixSets, error)
	//
	// FakeSecListDetails func(string) (response.SecList, error)
	// FakeDeleteSecList  func(string) error
	// FakeCreateSecList  func(string, string,
	// 	common.SecRuleAction, common.SecRuleAction) (response.SecList, error)
	//
	// FakeAllSecurityRules   func([]api.Filter) (response.AllSecurityRules, error)
	// FakeDeleteSecurityRule func(string) error
	// FakeCreateSecurityRule func(api.SecurityRuleParams) (response.SecurityRule, error)
	//
	// FakeAllSecApplications     func([]api.Filter) (response.AllSecApplications, error)
	// FakeDefaultSecApplications func([]api.Filter) (response.AllSecApplications, error)
	// FakeCreateSecApplication   func(api.SecApplicationParams) (response.SecApplication, error)
	//
	// FakeAllSecAssociations func([]api.Filter) (response.AllSecAssociations, error)
}

func (f FakeFirewallAPI) ComposeName(name string) string {
	return f.ComposeName(name)
}
func (f FakeFirewallAPI) AllSecRules(filter []api.Filter) (response.AllSecRules, error) {
	return response.AllSecRules{}, nil
	//return f.FakeAllSecRules(filter)
}
func (f FakeFirewallAPI) DeleteSecRule(name string) error {
	return nil
	//return f.FakeDeleteSecRule(name)
}
func (f FakeFirewallAPI) CreateSecRule(p api.SecRuleParams) (response.SecRule, error) {
	return response.SecRule{}, nil
	//return f.FakeCreateSecRule(p)
}
func (f FakeFirewallAPI) AclDetails(name string) (response.Acl, error) {
	return response.Acl{}, nil
	//return f.FakeAclDetails(name)
}
func (f FakeFirewallAPI) CreateAcl(
	name string,
	description string,
	flag bool,
	tags []string,
) (response.Acl, error) {
	return response.Acl{}, nil
	//return f.FakeCreateAcl(name, description, flag, tags)
}
func (f FakeFirewallAPI) DeleteAcl(name string) error {
	return nil
	//return f.FakeDeleteAcl(name)
}

func (f FakeFirewallAPI) AllSecIpLists(
	filter []api.Filter,
) (response.AllSecIpLists, error) {
	return response.AllSecIpLists{}, nil
	//return f.FakeAllSecIpLists(filter)
}

func (f FakeFirewallAPI) CreateSecIpList(
	description string,
	name string,
	secipentries []string,
) (response.SecIpList, error) {
	return response.SecIpList{}, nil
	//return f.FakeCreateSecIpList(description, name, secipentries)
}

func (f FakeFirewallAPI) AllDefaultSecIpLists(
	filter []api.Filter,
) (response.AllSecIpLists, error) {
	return response.AllSecIpLists{}, nil
	//return f.FakeAllDefaultSecIpLists(filter)
}

func (f FakeFirewallAPI) CreateIpAddressPrefixSet(
	p api.IpAddressPrefixSetParams,
) (response.IpAddressPrefixSet, error) {
	return response.IpAddressPrefixSet{}, nil
	//return f.FakeCreateIpAddressPrefixSet(p)
}

func (f FakeFirewallAPI) AllIpAddressPrefixSets(
	filter []api.Filter,
) (response.AllIpAddressPrefixSets, error) {
	return response.AllIpAddressPrefixSets{}, nil
	//return f.FakeAllIpAddressPrefixSets(filter)
}

func (f FakeFirewallAPI) SecListDetails(name string) (response.SecList, error) {
	return response.SecList{}, nil
	//return f.FakeSecListDetails(name)
}

func (f FakeFirewallAPI) DeleteSecList(name string) error {
	return nil
	//return f.FakeDeleteSecList(name)
}

func (f FakeFirewallAPI) CreateSecList(
	description string,
	name string,
	outbound_cidr_policy common.SecRuleAction,
	policy common.SecRuleAction,
) (response.SecList, error) {
	return response.SecList{}, nil
	//return f.FakeCreateSecList(description, name, outbound_cidr_policy, policy)
}

func (f FakeFirewallAPI) AllSecurityRules(
	filter []api.Filter,
) (response.AllSecurityRules, error) {
	return response.AllSecurityRules{}, nil
	//return f.FakeAllSecurityRules(filter)
}

func (f FakeFirewallAPI) DeleteSecurityRule(name string) error {
	return nil
	//return f.FakeDeleteSecurityRule(name)

}

func (f FakeFirewallAPI) CreateSecurityRule(
	p api.SecurityRuleParams,
) (response.SecurityRule, error) {
	return response.SecurityRule{}, nil
	//return f.FakeCreateSecurityRule(p)
}

func (f FakeFirewallAPI) AllSecApplications(
	filter []api.Filter,
) (response.AllSecApplications, error) {
	return response.AllSecApplications{}, nil
	//return f.FakeAllSecApplications(filter)
}

func (f FakeFirewallAPI) DefaultSecApplications(
	filter []api.Filter,
) (response.AllSecApplications, error) {
	return response.AllSecApplications{}, nil
	//return f.FakeDefaultSecApplications(filter)
}

func (f FakeFirewallAPI) CreateSecApplication(
	p api.SecApplicationParams,
) (response.SecApplication, error) {
	return response.SecApplication{}, nil
	//return f.FakeCreateSecApplication(p)
}

func (f FakeFirewallAPI) AllSecAssociations(
	filter []api.Filter,
) (response.AllSecAssociations, error) {
	return response.AllSecAssociations{}, nil
	//return f.FakeAllSecAssociations(filter)
}
