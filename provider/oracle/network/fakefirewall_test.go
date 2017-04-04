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

var (
	DefaultFakeFirewallAPI = &FakeFirewallAPI{
		FakeComposer: FakeComposer{
			compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeRules: FakeRules{
			All: response.AllSecRules{
				Result: []response.SecRule{
					response.SecRule{
						Action:      common.SecRulePermit,
						Application: "/Compute-acme/jack.jones@example.com/video_streaming_udp",
						Name:        "/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
						Dst_list:    "seclist:/Compute-acme/jack.jones@example.com/allowed_video_servers",
						Src_list:    "seciplist:/Compute-acme/jack.jones@example.com/es_iplist",
						Uri:         "https://api-z999.compute.us0.oraclecloud.com/secrule/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
						Src_is_ip:   "true",
						Dst_is_ip:   "false",
					},
				},
			},
			AllErr: nil,
		},
		FakeApplication: FakeApplication{
			All: response.AllSecApplications{
				Result: []response.SecApplication{
					response.SecApplication{
						Description: "Juju created security application",
						Dport:       "17070",
						Icmpcode:    "",
						Icmptype:    "",
						Name:        "/Compute-a432100/sgiulitti@cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-7993630e-d13b-43a3-850e-a1778c7e394e",
						Protocol:    "tcp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/Compute-a432100/sgiulitti%40cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-7993630e-d13b-43a3-850e-a1778c7e394e",
						Value1:      17070,
						Value2:      -1,
						Id:          "1869cb17-5b12-49c5-a09a-046da8899bc9",
					},
					response.SecApplication{
						Description: "Juju created security application",
						Dport:       "37017",
						Icmpcode:    "",
						Icmptype:    "",
						Name:        "/Compute-a432100/sgiulitti@cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-ef8a7955-4315-47a2-83c1-8d2978ab77c7",
						Protocol:    "tcp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/Compute-a432100/sgiulitti%40cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-ef8a7955-4315-47a2-83c1-8d2978ab77c7",
						Value1:      37017,
						Value2:      -1,
						Id:          "cbefdac0-7684-4f81-a575-825c175aa7b4",
					},
				},
			},
			AllErr: nil,
			Default: response.AllSecApplications{
				Result: []response.SecApplication{
					response.SecApplication{
						Description: "",
						Dport:       "",
						Icmpcode:    "",
						Icmptype:    "",
						Name:        "/oracle/public/all",
						Protocol:    "all",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/all",
						Value1:      0,
						Value2:      0,
						Id:          "381c2267-1b38-4bbd-b53d-5149deddb094",
					},
					response.SecApplication{
						Description: "",
						Dport:       "",
						Icmpcode:    "",
						Icmptype:    "echo",
						Name:        "/oracle/public/pings",
						Protocol:    "icmp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/pings",
						Value1:      8,
						Value2:      0,
						Id:          "57b0350b-2f02-4a2d-b5ec-cf731de36027",
					},
					response.SecApplication{
						Description: "",
						Dport:       "",
						Icmpcode:    "",
						Icmptype:    "",
						Name:        "/oracle/public/icmp",
						Protocol:    "icmp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/icmp",
						Value1:      255,
						Value2:      255,
						Id:          "abb27ccd-1872-48f9-86ef-38c72d6f8a38",
					},
					response.SecApplication{
						Description: "",
						Dport:       "",
						Icmpcode:    "",
						Icmptype:    "reply",
						Name:        "/oracle/public/ping-reply",
						Protocol:    "icmp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/ping-reply",
						Value1:      0,
						Value2:      0,
						Id:          "3ad808d4-b740-42c1-805c-57feb7c96d40",
					},
					response.SecApplication{
						Description: "",
						Dport:       "3306",
						Icmpcode:    "",
						Icmptype:    "",
						Name:        "/oracle/public/mysql",
						Protocol:    "tcp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/mysql",
						Value1:      3306,
						Value2:      -1,
						Id:          "2fb5eaff-3127-4334-8b03-367a44bb83bd",
					},
					response.SecApplication{
						Description: "",
						Dport:       "22",
						Icmpcode:    "",
						Icmptype:    "",
						Name:        "/oracle/public/ssh",
						Protocol:    "tcp",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/ssh",
						Value1:      22, Value2: -1,
						Id: "5f027043-f6b3-4e1a-b9fa-a10d075744de",
					},
				},
			},
			DefaultErr: nil,
		},
		FakeSecIp: FakeSecIp{
			All: response.AllSecIpLists{
				Result: []response.SecIpList{
					response.SecIpList{
						Description: nil,
						Name:        "/oracle/public/site",
						Secipentries: []string{
							"10.60.32.128/26",
							"10.60.32.0/26",
							"10.60.37.0/26",
							"10.60.33.0/26",
							"10.60.36.128/26",
							"10.60.36.0/26",
						},
						Uri:      "https://compute.uscom-central-1.oraclecloud.com/seciplist/oracle/public/site",
						Group_id: "1003",
						Id:       "492ad26e-4c86-44bb-a439-535614d25f56",
					},
					response.SecIpList{
						Description: nil,
						Name:        "/oracle/public/paas-infra",
						Secipentries: []string{
							"10.199.34.192/26",
							"160.34.15.48/29",
							"100.64.0.0/24",
						},
						Uri:      "https://compute.uscom-central-1.oraclecloud.com/seciplist/oracle/public/paas-infra",
						Group_id: "1006",
						Id:       "a671b8b6-2422-45ef-84fc-c65010f0c1a5",
					},
					response.SecIpList{
						Description: nil,
						Name:        "/oracle/public/instance",
						Secipentries: []string{
							"10.31.0.0/19",
							"10.2.0.0/26",
							"10.16.0.0/19",
							"10.31.32.0/19",
							"10.16.64.0/19",
							"10.16.32.0/19",
							"10.16.128.0/19",
							"10.16.160.0/19",
							"10.16.192.0/19",
							"10.16.224.0/19",
							"10.28.192.0/19",
							"10.28.224.0/19",
						},
						Uri:      "https://compute.uscom-central-1.oraclecloud.com/seciplist/oracle/public/instance",
						Group_id: "1004",
						Id:       "5c3a5100-ced7-43f8-a5cd-10dce263db33",
					},
					response.SecIpList{
						Description:  nil,
						Name:         "/oracle/public/public-internet",
						Secipentries: []string{"0.0.0.0/0"},
						Uri:          "https://compute.uscom-central-1.oraclecloud.com/seciplist/oracle/public/public-internet",
						Group_id:     "1002",
						Id:           "26fc6f14-4c3c-4059-a813-8a76ff141a0b",
					},
				},
			},
			AllErr: nil,
		},
		FakeSecList: FakeSecList{
			SecList: response.SecList{
				Account:              "/Compute-acme/default",
				Name:                 "/Compute-acme/jack.jones@example.com/allowed_video_servers",
				Uri:                  "https://api-z999.compute.us0.oraclecloud.com/seclist/Compute-acme/jack.jones@example.com/allowed_video_servers",
				Outbound_cidr_policy: "PERMIT",
				Policy:               common.SecRulePermit,
			},
			SecListErr: nil,
			Create: response.SecList{
				Account:              "/Compute-acme/default",
				Name:                 "/Compute-acme/jack.jones@example.com/allowed_video_servers",
				Uri:                  "https://api-z999.compute.us0.oraclecloud.com/seclist/Compute-acme/jack.jones@example.com/allowed_video_servers",
				Outbound_cidr_policy: "PERMIT",
				Policy:               common.SecRuleDeny,
			},
			CreateErr: nil,
		},
		FakeAssociation: FakeAssociation{
			All: response.AllSecAssociations{
				Result: []response.SecAssociation{
					response.SecAssociation{
						Name:    "/Compute-a432100/sgiulitti@cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6/9e5c3f31-1769-46b6-bdc3-b8f3db0f0479",
						Seclist: "/Compute-a432100/default/default",
						Vcable:  "/Compute-a432100/sgiulitti@cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6",
						Uri:     "https://compute.uscom-central-1.oraclecloud.com/secassociation/Compute-a432100/sgiulitti%40cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6/9e5c3f31-1769-46b6-bdc3-b8f3db0f0479",
					},
				},
			},
			AllErr: nil,
		},
		FakeAcl: FakeAcl{
			Acl: response.Acl{
				Name:        "/Compute-a432100/gsamfira@cloudbase.com/juju-b3329a64-58f5-416c-85f7-e24de0beb979-0",
				Description: "ACL for machine 0",
				EnableFlag:  false,
				Tags:        []string{},
				Uri:         "https://compute.uscom-central-1.oraclecloud.com:443/network/v1/acl/Compute-a432100/gsamfira@cloudbase.com/juju-b3329a64-58f5-416c-85f7-e24de0beb979-0",
			},
			AclErr: nil,
			Create: response.Acl{
				Name:        "/Compute-a432100/gsamfira@cloudbase.com/juju-b3329a64-58f5-416c-85f7-e24de0beb979-0",
				Description: "ACL for machine 0",
				EnableFlag:  false,
				Tags:        []string{},
				Uri:         "https://compute.uscom-central-1.oraclecloud.com:443/network/v1/acl/Compute-a432100/gsamfira@cloudbase.com/juju-b3329a64-58f5-416c-85f7-e24de0beb979-0",
			},
		},
		FakeSecRules: FakeSecRules{
			All:    response.AllSecurityRules{},
			AllErr: nil,
			Create: response.SecurityRule{
				Name:                   "/Compute-acme/jack.jones@example.com/secrule1",
				Uri:                    "https://api-z999.compute.us0.oraclecloud.com:443/network/v1/secrule/Compute-acme/jack.jones@example.com/secrule1",
				Description:            "Sample security rule",
				Tags:                   nil,
				Acl:                    "/Compute-acme/jack.jones@example.com/acl1",
				FlowDirection:          common.Egress,
				SrcVnicSet:             "/Compute-acme/jack.jones@example.com/vnicset1",
				DstVnicSet:             "/Compute-acme/jack.jones@example.com/vnicset2",
				SrcIpAddressPrefixSets: []string{"/Compute-acme/jack.jones@example.com/ipaddressprefixset1"},
				DstIpAddressPrefixSets: nil,
				SecProtocols:           []string{"/Compute-acme/jack.jones@example.com/secprotocol1"},
				EnabledFlag:            true,
			},
		},
	}
)
