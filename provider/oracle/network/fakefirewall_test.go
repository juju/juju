package network_test

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// FakeComposer implements common.Composer interface
type FakeComposer struct {
	compose string
}

func (f FakeComposer) ComposeName(name string) string {
	return f.compose
}

// FakeRules implement common.RuleAPI interface
type FakeRules struct {
	All       response.AllSecRules
	AllErr    error
	Create    response.SecRule
	CreateErr error
	DeleteErr error
}

func (f FakeRules) AllSecRules([]api.Filter) (response.AllSecRules, error) {
	return f.All, f.AllErr
}
func (f FakeRules) CreateSecRule(api.SecRuleParams) (response.SecRule, error) {
	return f.Create, f.CreateErr
}
func (f FakeRules) DeleteSecRule(name string) error {
	return f.DeleteErr
}

// FakeAcl implements the common.AclAPI interface
type FakeAcl struct {
	Acl       response.Acl
	AclErr    error
	Create    response.Acl
	CreateErr error
	DeleteErr error
}

func (f FakeAcl) AclDetails(string) (response.Acl, error) {
	return f.Acl, f.AclErr
}

func (f FakeAcl) CreateAcl(string, string, bool, []string) (response.Acl, error) {
	return f.Create, f.CreateErr
}

func (f FakeAcl) DeleteAcl(string) error {
	return f.DeleteErr
}

// FakeSecIp implements common.SecIpAPI interface
type FakeSecIp struct {
	All           response.AllSecIpLists
	AllErr        error
	Create        response.SecIpList
	CreateErr     error
	AllDefault    response.AllSecIpLists
	AllDefaultErr error
}

func (f FakeSecIp) AllSecIpLists([]api.Filter) (response.AllSecIpLists, error) {
	return f.All, f.AllErr
}

func (f FakeSecIp) CreateSecIpList(string, string, []string) (response.SecIpList, error) {
	return f.Create, f.CreateErr
}
func (f FakeSecIp) AllDefaultSecIpLists([]api.Filter) (response.AllSecIpLists, error) {
	return f.AllDefault, f.AllDefaultErr
}

// FakeIpAddressPrefixSet type implements the common.IpAddressPrefixSetAPI interface
type FakeIpAddressprefixSet struct {
	Create    response.IpAddressPrefixSet
	CreateErr error
	All       response.AllIpAddressPrefixSets
	AllErr    error
}

func (f FakeIpAddressprefixSet) CreateIpAddressPrefixSet(
	api.IpAddressPrefixSetParams) (response.IpAddressPrefixSet, error) {
	return f.Create, f.CreateErr
}

func (f FakeIpAddressprefixSet) AllIpAddressPrefixSets(
	[]api.Filter,
) (response.AllIpAddressPrefixSets, error) {
	return f.All, f.AllErr
}

// FakeSecList implement the common.SecListAPI interface
type FakeSecList struct {
	SecList    response.SecList
	SecListErr error
	DeleteErr  error
	Create     response.SecList
	CreateErr  error
}

func (f FakeSecList) SecListDetails(string) (response.SecList, error) {
	return f.SecList, f.SecListErr
}
func (f FakeSecList) DeleteSecList(string) error {
	return f.DeleteErr
}
func (f FakeSecList) CreateSecList(string, string, common.SecRuleAction, common.SecRuleAction) (response.SecList, error) {
	return f.Create, f.CreateErr
}

// type FakeSecRules imeplements the common.SecRulesAPI interface
type FakeSecRules struct {
	All       response.AllSecurityRules
	AllErr    error
	DeleteErr error
	Create    response.SecurityRule
	CreateErr error
}

func (f FakeSecRules) AllSecurityRules([]api.Filter) (response.AllSecurityRules, error) {
	return f.All, f.AllErr
}
func (f FakeSecRules) DeleteSecurityRule(string) error {
	return f.DeleteErr
}
func (f FakeSecRules) CreateSecurityRule(
	api.SecurityRuleParams,
) (response.SecurityRule, error) {
	return f.Create, f.CreateErr
}

// FakeApplications type implements the common.ApplicationsAPI
type FakeApplication struct {
	All        response.AllSecApplications
	AllErr     error
	Default    response.AllSecApplications
	DefaultErr error
	Create     response.SecApplication
	CreateErr  error
}

func (f FakeApplication) AllSecApplications([]api.Filter) (response.AllSecApplications, error) {
	return f.All, f.AllErr
}

func (f FakeApplication) DefaultSecApplications([]api.Filter) (response.AllSecApplications, error) {
	return f.Default, f.DefaultErr
}

func (f FakeApplication) CreateSecApplication(api.SecApplicationParams) (response.SecApplication, error) {
	return f.Create, f.CreateErr
}

type FakeAssociation struct {
	All    response.AllSecAssociations
	AllErr error
}

func (f FakeAssociation) AllSecAssociations([]api.Filter) (response.AllSecAssociations, error) {
	return f.All, f.AllErr
}

// FakeFirewallAPI used to mock the internal Firewaller implementation
// This type implements the network.FirewallerAPI interface
type FakeFirewallAPI struct {
	FakeComposer
	FakeRules
	FakeAcl
	FakeSecIp
	FakeIpAddressprefixSet
	FakeSecList
	FakeSecRules
	FakeApplication
	FakeAssociation
}
