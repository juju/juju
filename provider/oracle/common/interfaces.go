// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// Instancer used to retrive details from a given instance
// in the oracle cloud infrastracture
type Instancer interface {
	// InstanceDetails takes and instance name and retrives
	// the instnace raw response from the oracle api
	InstanceDetails(string) (response.Instance, error)
}

// Composer user to compose the name with the indeitity domain name
// inside the oracle cloud environmnet
type Composer interface {
	// ComposeName composes the name for the oracle cloud api
	// for a specific resource. Oracle cloud attaches some extra
	// metadata information such as identity domain name and the
	// username of the oracle cloud account.
	ComposeName(string) string
}

// RulesAPI defines methods for retriving, creating and deleting
// Sec rules under the oracle cloud endpoint
type RulesAPI interface {
	// AllSecRules takes a filter and based on that filter
	// it can return all sec rules from the oracle cloud api
	// If the filter is nil then it will return all sec rules
	AllSecRules([]api.Filter) (response.AllSecRules, error)
	// DeleteSecRule deletes the security rule with the given name
	DeleteSecRule(string) error
	// CreateSecRule creates the security rule inside oracle cloud
	// given the sec rule params
	CreateSecRule(api.SecRuleParams) (response.SecRule, error)
}

// AclAPI defines methods for retriving, creating and deleting
// access controll lists under the oracle cloud endpoint
type AclAPI interface {
	// AclDetails retrives the access controll list details of the
	// given name provided
	AclDetails(string) (response.Acl, error)
	// CreateAcl creates the access controll list
	CreateAcl(string, string, bool, []string) (response.Acl, error)
	// DeleteAcl deletes the access controll list of the given name
	// in the oracle cloud
	DeleteAcl(string) error
}

// SecIpAPI defines methods for retriving creating sec ip lists
// in the oracle cloud
type SecIpAPI interface {
	// AllSecIpLists takes a filter and based on that filter
	// it can return all sec ip lists from the oracle cloud api.
	// If the filter is nil then it will return all sec ip lists.
	AllSecIpLists([]api.Filter) (response.AllSecIpLists, error)
	// CreateSecIpList creates the sec ip list under the oracle cloud endpoint
	CreateSecIpList(string, string, []string) (response.SecIpList, error)
	// ALlDefaultSecIpLists retrives all default sec ip lists from the
	// oracle cloud account. This also can have filter rules attach.
	AllDefaultSecIpLists([]api.Filter) (response.AllSecIpLists, error)
}

// IpAddressPrefixSetAPI defines methods for creating and listing
// ip addresss prefix sets under the oracle cloud endpoint
type IpAddressPrefixSetAPI interface {
	// CreateIpAddressPrefixSet creates the address prefix set based on the
	// ip address prefix set params under the oracle cloud endpoint.
	CreateIpAddressPrefixSet(
		api.IpAddressPrefixSetParams) (response.IpAddressPrefixSet, error)

	// AllIpAddressPrefixSets takes a filter and based on that filter
	// it can return all ip prefix sets of ip addresses from the oracle cloud api
	// If the filter is nil then it will return all ip prefix sets addresses
	AllIpAddressPrefixSets([]api.Filter) (response.AllIpAddressPrefixSets, error)
}

// SecListAPI defines methods for retriving, createing and deleting
// sec lists under the oracle cloud endpoint
type SecListAPI interface {
	// SecListDetails retrives sec list details of the given sec list name
	SecListDetails(string) (response.SecList, error)
	// DeleteSecList deletes sec list with the given sec list name
	DeleteSecList(string) error
	// CreateSecList creates a sec list based on the given params
	// under the oracle cloud endpoint
	CreateSecList(string, string,
		common.SecRuleAction, common.SecRuleAction) (response.SecList, error)
}

// SecRulesAPI defines methods for retriving, deleting and creating
/// security rules under the oracle cloud endpoint
type SecRulesAPI interface {
	// AllSecurityRules retrives all security rules under
	// the oracle cloud endpoint. The results can be filtered.
	// If the filter is nil it returns all security rules.
	AllSecurityRules([]api.Filter) (response.AllSecurityRules, error)
	// DeleteSecurityRule deletes the sercurity rule with the given name
	// under the oracle cloud endpoint
	DeleteSecurityRule(string) error
	// CreateSecurityRule creates a security rule based on the security rule
	// params under the oracle cloud enpodint
	CreateSecurityRule(api.SecurityRuleParams) (response.SecurityRule, error)
}

// ApplicationsAPI also named protocols in the oracle cloud defines methods
// for retriving and creating applications rules/protocol rules
// under the oracle cloud endpoint
type ApplicationsAPI interface {
	// AllSecApplications retrives all security application under the oracle
	// cloud endpoint. The results can be filtered
	AllSecApplications([]api.Filter) (response.AllSecApplications, error)
	// DefaultSecApplications returns all security applications that are default
	// under the oracle cloud endpoint. The results can be filtered
	DefaultSecApplications([]api.Filter) (response.AllSecApplications, error)
	// CreateSecApplications creates a security application based on the
	// security application params under the oracle cloud endpoint
	CreateSecApplication(api.SecApplicationParams) (response.SecApplication, error)
}

// AssociationAPI defines a rule for listing, retriving all security
// asoociations under the oracle cloud api
type AssociationAPI interface {
	// AllSecAssociations returns all security associations under the oracle
	// cloud account. The results can be filtered.
	AllSecAssociations([]api.Filter) (response.AllSecAssociations, error)
}

// StorageVolumeAPI defines methods for retriving, creating, deleting and
// updating storage volumes under the oracle cloud endpoint
type StorageVolumeAPI interface {
	// CreateStorageVolume creates a storge volume based on the given storage
	// volume params under the oracle cloud endpoint
	CreateStorageVolume(p api.StorageVolumeParams) (resp response.StorageVolume, err error)
	// DeleteStorageVolume deletes the storage volume with the given
	// storage volume name
	DeleteStorageVolume(name string) (err error)
	// StorageVolumeDetails retrives storage volume details based on the given
	// storage volume name under the oracle cloud endpoint
	StorageVolumeDetails(name string) (resp response.StorageVolume, err error)
	// AllStoragevolumes retrives all storage volumes under the oracle cloud
	// endpoint. The reults can be filtered.
	AllStorageVolumes(filter []api.Filter) (resp response.AllStorageVolumes, err error)
	// UpdateStorageVolume updates the state of the storage volume based
	// on the given storage volume params.
	UpdateStorageVolume(p api.StorageVolumeParams, currentName string) (resp response.StorageVolume, err error)
}

// StorageAttachmentAPI defines methods for attaching, detaching storages to
// instances under the oracle cloud endpoint
type StorageAttachmentAPI interface {
	// CreateStorageAttachment creates a storage attachment based on the given
	// storage attachment params
	CreateStorageAttachment(p api.StorageAttachmentParams) (response.StorageAttachment, error)
	// DeleteStorageAttachment delets the storage attachment based on the given
	// storage attachment name
	DeleteStorageAttachment(name string) error
	// StorageAttachmentDetails retrives details of the storage attachment
	// based on the given storage attachment name
	StorageAttachmentDetails(name string) (response.StorageAttachment, error)
	// AllStorageAttachments retrives all storage attachments under the oracle
	// cloud endpoint. This results can be filtered
	AllStorageAttachments(filter []api.Filter) (response.AllStorageAttachments, error)
}
