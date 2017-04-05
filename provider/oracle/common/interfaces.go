// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// Instancer used to retrieve details from a given instance
// in the oracle cloud infrastructure
type Instancer interface {
	// InstanceDetails retrieves information from the provider
	// about one instance
	InstanceDetails(string) (response.Instance, error)
}

// Composer has the simple task of composing an oracle compatible
// resource name
type Composer interface {
	// ComposeName composes the name for a provider resource. The name
	// for an oracle API resource topically has the following form:
	//
	// /Compute-<Identity Domain>/<username>/<resource name>
	//
	// The Identity Domain in this case equates to what some other providers
	// like OpenStack refer to as tenants or projects.
	// This information is supplied by the user in the cloud configuration
	// information.
	// This function is generally needed
	ComposeName(string) string
}

// RulesAPI defines methods for retrieving, creating and deleting
// Sec rules under the oracle cloud endpoint
// For more information on sec rules, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecRules.html
type RulesAPI interface {
	// AllSecRules returns all sec rules matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecRules([]api.Filter) (response.AllSecRules, error)
	// DeleteSecRule deletes the security rule with the given name
	DeleteSecRule(string) error
	// CreateSecRule creates the security rule inside oracle cloud
	// given the sec rule parameters
	CreateSecRule(api.SecRuleParams) (response.SecRule, error)
}

// AclAPI defines methods for retrieving, creating and deleting
// access control lists under the oracle cloud endpoint
type AclAPI interface {
	// AclDetails retrieves the access control list details for one list
	AclDetails(string) (response.Acl, error)
	// CreateAcl creates the access control list
	CreateAcl(string, string, bool, []string) (response.Acl, error)
	// DeleteAcl deletes one access control list
	DeleteAcl(string) error
}

// SecIpAPI defines methods for retrieving creating sec IP lists
// in the oracle cloud
type SecIpAPI interface {
	// AllSecIpLists returns all sec IP lists that match a filter. A nil valued
	// filter will return all entries in the API.
	AllSecIpLists([]api.Filter) (response.AllSecIpLists, error)
	// CreateSecIpList creates the sec IP list under the oracle cloud endpoint
	CreateSecIpList(string, string, []string) (response.SecIpList, error)
	// AllDefaultSecIpLists retrieves all default sec IP lists from the
	// oracle cloud account. Default lists are defined by the cloud and cannot
	// be changed in any way.
	AllDefaultSecIpLists([]api.Filter) (response.AllSecIpLists, error)
}

// IpAddressPrefixSetAPI defines methods for creating and listing
// IP address prefix sets under the oracle cloud endpoint
// For information about IP address prefix sets, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-IPAddressPrefixSets.html
type IpAddressPrefixSetAPI interface {
	// CreateIpAddressPrefixSet creates a new IP address prefix set inside the oracle
	// cloud, for the current user
	CreateIpAddressPrefixSet(
		api.IpAddressPrefixSetParams) (response.IpAddressPrefixSet, error)

	// AllIpAddressPrefixSets returns all IP address prefix sets that match the given filter.
	// A nil valued filter will return all entries in the API.
	AllIpAddressPrefixSets([]api.Filter) (response.AllIpAddressPrefixSets, error)
}

// SecListAPI defines methods for retrieving, creating and deleting
// sec lists under the oracle cloud endpoint
type SecListAPI interface {
	// SecListDetails retrieves sec list details for the given list
	SecListDetails(string) (response.SecList, error)
	// DeleteSecList deletes one sec list
	DeleteSecList(string) error
	// CreateSecList creates a sec list
	CreateSecList(string, string,
		common.SecRuleAction, common.SecRuleAction) (response.SecList, error)
}

// SecRulesAPI defines methods for retrieving, deleting and creating
// security rules under the oracle cloud endpoint
type SecRulesAPI interface {
	// AllSecurityRules returns all security rules matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecurityRules([]api.Filter) (response.AllSecurityRules, error)
	// DeleteSecurityRule deletes the security rule with the given name
	DeleteSecurityRule(string) error
	// CreateSecurityRule creates a security rule based on the security rule
	// parameters under the oracle cloud endpoint
	CreateSecurityRule(api.SecurityRuleParams) (response.SecurityRule, error)
}

// ApplicationsAPI also named protocols in the oracle cloud defines methods
// for retriving and creating applications rules/protocol rules
// under the oracle cloud endpoint
type ApplicationsAPI interface {
	// AllSecApplications returns all sec applications matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecApplications([]api.Filter) (response.AllSecApplications, error)
	// DefaultSecApplications returns all default security applications matching a filter.
	// A nil valued filter will return all entries in the API.
	DefaultSecApplications([]api.Filter) (response.AllSecApplications, error)
	// CreateSecApplications creates a security applications
	CreateSecApplication(api.SecApplicationParams) (response.SecApplication, error)
}

// AssociationAPI defines a rule for listing, retrieving all security
// associations under the oracle cloud API
type AssociationAPI interface {
	// AllSecAssociations returns all security associations matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecAssociations([]api.Filter) (response.AllSecAssociations, error)
}

// StorageVolumeAPI defines methods for retrieving, creating, deleting and
// updating storage volumes under the oracle cloud endpoint
type StorageVolumeAPI interface {
	// CreateStorageVolume creates a storage volume
	CreateStorageVolume(p api.StorageVolumeParams) (resp response.StorageVolume, err error)
	// DeleteStorageVolume deletes the storage volume
	DeleteStorageVolume(name string) (err error)
	// StorageVolumeDetails retrieves storage volume details
	StorageVolumeDetails(name string) (resp response.StorageVolume, err error)
	// AllStoragevolumes retrieves all storage volumes matching a filter. A nil valued
	// filter will return all entries in the API.
	AllStorageVolumes(filter []api.Filter) (resp response.AllStorageVolumes, err error)
	// UpdateStorageVolume updates the state of the storage volume
	UpdateStorageVolume(p api.StorageVolumeParams, currentName string) (resp response.StorageVolume, err error)
}

// StorageAttachmentAPI defines methods for attaching, detaching storages to
// instances under the oracle cloud endpoint
type StorageAttachmentAPI interface {
	// CreateStorageAttachment creates a storage attachment
	CreateStorageAttachment(p api.StorageAttachmentParams) (response.StorageAttachment, error)
	// DeleteStorageAttachment deletes the storage attachment
	DeleteStorageAttachment(name string) error
	// StorageAttachmentDetails retrieves details of the storage attachment
	StorageAttachmentDetails(name string) (response.StorageAttachment, error)
	// AllStorageAttachments retrieves all storage attachments matching a filter. A nil valued
	// filter will return all entries in the API.
	AllStorageAttachments(filter []api.Filter) (response.AllStorageAttachments, error)
}
